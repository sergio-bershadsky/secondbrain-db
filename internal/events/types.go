// Package events implements the append-only event log described in
// docs/superpowers/specs/2026-04-24-sbdb-events-design.md.
//
// Events are immutable, append-only facts. The only mutable artifact in the
// system is markdown file content (governed by git). Every other byte sbdb
// writes here is write-once during routine operation.
package events

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Actor enumerates valid values for the closed `actor` field.
type Actor string

const (
	ActorCLI    Actor = "cli"
	ActorHook   Actor = "hook"
	ActorWorker Actor = "worker"
	ActorAgent  Actor = "agent"
)

func (a Actor) Valid() bool {
	switch a {
	case ActorCLI, ActorHook, ActorWorker, ActorAgent:
		return true
	}
	return false
}

// Event is the in-memory representation of one event line. JSON output uses
// fixed key order (see MarshalJSON) for diff readability.
//
// Events are pure pointers: ts says when, type says what, id says where,
// sha says which version (a git blob hash — `git cat-file blob <sha>`
// retrieves the exact bytes the event describes). There is no `data`
// payload — anything a worker needs about the change comes from reading
// the file at sha, or diffing against prev.
type Event struct {
	TS    time.Time `json:"-"`
	Type  string    `json:"-"`
	ID    string    `json:"-"`
	SHA   string    `json:"-"` // optional, git blob hash
	Prev  string    `json:"-"` // optional, git blob hash
	Op    string    `json:"-"` // optional, ULID
	Phase string    `json:"-"` // optional
	Actor Actor     `json:"-"` // optional
}

// MaxLineBytes is the hard cap on a serialized event line including trailing \n.
// Spec §7.3.
const MaxLineBytes = 4096

// TypeRegex is a permissive structural check; ValidTypeName adds the
// segment-count rule that an author type (x.*) requires three or more
// segments while built-ins require two.
var TypeRegex = regexp.MustCompile(`^[a-z][a-z0-9_]*(?:\.[a-z][a-z0-9_]*)+$`)

// ValidTypeName reports whether the type name conforms to spec §3.
// Built-in: <bucket>.<verb> (≥ 2 segments). Author: x.<bucket>.<verb> (≥ 3 segments).
func ValidTypeName(t string) bool {
	if !TypeRegex.MatchString(t) {
		return false
	}
	if strings.HasPrefix(t, "x.") && strings.Count(t, ".") < 2 {
		return false
	}
	return true
}

// IDForbidden rejects ids with backslashes (POSIX paths only) or control chars.
var idBackslash = regexp.MustCompile(`\\`)

// ErrLineTooLarge is returned when serialized event would exceed MaxLineBytes.
var ErrLineTooLarge = errors.New("event line exceeds 4 KiB cap")

// ErrInvalidType is returned for malformed type names.
var ErrInvalidType = errors.New("invalid event type")

// ErrInvalidID is returned for malformed ids.
var ErrInvalidID = errors.New("invalid event id")

// ErrInvalidActor is returned for unknown actor values.
var ErrInvalidActor = errors.New("invalid actor")

// IsAuthorType reports whether the type belongs to the author namespace (x.*).
func IsAuthorType(typeName string) bool {
	return strings.HasPrefix(typeName, "x.")
}

// Bucket extracts the bucket from a type name. For built-in `note.created`
// returns `note`. For author `x.recipe.created` returns `x.recipe`.
func Bucket(typeName string) string {
	if !ValidTypeName(typeName) {
		return ""
	}
	if IsAuthorType(typeName) {
		// strip "x." then take everything except the last segment
		rest := typeName[2:]
		if idx := strings.LastIndex(rest, "."); idx >= 0 {
			return "x." + rest[:idx]
		}
		return "x." + rest
	}
	if idx := strings.Index(typeName, "."); idx >= 0 {
		return typeName[:idx]
	}
	return typeName
}

// Verb extracts the verb segment from a type name.
func Verb(typeName string) string {
	if !ValidTypeName(typeName) {
		return ""
	}
	if idx := strings.LastIndex(typeName, "."); idx >= 0 {
		return typeName[idx+1:]
	}
	return ""
}

// Validate checks structural invariants. Does NOT check registry membership;
// that's the appender's job.
func (e *Event) Validate() error {
	if e.TS.IsZero() {
		return errors.New("ts is required")
	}
	if !ValidTypeName(e.Type) {
		return fmt.Errorf("%w: %q", ErrInvalidType, e.Type)
	}
	if e.ID == "" {
		return fmt.Errorf("%w: empty", ErrInvalidID)
	}
	if idBackslash.MatchString(e.ID) {
		return fmt.Errorf("%w: backslash in id (POSIX paths only)", ErrInvalidID)
	}
	for _, r := range e.ID {
		if unicode.IsControl(r) {
			return fmt.Errorf("%w: control char in id", ErrInvalidID)
		}
	}
	if !norm.NFC.IsNormalString(e.ID) {
		return fmt.Errorf("%w: id must be NFC-normalized", ErrInvalidID)
	}
	if e.Actor != "" && !e.Actor.Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidActor, e.Actor)
	}
	return nil
}

// MarshalLine serializes the event to a single JSON line with trailing \n.
// Returns ErrLineTooLarge if the result exceeds MaxLineBytes.
//
// Key order is fixed: ts, type, id, sha, prev, op, phase, actor.
// Empty/zero optional fields are omitted (never null).
func (e *Event) MarshalLine() ([]byte, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}

	// Build ordered output by writing to a strings.Builder then re-parsing
	// is wasteful; instead use a small custom serializer that respects order.
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
	if e.Phase != "" {
		if err := write("phase", e.Phase); err != nil {
			return nil, err
		}
	}
	if e.Actor != "" {
		if err := write("actor", string(e.Actor)); err != nil {
			return nil, err
		}
	}
	sb.WriteByte('}')
	sb.WriteByte('\n')

	out := []byte(sb.String())
	if len(out) > MaxLineBytes {
		return nil, fmt.Errorf("%w: %d bytes", ErrLineTooLarge, len(out))
	}
	return out, nil
}

// ParseLine decodes one JSON event line into an Event.
func ParseLine(line []byte) (*Event, error) {
	// strip trailing newline if present
	for len(line) > 0 && (line[len(line)-1] == '\n' || line[len(line)-1] == '\r') {
		line = line[:len(line)-1]
	}
	// Note: any "data" key on the input is ignored — pre-1.2 logs may
	// still carry it, but emit-side never produces it.
	var raw struct {
		TS    string `json:"ts"`
		Type  string `json:"type"`
		ID    string `json:"id"`
		SHA   string `json:"sha,omitempty"`
		Prev  string `json:"prev,omitempty"`
		Op    string `json:"op,omitempty"`
		Phase string `json:"phase,omitempty"`
		Actor string `json:"actor,omitempty"`
	}
	if err := json.Unmarshal(line, &raw); err != nil {
		return nil, err
	}
	ts, err := time.Parse("2006-01-02T15:04:05.000Z", raw.TS)
	if err != nil {
		// fall back to a more permissive parse
		ts, err = time.Parse(time.RFC3339Nano, raw.TS)
		if err != nil {
			return nil, fmt.Errorf("invalid ts %q: %w", raw.TS, err)
		}
	}
	e := &Event{
		TS:    ts,
		Type:  raw.Type,
		ID:    raw.ID,
		SHA:   raw.SHA,
		Prev:  raw.Prev,
		Op:    raw.Op,
		Phase: raw.Phase,
		Actor: Actor(raw.Actor),
	}
	return e, nil
}
