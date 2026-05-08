package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/schema"
)

var schemaCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Validate every existing doc against its current schema",
	RunE: func(cmd *cobra.Command, args []string) error {
		root, _ := os.Getwd()
		schemasDir := filepath.Join(root, "schemas")
		entries, err := os.ReadDir(schemasDir)
		if err != nil {
			return fmt.Errorf("read schemas dir: %w", err)
		}
		anyFail := false
		for _, e := range entries {
			ext := filepath.Ext(e.Name())
			if e.IsDir() || (ext != ".yaml" && ext != ".yml" && ext != ".json") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(schemasDir, e.Name()))
			if err != nil {
				return err
			}
			s, err := schema.Parse(data)
			if err != nil {
				return fmt.Errorf("%s: %w", e.Name(), err)
			}
			rep, err := schema.CheckExisting(s, root)
			if err != nil {
				return err
			}
			for _, f := range rep.Failures {
				fmt.Fprintf(cmd.OutOrStderr(), "%s: %s: %s\n", e.Name(), f.Path, f.Error)
				anyFail = true
			}
		}
		if anyFail {
			return fmt.Errorf("compat check failed")
		}
		return nil
	},
}

func init() {
	schemaCmd.AddCommand(schemaCheckCmd)
}
