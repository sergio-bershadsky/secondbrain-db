package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/sergio-bershadsky/secondbrain-db/internal/cli/output"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/integrity"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/schema"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/storage"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/untracked"
)

var untrackedCmd = &cobra.Command{
	Use:   "untracked",
	Short: "Manage non-schema files with integrity signing",
}

// --- create ---

var (
	untrackedCreateContent     string
	untrackedCreateContentFile string
)

var untrackedCreateCmd = &cobra.Command{
	Use:   "create [file-path]",
	Short: "Create and sign a non-schema file",
	Args:  cobra.ExactArgs(1),
	RunE:  runUntrackedCreate,
}

// --- list ---

var untrackedListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all tracked non-schema files",
	RunE:  runUntrackedList,
}

// --- get ---

var untrackedGetCmd = &cobra.Command{
	Use:   "get [file-path]",
	Short: "Read an untracked file with its integrity info",
	Args:  cobra.ExactArgs(1),
	RunE:  runUntrackedGet,
}

// --- delete ---

var untrackedDeleteYes bool

var untrackedDeleteCmd = &cobra.Command{
	Use:   "delete [file-path]",
	Short: "Delete an untracked file and its registry entry",
	Args:  cobra.ExactArgs(1),
	RunE:  runUntrackedDelete,
}

// --- sign ---

var untrackedSignCmd = &cobra.Command{
	Use:   "sign [file-path]",
	Short: "Sign an existing file and add it to the untracked registry",
	Args:  cobra.ExactArgs(1),
	RunE:  runUntrackedSign,
}

// --- sign-all ---

var untrackedSignAllCmd = &cobra.Command{
	Use:   "sign-all [docs-dir]",
	Short: "Sign all unregistered .md files in a directory",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runUntrackedSignAll,
}

func init() {
	untrackedCreateCmd.Flags().StringVar(&untrackedCreateContent, "content", "", "inline markdown content")
	untrackedCreateCmd.Flags().StringVar(&untrackedCreateContentFile, "content-file", "", "read content from file")

	untrackedDeleteCmd.Flags().BoolVar(&untrackedDeleteYes, "yes", false, "confirm deletion")

	untrackedCmd.AddCommand(untrackedCreateCmd)
	untrackedCmd.AddCommand(untrackedListCmd)
	untrackedCmd.AddCommand(untrackedGetCmd)
	untrackedCmd.AddCommand(untrackedDeleteCmd)
	untrackedCmd.AddCommand(untrackedSignCmd)
	untrackedCmd.AddCommand(untrackedSignAllCmd)
	rootCmd.AddCommand(untrackedCmd)
}

func runUntrackedCreate(_ *cobra.Command, args []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}
	format := outputFormat(cfg)
	relPath := args[0]
	fullPath := filepath.Join(cfg.BasePath, relPath)

	// Get content
	var content string
	if untrackedCreateContentFile != "" {
		data, err := os.ReadFile(untrackedCreateContentFile)
		if err != nil {
			return fmt.Errorf("reading content file: %w", err)
		}
		content = string(data)
	} else if untrackedCreateContent != "" {
		content = untrackedCreateContent
	} else {
		return fmt.Errorf("provide --content or --content-file")
	}

	// Write file
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		return err
	}

	// Sign and register
	key, _ := integrity.LoadKey()
	entry, err := untracked.SignFile(cfg.BasePath, relPath, key)
	if err != nil {
		return err
	}

	reg, err := untracked.Load(cfg.BasePath)
	if err != nil {
		return err
	}
	reg.Add(*entry)
	if err := reg.Save(cfg.BasePath); err != nil {
		return err
	}

	return output.PrintData(format, map[string]any{
		"action": "created",
		"file":   relPath,
		"sha":    entry.ContentSHA,
	})
}

func runUntrackedList(_ *cobra.Command, _ []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}
	format := outputFormat(cfg)

	reg, err := untracked.Load(cfg.BasePath)
	if err != nil {
		return err
	}

	var data []map[string]any
	for _, e := range reg.Entries {
		data = append(data, map[string]any{
			"file":        e.File,
			"content_sha": e.ContentSHA,
			"updated_at":  e.UpdatedAt,
		})
	}

	return output.PrintData(format, data)
}

