package sbdb_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb"
)

func setupTestProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "schemas"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "schemas/notes.yaml"),
		[]byte(`version: 1
entity: notes
docs_dir: docs/notes
filename: "{id}.md"
id_field: id
integrity: off
fields:
  id:      { type: string, required: true }
  status:  { type: string }
  created: { type: date, required: true }
`),
		0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".sbdb.toml"),
		[]byte("schema_dir = \"./schemas\"\nbase_path = \".\"\n"),
		0o644,
	))
	return dir
}

func TestPublicAPI_RoundTrip(t *testing.T) {
	ctx := context.Background()
	dir := setupTestProject(t)

	db, err := sbdb.Open(ctx, sbdb.Config{Root: dir})
	require.NoError(t, err)
	defer db.Close()

	notes := db.Repo("notes")

	// Create
	_, err = notes.Create(ctx, sbdb.Doc{
		Frontmatter: map[string]any{
			"id":      "hello",
			"status":  "active",
			"created": "2026-04-28",
		},
		Content: "# Hello\n",
	})
	require.NoError(t, err)

	// Get
	got, err := notes.Get(ctx, "hello")
	require.NoError(t, err)
	assert.Equal(t, "hello", got.ID)
	assert.Equal(t, "active", got.Frontmatter["status"])

	// Query
	recs, err := notes.Query().Filter(map[string]any{"status": "active"}).Records()
	require.NoError(t, err)
	assert.Len(t, recs, 1)

	// Update via mutator
	_, err = notes.Update(ctx, "hello", func(d sbdb.Doc) sbdb.Doc {
		d.Frontmatter["status"] = "archived"
		return d
	})
	require.NoError(t, err)

	after, err := notes.Get(ctx, "hello")
	require.NoError(t, err)
	assert.Equal(t, "archived", after.Frontmatter["status"])

	// Delete
	require.NoError(t, notes.Delete(ctx, "hello"))

	// Subsequent Get reports ErrNotFound.
	_, err = notes.Get(ctx, "hello")
	require.Error(t, err)
	assert.ErrorIs(t, err, sbdb.ErrNotFound)
}

func TestPublicAPI_OpenUnknownEntity(t *testing.T) {
	ctx := context.Background()
	dir := setupTestProject(t)
	db, err := sbdb.Open(ctx, sbdb.Config{Root: dir})
	require.NoError(t, err)
	defer db.Close()

	_, err = db.RepoErr("nonexistent")
	assert.ErrorIs(t, err, sbdb.ErrUnknownEntity)
}

func TestPublicAPI_CreateDuplicateRejected(t *testing.T) {
	ctx := context.Background()
	dir := setupTestProject(t)
	db, err := sbdb.Open(ctx, sbdb.Config{Root: dir})
	require.NoError(t, err)
	defer db.Close()
	notes := db.Repo("notes")

	body := sbdb.Doc{
		Frontmatter: map[string]any{
			"id": "hello", "created": "2026-04-28", "status": "active",
		},
		Content: "# Hi",
	}
	_, err = notes.Create(ctx, body)
	require.NoError(t, err)
	_, err = notes.Create(ctx, body)
	require.Error(t, err)
	assert.ErrorIs(t, err, sbdb.ErrConflict)
}
