package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSchemaMigrate_RewritesLegacy(t *testing.T) {
	tmp := t.TempDir()
	in := filepath.Join(tmp, "notes.yaml")
	require.NoError(t, os.WriteFile(in, []byte(`
version: 1
entity: notes
docs_dir: docs/notes
filename: "{id}.md"
id_field: id
integrity: strict
fields:
  id: { type: string, required: true }
`), 0o644))
	_, err := runInProcess(t, "schema", "migrate", "--in-place", in)
	require.NoError(t, err)
	out, _ := os.ReadFile(in)
	require.True(t, strings.Contains(string(out), "x-entity"))
	require.True(t, strings.Contains(string(out), "$schema"))
	require.False(t, strings.Contains(string(out), "fields:"))
}

func TestSchemaMigrate_CheckExitsNonzeroOnLegacy(t *testing.T) {
	tmp := t.TempDir()
	in := filepath.Join(tmp, "notes.yaml")
	require.NoError(t, os.WriteFile(in, []byte(`
version: 1
entity: notes
docs_dir: docs/notes
filename: "{id}.md"
id_field: id
fields:
  id: { type: string, required: true }
`), 0o644))
	_, err := runInProcess(t, "schema", "migrate", "--check", in)
	require.Error(t, err)
}
