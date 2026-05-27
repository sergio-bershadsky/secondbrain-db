package cmd

import "github.com/spf13/cobra"

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "External-sync bookkeeping for docs mirrored to Confluence/Jira/Slack",
	Long: `Manages declared sync targets and per-doc sync state. sbdb does NOT
talk to external services — pushes are performed by an integration runtime
(Claude + MCP). These commands let the runtime read what needs to happen and
record what it did.`,
}

func init() {
	rootCmd.AddCommand(syncCmd)
}
