package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/sergio-bershadsky/secondbrain-db/internal/cli/output"
	clir "github.com/sergio-bershadsky/secondbrain-db/internal/cli/runtime"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb"
)

var (
	deleteID   string
	deleteYes  bool
	deleteSoft bool
)

var deleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a document",
	RunE:  runDelete,
}

func init() {
	deleteCmd.Flags().StringVar(&deleteID, "id", "", "document ID (required)")
	deleteCmd.MarkFlagRequired("id")
	deleteCmd.Flags().BoolVar(&deleteYes, "yes", false, "confirm deletion without prompting")
	deleteCmd.Flags().BoolVar(&deleteSoft, "soft", false, "soft delete: set status=archived instead of removing")
	rootCmd.AddCommand(deleteCmd)
}

func runDelete(cmd *cobra.Command, _ []string) error {
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

	if !deleteYes {
		output.PrintError(format, "CONFIRMATION_REQUIRED",
			fmt.Sprintf("use --yes to confirm deletion of %q", deleteID), nil)
		os.Exit(1)
	}

	if flagDryRun {
		return output.PrintData(format, map[string]any{
			"action": "delete", "id": deleteID, "soft": deleteSoft,
		})
	}

	if deleteSoft {
		saved, err := repo.Update(ctx, deleteID, func(d sbdb.Doc) sbdb.Doc {
			if d.Frontmatter == nil {
				d.Frontmatter = make(map[string]any)
			}
			d.Frontmatter["status"] = "archived"
			return d
		})
		if err != nil {
			return err
		}
		return clir.PrintData(cfg, map[string]any{
			"action": "soft_delete", "id": saved.ID, "status": "archived",
		})
	}

	if err := repo.Delete(ctx, deleteID); err != nil {
		if errors.Is(err, sbdb.ErrNotFound) {
			output.PrintError(format, "NOT_FOUND", err.Error(), nil)
			os.Exit(2)
		}
		return err
	}

	return clir.PrintData(cfg, map[string]any{
		"action": "deleted", "id": deleteID,
	})
}
