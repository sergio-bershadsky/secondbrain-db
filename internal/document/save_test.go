package document

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sergio-bershadsky/secondbrain-db/internal/integrity"
)

func TestSave_WritesSidecar(t *testing.T) {
	s, rt, basePath := setupTest(t)

	doc := New(s, basePath)
	doc.Data = map[string]any{
		"id":      "alpha",
		"created": "2026-04-28",
		"status":  "active",
	}
	doc.Content = "# Alpha"

	require.NoError(t, doc.Save(rt))

	mdPath := filepath.Join(basePath, "docs/notes/alpha.md")
	sidecarPath := filepath.Join(basePath, "docs/notes/alpha.yaml")
	assert.FileExists(t, mdPath)
	assert.FileExists(t, sidecarPath)
	// In sidecar mode, no aggregate files should be written.
	assert.NoFileExists(t, filepath.Join(basePath, "data/notes/records.yaml"))
	assert.NoFileExists(t, filepath.Join(basePath, "data/notes/.integrity.yaml"))

	sc, err := integrity.LoadSidecar(mdPath)
	require.NoError(t, err)
	assert.Equal(t, "alpha.md", sc.File)
	assert.NotEmpty(t, sc.ContentSHA)
	assert.NotEmpty(t, sc.FrontmatterSHA)
	assert.NotEmpty(t, sc.RecordSHA)
}

func TestDelete_RemovesSidecar(t *testing.T) {
	s, rt, basePath := setupTest(t)

	doc := New(s, basePath)
	doc.Data = map[string]any{
		"id":      "alpha",
		"created": "2026-04-28",
		"status":  "active",
	}
	doc.Content = "# Alpha"
	require.NoError(t, doc.Save(rt))

	require.NoError(t, doc.Delete())

	mdPath := filepath.Join(basePath, "docs/notes/alpha.md")
	assert.NoFileExists(t, mdPath)
	assert.NoFileExists(t, filepath.Join(basePath, "docs/notes/alpha.yaml"))
}
