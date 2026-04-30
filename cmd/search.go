package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/sergio-bershadsky/secondbrain-db/internal/cli/output"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/config"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/kg"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/query"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/semantic"
)

var (
	searchSemantic bool
	searchK        int
	searchExpand   bool
	searchDepth    int
)

var searchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Full-text or semantic search over documents",
	Args:  cobra.ExactArgs(1),
	RunE:  runSearch,
}

func init() {
	searchCmd.Flags().BoolVar(&searchSemantic, "semantic", false, "use semantic (vector) search")
	searchCmd.Flags().IntVar(&searchK, "k", 10, "number of results for semantic search")
	searchCmd.Flags().BoolVar(&searchExpand, "expand", false, "expand results with graph neighbors")
	searchCmd.Flags().IntVar(&searchDepth, "depth", 1, "graph expansion depth")
	rootCmd.AddCommand(searchCmd)
}

func runSearch(cmd *cobra.Command, args []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}

	s, err := loadSchema(cfg)
	if err != nil {
		return err
	}

	format := outputFormat(cfg)
	searchQuery := args[0]

	if searchSemantic {
		return runSemanticSearch(cfg, format, searchQuery)
	}

	results, err := query.Search(s, cfg.BasePath, searchQuery)
	if err != nil {
		return err
	}

	var data []map[string]any
	for _, r := range results {
		data = append(data, map[string]any{
			"id":      r.ID,
			"file":    r.File,
			"snippet": r.Snippet,
		})
	}

	return output.PrintData(format, data)
}

func runSemanticSearch(cfg *config.Config, format, searchQuery string) error {
	apiKey := os.Getenv("SBDB_EMBED_API_KEY")
	if apiKey == "" {
		output.PrintError(format, "CONFIG_ERROR",
			"semantic search requires SBDB_EMBED_API_KEY environment variable", nil)
		os.Exit(1)
	}

	embedder, err := semantic.NewOpenAIEmbedder(semantic.OpenAIConfig{
		BaseURL: cfg.KnowledgeGraph.Embeddings.BaseURL,
		APIKey:  apiKey,
		Model:   cfg.KnowledgeGraph.Embeddings.Model,
		Dim:     cfg.KnowledgeGraph.Embeddings.Dimension,
	})
	if err != nil {
		return fmt.Errorf("creating embedder: %w", err)
	}

	dbPath := cfg.KnowledgeGraph.DBPath
	if !filepath.IsAbs(dbPath) {
		dbPath = filepath.Join(cfg.BasePath, dbPath)
	}

	kgdb, err := kg.Open(dbPath, embedder)
	if err != nil {
		return err
	}
	defer kgdb.Close()

	if searchExpand {
		results, edges, err := kgdb.SearchWithExpand(searchQuery, searchK, searchDepth)
		if err != nil {
			return err
		}
		return output.PrintData(format, map[string]any{
			"results": results,
			"related": edges,
		})
	}

	results, err := kgdb.Search(searchQuery, searchK)
	if err != nil {
		return err
	}

	return output.PrintData(format, results)
}
