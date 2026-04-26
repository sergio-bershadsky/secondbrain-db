package events

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

// Emitter is the high-level write API. It holds a Registry snapshot for
// fast type-membership checks and delegates the actual write to an Appender.
//
// Emitter is safe for concurrent use across goroutines. Per spec §11, the
// Appender path is lock-free at the kernel level; the only mutex here
// guards the in-memory registry pointer for hot-swap on evolution events.
type Emitter struct {
	appender *Appender

	regMu    sync.RWMutex
	registry atomic.Pointer[Registry] // hot-swappable snapshot
}

// NewEmitter wraps an Appender with registry-aware Emit().
func NewEmitter(appender *Appender, registry *Registry) *Emitter {
	em := &Emitter{appender: appender}
	em.registry.Store(registry)
	return em
}

// SwapRegistry installs a new registry snapshot. Used after a meta.event_type_*
// event lands and we want subsequent appends to recognize the new type.
func (em *Emitter) SwapRegistry(r *Registry) {
	em.regMu.Lock()
	defer em.regMu.Unlock()
	em.registry.Store(r)
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
