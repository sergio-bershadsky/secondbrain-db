package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/sergio-bershadsky/secondbrain-db/internal/cli/output"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new secondbrain-db project",
	RunE:  runInit,
}

func init() { rootCmd.AddCommand(initCmd) }

func runInit(cmd *cobra.Command, _ []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}
	format := outputFormat(cfg)
	basePath := cfg.BasePath

	if initInteractive {
		return runInteractiveInit(basePath, format)
	}

	for _, d := range []string{"schemas", "docs"} {
		if err := os.MkdirAll(filepath.Join(basePath, d), 0o755); err != nil {
			return fmt.Errorf("creating %s: %w", d, err)
		}
	}

	tomlContent := `schema_dir = "./schemas"
base_path = "."

[output]
format = "auto"

[integrity]
key_source = "env"
`
	tomlPath := filepath.Join(basePath, ".sbdb.toml")
	if err := os.WriteFile(tomlPath, []byte(tomlContent), 0o644); err != nil {
		return err
	}

	return output.PrintData(format, map[string]any{
		"action":  "init",
		"config":  tomlPath,
		"schemas": filepath.Join(basePath, "schemas"),
		"next":    "Add a schema under schemas/<entity>.yaml — see the secondbrain-db plugin for reference schemas.",
	})
}
