package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/sergio-bershadsky/secondbrain-db/internal/cli/output"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/config"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/kg"
)

var graphCmd = &cobra.Command{
	Use:   "graph",
	Short: "Knowledge graph queries and export",
}

var (
	graphID    string
	graphDepth int
)

var graphIncomingCmd = &cobra.Command{
	Use:   "incoming",
	Short: "Show edges pointing TO a document",
	RunE:  runGraphIncoming,
}

var graphOutgoingCmd = &cobra.Command{
	Use:   "outgoing",
	Short: "Show edges FROM a document",
	RunE:  runGraphOutgoing,
}

var graphNeighborsCmd = &cobra.Command{
	Use:   "neighbors",
	Short: "Show documents within N hops",
	RunE:  runGraphNeighbors,
}

var graphExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export graph as mermaid or DOT",
	RunE:  runGraphExport,
}

var graphStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show knowledge graph statistics",
	RunE:  runGraphStats,
}

var graphExportFormat string

func init() {
	graphIncomingCmd.Flags().StringVar(&graphID, "id", "", "document ID (required)")
	graphIncomingCmd.MarkFlagRequired("id")

	graphOutgoingCmd.Flags().StringVar(&graphID, "id", "", "document ID (required)")
	graphOutgoingCmd.MarkFlagRequired("id")

	graphNeighborsCmd.Flags().StringVar(&graphID, "id", "", "document ID (required)")
	graphNeighborsCmd.MarkFlagRequired("id")
	graphNeighborsCmd.Flags().IntVar(&graphDepth, "depth", 1, "traversal depth")

	graphExportCmd.Flags().StringVar(&graphExportFormat, "export-format", "mermaid", "export format: mermaid, dot")

	graphCmd.AddCommand(graphIncomingCmd)
	graphCmd.AddCommand(graphOutgoingCmd)
	graphCmd.AddCommand(graphNeighborsCmd)
	graphCmd.AddCommand(graphExportCmd)
	graphCmd.AddCommand(graphStatsCmd)
	rootCmd.AddCommand(graphCmd)
}

func openKGDB(cfg *config.Config) (*kg.DB, error) {
	dbPath := cfg.KnowledgeGraph.DBPath
	if !filepath.IsAbs(dbPath) {
		dbPath = filepath.Join(cfg.BasePath, dbPath)
	}

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("knowledge graph not built yet — run 'sbdb index build' first")
	}

	return kg.Open(dbPath, nil) // nil embedder for read-only graph queries
}

func runGraphIncoming(cmd *cobra.Command, _ []string) error {
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

	edges, err := kgdb.Incoming(graphID)
	if err != nil {
		return err
	}

	return output.PrintData(format, edges)
}

func runGraphOutgoing(cmd *cobra.Command, _ []string) error {
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

	edges, err := kgdb.Outgoing(graphID)
	if err != nil {
		return err
	}

	return output.PrintData(format, edges)
}

func runGraphNeighbors(cmd *cobra.Command, _ []string) error {
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

	edges, err := kgdb.Neighbors(graphID, graphDepth)
	if err != nil {
		return err
	}

	return output.PrintData(format, edges)
}

func runGraphExport(cmd *cobra.Command, _ []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}

	kgdb, err := openKGDB(cfg)
	if err != nil {
		return err
	}
	defer kgdb.Close()

	switch graphExportFormat {
	case "json":
		format := outputFormat(cfg)
		graph, jsonErr := kgdb.ExportJSON(nil)
		if jsonErr != nil {
			return jsonErr
		}
		return output.PrintData(format, graph)
	case "dot":
		result, dotErr := kgdb.ExportDOT(nil)
		if dotErr != nil {
			return dotErr
		}
		fmt.Print(result)
		return nil
	default: // mermaid
		result, merErr := kgdb.ExportMermaid(nil)
		if merErr != nil {
			return merErr
		}
		fmt.Print(result)
		return nil
	}
}

func runGraphStats(cmd *cobra.Command, _ []string) error {
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
