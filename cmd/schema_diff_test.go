package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSchemaDiff_BreakingExitsNonzero(t *testing.T) {
	tmp := t.TempDir()
	oldP := filepath.Join(tmp, "old.yaml")
	newP := filepath.Join(tmp, "new.yaml")
	require.NoError(t, os.WriteFile(oldP, []byte(`
x-entity: notes
x-storage: { docs_dir: d, filename: "{id}.md" }
x-id: id
type: object
required: [id]
properties:
  id: { type: string }
`), 0o644))
	require.NoError(t, os.WriteFile(newP, []byte(`
x-entity: notes
x-storage: { docs_dir: d, filename: "{id}.md" }
x-id: id
type: object
required: [id, created]
properties:
  id:      { type: string }
  created: { type: string, format: date }
`), 0o644))
	out, err := runInProcess(t, "schema", "diff", oldP, newP)
	require.Error(t, err, "out: %s", out)
}

func TestSchemaDiff_AdditiveExitsZero(t *testing.T) {
	tmp := t.TempDir()
	oldP := filepath.Join(tmp, "old.yaml")
	newP := filepath.Join(tmp, "new.yaml")
	require.NoError(t, os.WriteFile(oldP, []byte(`
x-entity: notes
x-storage: { docs_dir: d, filename: "{id}.md" }
x-id: id
type: object
required: [id]
properties:
  id: { type: string }
`), 0o644))
	require.NoError(t, os.WriteFile(newP, []byte(`
x-entity: notes
x-storage: { docs_dir: d, filename: "{id}.md" }
x-id: id
type: object
required: [id]
properties:
  id:    { type: string }
  notes: { type: string }
`), 0o644))
	_, err := runInProcess(t, "schema", "diff", oldP, newP)
	require.NoError(t, err)
}
