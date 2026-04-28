package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWalkDocs_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	docs, err := WalkDocsToSlice(dir)
	require.NoError(t, err)
	assert.Empty(t, docs)
}

func TestWalkDocs_FindsMDFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "alpha.md"),
		[]byte("---\nid: alpha\nstatus: active\n---\n# Alpha\n\nbody"),
		0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "beta.md"),
		[]byte("---\nid: beta\nstatus: archived\n---\n# Beta\n"),
		0o644,
	))

	docs, err := WalkDocsToSlice(dir)
	require.NoError(t, err)
	require.Len(t, docs, 2)

	byID := map[string]Doc{}
	for _, d := range docs {
		id, _ := d.Frontmatter["id"].(string)
		byID[id] = d
	}
	assert.Equal(t, "active", byID["alpha"].Frontmatter["status"])
	assert.Equal(t, "archived", byID["beta"].Frontmatter["status"])
	assert.Contains(t, byID["alpha"].Body, "body")
}

func TestWalkDocs_SkipsSidecarsAndNonMD(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "alpha.md"), []byte("---\nid: alpha\n---\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "alpha.yaml"), []byte("file: alpha.md\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.txt"), []byte("noise"), 0o644))

	docs, err := WalkDocsToSlice(dir)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "alpha", docs[0].Frontmatter["id"])
}

func TestWalkDocs_RecursesSubdirs(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "2026-04")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(sub, "alpha.md"),
		[]byte("---\nid: alpha\n---\n"),
		0o644,
	))

	docs, err := WalkDocsToSlice(dir)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, filepath.Join(sub, "alpha.md"), docs[0].Path)
}

func TestWalkDocs_MissingDirIsEmpty(t *testing.T) {
	docs, err := WalkDocsToSlice(filepath.Join(t.TempDir(), "missing"))
	require.NoError(t, err)
	assert.Empty(t, docs)
}

func TestWalkDocs_PropagatesParseError(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "bad.md"),
		[]byte("---\nthis: [is: not: valid: yaml\n---\nbody"),
		0o644,
	))
	_, err := WalkDocsToSlice(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bad.md")
}
