package events

import (
	"context"
	"fmt"
	"sync/atomic"
)

// Emitter is the high-level write API. It holds a Registry snapshot for
// fast type-membership checks and delegates the actual write to an Appender.
//
// Emitter is safe for concurrent use across goroutines. The catalog is
// closed (see builtin.go), so the registry never changes after construction —
// the atomic.Pointer is retained only for cheap concurrent reads.
type Emitter struct {
	appender *Appender
	registry atomic.Pointer[Registry]
}

// NewEmitter wraps an Appender with registry-aware Emit().
func NewEmitter(appender *Appender, registry *Registry) *Emitter {
	em := &Emitter{appender: appender}
	em.registry.Store(registry)
	return em
}

// Registry returns the current registry snapshot.
func (em *Emitter) Registry() *Registry {
	return em.registry.Load()
}

// Emit validates the event against the registry, then appends.
func (em *Emitter) Emit(ctx context.Context, e *Event) error {
	if err := e.Validate(); err != nil {
		return err
	}
	r := em.registry.Load()
	if r == nil {
		return fmt.Errorf("emitter: registry not loaded")
	}
	if !r.IsKnownType(e.Type) {
		return fmt.Errorf("event type %q not registered", e.Type)
	}
	return em.appender.Append(ctx, e)
}

// Close releases the underlying Appender.
func (em *Emitter) Close() error {
	if em.appender == nil {
		return nil
	}
	return em.appender.Close()
}
