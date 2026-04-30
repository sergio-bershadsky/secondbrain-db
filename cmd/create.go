package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sergio-bershadsky/secondbrain-db/internal/cli/output"
	clir "github.com/sergio-bershadsky/secondbrain-db/internal/cli/runtime"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb"
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
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	db, cfg, err := clir.OpenDB(ctx, flagBasePath, flagSchemaDir, flagSchema, flagFormat)
	if err != nil {
		return err
	}
	defer db.Close()

	repo, err := db.RepoErr(cfg.DefaultSchema)
	if err != nil {
		return err
	}

	format := clir.OutputFormat(cfg)

	fm, content, err := buildCreatePayload(format)
	if err != nil {
		return err
	}

	if flagDryRun {
		result := map[string]any{"action": "create", "data": fm, "content_length": len(content)}
		return output.PrintData(format, result)
	}

	saved, err := repo.Create(ctx, sbdb.Doc{Frontmatter: fm, Content: content})
	if err != nil {
		return err
	}
	return clir.PrintData(cfg, map[string]any{
		"action":      "create",
		"id":          saved.ID,
		"frontmatter": saved.Frontmatter,
	})
}

// buildCreatePayload parses --input, --field, --content, --content-file flags
// and returns the frontmatter map and content string.
func buildCreatePayload(format string) (map[string]any, string, error) {
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
				return nil, "", err
			}
			defer f.Close()
			reader = f
		}

		raw, err := io.ReadAll(reader)
		if err != nil {
			return nil, "", err
		}

		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			output.PrintError(format, "PARSE_ERROR", "invalid JSON input", nil)
			return nil, "", err
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
			return nil, "", fmt.Errorf("invalid --field format: %q (expected KEY=VALUE)", kv)
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
			return nil, "", fmt.Errorf("reading content file: %w", err)
		}
		content = string(raw)
	}

	return data, content, nil
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
