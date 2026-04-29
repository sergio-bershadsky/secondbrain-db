package sbdb_test

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/integrity"
)

// optProject is a shared setup helper for option tests.
func optProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "schemas"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "schemas/notes.yaml"), []byte(`version: 1
entity: notes
docs_dir: docs/notes
filename: "{id}.md"
id_field: id
integrity: strict
fields:
  id:      { type: string, required: true }
  created: { type: date, required: true }
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".sbdb.toml"), []byte(`schema_dir = "./schemas"
base_path = "."
[output]
format = "json"
[integrity]
key_source = "env"
`), 0o644))
	return dir
}

func TestWithClock_AffectsSidecarTimestamp(t *testing.T) {
	fixed := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	dir := optProject(t)
	db, err := sbdb.Open(context.Background(), sbdb.Config{Root: dir},
		sbdb.WithClock(func() time.Time { return fixed }))
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Repo("notes").Create(context.Background(), sbdb.Doc{
		Frontmatter: map[string]any{"id": "x", "created": "2026-04-28"},
		Content:     "# X",
	})
	require.NoError(t, err)

	sc, err := integrity.LoadSidecar(filepath.Join(dir, "docs/notes/x.md"))
	require.NoError(t, err)
	assert.Equal(t, fixed.Format(time.RFC3339), sc.UpdatedAt)
}

func TestWithIntegrityKey_SignsSidecar(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	dir := optProject(t)
	db, err := sbdb.Open(context.Background(), sbdb.Config{Root: dir},
		sbdb.WithIntegrityKey(key))
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Repo("notes").Create(context.Background(), sbdb.Doc{
		Frontmatter: map[string]any{"id": "k", "created": "2026-04-28"},
		Content:     "# K",
	})
	require.NoError(t, err)

	sc, err := integrity.LoadSidecar(filepath.Join(dir, "docs/notes/k.md"))
	require.NoError(t, err)
	assert.True(t, sc.HMAC, "HMAC should be true with key configured")
	assert.NotEmpty(t, sc.Sig)
	assert.True(t, sc.VerifyWith(key), "sig should verify with the configured key")
}

func TestWithIntegrityKeyLoader_OverridesDefault(t *testing.T) {
	customKey := []byte("aaaa1111bbbb2222cccc3333dddd4444")
	dir := optProject(t)

	called := false
	db, err := sbdb.Open(context.Background(), sbdb.Config{Root: dir},
		sbdb.WithIntegrityKeyLoader(func(ctx context.Context) ([]byte, error) {
			called = true
			return customKey, nil
		}))
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Repo("notes").Create(context.Background(), sbdb.Doc{
		Frontmatter: map[string]any{"id": "L", "created": "2026-04-28"},
		Content:     "# L",
	})
	require.NoError(t, err)

	assert.True(t, called, "custom key loader should have been invoked")
	sc, err := integrity.LoadSidecar(filepath.Join(dir, "docs/notes/L.md"))
	require.NoError(t, err)
	assert.True(t, sc.VerifyWith(customKey))
}

func TestWithLogger_RoutesWarnings(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	// Use a schema with a deprecated field so the loader emits a warning.
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "schemas"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "schemas/notes.yaml"), []byte(`version: 1
entity: notes
docs_dir: docs/notes
filename: "{id}.md"
id_field: id
records_dir: data/notes
fields:
  id: { type: string, required: true }
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".sbdb.toml"), []byte(`schema_dir = "./schemas"
base_path = "."
[output]
format = "json"
`), 0o644))

	db, err := sbdb.Open(context.Background(), sbdb.Config{Root: dir},
		sbdb.WithLogger(logger))
	require.NoError(t, err)
	defer db.Close()

	// Don't pin down exact log message text — just that *something* was logged
	// through the injected logger when the deprecated schema was loaded.
	assert.NotEmpty(t, buf.String(), "logger should have captured at least one warning")
}

func TestWithWalkWorkers_AcceptsCustom(t *testing.T) {
	dir := optProject(t)
	db, err := sbdb.Open(context.Background(), sbdb.Config{Root: dir},
		sbdb.WithWalkWorkers(2))
	require.NoError(t, err)
	defer db.Close()

	// Create some docs and verify the query still works with the custom
	// worker count. We don't assert HOW concurrency was applied (that would
	// pin internals); we assert the feature still functions.
	dates := []string{"2026-04-20", "2026-04-21", "2026-04-22"}
	for i, id := range []string{"a", "b", "c"} {
		_, err := db.Repo("notes").Create(context.Background(), sbdb.Doc{
			Frontmatter: map[string]any{"id": id, "created": dates[i]},
			Content:     "# " + id,
		})
		require.NoError(t, err)
	}

	recs, err := db.Repo("notes").Query().Records()
	require.NoError(t, err)
	assert.Len(t, recs, 3)
}
