// Package runtime carries dependency-injected services that internal/* and
// (later) pkg/sbdb/* need: a logger, a clock, an HMAC key source, and a
// walker concurrency knob. Construction is lightweight and zero-value-safe;
// any nil field falls back to the documented default.
package runtime

import (
	"context"
	"log/slog"
	goruntime "runtime"
	"time"
)

// Runtime is passed by value (it's small) into every constructor that
// previously read these knobs from globals. Sub-packages depend on the
// fields they actually use, not on the whole runtime.
type Runtime struct {
	Logger      *slog.Logger
	Clock       func() time.Time
	KeyLoader   KeyLoader // for HMAC integrity sigs
	WalkWorkers int       // 0 = GOMAXPROCS
}

// KeyLoader resolves an HMAC integrity key on demand.
// nil result with nil error means "no key configured" (integrity off / warn).
type KeyLoader func(ctx context.Context) ([]byte, error)

// Default returns a Runtime with safe zero-equivalent defaults: slog.Default()
// as the logger, time.Now as the clock, no key loader (nil result → integrity
// unsigned), and GOMAXPROCS workers.
func Default() Runtime {
	return Runtime{
		Logger:      slog.Default(),
		Clock:       time.Now,
		KeyLoader:   nil,
		WalkWorkers: goruntime.GOMAXPROCS(0),
	}
}

// WithDefaults backfills any nil fields in the receiver from Default().
// Use at constructor entry: rt = rt.WithDefaults().
func (r Runtime) WithDefaults() Runtime {
	d := Default()
	if r.Logger == nil {
		r.Logger = d.Logger
	}
	if r.Clock == nil {
		r.Clock = d.Clock
	}
	if r.WalkWorkers == 0 {
		r.WalkWorkers = d.WalkWorkers
	}
	return r
}
