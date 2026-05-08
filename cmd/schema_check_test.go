package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSchemaCheck_PassesWhenAllValid(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "schemas"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "schemas/notes.yaml"), []byte(`
x-entity: notes
x-storage: { docs_dir: docs/notes, filename: "{id}.md" }
x-id: id
type: object
required: [id]
properties:
  id: { type: string }
`), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "docs/notes"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "docs/notes/a.md"),
		[]byte("---\nid: a\n---\n# hi\n"), 0o644))

	_, err := runInProcess(t, "schema", "check")
	require.NoError(t, err)
}
