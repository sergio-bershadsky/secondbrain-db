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
		Data:  map[string]interface{}{"changed_keys": []string{"title"}},
	}
	line, err := e.MarshalLine()
	require.NoError(t, err)
	got := string(line)
	require.True(t, strings.HasSuffix(got, "\n"), "must end with newline")

	// Key order ts, type, id, sha, op, actor, data — must appear in this order.
	keys := []string{`"ts":`, `"type":`, `"id":`, `"sha":`, `"op":`, `"actor":`, `"data":`}
	last := -1
	for _, k := range keys {
		idx := strings.Index(got, k)
		require.GreaterOrEqual(t, idx, 0, "missing key %s in %s", k, got)
		require.Greater(t, idx, last, "key %s out of order in %s", k, got)
		last = idx
	}

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

func TestMarshalLine_RejectsTooLarge(t *testing.T) {
	huge := strings.Repeat("x", 5000)
	e := &Event{
		TS:   time.Date(2026, 4, 24, 14, 32, 1, 0, time.UTC),
		Type: "note.created",
		ID:   "notes/foo.md",
		Data: map[string]interface{}{"big": huge},
	}
	_, err := e.MarshalLine()
	require.ErrorIs(t, err, ErrLineTooLarge)
}

func TestValidate_Type(t *testing.T) {
	cases := map[string]bool{
		"note.created":            true,
		"task.status_changed":     true,
		"x.recipe.created":        true,
		"x.recipe.cooked":         true,
		"meta.event_type_evolved": true,
		"":                        false,
		"NoteCreated":             false,
		"note":                    false, // need at least bucket.verb
		"x.recipe":                false, // need three segments for author
		"x.":                      false,
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
		{"meta.event_type_registered", "meta", "event_type_registered"},
	}
	for _, c := range cases {
		require.Equal(t, c.bucket, Bucket(c.in), "bucket of %s", c.in)
		require.Equal(t, c.verb, Verb(c.in), "verb of %s", c.in)
	}
}

func TestRegistry_Builtin(t *testing.T) {
	r := NewBuiltinRegistry()
	require.True(t, r.IsKnownType("note.created"))
	require.True(t, r.IsKnownType("meta.event_type_registered"))
	require.False(t, r.IsKnownType("x.recipe.created"))
	require.False(t, r.IsKnownType("note.unknown_verb"))
}

func TestRegistry_RegisterAuthor(t *testing.T) {
	r := NewBuiltinRegistry()
	now := time.Date(2026, 4, 24, 0, 0, 0, 0, time.UTC)
	require.NoError(t, r.RegisterType("x.recipe.created", "schemas/recipe.yaml", now))
	require.NoError(t, r.RegisterType("x.recipe.updated", "schemas/recipe.yaml", now))
	require.NoError(t, r.RegisterType("x.recipe.cooked", "schemas/recipe.yaml", now))
	require.True(t, r.IsKnownType("x.recipe.created"))

	// Re-registration of same type should fail.
	err := r.RegisterType("x.recipe.created", "schemas/recipe.yaml", now)
	require.Error(t, err)

	// Different owner claim same bucket should fail.
	err = r.RegisterType("x.recipe.tossed", "schemas/other.yaml", now)
	require.Error(t, err)
}

func TestRegistry_RejectsReservedBucket(t *testing.T) {
	r := NewBuiltinRegistry()
	now := time.Now().UTC()
	// `note` is reserved — author cannot claim x.note (stripped == "note").
	err := r.RegisterType("x.note.created", "schemas/note2.yaml", now)
	require.Error(t, err)
}

func TestRegistry_BuiltinCannotBeAuthor(t *testing.T) {
	r := NewBuiltinRegistry()
	now := time.Now().UTC()
	err := r.RegisterType("note.created", "schemas/note.yaml", now)
	require.Error(t, err, "built-in type cannot be claimed by non-builtin owner")
}

func TestRegistry_Deprecation(t *testing.T) {
	r := NewBuiltinRegistry()
	now := time.Now().UTC()
	require.NoError(t, r.RegisterType("x.recipe.cooked", "schemas/recipe.yaml", now))
	require.True(t, r.IsKnownType("x.recipe.cooked"))
	require.NoError(t, r.MarkDeprecated("x.recipe.cooked"))
	// Deprecated still valid for emission per §6.6.
	require.True(t, r.IsKnownType("x.recipe.cooked"))
}
