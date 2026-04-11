package integrity

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManifest_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()

	m := &Manifest{
		Version: 1,
		Algo:    "sha256",
		Entries: map[string]*Entry{
			"note-1": {
				File:           "docs/notes/note-1.md",
				ContentSHA:     "abc123",
				FrontmatterSHA: "def456",
				RecordSHA:      "ghi789",
			},
		},
	}

	require.NoError(t, m.Save(dir))

	loaded, err := LoadManifest(dir)
	require.NoError(t, err)
	require.Len(t, loaded.Entries, 1)
	assert.Equal(t, "abc123", loaded.Entries["note-1"].ContentSHA)
	assert.Equal(t, "sha256", loaded.Algo)
}

func TestManifest_LoadNonExistent(t *testing.T) {
	m, err := LoadManifest("/nonexistent/path")
	require.NoError(t, err)
	assert.Empty(t, m.Entries)
	assert.Equal(t, "sha256", m.Algo)
}

func TestManifest_SetAndRemoveEntry(t *testing.T) {
	m := &Manifest{
		Version: 1,
		Algo:    "sha256",
		Entries: make(map[string]*Entry),
	}

	m.SetEntry("a", &Entry{ContentSHA: "hash_a"})
	m.SetEntry("b", &Entry{ContentSHA: "hash_b"})
	assert.Len(t, m.Entries, 2)

	m.RemoveEntry("a")
	assert.Len(t, m.Entries, 1)
	assert.Contains(t, m.Entries, "b")
}

func TestVerify_Match(t *testing.T) {
	entry := &Entry{
		ContentSHA:     "aaa",
		FrontmatterSHA: "bbb",
		RecordSHA:      "ccc",
	}
	check := Verify(entry, "aaa", "bbb", "ccc")
	assert.Nil(t, check, "matching hashes should return nil")
}

func TestVerify_ContentMismatch(t *testing.T) {
	entry := &Entry{
		ContentSHA:     "aaa",
		FrontmatterSHA: "bbb",
		RecordSHA:      "ccc",
	}
	check := Verify(entry, "CHANGED", "bbb", "ccc")
	require.NotNil(t, check)
	assert.Contains(t, check.Mismatched, "content")
	assert.NotContains(t, check.Mismatched, "frontmatter")
}

func TestVerify_MultipleMismatches(t *testing.T) {
	entry := &Entry{
		ContentSHA:     "aaa",
		FrontmatterSHA: "bbb",
		RecordSHA:      "ccc",
	}
	check := Verify(entry, "CHANGED", "bbb", "ALSO_CHANGED")
	require.NotNil(t, check)
	assert.Contains(t, check.Mismatched, "content")
	assert.Contains(t, check.Mismatched, "record")
}

func TestHMAC_SignAndVerify(t *testing.T) {
	key := []byte("test-secret-key-for-signing-000")
	entry := &Entry{
		ContentSHA:     "aaa",
		FrontmatterSHA: "bbb",
		RecordSHA:      "ccc",
	}

	entry.Sig = SignEntry(entry, key)
	assert.NotEmpty(t, entry.Sig)
	assert.True(t, VerifySignature(entry, key))

	// Wrong key should fail
	assert.False(t, VerifySignature(entry, []byte("wrong-key-here-000000000000000")))

	// Tampered entry should fail
	entry.ContentSHA = "tampered"
	assert.False(t, VerifySignature(entry, key))
}

func TestGenerateKey(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)
	assert.Len(t, key, 32)
}

func TestSaveAndLoadKeyFile(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "test.key")

	key, _ := GenerateKey()
	require.NoError(t, SaveKeyFile(keyPath, key))

	// Verify permissions
	fi, err := os.Stat(keyPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), fi.Mode().Perm())
}
