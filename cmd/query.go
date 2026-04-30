package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	clir "github.com/sergio-bershadsky/secondbrain-db/internal/cli/runtime"
)

var (
	queryFilters     []string
	queryExcludes    []string
	queryOrder       []string
	queryLimit       int
	queryOffset      int
	queryCount       bool
	queryExists      bool
	queryLoadContent bool
)

var queryCmd = &cobra.Command{
	Use:   "query",
	Short: "Query records with filters, ordering, and pagination",
	RunE:  runQuery,
}

func init() {
	queryCmd.Flags().StringArrayVar(&queryFilters, "filter", nil, "filter as KEY=VALUE with optional lookup suffix (repeatable)")
	queryCmd.Flags().StringArrayVar(&queryExcludes, "exclude", nil, "exclude as KEY=VALUE (repeatable)")
	queryCmd.Flags().StringArrayVar(&queryOrder, "order", nil, "order by field, prefix - for desc (repeatable)")
	queryCmd.Flags().IntVar(&queryLimit, "limit", 0, "max results")
	queryCmd.Flags().IntVar(&queryOffset, "offset", 0, "skip N results")
	queryCmd.Flags().BoolVar(&queryCount, "count", false, "return count only")
	queryCmd.Flags().BoolVar(&queryExists, "exists", false, "return existence check only")
	queryCmd.Flags().BoolVar(&queryLoadContent, "load-content", false, "load markdown content for each result")
	rootCmd.AddCommand(queryCmd)
}

func runQuery(cmd *cobra.Command, _ []string) error {
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

	// Apply filters
	if len(queryFilters) > 0 {
		conditions := parseKVPairs(queryFilters)
		qs = qs.Filter(conditions)
	}
	if len(queryExcludes) > 0 {
		conditions := parseKVPairs(queryExcludes)
		qs = qs.Exclude(conditions)
	}

	// Ordering
	if len(queryOrder) > 0 {
		qs = qs.OrderBy(queryOrder...)
	}
	if queryLimit > 0 {
		qs = qs.Limit(queryLimit)
	}
	if queryOffset > 0 {
		qs = qs.Offset(queryOffset)
	}

	// Count-only
	if queryCount {
		count, err := qs.Count()
		if err != nil {
			return err
		}
		return clir.PrintData(cfg, map[string]any{"count": count})
	}

	// Exists-only
	if queryExists {
		exists, err := qs.Exists()
		if err != nil {
			return err
		}
		return clir.PrintData(cfg, map[string]any{"exists": exists})
	}

	// Full query
	if queryLoadContent {
		docs, err := qs.All()
		if err != nil {
			return err
		}
		var results []map[string]any
		for _, doc := range docs {
			if err := doc.EnsureLoaded(); err != nil {
				return err
			}
			data := doc.AllData()
			data["content"] = doc.Content
			data["file"] = doc.RelativeFilePath()
			results = append(results, data)
		}
		return clir.PrintData(cfg, results)
	}

	records, err := qs.Records()
	if err != nil {
		return err
	}
	return clir.PrintData(cfg, records)
}

func parseKVPairs(pairs []string) map[string]any {
	result := make(map[string]any)
	for _, kv := range pairs {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 {
			result[parts[0]] = parseFieldValue(parts[1])
		} else {
			result[parts[0]] = fmt.Sprintf("%v", true)
		}
	}
	return result
}
