package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/sergio-bershadsky/secondbrain-db/internal/cli/output"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/document"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/query"
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

	doc, err := qs.Get(map[string]any{s.IDField: getID})
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

	if err := doc.VerifyIntegrity(); err != nil {
		if intErr, ok := err.(*document.IntegrityError); ok {
			output.PrintError(format, "INTEGRITY_ERROR", intErr.Error(), nil)
			os.Exit(6)
		}
		return err
	}

	result := doc.AllData()
	result["file"] = doc.RelativeFilePath()
	if !getNoContent {
		result["content"] = doc.Content
	}

	return output.PrintData(format, result)
}
