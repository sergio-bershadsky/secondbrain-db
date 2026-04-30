package integrity

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManifest_LoadNonExistent(t *testing.T) {
	m, err := LoadManifest("/nonexistent/path")
	require.NoError(t, err)
	assert.Empty(t, m.Entries)
	assert.Equal(t, "sha256", m.Algo)
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

	// Verify permissions (POSIX only)
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(keyPath)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), fi.Mode().Perm())
	}
}
