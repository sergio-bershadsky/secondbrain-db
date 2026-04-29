package untracked

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/schema"
)

// TestClassifyFile_SchemaManagedHasSidecar: a file under docs_dir with a
// sibling .yaml sidecar is classified as schema-managed.
func TestClassifyFile_SchemaManagedHasSidecar(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs/notes"), 0o755))
	mdPath := filepath.Join(dir, "docs/notes/hello.md")
	require.NoError(t, os.WriteFile(mdPath, []byte("---\nid: hello\n---\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs/notes/hello.yaml"), []byte("version: 1\n"), 0o644))

	s := &schema.Schema{Entity: "notes", DocsDir: "docs/notes"}
	reg := &Registry{Version: 1}
	cls := ClassifyFile("docs/notes/hello.md", []*schema.Schema{s}, dir, reg)
	assert.Equal(t, ClassSchemaManaged, cls)
}

// TestClassifyFile_NoSidecarIsUnregistered: a file under docs_dir WITHOUT a
// sidecar is unregistered.
func TestClassifyFile_NoSidecarIsUnregistered(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs/notes"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs/notes/orphan.md"), []byte("---\nid: orphan\n---\n"), 0o644))

	s := &schema.Schema{Entity: "notes", DocsDir: "docs/notes"}
	reg := &Registry{Version: 1}
	cls := ClassifyFile("docs/notes/orphan.md", []*schema.Schema{s}, dir, reg)
	assert.Equal(t, ClassUnregistered, cls)
}

// TestClassifyFile_OutsideDocsDir: a file outside any schema's docs_dir is
// unregistered.
func TestClassifyFile_OutsideDocsDir(t *testing.T) {
	dir := t.TempDir()

	s := &schema.Schema{Entity: "notes", DocsDir: "docs/notes"}
	reg := &Registry{Version: 1}
	cls := ClassifyFile("README.md", []*schema.Schema{s}, dir, reg)
	assert.Equal(t, ClassUnregistered, cls)
}

// TestClassifyFile_RegisteredAsUntracked: a file in the registry is classified
// as Untracked regardless of docs_dir membership.
func TestClassifyFile_RegisteredAsUntracked(t *testing.T) {
	dir := t.TempDir()
	reg := &Registry{Version: 1}
	reg.Add(Entry{File: "README.md", ContentSHA: "abc"})

	s := &schema.Schema{Entity: "notes", DocsDir: "docs/notes"}
	cls := ClassifyFile("README.md", []*schema.Schema{s}, dir, reg)
	assert.Equal(t, ClassUntracked, cls)
}
