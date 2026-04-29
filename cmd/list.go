package cmd

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/sergio-bershadsky/secondbrain-db/internal/cli/output"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/query"
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

	return output.PrintData(format, records)
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
