// Package events implements sbdb's event projection: a derived view of
// git history shaped as JSONL events, emitted on demand by `sbdb events emit`.
//
// There is no on-disk events log. Events are not stored; they are computed
// from `git log --raw` on a commit range. Workers consume the projection
// by piping the command's output. The repo's git history IS the event log.
//
// One commit produces zero or more events: one per file changed under a
// schema's docs_dir. Verb mapping is purely structural — A → created,
// M → updated, D → deleted. Each event names a place (`id`, the file path)
// and a version (`sha`, git's blob hash from the post-image tree); a worker
// resolves content via `git cat-file blob <sha>`.
package events

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Event is the wire-format envelope. Fields are tagged json:"-" because the
// custom MarshalLine controls key order for diff readability and skips
// empty optionals (never null).
type Event struct {
	TS    time.Time `json:"-"`
	Type  string    `json:"-"` // bucket.verb
	ID    string    `json:"-"` // repo-relative POSIX path
	SHA   string    `json:"-"` // git blob hash after the change (omitted on delete)
	Prev  string    `json:"-"` // git blob hash before the change (omitted on create)
	Op    string    `json:"-"` // commit hash — groups events from one commit
	Actor string    `json:"-"` // commit author email (or "git" if none)
}

// TypeRegex enforces the dotted-name format. Catalog membership is decided
// at projection time; this only checks the structural shape.
var TypeRegex = regexp.MustCompile(`^[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*$`)

// ValidTypeName reports whether the type matches `<bucket>.<verb>`.
func ValidTypeName(t string) bool { return TypeRegex.MatchString(t) }

// ErrInvalidType is returned for malformed type names.
var ErrInvalidType = errors.New("invalid event type")

// Validate checks structural invariants. Returns nil on a well-formed event.
func (e *Event) Validate() error {
	if e.TS.IsZero() {
		return errors.New("ts is required")
	}
	if !ValidTypeName(e.Type) {
		return fmt.Errorf("%w: %q", ErrInvalidType, e.Type)
	}
	if e.ID == "" {
		return errors.New("id is required")
	}
	return nil
}

// MarshalLine serializes the event to a single JSON line with trailing \n.
// Key order is fixed: ts, type, id, sha, prev, op, actor.
func (e *Event) MarshalLine() ([]byte, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}

	var sb strings.Builder
	sb.WriteByte('{')
	first := true

	write := func(key string, value any) error {
		raw, err := json.Marshal(value)
		if err != nil {
			return err
		}
		if !first {
			sb.WriteByte(',')
		}
		first = false
		sb.WriteByte('"')
		sb.WriteString(key)
		sb.WriteString(`":`)
		sb.Write(raw)
		return nil
	}

	if err := write("ts", e.TS.UTC().Format("2006-01-02T15:04:05.000Z")); err != nil {
		return nil, err
	}
	if err := write("type", e.Type); err != nil {
		return nil, err
	}
	if err := write("id", e.ID); err != nil {
		return nil, err
	}
	if e.SHA != "" {
		if err := write("sha", e.SHA); err != nil {
			return nil, err
		}
	}
	if e.Prev != "" {
		if err := write("prev", e.Prev); err != nil {
			return nil, err
		}
	}
	if e.Op != "" {
		if err := write("op", e.Op); err != nil {
			return nil, err
		}
	}
	if e.Actor != "" {
		if err := write("actor", e.Actor); err != nil {
			return nil, err
		}
	}
	sb.WriteByte('}')
	sb.WriteByte('\n')
	return []byte(sb.String()), nil
}

// Bucket extracts the bucket from a type name. `note.created` → `note`.
func Bucket(typeName string) string {
	idx := strings.Index(typeName, ".")
	if idx < 0 {
		return ""
	}
	return typeName[:idx]
}

// Verb extracts the verb from a type name. `note.created` → `created`.
func Verb(typeName string) string {
	idx := strings.LastIndex(typeName, ".")
	if idx < 0 {
		return ""
	}
	return typeName[idx+1:]
}
