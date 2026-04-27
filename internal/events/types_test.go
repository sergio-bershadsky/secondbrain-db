package events

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMarshalLine_KeyOrder(t *testing.T) {
	e := &Event{
		TS:    time.Date(2026, 4, 24, 14, 32, 1, 123_000_000, time.UTC),
		Type:  "note.created",
		ID:    "notes/2026/04/foo.md",
		SHA:   "abc123",
		Op:    "01HW3R8M",
		Actor: ActorCLI,
	}
	line, err := e.MarshalLine()
	require.NoError(t, err)
	got := string(line)
	require.True(t, strings.HasSuffix(got, "\n"), "must end with newline")

	// Key order ts, type, id, sha, op, actor — must appear in this order.
	keys := []string{`"ts":`, `"type":`, `"id":`, `"sha":`, `"op":`, `"actor":`}
	last := -1
	for _, k := range keys {
		idx := strings.Index(got, k)
		require.GreaterOrEqual(t, idx, 0, "missing key %s in %s", k, got)
		require.Greater(t, idx, last, "key %s out of order in %s", k, got)
		last = idx
	}

	// `data` field was removed in favor of pure pointer events (see #8).
	// Verify it never appears in the wire format under any circumstance.
	require.NotContains(t, got, `"data":`, "data field must not be emitted")

	// Round-trip parse.
	parsed, err := ParseLine(line)
	require.NoError(t, err)
	require.Equal(t, e.Type, parsed.Type)
	require.Equal(t, e.ID, parsed.ID)
	require.Equal(t, e.SHA, parsed.SHA)
	require.Equal(t, e.Op, parsed.Op)
	require.Equal(t, e.Actor, parsed.Actor)
}

func TestMarshalLine_OmitsEmptyOptionals(t *testing.T) {
	e := &Event{
		TS:   time.Date(2026, 4, 24, 14, 32, 1, 0, time.UTC),
		Type: "note.created",
		ID:   "notes/foo.md",
	}
	line, err := e.MarshalLine()
	require.NoError(t, err)
	got := string(line)
	for _, key := range []string{`"sha":`, `"prev":`, `"op":`, `"phase":`, `"actor":`, `"data":`} {
		require.NotContains(t, got, key, "expected %s omitted, got %s", key, got)
	}
	require.NotContains(t, got, `null`, "must never emit null")
}

// TestMarshalLine_RejectsTooLarge verifies the 4 KiB cap is still enforced.
// Now that `data` is gone, the only field large enough to push past the cap
// is `id` — exercise it with a pathological repo path.
func TestMarshalLine_RejectsTooLarge(t *testing.T) {
	huge := "notes/" + strings.Repeat("x", 5000) + ".md"
	e := &Event{
		TS:   time.Date(2026, 4, 24, 14, 32, 1, 0, time.UTC),
		Type: "note.created",
		ID:   huge,
	}
	_, err := e.MarshalLine()
	require.ErrorIs(t, err, ErrLineTooLarge)
}

// TestParseLine_IgnoresLegacyData verifies pre-1.2 lines that still carry
// a `data` field round-trip cleanly: the field is silently dropped, every
// other field survives.
func TestParseLine_IgnoresLegacyData(t *testing.T) {
	legacy := []byte(`{"ts":"2026-04-24T14:32:01.000Z","type":"note.updated","id":"notes/foo.md","sha":"abc","data":{"changed_keys":["title"]}}` + "\n")
	e, err := ParseLine(legacy)
	require.NoError(t, err)
	require.Equal(t, "note.updated", e.Type)
	require.Equal(t, "notes/foo.md", e.ID)
	require.Equal(t, "abc", e.SHA)
}

func TestValidate_Type(t *testing.T) {
	// ValidTypeName checks structural format only — registry membership is
	// a separate check. The `x.` author namespace is preserved as a
	// forward-compatibility reservation even though authors cannot
	// currently register types.
	cases := map[string]bool{
		"note.created":        true,
		"task.status_changed": true,
		"x.recipe.created":    true,
		"x.recipe.cooked":     true,
		"meta.archived":       true,
		"":                    false,
		"NoteCreated":         false,
		"note":                false, // need at least bucket.verb
		"x.recipe":            false, // need three segments for author
		"x.":                  false,
	}
	for typeName, want := range cases {
		t.Run(typeName, func(t *testing.T) {
			got := ValidTypeName(typeName)
			require.Equal(t, want, got, "type %q", typeName)
		})
	}
}

func TestValidate_ID_NFC(t *testing.T) {
	// Composed (NFC) é = U+00E9
	composed := "notes/café.md"
	// Decomposed (NFD) é = e + combining acute
	decomposed := "notes/café.md"

	e := &Event{TS: time.Now().UTC(), Type: "note.created", ID: composed}
	require.NoError(t, e.Validate())

	e.ID = decomposed
	err := e.Validate()
	require.ErrorIs(t, err, ErrInvalidID)
}

func TestBucket_Verb(t *testing.T) {
	cases := []struct{ in, bucket, verb string }{
		{"note.created", "note", "created"},
		{"task.status_changed", "task", "status_changed"},
		{"x.recipe.created", "x.recipe", "created"},
		{"x.recipe.cooked", "x.recipe", "cooked"},
		{"meta.archived", "meta", "archived"},
	}
	for _, c := range cases {
		require.Equal(t, c.bucket, Bucket(c.in), "bucket of %s", c.in)
		require.Equal(t, c.verb, Verb(c.in), "verb of %s", c.in)
	}
}

func TestRegistry_Builtin(t *testing.T) {
	r := NewBuiltinRegistry()
	require.True(t, r.IsKnownType("note.created"))
	require.True(t, r.IsKnownType("meta.archived"))
	// Catalog is closed — author types are never known.
	require.False(t, r.IsKnownType("x.recipe.created"))
	require.False(t, r.IsKnownType("note.unknown_verb"))
}
