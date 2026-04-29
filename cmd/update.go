package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sergio-bershadsky/secondbrain-db/internal/cli/output"
	clir "github.com/sergio-bershadsky/secondbrain-db/internal/cli/runtime"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb"
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

	// Capture flag state for the mutator closure.
	inputFile := updateInput
	contentFile := updateContentFile
	fields := updateFields

	format := clir.OutputFormat(cfg)

	if flagDryRun {
		cur, err := repo.Get(ctx, updateID)
		if err != nil {
			if errors.Is(err, sbdb.ErrNotFound) {
				output.PrintError(format, "NOT_FOUND", err.Error(), nil)
				os.Exit(2)
			}
			return err
		}
		d := applyDocUpdates(cur, inputFile, contentFile, fields)
		return clir.PrintData(cfg, map[string]any{"action": "update", "id": updateID, "data": d.Frontmatter})
	}

	saved, err := repo.Update(ctx, updateID, func(d sbdb.Doc) sbdb.Doc {
		return applyDocUpdates(d, inputFile, contentFile, fields)
	})
	if err != nil {
		if errors.Is(err, sbdb.ErrNotFound) {
			output.PrintError(format, "NOT_FOUND", err.Error(), nil)
			os.Exit(2)
		}
		return err
	}
	return clir.PrintData(cfg, map[string]any{
		"action":      "update",
		"id":          saved.ID,
		"frontmatter": saved.Frontmatter,
	})
}

// applyDocUpdates merges --input, --field, and --content-file into d and
// returns the modified Doc. All errors inside are logged to stderr and skipped
// to match the original "apply best-effort" behaviour.
func applyDocUpdates(d sbdb.Doc, inputFile, contentFile string, fields []string) sbdb.Doc {
	if d.Frontmatter == nil {
		d.Frontmatter = make(map[string]any)
	}

	// Merge from --input
	if inputFile != "" {
		var reader io.Reader
		if inputFile == "-" {
			reader = os.Stdin
		} else {
			f, err := os.Open(inputFile)
			if err == nil {
				defer f.Close()
				reader = f
			}
		}
		if reader != nil {
			raw, err := io.ReadAll(reader)
			if err == nil {
				var payload map[string]any
				if err := json.Unmarshal(raw, &payload); err == nil {
					if c, ok := payload["content"]; ok {
						d.Content = fmt.Sprintf("%v", c)
						delete(payload, "content")
					}
					for k, v := range payload {
						d.Frontmatter[k] = v
					}
				}
			}
		}
	}

	// Apply --field updates
	for _, kv := range fields {
		if err := applyFieldToMap(d.Frontmatter, kv); err != nil {
			fmt.Fprintf(os.Stderr, "update: skipping field %q: %v\n", kv, err)
		}
	}

	// Replace content from file
	if contentFile != "" {
		raw, err := os.ReadFile(contentFile)
		if err == nil {
			d.Content = string(raw)
		}
	}

	return d
}

// applyFieldToMap applies a single KEY[op]=VALUE string to a map[string]any.
// Operators: += (append to list), -= (remove from list), ~= (delete key), = (set).
func applyFieldToMap(m map[string]any, kv string) error {
	if idx := strings.Index(kv, "+="); idx > 0 {
		key := kv[:idx]
		val := parseFieldValue(kv[idx+2:])
		return appendToMap(m, key, val)
	}
	if idx := strings.Index(kv, "-="); idx > 0 {
		key := kv[:idx]
		val := fmt.Sprintf("%v", parseFieldValue(kv[idx+2:]))
		return removeFromMap(m, key, val)
	}
	if idx := strings.Index(kv, "~="); idx > 0 {
		key := kv[:idx]
		delete(m, key)
		return nil
	}

	// Standard set
	parts := strings.SplitN(kv, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid --field format: %q", kv)
	}
	m[parts[0]] = parseFieldValue(parts[1])
	return nil
}

// appendToMap appends val to the list stored at key in m.
func appendToMap(m map[string]any, key string, val any) error {
	existing := m[key]
	switch v := existing.(type) {
	case []any:
		m[key] = append(v, val)
	case nil:
		m[key] = []any{val}
	default:
		return fmt.Errorf("cannot append to non-list field %q", key)
	}
	return nil
}

// removeFromMap removes the first element matching val (as string) from the list at key.
func removeFromMap(m map[string]any, key, val string) error {
	existing := m[key]
	switch v := existing.(type) {
	case []any:
		var result []any
		for _, item := range v {
			if fmt.Sprintf("%v", item) != val {
				result = append(result, item)
			}
		}
		m[key] = result
	default:
		return fmt.Errorf("cannot remove from non-list field %q", key)
	}
	return nil
}
