package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/sergio-bershadsky/secondbrain-db/internal/cli/output"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/kg"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/query"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/semantic"
)

var indexCmd = &cobra.Command{
	Use:   "index",
	Short: "Manage the knowledge graph and semantic index",
}

var indexBuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build or update the knowledge graph and semantic index",
	RunE:  runIndexBuild,
}

var indexStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show index statistics",
	RunE:  runIndexStats,
}

var indexDropCmd = &cobra.Command{
	Use:   "drop",
	Short: "Drop the entire knowledge graph and index",
	RunE:  runIndexDrop,
}

var (
	indexForce   bool
	indexIDs     []string
	indexDropYes bool
	indexCrawl   bool
	indexDocsDir string
)

func init() {
	indexBuildCmd.Flags().BoolVar(&indexForce, "force", false, "re-index all documents (ignore staleness)")
	indexBuildCmd.Flags().StringArrayVar(&indexIDs, "id", nil, "index specific document IDs only")
	indexBuildCmd.Flags().BoolVar(&indexCrawl, "crawl", false, "walk all .md files in docs/ regardless of schema")
	indexBuildCmd.Flags().StringVar(&indexDocsDir, "docs-dir", "", "docs root for crawl mode (default: docs/)")
	indexDropCmd.Flags().BoolVar(&indexDropYes, "yes", false, "confirm drop")

	indexCmd.AddCommand(indexBuildCmd)
	indexCmd.AddCommand(indexStatsCmd)
	indexCmd.AddCommand(indexDropCmd)
	rootCmd.AddCommand(indexCmd)
}

func runIndexBuild(cmd *cobra.Command, _ []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}

	format := outputFormat(cfg)

	// Create embedder (optional — graph works without it)
	var embedder semantic.Embedder
	apiKey := os.Getenv("SBDB_EMBED_API_KEY")
	if apiKey != "" {
		embedder, err = semantic.NewOpenAIEmbedder(semantic.OpenAIConfig{
			BaseURL: cfg.KnowledgeGraph.Embeddings.BaseURL,
			APIKey:  apiKey,
			Model:   cfg.KnowledgeGraph.Embeddings.Model,
			Dim:     cfg.KnowledgeGraph.Embeddings.Dimension,
		})
		if err != nil {
			return fmt.Errorf("creating embedder: %w", err)
		}
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

	// Crawl mode: walk all .md files regardless of schema
	if indexCrawl {
		docsDir := indexDocsDir
		if docsDir == "" {
			docsDir = filepath.Join(cfg.BasePath, "docs")
		} else if !filepath.IsAbs(docsDir) {
			docsDir = filepath.Join(cfg.BasePath, docsDir)
		}

		crawlResult, err := kgdb.CrawlAndIndex(kg.CrawlOptions{
			DocsRoot: docsDir,
			Force:    indexForce,
		})
		if err != nil {
			return err
		}

		return output.PrintData(format, map[string]any{
			"action":        "crawl",
			"files_found":   crawlResult.FilesFound,
			"files_indexed": crawlResult.FilesIndexed,
			"files_skipped": crawlResult.FilesSkipped,
			"edges_found":   crawlResult.EdgesFound,
			"embeddings":    embedder != nil,
		})
	}

	// Schema mode: index documents from records.yaml
	s, err := loadSchema(cfg)
	if err != nil {
		return err
	}

	qs := query.NewQuerySet(s, cfg.BasePath)
	docs, err := qs.All()
	if err != nil {
		return err
	}

	rt, err := loadRuntime(s)
	if err != nil {
		return err
	}

	for _, doc := range docs {
		if err := doc.EnsureLoaded(); err != nil {
			continue
		}
		if rt != nil && len(s.Virtuals) > 0 {
			vResults, vErr := rt.EvaluateAll(doc.Content, doc.Data)
			if vErr == nil {
				doc.SetVirtuals(vResults)
			}
		}
	}

	indexed, err := kgdb.BuildAll(docs, s, indexForce)
	if err != nil {
		return err
	}

	return output.PrintData(format, map[string]any{
		"action":     "build",
		"indexed":    indexed,
		"total":      len(docs),
		"embeddings": embedder != nil,
	})
}

func runIndexStats(cmd *cobra.Command, _ []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}
	format := outputFormat(cfg)

	kgdb, err := openKGDB(cfg)
	if err != nil {
		return err
	}
	defer kgdb.Close()

	stats, err := kgdb.Stats()
	if err != nil {
		return err
	}

	return output.PrintData(format, stats)
}

func runIndexDrop(cmd *cobra.Command, _ []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}
	format := outputFormat(cfg)

	if !indexDropYes {
		output.PrintError(format, "CONFIRMATION_REQUIRED",
			"use --yes to confirm dropping the knowledge graph", nil)
		os.Exit(1)
	}

	kgdb, err := openKGDB(cfg)
	if err != nil {
		return err
	}
	defer kgdb.Close()

	if err := kgdb.Drop(); err != nil {
		return err
	}

	return output.PrintData(format, map[string]any{"action": "drop"})
}
