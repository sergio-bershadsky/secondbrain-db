package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sergio-bershadsky/secondbrain-db/internal/document"
	"github.com/sergio-bershadsky/secondbrain-db/internal/output"
	"github.com/sergio-bershadsky/secondbrain-db/internal/query"
)

var (
	updateID          string
	updateFields      []string
	updateInput       string
	updateContentFile string
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update an existing document",
	RunE:  runUpdate,
}

func init() {
	updateCmd.Flags().StringVar(&updateID, "id", "", "document ID (required)")
	updateCmd.MarkFlagRequired("id")
	updateCmd.Flags().StringArrayVar(&updateFields, "field", nil, "field update as KEY=VALUE, KEY+=VALUE (append), KEY-=VALUE (remove)")
	updateCmd.Flags().StringVar(&updateInput, "input", "", "read JSON payload from file (use - for stdin)")
	updateCmd.Flags().StringVar(&updateContentFile, "content-file", "", "replace markdown body from file")
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, _ []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}

	s, err := loadSchema(cfg)
	if err != nil {
		return err
	}

	format := outputFormat(cfg)

	// Load existing document
	qs := query.NewQuerySet(s, cfg.BasePath)
	doc, err := qs.Get(map[string]any{s.IDField: updateID})
	if err != nil {
		if _, ok := err.(*document.NotFoundError); ok {
			output.PrintError(format, "NOT_FOUND", err.Error(), nil)
			os.Exit(2)
		}
		return err
	}

	if err := doc.EnsureLoaded(); err != nil {
		return err
	}

	// Merge from --input
	if updateInput != "" {
		var reader io.Reader
		if updateInput == "-" {
			reader = os.Stdin
		} else {
			f, err := os.Open(updateInput)
			if err != nil {
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
			return fmt.Errorf("invalid JSON input: %w", err)
		}

		if c, ok := payload["content"]; ok {
			doc.Content = fmt.Sprintf("%v", c)
			delete(payload, "content")
		}

		for k, v := range payload {
			doc.Set(k, v)
		}
	}

	// Apply --field updates
	for _, kv := range updateFields {
		if err := applyFieldUpdate(doc, kv); err != nil {
			return err
		}
	}

	// Replace content from file
	if updateContentFile != "" {
		raw, err := os.ReadFile(updateContentFile)
		if err != nil {
			return fmt.Errorf("reading content file: %w", err)
		}
		doc.Content = string(raw)
	}

	if flagDryRun {
		result := map[string]any{"action": "update", "id": updateID, "data": doc.AllData()}
		return output.PrintData(format, result)
	}

	rt, err := loadRuntime(s)
	if err != nil {
		return err
	}

	if err := doc.Save(rt); err != nil {
		return err
	}

	// Spec §4.1: emit <bucket>.updated on successful write.
	emitDocEvent(cfg, eventBucket(s), "updated", doc.ID(),
		shaFile(doc.FilePath()))

	result := doc.AllData()
	result["file"] = doc.RelativeFilePath()
	return output.PrintData(format, result)
}

func applyFieldUpdate(doc *document.Document, kv string) error {
	// Check for operators: +=, -=, ~=
	if idx := strings.Index(kv, "+="); idx > 0 {
		key := kv[:idx]
		val := parseFieldValue(kv[idx+2:])
		return appendToList(doc, key, val)
	}
	if idx := strings.Index(kv, "-="); idx > 0 {
		key := kv[:idx]
		val := fmt.Sprintf("%v", parseFieldValue(kv[idx+2:]))
		return removeFromList(doc, key, val)
	}
	if idx := strings.Index(kv, "~="); idx > 0 {
		key := kv[:idx]
		delete(doc.Data, key)
		_ = kv[idx+2:] // value is ignored for delete
		return nil
	}

	// Standard set
	parts := strings.SplitN(kv, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid --field format: %q", kv)
	}
	doc.Set(parts[0], parseFieldValue(parts[1]))
	return nil
}

func appendToList(doc *document.Document, key string, val any) error {
	existing, _ := doc.Get(key)
	switch v := existing.(type) {
	case []any:
		doc.Set(key, append(v, val))
	case nil:
		doc.Set(key, []any{val})
	default:
		return fmt.Errorf("cannot append to non-list field %q", key)
	}
	return nil
}

func removeFromList(doc *document.Document, key, val string) error {
	existing, _ := doc.Get(key)
	switch v := existing.(type) {
	case []any:
		var result []any
		for _, item := range v {
			if fmt.Sprintf("%v", item) != val {
				result = append(result, item)
			}
		}
		doc.Set(key, result)
	default:
		return fmt.Errorf("cannot remove from non-list field %q", key)
	}
	return nil
}
