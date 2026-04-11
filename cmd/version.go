package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sergio-bershadsky/secondbrain-db/internal/version"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("sbdb %s\n", version.Version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
