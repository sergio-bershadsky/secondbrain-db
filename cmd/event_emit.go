package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"sync"
	"time"

	"github.com/sergio-bershadsky/secondbrain-db/internal/config"
	"github.com/sergio-bershadsky/secondbrain-db/internal/events"
)

// emitDocEvent records a document mutation event at the write site.
//
// The verb is one of "created" | "updated" | "deleted". The bucket comes
// from the schema's Bucket field if set, otherwise the entity name.
//
// Errors here are intentionally swallowed-with-log: the events stream is
// a best-effort audit channel; a full disk should not fail an otherwise-
// successful CRUD operation. (This matches the spec's at-least-once
// semantics — the event might not land, but the next sbdb invocation can
// reconstruct state from the underlying MD files.)
func emitDocEvent(cfg *config.Config, bucket, verb, id, contentSHA string) {
	if !cfg.Events.Enabled {
		return
	}
	em, err := getEmitter(cfg)
	if err != nil {
		// First-time init failed (no registry, etc.); silently skip.
		return
	}
	ev := &events.Event{
		TS:    time.Now().UTC(),
		Type:  bucket + "." + verb,
		ID:    id,
		SHA:   contentSHA,
		Actor: events.ActorCLI,
	}
	_ = em.Emit(context.Background(), ev) // intentional: best-effort
}

// emitIntegrityEvent records integrity.* events.
func emitIntegrityEvent(cfg *config.Config, verb, id string, count int) {
	if !cfg.Events.Enabled {
		return
	}
	em, err := getEmitter(cfg)
	if err != nil {
		return
	}
	ev := &events.Event{
		TS:    time.Now().UTC(),
		Type:  "integrity." + verb,
		ID:    id,
		Actor: events.ActorCLI,
		Data:  map[string]interface{}{"count": count},
	}
	_ = em.Emit(context.Background(), ev)
}

// shaContent returns the hex SHA-256 of the given byte slice.
func shaContent(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// shaFile reads a file and returns its hex SHA-256, or empty on error.
func shaFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return shaContent(data)
}

// emitter cache — one per process, lazy.
var (
	emitterMu     sync.Mutex
	emitterShared *events.Emitter
	emitterRoot   string
)

func getEmitter(cfg *config.Config) (*events.Emitter, error) {
	emitterMu.Lock()
	defer emitterMu.Unlock()
	if emitterShared != nil && emitterRoot == cfg.BasePath {
		return emitterShared, nil
	}
	if emitterShared != nil {
		_ = emitterShared.Close()
		emitterShared = nil
	}
	registry, err := loadOrSeedRegistry(cfg.BasePath)
	if err != nil {
		return nil, err
	}
	app := events.NewAppender(cfg.BasePath, cfg.Events.RotationLines)
	emitterShared = events.NewEmitter(app, registry)
	emitterRoot = cfg.BasePath
	return emitterShared, nil
}
