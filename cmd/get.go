package cmd

import (
	"context"
	"errors"
	"os"

	"github.com/spf13/cobra"

	"github.com/sergio-bershadsky/secondbrain-db/internal/cli/output"
	clir "github.com/sergio-bershadsky/secondbrain-db/internal/cli/runtime"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb"
)

var (
	getID        string
	getNoContent bool
)

var getCmd = &cobra.Command{
	Use:   "get",
	Short: "Get a single document by ID",
	RunE:  runGet,
}

func init() {
	getCmd.Flags().StringVar(&getID, "id", "", "document ID (required)")
	getCmd.MarkFlagRequired("id")
	getCmd.Flags().BoolVar(&getNoContent, "no-content", false, "exclude markdown content from output")
	rootCmd.AddCommand(getCmd)
}

func runGet(cmd *cobra.Command, _ []string) error {
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

	doc, err := repo.Get(ctx, getID)
	if err != nil {
		if errors.Is(err, sbdb.ErrNotFound) {
			output.PrintError(clir.OutputFormat(cfg), "NOT_FOUND", err.Error(), nil)
			os.Exit(2)
		}
		return err
	}

	result := map[string]any{
		"id":          doc.ID,
		"frontmatter": doc.Frontmatter,
	}
	if !getNoContent {
		result["content"] = doc.Content
	}

	return clir.PrintData(cfg, result)
}
