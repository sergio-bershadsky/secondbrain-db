package runtime

import (
	"github.com/sergio-bershadsky/secondbrain-db/internal/cli/output"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/config"
)

// PrintData re-exports output.PrintData using the format resolved from cfg.
func PrintData(cfg *config.Config, data any) error {
	return output.PrintData(OutputFormat(cfg), data)
}

// OutputFormat resolves the output format with the same precedence the CLI
// has always used: --format flag > config.Output.Format > "auto".
func OutputFormat(cfg *config.Config) string {
	if cfg == nil || cfg.Output.Format == "" {
		return "auto"
	}
	return config.ResolveFormat(cfg.Output.Format)
}
