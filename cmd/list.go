package cmd

import (
	"context"
	"strings"

	"github.com/spf13/cobra"

	clir "github.com/sergio-bershadsky/secondbrain-db/internal/cli/runtime"
)

var (
	listLimit  int
	listOffset int
	listOrder  string
	listFields string
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all records (fast — reads records.yaml only)",
	RunE:  runList,
}

func init() {
	listCmd.Flags().IntVar(&listLimit, "limit", 0, "max number of records")
	listCmd.Flags().IntVar(&listOffset, "offset", 0, "skip first N records")
	listCmd.Flags().StringVar(&listOrder, "order", "", "order by field (-field for descending)")
	listCmd.Flags().StringVar(&listFields, "fields", "", "comma-separated field projection")
	rootCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, _ []string) error {
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

	qs := repo.Query()

	if listOrder != "" {
		qs = qs.OrderBy(listOrder)
	}
	if listLimit > 0 {
		qs = qs.Limit(listLimit)
	}
	if listOffset > 0 {
		qs = qs.Offset(listOffset)
	}

	records, err := qs.Records()
	if err != nil {
		return err
	}

	// Apply field projection
	if listFields != "" {
		fields := strings.Split(listFields, ",")
		records = projectFields(records, fields)
	}

	return clir.PrintData(cfg, records)
}

func projectFields(records []map[string]any, fields []string) []map[string]any {
	var result []map[string]any
	for _, rec := range records {
		projected := make(map[string]any)
		for _, f := range fields {
			f = strings.TrimSpace(f)
			if v, ok := rec[f]; ok {
				projected[f] = v
			}
		}
		result = append(result, projected)
	}
	return result
}
