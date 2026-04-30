package sbdb

import (
	"context"
	"log/slog"
	"time"

	docpkg "github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/document"
	intpkg "github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/integrity"
	schpkg "github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/schema"
	stopkg "github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/storage"
)

// Option mutates internal options at Open time.
type Option func(*options)

type options struct {
	logger      *slog.Logger
	clock       func() time.Time
	key         []byte
	keyLoader   func(ctx context.Context) ([]byte, error)
	walkWorkers int
}

func defaultOptions() options { return options{} }

// apply pushes the configured options into the package-level vars in each
// domain package. Done once per Open call. Subsequent Opens with different
// options OVERRIDE the previous values — for now the library expects a
// single DB per process; tests that open many DBs sequentially will see
// the last winner. (A future enhancement could route options through
// per-call structs.)
func (o options) apply() error {
	if o.logger != nil {
		docpkg.Logger = o.logger
		schpkg.Logger = o.logger
	}
	if o.clock != nil {
		intpkg.Clock = o.clock
	}
	if o.key != nil {
		intpkg.KeyLoader = func(ctx context.Context) ([]byte, error) {
			return append([]byte(nil), o.key...), nil
		}
	} else if o.keyLoader != nil {
		intpkg.KeyLoader = o.keyLoader
	}
	if o.walkWorkers > 0 {
		stopkg.WorkerCount = func() int { return o.walkWorkers }
	}
	return nil
}

// WithLogger overrides the slog logger used by the library for warnings
// (post-hook failures, deprecation notices, etc.). Default: slog.Default().
func WithLogger(l *slog.Logger) Option {
	return func(o *options) { o.logger = l }
}

// WithClock overrides the clock used for sidecar timestamps and KG indexing.
// Default: time.Now.
func WithClock(now func() time.Time) Option {
	return func(o *options) { o.clock = now }
}

// WithIntegrityKey supplies a static HMAC key (overrides env-loaded key).
// Mutually exclusive with WithIntegrityKeyLoader; the last call wins.
func WithIntegrityKey(key []byte) Option {
	return func(o *options) {
		o.key = append([]byte(nil), key...)
		o.keyLoader = nil
	}
}

// WithIntegrityKeyLoader installs a custom HMAC key resolver (e.g. a
// secrets-manager call). Returning nil bytes with nil error means "no key
// configured" (sidecars will not be HMAC-signed).
func WithIntegrityKeyLoader(fn func(ctx context.Context) ([]byte, error)) Option {
	return func(o *options) {
		o.keyLoader = fn
		o.key = nil
	}
}

// WithWalkWorkers overrides walker concurrency. Default: GOMAXPROCS or
// SBDB_WALK_WORKERS env if set.
func WithWalkWorkers(n int) Option {
	return func(o *options) { o.walkWorkers = n }
}
