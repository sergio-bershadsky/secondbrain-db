package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/config"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/schema"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/version"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/virtuals"
)

var (
	flagSchemaDir string
	flagSchema    string
	flagBasePath  string
	flagFormat    string
	flagQuiet     bool
	flagVerbose   bool
	flagDryRun    bool
	flagConfig    string
)

var rootCmd = &cobra.Command{
	Use:   "sbdb",
	Short: "secondbrain-db — file-backed knowledge base ORM",
	Long: `sbdb is a CLI tool for managing markdown knowledge bases with YAML schemas,
Starlark virtual fields, and integrity signing.

Every operation is available as a machine-readable JSON API for AI agents.`,
	Version: version.Version,
	// Match the `sbdb version` subcommand output exactly so users see the
	// same string regardless of which form they use.
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&flagSchemaDir, "schema-dir", "S", "", "schemas directory (default: ./schemas)")
	rootCmd.PersistentFlags().StringVarP(&flagSchema, "schema", "s", "", "schema name to use")
	rootCmd.PersistentFlags().StringVarP(&flagBasePath, "base-path", "b", "", "project root directory")
	rootCmd.PersistentFlags().StringVarP(&flagFormat, "format", "f", "", "output format: json, yaml, table (default: auto)")
	rootCmd.PersistentFlags().BoolVar(&flagQuiet, "quiet", false, "suppress progress output")
	rootCmd.PersistentFlags().BoolVar(&flagVerbose, "verbose", false, "increase logging")
	rootCmd.PersistentFlags().BoolVar(&flagDryRun, "dry-run", false, "show what would change without writing")
	rootCmd.PersistentFlags().StringVar(&flagConfig, "config", "", "config file path (default: .sbdb.toml)")

	// Match the `sbdb version` subcommand output: just `sbdb <ver>` on its
	// own line. Cobra's default would prefix with `sbdb version `, which
	// would leave the flag and the subcommand printing different strings.
	rootCmd.SetVersionTemplate("sbdb {{.Version}}\n")
}

// Execute runs the root command.
func Execute() error {
	if err := rootCmd.Execute(); err != nil {
		return err
	}
	return nil
}

// newRootCmd builds a fresh root command tree. Production uses Execute()
// which lazily constructs and runs it; tests construct their own with
// SetOut/SetErr/SetIn redirected.
func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "sbdb",
		Short:         "secondbrain-db — file-backed knowledge base ORM",
		Long:          rootCmd.Long,
		Version:       version.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.SetVersionTemplate("sbdb {{.Version}}\n")

	cmd.PersistentFlags().StringVarP(&flagSchemaDir, "schema-dir", "S", "", "schemas directory (default: ./schemas)")
	cmd.PersistentFlags().StringVarP(&flagSchema, "schema", "s", "", "schema name to use")
	cmd.PersistentFlags().StringVarP(&flagBasePath, "base-path", "b", "", "project root directory")
	cmd.PersistentFlags().StringVarP(&flagFormat, "format", "f", "", "output format: json, yaml, table (default: auto)")
	cmd.PersistentFlags().BoolVar(&flagQuiet, "quiet", false, "suppress progress output")
	cmd.PersistentFlags().BoolVar(&flagVerbose, "verbose", false, "increase logging")
	cmd.PersistentFlags().BoolVar(&flagDryRun, "dry-run", false, "show what would change without writing")
	cmd.PersistentFlags().StringVar(&flagConfig, "config", "", "config file path (default: .sbdb.toml)")

	for _, sub := range rootCmd.Commands() {
		cmd.AddCommand(sub)
	}
	return cmd
}

// resetFlagsForTest zeroes the package-level flag variables. Call from
// each in-process cmd test before executing.
func resetFlagsForTest() {
	flagSchemaDir = ""
	flagSchema = ""
	flagBasePath = ""
	flagFormat = ""
	flagQuiet = false
	flagVerbose = false
	flagDryRun = false
	flagConfig = ""

	// Per-command flag vars
	createFields = nil
	createInput = ""
	createContent = ""
	createContentFile = ""

	deleteID = ""
	deleteYes = false
	deleteSoft = false

	getID = ""
	getNoContent = false

	updateID = ""
	updateFields = nil
	updateInput = ""
	updateContentFile = ""

	healMeantIt = false
	healIDs = nil
	healSince = ""
	healAll = false
}

// resolveConfig loads config and resolves flags.
func resolveConfig() (*config.Config, error) {
	basePath := flagBasePath
	if basePath == "" {
		var err error
		basePath, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("getting working directory: %w", err)
		}
	}

	cfg, err := config.Load(basePath)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	// CLI flags override config
	if flagSchemaDir != "" {
		if filepath.IsAbs(flagSchemaDir) {
			cfg.SchemaDir = flagSchemaDir
		} else {
			cfg.SchemaDir = filepath.Join(basePath, flagSchemaDir)
		}
	}
	if flagSchema != "" {
		cfg.DefaultSchema = flagSchema
	}
	if flagFormat != "" {
		cfg.Output.Format = flagFormat
	}
	cfg.BasePath = basePath

	return cfg, nil
}

// loadSchema loads the active schema using resolved config.
func loadSchema(cfg *config.Config) (*schema.Schema, error) {
	if cfg.DefaultSchema == "" {
		return nil, fmt.Errorf("no schema specified — use --schema or set default_schema in .sbdb.toml")
	}
	return schema.LoadFromDir(cfg.SchemaDir, cfg.DefaultSchema)
}

// loadRuntime creates a Starlark runtime with all virtuals compiled.
func loadRuntime(s *schema.Schema) (*virtuals.Runtime, error) {
	rt := virtuals.NewRuntime()
	for name, v := range s.Virtuals {
		if err := rt.Compile(name, v.Source, v.Returns); err != nil {
			return nil, err
		}
	}
	return rt, nil
}

// outputFormat returns the resolved output format.
func outputFormat(cfg *config.Config) string {
	return config.ResolveFormat(cfg.Output.Format)
}

// loadAllSchemas returns every schema found in cfg.SchemaDir.
func loadAllSchemas(cfg *config.Config) ([]*schema.Schema, error) {
	entries, err := os.ReadDir(cfg.SchemaDir)
	if err != nil {
		return nil, fmt.Errorf("reading schema dir: %w", err)
	}
	var out []*schema.Schema
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		s, err := schema.Load(filepath.Join(cfg.SchemaDir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		out = append(out, s)
	}
	return out, nil
}
