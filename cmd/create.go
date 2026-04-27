package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sergio-bershadsky/secondbrain-db/internal/document"
	"github.com/sergio-bershadsky/secondbrain-db/internal/output"
	schemapkg "github.com/sergio-bershadsky/secondbrain-db/internal/schema"
)

var (
	createFields      []string
	createInput       string
	createContent     string
	createContentFile string
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new document",
	Long:  `Creates a new document from field values or JSON input, writes the .md file and records entry.`,
	RunE:  runCreate,
}

func init() {
	createCmd.Flags().StringArrayVar(&createFields, "field", nil, "field value as KEY=VALUE (repeatable)")
	createCmd.Flags().StringVar(&createInput, "input", "", "read JSON payload from file (use - for stdin)")
	createCmd.Flags().StringVar(&createContent, "content", "", "markdown body content")
	createCmd.Flags().StringVar(&createContentFile, "content-file", "", "read markdown body from file")
	rootCmd.AddCommand(createCmd)
}

func runCreate(cmd *cobra.Command, _ []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}

	s, err := loadSchema(cfg)
	if err != nil {
		return err
	}

	format := outputFormat(cfg)
	data := make(map[string]any)
	content := ""

	// Load from --input
	if createInput != "" {
		var reader io.Reader
		if createInput == "-" {
			reader = os.Stdin
		} else {
			f, err := os.Open(createInput)
			if err != nil {
				output.PrintError(format, "INPUT_ERROR", err.Error(), nil)
				return err
			}
			defer f.Close()
			reader = f
		}

		raw, err := io.ReadAll(reader)
		if err != nil {
			return err
		}

		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			output.PrintError(format, "PARSE_ERROR", "invalid JSON input", nil)
			return err
		}

		// Extract content if present
		if c, ok := payload["content"]; ok {
			content = fmt.Sprintf("%v", c)
			delete(payload, "content")
		}
		data = payload
	}

	// Override/merge with --field flags
	for _, kv := range createFields {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid --field format: %q (expected KEY=VALUE)", kv)
		}
		data[parts[0]] = parseFieldValue(parts[1])
	}

	// Content from flags
	if createContent != "" {
		content = createContent
	}
	if createContentFile != "" {
		raw, err := os.ReadFile(createContentFile)
		if err != nil {
			return fmt.Errorf("reading content file: %w", err)
		}
		content = string(raw)
	}

	// Validate
	if errs := schemapkg.ValidateRecord(s, data); len(errs) > 0 {
		output.PrintError(format, "VALIDATION_ERROR", errs.Error(), errs)
		os.Exit(3)
	}

	if flagDryRun {
		result := map[string]any{"action": "create", "data": data, "content_length": len(content)}
		return output.PrintData(format, result)
	}

	// Create document
	doc := document.New(s, cfg.BasePath)
	doc.Data = data
	doc.Content = content

	rt, err := loadRuntime(s)
	if err != nil {
		return err
	}

	if err := doc.Save(rt); err != nil {
		output.PrintError(format, "SAVE_ERROR", err.Error(), nil)
		return err
	}

	// Output the created record
	result := doc.AllData()
	result["file"] = doc.RelativeFilePath()
	return output.PrintData(format, result)
}

// parseFieldValue attempts to interpret a CLI string value as a typed value.
func parseFieldValue(s string) any {
	// Try JSON array/object
	if strings.HasPrefix(s, "[") || strings.HasPrefix(s, "{") {
		var v any
		if err := json.Unmarshal([]byte(s), &v); err == nil {
			return v
		}
	}
	// Booleans
	if s == "true" {
		return true
	}
	if s == "false" {
		return false
	}
	// Integers
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return int(i)
	}
	// Floats
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return s
}
