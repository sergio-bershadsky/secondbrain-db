package document

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/integrity"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/schema"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/storage"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/virtuals"
)

func setupLoadTest(t *testing.T) (*schema.Schema, *virtuals.Runtime, string) {
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

func TestDocument_GetAndSet(t *testing.T) {
	s, _, basePath := setupLoadTest(t)
	doc := New(s, basePath)

	doc.Set("id", "test-1")
	doc.Set("status", "active")

	val, ok := doc.Get("id")
	assert.True(t, ok)
	assert.Equal(t, "test-1", val)

	val, ok = doc.Get("status")
	assert.True(t, ok)
	assert.Equal(t, "active", val)

	// Missing field
	_, ok = doc.Get("nonexistent")
	assert.False(t, ok)
}

func TestDocument_GetFromVirtuals(t *testing.T) {
	s, _, basePath := setupLoadTest(t)
	doc := New(s, basePath)
	doc.SetVirtuals(map[string]any{"title": "Virtual Title"})

	// Get checks data first, then virtuals
	val, ok := doc.Get("title")
	assert.True(t, ok)
	assert.Equal(t, "Virtual Title", val)
}

func TestDocument_AllData(t *testing.T) {
	s, _, basePath := setupLoadTest(t)
	doc := New(s, basePath)
	doc.Set("id", "test-1")
	doc.Set("status", "active")
	doc.SetVirtuals(map[string]any{"title": "Title", "word_count": 42})

	all := doc.AllData()
	assert.Equal(t, "test-1", all["id"])
	assert.Equal(t, "active", all["status"])
	assert.Equal(t, "Title", all["title"])
	assert.Equal(t, 42, all["word_count"])
}

func TestLoadFromRecord_Lazy(t *testing.T) {
	s, rt, basePath := setupLoadTest(t)

	// Save a doc first
	doc := New(s, basePath)
	doc.Data = map[string]any{"id": "lazy-test", "created": "2026-04-08", "status": "active"}
	doc.Content = "# Lazy Test\n\nBody content here.\n"
	require.NoError(t, doc.Save(rt))

	// Load from record (lazy — no file I/O)
	record := map[string]any{
		"id":     "lazy-test",
		"status": "active",
		"file":   "docs/notes/lazy-test.md",
	}
	lazy := LoadFromRecord(s, basePath, record)

	assert.Equal(t, "lazy-test", lazy.ID())
	assert.Empty(t, lazy.Content, "content should not be loaded yet")

	// EnsureLoaded triggers file read
	require.NoError(t, lazy.EnsureLoaded())
	assert.Contains(t, lazy.Content, "Lazy Test")
	assert.Contains(t, lazy.Content, "Body content")
}

func TestEnsureLoaded_Idempotent(t *testing.T) {
	s, rt, basePath := setupLoadTest(t)

	doc := New(s, basePath)
	doc.Data = map[string]any{"id": "idem-load", "created": "2026-04-08", "status": "active"}
	doc.Content = "# Idempotent\n"
	require.NoError(t, doc.Save(rt))

	lazy := LoadFromRecord(s, basePath, map[string]any{"id": "idem-load", "file": "docs/notes/idem-load.md"})
	require.NoError(t, lazy.EnsureLoaded())
	content1 := lazy.Content

	require.NoError(t, lazy.EnsureLoaded()) // second call
	assert.Equal(t, content1, lazy.Content, "second EnsureLoaded should be a no-op")
}

func TestVerifyIntegrity_Clean(t *testing.T) {
	s, rt, basePath := setupLoadTest(t)

	doc := New(s, basePath)
	doc.Data = map[string]any{"id": "integ-clean", "created": "2026-04-08", "status": "active"}
	doc.Content = "# Clean Doc\n"
	require.NoError(t, doc.Save(rt))

	// Reload and verify
	loaded, err := LoadFromFile(s, basePath, doc.FilePath())
	require.NoError(t, err)

	// Need to re-eval virtuals for correct hash comparison
	vResults, _ := rt.EvaluateAll(loaded.Content, loaded.Data)
	loaded.SetVirtuals(vResults)

	err = loaded.VerifyIntegrity()
	assert.NoError(t, err, "clean doc should pass integrity check")
}

func TestVerifyIntegrity_Tampered(t *testing.T) {
	s, rt, basePath := setupLoadTest(t)

	doc := New(s, basePath)
	doc.Data = map[string]any{"id": "integ-tamper", "created": "2026-04-08", "status": "active"}
	doc.Content = "# Will Be Tampered\n"
	require.NoError(t, doc.Save(rt))

	// Tamper the file
	mdPath := doc.FilePath()
	data, _ := os.ReadFile(mdPath)
	os.WriteFile(mdPath, append(data, []byte("\nTAMPERED CONTENT\n")...), 0o644)

	// Reload and verify
	loaded, err := LoadFromFile(s, basePath, mdPath)
	require.NoError(t, err)
	vResults, _ := rt.EvaluateAll(loaded.Content, loaded.Data)
	loaded.SetVirtuals(vResults)

	err = loaded.VerifyIntegrity()
	assert.Error(t, err)
	intErr, ok := err.(*IntegrityError)
	assert.True(t, ok)
	assert.Contains(t, intErr.Mismatched, "content")
}

func TestVerifyIntegrity_Off(t *testing.T) {
	schemaYAML := `
version: 1
entity: notes
docs_dir: docs/notes
filename: "{id}.md"
records_dir: data/notes
partition: none
id_field: id
integrity: "off"
fields:
  id: { type: string, required: true }
`
	s, err := schema.Parse([]byte(schemaYAML))
	require.NoError(t, err)

	basePath := t.TempDir()
	os.MkdirAll(filepath.Join(basePath, "docs", "notes"), 0o755)
	os.MkdirAll(filepath.Join(basePath, "data", "notes"), 0o755)

	doc := New(s, basePath)
	doc.Data = map[string]any{"id": "off-test"}
	doc.Content = "# Off\n"
	require.NoError(t, doc.Save(nil))

	// Tamper
	os.WriteFile(doc.FilePath(), []byte("TAMPERED"), 0o644)

	loaded, _ := LoadFromFile(s, basePath, doc.FilePath())
	err = loaded.VerifyIntegrity()
	assert.NoError(t, err, "integrity=off should skip check")
}

func TestVerifyIntegrity_Warn(t *testing.T) {
	schemaYAML := `
version: 1
entity: notes
docs_dir: docs/notes
filename: "{id}.md"
records_dir: data/notes
partition: none
id_field: id
integrity: warn
fields:
  id: { type: string, required: true }
`
	s, err := schema.Parse([]byte(schemaYAML))
	require.NoError(t, err)

	basePath := t.TempDir()
	os.MkdirAll(filepath.Join(basePath, "docs", "notes"), 0o755)
	os.MkdirAll(filepath.Join(basePath, "data", "notes"), 0o755)

	doc := New(s, basePath)
	doc.Data = map[string]any{"id": "warn-test"}
	doc.Content = "# Warn\n"
	require.NoError(t, doc.Save(nil))

	// Tamper
	os.WriteFile(doc.FilePath(), []byte("TAMPERED"), 0o644)

	loaded, _ := LoadFromFile(s, basePath, doc.FilePath())
	err = loaded.VerifyIntegrity()
	assert.NoError(t, err, "integrity=warn should return nil (warning only)")
}

func TestDocument_Errors(t *testing.T) {
	nfe := &NotFoundError{ID: "x", Entity: "notes"}
	assert.Contains(t, nfe.Error(), "x")
	assert.Contains(t, nfe.Error(), "notes")

	mfe := &MultipleFoundError{Entity: "notes", Count: 3}
	assert.Contains(t, mfe.Error(), "3")

	ie := &IntegrityError{ID: "x", File: "f.md", Mismatched: []string{"content"}}
	assert.Contains(t, ie.Error(), "content")

	de := &DriftError{ID: "x", Field: "status", FM: "a", Rec: "b"}
	assert.Contains(t, de.Error(), "status")
}

func TestDocument_PathTraversalProtection(t *testing.T) {
	s, _, basePath := setupLoadTest(t)

	doc := New(s, basePath)
	doc.Data = map[string]any{"id": "../../etc/passwd"}

	// resolveFilename should sanitize to safe fallback
	filename := doc.resolveFilename()
	assert.Equal(t, "untitled.md", filename)
}

func TestDocument_PostSaveHook(t *testing.T) {
	s, _, basePath := setupLoadTest(t)

	hookCalled := false
	doc := New(s, basePath)
	doc.Data = map[string]any{"id": "hook-test", "created": "2026-04-08", "status": "active"}
	doc.Content = "# Hook Test\n"
	doc.OnSave = func(d *Document) error {
		hookCalled = true
		return nil
	}

	require.NoError(t, doc.Save(nil))
	assert.True(t, hookCalled, "OnSave hook should have been called")
}

func TestDocument_PostDeleteHook(t *testing.T) {
	s, _, basePath := setupLoadTest(t)

	deletedID := ""
	doc := New(s, basePath)
	doc.Data = map[string]any{"id": "del-hook", "created": "2026-04-08", "status": "active"}
	doc.Content = "# Delete Hook\n"
	doc.OnDelete = func(id string) error {
		deletedID = id
		return nil
	}

	require.NoError(t, doc.Save(nil))
	require.NoError(t, doc.Delete())
	assert.Equal(t, "del-hook", deletedID)
}

// Integration: save via ORM, verify sidecar consistency
func TestDocument_RecordConsistency(t *testing.T) {
	s, rt, basePath := setupLoadTest(t)

	doc := New(s, basePath)
	doc.Data = map[string]any{
		"id":      "consistency",
		"created": "2026-04-08",
		"status":  "active",
		"tags":    []any{"go", "test"},
	}
	doc.Content = "# Consistency Check\n\nBody.\n"
	require.NoError(t, doc.Save(rt))

	// Sidecar should exist
	mdPath := filepath.Join(basePath, "docs", "notes", "consistency.md")
	sc, err := integrity.LoadSidecar(mdPath)
	require.NoError(t, err)
	assert.Equal(t, "consistency.md", sc.File)
	assert.NotEmpty(t, sc.ContentSHA)
	assert.NotEmpty(t, sc.FrontmatterSHA)
	assert.NotEmpty(t, sc.RecordSHA)

	// Frontmatter should have the title virtual and id
	fm, _, ferr := storage.ParseMarkdown(mdPath)
	require.NoError(t, ferr)
	assert.Equal(t, "consistency", fm["id"])
	assert.Equal(t, "active", fm["status"])
	assert.Equal(t, "Consistency Check", fm["title"]) // virtual scalar

	// records.yaml should NOT be written
	assert.NoFileExists(t, filepath.Join(basePath, "data", "notes", "records.yaml"))
}
