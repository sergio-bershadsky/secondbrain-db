package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSchemaLint_AcceptsValidNew(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)
	path := filepath.Join(tmp, "notes.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
x-entity: notes
x-storage: { docs_dir: docs/notes, filename: "{id}.md" }
x-id: id
type: object
properties:
  id: { type: string }
`), 0o644))
	out, err := runInProcess(t, "schema", "lint", path)
	require.NoError(t, err, "out: %s", out)
}

func TestSchemaLint_RejectsMissingXEntity(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)
	path := filepath.Join(tmp, "bad.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
x-storage: { docs_dir: docs/notes, filename: "{id}.md" }
x-id: id
type: object
properties:
  id: { type: string }
`), 0o644))
	out, err := runInProcess(t, "schema", "lint", path)
	require.Error(t, err, "out: %s", out)
	require.True(t, strings.Contains(out, "x-entity") || strings.Contains(out, "missing"),
		"expected output to mention x-entity or missing, got: %s", out)
}
