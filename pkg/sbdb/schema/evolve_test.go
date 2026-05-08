package schema

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompatCheck_AllValid(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs/notes"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs/notes", "a.md"),
		[]byte("---\nid: a\n---\n# hello\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs/notes", "b.md"),
		[]byte("---\nid: b\n---\n# world\n"), 0o644))

	src := []byte(`
x-entity: notes
x-storage: { docs_dir: docs/notes, filename: "{id}.md" }
x-id: id
type: object
required: [id]
properties:
  id: { type: string }
`)
	s, err := ParseJSONSchema(src)
	require.NoError(t, err)
	report, err := CheckExisting(s, dir)
	require.NoError(t, err)
	require.Empty(t, report.Failures)
}

func TestCompatCheck_ReportsMissingRequired(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs/notes"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs/notes", "a.md"),
		[]byte("---\nid: a\n---\n# hello\n"), 0o644))

	src := []byte(`
x-entity: notes
x-storage: { docs_dir: docs/notes, filename: "{id}.md" }
x-id: id
type: object
required: [id, created]
properties:
  id:      { type: string }
  created: { type: string, format: date }
`)
	s, _ := ParseJSONSchema(src)
	report, err := CheckExisting(s, dir)
	require.NoError(t, err)
	require.Len(t, report.Failures, 1)
}
