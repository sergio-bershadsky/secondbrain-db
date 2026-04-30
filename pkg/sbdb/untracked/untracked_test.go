package untracked

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "data"), 0o755)
	os.MkdirAll(filepath.Join(dir, "docs", "notes"), 0o755)
	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

// --- Registry tests ---

func TestRegistry_LoadEmpty(t *testing.T) {
	dir := setupTestDir(t)
	reg, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, 1, reg.Version)
	assert.Empty(t, reg.Entries)
}

func TestRegistry_SaveAndLoad(t *testing.T) {
	dir := setupTestDir(t)
	reg := &Registry{Version: 1}
	reg.Add(Entry{File: "docs/notes/TEMPLATE.md", ContentSHA: "abc123"})
	reg.Add(Entry{File: "docs/index.md", ContentSHA: "def456"})

	require.NoError(t, reg.Save(dir))

	loaded, err := Load(dir)
	require.NoError(t, err)
	assert.Len(t, loaded.Entries, 2)
	assert.Equal(t, "docs/notes/TEMPLATE.md", loaded.Entries[0].File)
	assert.Equal(t, "abc123", loaded.Entries[0].ContentSHA)
}

func TestRegistry_AddUpdate(t *testing.T) {
	reg := &Registry{Version: 1}
	reg.Add(Entry{File: "a.md", ContentSHA: "v1"})
	reg.Add(Entry{File: "a.md", ContentSHA: "v2"}) // update

	assert.Len(t, reg.Entries, 1)
	assert.Equal(t, "v2", reg.Entries[0].ContentSHA)
}

func TestRegistry_Remove(t *testing.T) {
	reg := &Registry{Version: 1}
	reg.Add(Entry{File: "a.md"})
	reg.Add(Entry{File: "b.md"})

	assert.True(t, reg.Remove("a.md"))
	assert.Len(t, reg.Entries, 1)
	assert.Equal(t, "b.md", reg.Entries[0].File)

	assert.False(t, reg.Remove("nonexistent.md"))
}

func TestRegistry_GetAndHas(t *testing.T) {
	reg := &Registry{Version: 1}
	reg.Add(Entry{File: "a.md", ContentSHA: "sha1"})

	assert.True(t, reg.Has("a.md"))
	assert.False(t, reg.Has("b.md"))

	entry := reg.Get("a.md")
	require.NotNil(t, entry)
	assert.Equal(t, "sha1", entry.ContentSHA)

	assert.Nil(t, reg.Get("b.md"))
}

// --- Sign tests ---

func TestSignFile(t *testing.T) {
	dir := setupTestDir(t)
	relPath := "docs/notes/TEMPLATE.md"
	writeFile(t, filepath.Join(dir, relPath), "---\ntitle: Template\n---\n\n# Template\n\nUse this template.\n")

	entry, err := SignFile(dir, relPath, nil)
	require.NoError(t, err)
	assert.Equal(t, relPath, entry.File)
	assert.NotEmpty(t, entry.ContentSHA)
	assert.NotEmpty(t, entry.FrontmatterSHA)
	assert.Empty(t, entry.Sig) // no key
}

func TestSignFile_WithKey(t *testing.T) {
	dir := setupTestDir(t)
	relPath := "docs/notes/TEMPLATE.md"
	writeFile(t, filepath.Join(dir, relPath), "# Template\n")

	key := []byte("test-key-32-bytes-long-00000000")
	entry, err := SignFile(dir, relPath, key)
	require.NoError(t, err)
	assert.NotEmpty(t, entry.Sig)
}

func TestVerifyFile_Clean(t *testing.T) {
	dir := setupTestDir(t)
	relPath := "docs/notes/TEMPLATE.md"
	content := "# Template\n\nContent here.\n"
	writeFile(t, filepath.Join(dir, relPath), content)

	entry, _ := SignFile(dir, relPath, nil)
	check := VerifyFile(entry, dir)
	assert.Nil(t, check, "clean file should pass verification")
}

func TestVerifyFile_Tampered(t *testing.T) {
	dir := setupTestDir(t)
	relPath := "docs/notes/TEMPLATE.md"
	writeFile(t, filepath.Join(dir, relPath), "# Template\n\nOriginal.\n")

	entry, _ := SignFile(dir, relPath, nil)

	// Tamper the file
	writeFile(t, filepath.Join(dir, relPath), "# Template\n\nTAMPERED.\n")

	check := VerifyFile(entry, dir)
	require.NotNil(t, check)
	assert.Contains(t, check.Mismatched, "content")
}

func TestVerifyFile_Missing(t *testing.T) {
	dir := setupTestDir(t)
	entry := &Entry{File: "docs/nonexistent.md", ContentSHA: "abc"}

	check := VerifyFile(entry, dir)
	require.NotNil(t, check)
	assert.Contains(t, check.Mismatched, "missing")
}

// --- Discover tests ---

func TestDiscoverUnregistered(t *testing.T) {
	dir := setupTestDir(t)

	// Create some files
	writeFile(t, filepath.Join(dir, "docs", "notes", "note-1.md"), "# Note 1\n")
	writeFile(t, filepath.Join(dir, "docs", "notes", "TEMPLATE.md"), "# Template\n")
	writeFile(t, filepath.Join(dir, "docs", "index.md"), "# Home\n")
	writeFile(t, filepath.Join(dir, "docs", "guides", "setup.md"), "# Setup\n")

	// Register TEMPLATE.md as untracked
	reg := &Registry{Version: 1}
	reg.Add(Entry{File: "docs/notes/TEMPLATE.md", ContentSHA: "x"})

	// No schemas — everything is either untracked or unregistered
	unregistered, err := DiscoverUnregistered(
		filepath.Join(dir, "docs"), dir, nil, reg,
	)
	require.NoError(t, err)

	// TEMPLATE.md is tracked, so not in unregistered
	// note-1.md, index.md, guides/setup.md should be unregistered
	assert.Len(t, unregistered, 3)

	files := map[string]bool{}
	for _, f := range unregistered {
		files[f] = true
	}
	assert.True(t, files["docs/notes/note-1.md"])
	assert.True(t, files["docs/index.md"])
	assert.True(t, files["docs/guides/setup.md"])
	assert.False(t, files["docs/notes/TEMPLATE.md"])
}

func TestClassifyFile(t *testing.T) {
	reg := &Registry{Version: 1}
	reg.Add(Entry{File: "docs/notes/TEMPLATE.md"})

	// Untracked
	assert.Equal(t, ClassUntracked, ClassifyFile("docs/notes/TEMPLATE.md", nil, "", reg))

	// Unregistered (no schemas, not in registry)
	assert.Equal(t, ClassUnregistered, ClassifyFile("docs/notes/random.md", nil, "", reg))
}
