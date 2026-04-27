package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/sergio-bershadsky/secondbrain-db/internal/document"
	"github.com/sergio-bershadsky/secondbrain-db/internal/output"
	"github.com/sergio-bershadsky/secondbrain-db/internal/query"
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
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}

	s, err := loadSchema(cfg)
	if err != nil {
		return err
	}

	format := outputFormat(cfg)

	qs := query.NewQuerySet(s, cfg.BasePath)
	doc, err := qs.Get(map[string]any{s.IDField: deleteID})
	if err != nil {
		if _, ok := err.(*document.NotFoundError); ok {
			output.PrintError(format, "NOT_FOUND", err.Error(), nil)
			os.Exit(2)
		}
		return err
	}

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
		if err := doc.EnsureLoaded(); err != nil {
			return err
		}
		doc.Set("status", "archived")
		rt, err := loadRuntime(s)
		if err != nil {
			return err
		}
		if err := doc.Save(rt); err != nil {
			return err
		}
		return output.PrintData(format, map[string]any{
			"action": "soft_delete", "id": deleteID, "status": "archived",
		})
	}

	if err := doc.EnsureLoaded(); err != nil {
		return err
	}
	if err := doc.Delete(); err != nil {
		return err
	}

	return output.PrintData(format, map[string]any{
		"action": "deleted", "id": deleteID,
	})
}
