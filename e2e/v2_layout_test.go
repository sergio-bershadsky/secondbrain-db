//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// v2NoteSchema is a minimal v2-style schema: no records_dir, single-brace
// filename template, and integrity off so tests don't need an HMAC key.
const v2NoteSchema = `version: 1
entity: note
docs_dir: docs/note
filename: "{id}.md"
id_field: id
integrity: off
fields:
  id: { type: string, required: true }
  title: { type: string, required: true }
  status: { type: string }
`

// newV2Project creates a tempdir with the v2 note schema and a minimal
// .sbdb.toml. Unlike newProject it does NOT create a data/ directory.
func newV2Project(t *testing.T) *project {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "schemas"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs", "note"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "schemas", "note.yaml"),
		[]byte(v2NoteSchema),
		0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, ".sbdb.toml"),
		[]byte(`default_schema = "note"`+"\n"),
		0o644,
	))
	return &project{root: root, binary: ensureBinary(t)}
}

// TestE2E_V2_FullCRUD_NoDataDir runs a full create/update/check/delete cycle
// against a freshly-initialized project and asserts that no data/ directory
// is ever created. It is the central success criterion for sbdb v2 layout.
func TestE2E_V2_FullCRUD_NoDataDir(t *testing.T) {
	p := newV2Project(t)

	// Create
	out := p.runOK(t, "create", "-s", "note",
		"--field", "id=hello",
		"--field", "title=Hello",
		"--field", "status=active",
		"--content", "# Hello\n\nFirst note.")
	_ = out

	mdPath := filepath.Join(p.root, "docs/note/hello.md")
	sidecarPath := filepath.Join(p.root, "docs/note/hello.yaml")
	assert.FileExists(t, mdPath)
	assert.FileExists(t, sidecarPath)
	assert.NoDirExists(t, filepath.Join(p.root, "data"))

	// Check
	p.runOK(t, "doctor", "check", "--all", "-s", "note")

	// Update
	p.runOK(t, "update", "-s", "note", "--id", "hello",
		"--field", "status=archived")
	p.runOK(t, "doctor", "check", "--all", "-s", "note")

	// Delete
	p.runOK(t, "delete", "-s", "note", "--id", "hello", "--yes")
	_, err := os.Stat(mdPath)
	require.True(t, os.IsNotExist(err))
	_, err = os.Stat(sidecarPath)
	require.True(t, os.IsNotExist(err))

	// Still no data/ at the end.
	assert.NoDirExists(t, filepath.Join(p.root, "data"))
}
