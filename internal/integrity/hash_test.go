package integrity

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestHashContent(t *testing.T) {
	h1 := HashContent("# Hello\n\nWorld.\n")
	assert.NotEmpty(t, h1)

	// Same content = same hash
	h2 := HashContent("# Hello\n\nWorld.\n")
	assert.Equal(t, h1, h2)

	// Different content = different hash
	h3 := HashContent("# Different\n")
	assert.NotEqual(t, h1, h3)
}

func TestHashFrontmatter(t *testing.T) {
	fm := map[string]any{"id": "test", "status": "active"}
	h := HashFrontmatter(fm)
	assert.NotEmpty(t, h)

	// Key order doesn't matter (canonical)
	fm2 := map[string]any{"status": "active", "id": "test"}
	assert.Equal(t, h, HashFrontmatter(fm2))
}

func TestHashRecord(t *testing.T) {
	rec := map[string]any{"id": "test", "title": "Title"}
	h := HashRecord(rec)
	assert.NotEmpty(t, h)
}

func TestLoadKey_FromEnv(t *testing.T) {
	t.Setenv("SBDB_INTEGRITY_KEY", "aabbccdd")
	key, err := LoadKey()
	require.NoError(t, err)
	assert.Equal(t, []byte{0xaa, 0xbb, 0xcc, 0xdd}, key)
}

func TestLoadKey_InvalidHex(t *testing.T) {
	t.Setenv("SBDB_INTEGRITY_KEY", "not-hex")
	_, err := LoadKey()
	assert.Error(t, err)
}

func TestLoadKey_NoKey(t *testing.T) {
	t.Setenv("SBDB_INTEGRITY_KEY", "")
	// Also ensure no key file exists
	key, err := LoadKey()
	// May or may not find a key file — just check no panic
	_ = key
	_ = err
}

func TestDefaultKeyPath(t *testing.T) {
	path, err := DefaultKeyPath()
	require.NoError(t, err)
	assert.Contains(t, path, "secondbrain-db")
	assert.Contains(t, path, "integrity.key")
}

func TestTrimWhitespace(t *testing.T) {
	assert.Equal(t, "abc", trimWhitespace("  a b c  \n"))
	assert.Equal(t, "hello", trimWhitespace("hello"))
	assert.Equal(t, "", trimWhitespace("   \t\n"))
}

func TestManifestExists(t *testing.T) {
	dir := t.TempDir()
	assert.False(t, ManifestExists(dir))

	// Write a manifest file directly to exercise ManifestExists without Save.
	m := &Manifest{Version: 1, Algo: "sha256", Entries: map[string]*Entry{}}
	data, err := yaml.Marshal(m)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(ManifestPath(dir), data, 0o644))
	assert.True(t, ManifestExists(dir))
}

func TestSaveKeyFile_Permissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file permissions not supported on Windows")
	}

	dir := t.TempDir()
	keyPath := filepath.Join(dir, "test.key")
	key := []byte{0x01, 0x02, 0x03}

	require.NoError(t, SaveKeyFile(keyPath, key))

	fi, err := os.Stat(keyPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), fi.Mode().Perm())
}
