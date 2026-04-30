// Package runtime holds CLI-private bootstrap helpers shared across cobra
// command handlers. None of this is part of the public library surface.
package runtime

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/config"
)

// OpenDB constructs a *sbdb.DB from current CLI flag/config state. The
// returned DB is owned by the caller — must Close() when done.
func OpenDB(ctx context.Context, flagBasePath, flagSchemaDir, flagSchema, flagFormat string) (*sbdb.DB, *config.Config, error) {
	cfg, err := ResolveConfig(flagBasePath, flagSchemaDir, flagSchema, flagFormat)
	if err != nil {
		return nil, nil, err
	}

	var opts []sbdb.Option
	// Route library warnings to stderr (matches today's CLI behaviour).
	opts = append(opts, sbdb.WithLogger(slog.New(
		slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}),
	)))

	db, err := sbdb.Open(ctx, sbdb.Config{
		Root:      cfg.BasePath,
		SchemaDir: cfg.SchemaDir,
	}, opts...)
	if err != nil {
		return nil, cfg, err
	}
	return db, cfg, nil
}

// ResolveConfig wraps config.Load with the same flag-resolution semantics
// the CLI has used since v1: --base-path flag, and a default of the current
// directory.
func ResolveConfig(flagBasePath, flagSchemaDir, flagSchema, flagFormat string) (*config.Config, error) {
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