func runUntrackedGet(_ *cobra.Command, args []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}
	format := outputFormat(cfg)
	relPath := args[0]
	fullPath := filepath.Join(cfg.BasePath, relPath)

	reg, err := untracked.Load(cfg.BasePath)
	if err != nil {
		return err
	}

	entry := reg.Get(relPath)
	if entry == nil {
		output.PrintError(format, "NOT_FOUND", fmt.Sprintf("file %q is not in the untracked registry", relPath), nil)
		os.Exit(2)
	}

	fm, body, err := storage.ParseMarkdown(fullPath)
	if err != nil {
		return err
	}

	return output.PrintData(format, map[string]any{
		"file":        relPath,
		"frontmatter": fm,
		"content":     body,
		"content_sha": entry.ContentSHA,
		"updated_at":  entry.UpdatedAt,
	})
}

func runUntrackedDelete(_ *cobra.Command, args []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}
	format := outputFormat(cfg)
	relPath := args[0]

	if !untrackedDeleteYes {
		output.PrintError(format, "CONFIRMATION_REQUIRED",
			fmt.Sprintf("use --yes to confirm deletion of %q", relPath), nil)
		os.Exit(1)
	}

	// Remove file
	fullPath := filepath.Join(cfg.BasePath, relPath)
	os.Remove(fullPath)

	// Remove from registry
	reg, err := untracked.Load(cfg.BasePath)
	if err != nil {
		return err
	}
	reg.Remove(relPath)
	if err := reg.Save(cfg.BasePath); err != nil {
		return err
	}

	return output.PrintData(format, map[string]any{
		"action": "deleted",
		"file":   relPath,
	})
}

func runUntrackedSign(_ *cobra.Command, args []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}
	format := outputFormat(cfg)
	relPath := args[0]

	key, _ := integrity.LoadKey()
	entry, err := untracked.SignFile(cfg.BasePath, relPath, key)
	if err != nil {
		return err
	}

	reg, err := untracked.Load(cfg.BasePath)
	if err != nil {
		return err
	}
	reg.Add(*entry)
	if err := reg.Save(cfg.BasePath); err != nil {
		return err
	}

	return output.PrintData(format, map[string]any{
		"action": "signed",
		"file":   relPath,
		"sha":    entry.ContentSHA,
	})
}

func runUntrackedSignAll(_ *cobra.Command, args []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}
	format := outputFormat(cfg)

	docsDir := filepath.Join(cfg.BasePath, "docs")
	if len(args) > 0 {
		docsDir = args[0]
		if !filepath.IsAbs(docsDir) {
			docsDir = filepath.Join(cfg.BasePath, docsDir)
		}
	}

	// Load all schemas for classification
	schemaNames, _ := schema.ListSchemas(cfg.SchemaDir)
	var schemas []*schema.Schema
	for _, name := range schemaNames {
		s, err := schema.LoadFromDir(cfg.SchemaDir, name)
		if err == nil {
			schemas = append(schemas, s)
		}
	}

	reg, err := untracked.Load(cfg.BasePath)
	if err != nil {
		return err
	}

	// Discover unregistered files
	unregistered, err := untracked.DiscoverUnregistered(docsDir, cfg.BasePath, schemas, reg)
	if err != nil {
		return err
	}

	key, _ := integrity.LoadKey()
	signed := 0
	for _, relPath := range unregistered {
		entry, err := untracked.SignFile(cfg.BasePath, relPath, key)
		if err != nil {
			continue
		}
		reg.Add(*entry)
		signed++
	}

	if err := reg.Save(cfg.BasePath); err != nil {
		return err
	}

	return output.PrintData(format, map[string]any{
		"action":        "sign-all",
		"discovered":    len(unregistered),
		"signed":        signed,
		"total_tracked": reg.Count(),
	})
}
