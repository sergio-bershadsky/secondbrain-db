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
		ID:    "docs/notes/foo.md",
		SHA:   "abc123",
		Op:    "01HW3R8M",
		Actor: "alice@example.com",
	}
	line, err := e.MarshalLine()
	require.NoError(t, err)
	got := string(line)
	require.True(t, strings.HasSuffix(got, "\n"))

	keys := []string{`"ts":`, `"type":`, `"id":`, `"sha":`, `"op":`, `"actor":`}
	last := -1
	for _, k := range keys {
		idx := strings.Index(got, k)
		require.GreaterOrEqual(t, idx, 0, "missing %s in %s", k, got)
		require.Greater(t, idx, last, "key %s out of order in %s", k, got)
		last = idx
	}
}

func TestMarshalLine_OmitsEmptyOptionals(t *testing.T) {
	e := &Event{
		TS:   time.Date(2026, 4, 24, 14, 32, 1, 0, time.UTC),
		Type: "note.deleted",
		ID:   "docs/notes/foo.md",
	}
	line, err := e.MarshalLine()
	require.NoError(t, err)
	got := string(line)
	for _, key := range []string{`"sha":`, `"prev":`, `"op":`, `"actor":`} {
		require.NotContains(t, got, key)
	}
	require.NotContains(t, got, "null")
}

func TestValidate_RejectsBadType(t *testing.T) {
	cases := map[string]bool{
		"note.created":    true,
		"task.updated":    true,
		"meta.archived":   true,
		"":                false,
		"NoteCreated":     false,
		"note":            false, // need bucket.verb
		"note..created":   false,
		"x.recipe.cooked": false, // closed catalog: bucket.verb only
	}
	for typeName, want := range cases {
		t.Run(typeName, func(t *testing.T) {
			require.Equal(t, want, ValidTypeName(typeName))
		})
	}
}

func TestVerbForStatus(t *testing.T) {
	require.Equal(t, "created", VerbForStatus('A'))
	require.Equal(t, "created", VerbForStatus('C'))
	require.Equal(t, "updated", VerbForStatus('M'))
	require.Equal(t, "deleted", VerbForStatus('D'))
	require.Equal(t, "", VerbForStatus('T')) // type-change: ignored
	require.Equal(t, "", VerbForStatus('U')) // unmerged: ignored
	require.Equal(t, "", VerbForStatus('R')) // renames are projected with --no-renames; never seen
}

func TestBucket_Verb(t *testing.T) {
	require.Equal(t, "note", Bucket("note.created"))
	require.Equal(t, "created", Verb("note.created"))
	require.Equal(t, "task", Bucket("task.updated"))
	require.Equal(t, "updated", Verb("task.updated"))
}
