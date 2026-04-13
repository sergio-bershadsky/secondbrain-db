package document

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sergio-bershadsky/secondbrain-db/internal/integrity"
	"github.com/sergio-bershadsky/secondbrain-db/internal/schema"
	"github.com/sergio-bershadsky/secondbrain-db/internal/storage"
	"github.com/sergio-bershadsky/secondbrain-db/internal/virtuals"
)

const testSchemaYAML = `
version: 1
entity: notes
docs_dir: docs/notes
filename: "{id}.md"
records_dir: data/notes
partition: none
id_field: id
integrity: strict

fields:
  id:      { type: string, required: true }
  created: { type: date, required: true }
  status:  { type: enum, values: [active, archived], default: active }
  tags:    { type: list, items: { type: string } }

virtuals:
  title:
    returns: string
    source: |
      def compute(content, fields):
          for line in content.splitlines():
              if line.startswith("# "):
                  return line.removeprefix("# ").strip()
          return fields["id"]
  word_count:
    returns: int
    source: |
      def compute(content, fields):
          return len(content.split())
`

func setupTest(t *testing.T) (*schema.Schema, *virtuals.Runtime, string) {
	t.Helper()

	s, err := schema.Parse([]byte(testSchemaYAML))
	require.NoError(t, err)

	rt := virtuals.NewRuntime()
	for name, v := range s.Virtuals {
		require.NoError(t, rt.Compile(name, v.Source, v.Returns))
	}

	basePath := t.TempDir()
	os.MkdirAll(filepath.Join(basePath, "docs", "notes"), 0o755)
	os.MkdirAll(filepath.Join(basePath, "data", "notes"), 0o755)

	return s, rt, basePath
}

func TestDocument_CreateAndSave(t *testing.T) {
	s, rt, basePath := setupTest(t)

	doc := New(s, basePath)
	doc.Data = map[string]any{
		"id":      "test-note",
		"created": "2026-04-08",
		"status":  "active",
		"tags":    []any{"go", "test"},
	}
	doc.Content = "# My Test Note\n\nThis is a test note with some content.\n"

	require.NoError(t, doc.Save(rt))

	// Verify markdown file exists
	mdPath := filepath.Join(basePath, "docs", "notes", "test-note.md")
	_, err := os.Stat(mdPath)
	require.NoError(t, err)

	// Verify frontmatter
	fm, body, err := storage.ParseMarkdown(mdPath)
	require.NoError(t, err)
	assert.Equal(t, "test-note", fm["id"])
	assert.Equal(t, "active", fm["status"])
	assert.Equal(t, "My Test Note", fm["title"]) // virtual
	assert.Contains(t, body, "My Test Note")

	// Verify records.yaml
	records, err := storage.LoadRecords(filepath.Join(basePath, "data", "notes", "records.yaml"))
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, "test-note", records[0]["id"])
	assert.Equal(t, "My Test Note", records[0]["title"])
	assert.Equal(t, filepath.Join("docs", "notes", "test-note.md"), records[0]["file"])
	// Tags should NOT be in record (complex field)
	_, hasTags := records[0]["tags"]
	assert.False(t, hasTags, "complex fields should not be in records")

	// Verify integrity manifest
	manifest, err := integrity.LoadManifest(filepath.Join(basePath, "data", "notes"))
	require.NoError(t, err)
	require.Contains(t, manifest.Entries, "test-note")
	assert.NotEmpty(t, manifest.Entries["test-note"].ContentSHA)
}

func TestDocument_LoadFromFile(t *testing.T) {
	s, rt, basePath := setupTest(t)

	// Save first
	doc := New(s, basePath)
	doc.Data = map[string]any{"id": "load-test", "created": "2026-04-08", "status": "active"}
	doc.Content = "# Load Test\n\nContent here.\n"
	require.NoError(t, doc.Save(rt))

	// Load from file
	mdPath := filepath.Join(basePath, "docs", "notes", "load-test.md")
	loaded, err := LoadFromFile(s, basePath, mdPath)
	require.NoError(t, err)
	assert.Equal(t, "load-test", loaded.ID())
	assert.Contains(t, loaded.Content, "Load Test")
}

func TestDocument_Delete(t *testing.T) {
	s, rt, basePath := setupTest(t)

	doc := New(s, basePath)
	doc.Data = map[string]any{"id": "delete-me", "created": "2026-04-08", "status": "active"}
	doc.Content = "# Delete Me\n"
	require.NoError(t, doc.Save(rt))

	// Verify exists
	mdPath := filepath.Join(basePath, "docs", "notes", "delete-me.md")
	_, err := os.Stat(mdPath)
	require.NoError(t, err)

	// Delete
	require.NoError(t, doc.Delete())

	// Verify markdown file gone
	_, err = os.Stat(mdPath)
	assert.True(t, os.IsNotExist(err))

	// Verify record removed
	records, err := storage.LoadRecords(filepath.Join(basePath, "data", "notes", "records.yaml"))
	require.NoError(t, err)
	assert.Empty(t, records)

	// Verify manifest entry removed
	manifest, err := integrity.LoadManifest(filepath.Join(basePath, "data", "notes"))
	require.NoError(t, err)
	assert.NotContains(t, manifest.Entries, "delete-me")
}

func TestDocument_SaveIsIdempotent(t *testing.T) {
	s, rt, basePath := setupTest(t)

	doc := New(s, basePath)
	doc.Data = map[string]any{"id": "idem", "created": "2026-04-08", "status": "active"}
	doc.Content = "# Idempotent\n"

	require.NoError(t, doc.Save(rt))
	require.NoError(t, doc.Save(rt))

	records, err := storage.LoadRecords(filepath.Join(basePath, "data", "notes", "records.yaml"))
	require.NoError(t, err)
	assert.Len(t, records, 1, "save should upsert, not duplicate")
}

func TestDocument_VirtualFieldsMaterialized(t *testing.T) {
	s, rt, basePath := setupTest(t)

	doc := New(s, basePath)
	doc.Data = map[string]any{"id": "v-test", "created": "2026-04-08", "status": "active"}
	doc.Content = "# Virtual Test\n\none two three four five\n"
	require.NoError(t, doc.Save(rt))

	// Check virtuals were computed
	assert.Equal(t, "Virtual Test", doc.Virtuals()["title"])
	// word count: "# Virtual Test" (3) + "" (0) + "one two three four five" (5) = ~8
	wc := doc.Virtuals()["word_count"]
	assert.NotNil(t, wc)

	// Check record has title (scalar virtual) but not word_count if it's also scalar
	records, err := storage.LoadRecords(filepath.Join(basePath, "data", "notes", "records.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "Virtual Test", records[0]["title"])
}
