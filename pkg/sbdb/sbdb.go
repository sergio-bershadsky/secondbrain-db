// Package sbdb provides an embeddable Go library API over a secondbrain-db
// knowledge base. It exposes a service/repository facade — Open returns a
// *DB, db.Repo(entity) returns a *Repo, and Repo carries Create/Update/
// Delete/Get/Query.
//
// Stability: types and methods exported from this package and its sub-packages
// follow strict semver. Breaking changes ship in major versions only.
package sbdb

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/config"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/document"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/integrity"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/schema"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/storage"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/virtuals"
)

// Config carries the fields a consumer must supply at Open time. Optional
// knobs (logger, clock, key source, etc.) come through Option args.
type Config struct {
	Root      string // project root containing .sbdb.toml (or where it would live)
	SchemaDir string // overrides Root/schemas if non-empty
}

// DB is an opaque handle. Safe to call methods concurrently.
type DB struct {
	cfg     Config
	rootCfg *config.Config
	schemas map[string]*schema.Schema
	rt      *virtuals.Runtime
	opts    options
	closed  bool
	mu      sync.Mutex
}

// Open initialises a knowledge base handle. cfg.Root is required. It must
// either contain a .sbdb.toml or the directory must exist (Open will then
// use defaults). Returns an error if no schemas can be loaded.
func Open(ctx context.Context, cfg Config, opts ...Option) (*DB, error) {
	if cfg.Root == "" {
		return nil, fmt.Errorf("sbdb.Open: Root is required")
	}
	o := defaultOptions()
	for _, opt := range opts {
		opt(&o)
	}
	if err := o.apply(); err != nil {
		return nil, fmt.Errorf("sbdb.Open: applying options: %w", err)
	}

	// Load .sbdb.toml; tolerate missing file (use defaults).
	rootCfg, err := config.Load(cfg.Root)
	if err != nil {
		return nil, fmt.Errorf("sbdb.Open: loading config: %w", err)
	}
	if cfg.SchemaDir != "" {
		rootCfg.SchemaDir = cfg.SchemaDir
	}
	if !filepath.IsAbs(rootCfg.SchemaDir) {
		rootCfg.SchemaDir = filepath.Join(cfg.Root, rootCfg.SchemaDir)
	}

	// Discover and load every schema YAML.
	names, err := schema.ListSchemas(rootCfg.SchemaDir)
	if err != nil {
		return nil, fmt.Errorf("sbdb.Open: listing schemas at %s: %w", rootCfg.SchemaDir, err)
	}
	schemas := make(map[string]*schema.Schema, len(names))
	for _, name := range names {
		s, err := schema.LoadFromDir(rootCfg.SchemaDir, name)
		if err != nil {
			return nil, fmt.Errorf("sbdb.Open: loading schema %q: %w", name, err)
		}
		schemas[s.Entity] = s
	}

	// Boot a Starlark runtime with every virtual compiled in.
	rt := virtuals.NewRuntime()
	for _, s := range schemas {
		for vname, v := range s.Virtuals {
			if err := rt.Compile(vname, v.Source, v.Returns); err != nil {
				return nil, fmt.Errorf("sbdb.Open: compiling virtual %q in entity %q: %w",
					vname, s.Entity, err)
			}
		}
	}

	return &DB{
		cfg:     cfg,
		rootCfg: rootCfg,
		schemas: schemas,
		rt:      rt,
		opts:    o,
	}, nil
}

// Close releases resources. Idempotent.
func (db *DB) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.closed {
		return nil
	}
	db.closed = true
	return nil
}

// Schemas returns every entity schema known to the DB. Returned slice is
// safe to read but should not be mutated.
func (db *DB) Schemas() []*schema.Schema {
	out := make([]*schema.Schema, 0, len(db.schemas))
	for _, s := range db.schemas {
		out = append(out, s)
	}
	return out
}

// Repo returns the repository for the given entity name. Panics if the
// entity is unknown — use RepoErr for a checked variant.
func (db *DB) Repo(entity string) *Repo {
	r, err := db.RepoErr(entity)
	if err != nil {
		panic(err)
	}
	return r
}

// RepoErr returns the repository or an error if no schema is registered
// for the given entity name.
func (db *DB) RepoErr(entity string) (*Repo, error) {
	s, ok := db.schemas[entity]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownEntity, entity)
	}
	return &Repo{db: db, schema: s}, nil
}

// quiet helpers exist mainly for tests / future deprecation handling.
var _ = storage.WalkDocsToSlice
var _ = integrity.LoadKey
var _ = document.New
