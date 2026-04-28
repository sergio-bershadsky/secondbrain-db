package integrity

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestSidecarPath(t *testing.T) {
	assert.Equal(t, "/x/docs/notes/hello.yaml", SidecarPath("/x/docs/notes/hello.md"))
	assert.Equal(t, "/x/docs/notes/2026-04/hello.yaml", SidecarPath("/x/docs/notes/2026-04/hello.md"))
}

func TestSidecar_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "hello.md")
	require.NoError(t, os.WriteFile(mdPath, []byte("# hi"), 0o644))

	sc := &Sidecar{
		Version:        1,
		Algo:           "sha256",
		HMAC:           false,
		File:           "hello.md",
		ContentSHA:     "aaa",
		FrontmatterSHA: "bbb",
		RecordSHA:      "ccc",
		UpdatedAt:      "2026-04-28T00:00:00Z",
		Writer:         "secondbrain-db/test",
	}
	require.NoError(t, sc.Save(mdPath))

	got, err := LoadSidecar(mdPath)
	require.NoError(t, err)
	assert.Equal(t, sc.ContentSHA, got.ContentSHA)
	assert.Equal(t, sc.File, got.File)
	assert.False(t, got.HMAC)
}

func TestSidecar_LoadMissingReturnsErrIfNotExist(t *testing.T) {
	_, err := LoadSidecar(filepath.Join(t.TempDir(), "missing.md"))
	require.Error(t, err)
	assert.True(t, os.IsNotExist(err))
}

func TestSidecar_HMAC_SignAndVerify(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "hello.md")
	require.NoError(t, os.WriteFile(mdPath, []byte("# hi"), 0o644))

	key := []byte("test-key-32-bytes-aaaaaaaaaaaaaaaa")
	sc := &Sidecar{
		Version: 1, Algo: "sha256", HMAC: true,
		File:           "hello.md",
		ContentSHA:     "aaa",
		FrontmatterSHA: "bbb",
		RecordSHA:      "ccc",
		UpdatedAt:      "2026-04-28T00:00:00Z",
		Writer:         "secondbrain-db/test",
	}
	sc.Sig = sc.SignWith(key)
	require.NoError(t, sc.Save(mdPath))

	got, err := LoadSidecar(mdPath)
	require.NoError(t, err)
	assert.True(t, got.VerifyWith(key))
	assert.False(t, got.VerifyWith([]byte("wrong-key-32-bytes-bbbbbbbbbbbbbb")))
}

func TestSidecar_Verify_DriftDetection(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "hello.md")
	body := []byte("# hi\nbody\n")
	require.NoError(t, os.WriteFile(mdPath, body, 0o644))

	sc := &Sidecar{
		Version: 1, Algo: "sha256", HMAC: false,
		File:           "hello.md",
		ContentSHA:     HashContent("# hi\nbody\n"),
		FrontmatterSHA: HashFrontmatter(map[string]any{}),
		RecordSHA:      HashRecord(map[string]any{}),
	}

	d, err := sc.Verify(mdPath, map[string]any{}, "# hi\nbody\n", map[string]any{}, nil)
	require.NoError(t, err)
	assert.False(t, d.Any())

	d2, err := sc.Verify(mdPath, map[string]any{}, "# hi\ntampered\n", map[string]any{}, nil)
	require.NoError(t, err)
	assert.True(t, d2.ContentDrift)
}

func TestSidecar_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "hello.md")
	require.NoError(t, os.WriteFile(mdPath, []byte("# hi"), 0o644))

	sc := &Sidecar{Version: 1, Algo: "sha256", File: "hello.md"}
	require.NoError(t, sc.Save(mdPath))
	require.NoError(t, sc.Save(mdPath))

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		assert.False(t, filepath.Ext(e.Name()) == ".tmp", "leftover tmp file: %s", e.Name())
	}
}

func TestSidecar_YAMLLayoutStable(t *testing.T) {
	sc := &Sidecar{
		Version: 1, Algo: "sha256", HMAC: true, File: "hello.md",
		ContentSHA: "a", FrontmatterSHA: "b", RecordSHA: "c", Sig: "s",
		UpdatedAt: "2026-04-28T00:00:00Z", Writer: "secondbrain-db/test",
	}
	out, err := yaml.Marshal(sc)
	require.NoError(t, err)
	assert.Contains(t, string(out), "content_sha: a")
	assert.Contains(t, string(out), "hmac: true")
}
