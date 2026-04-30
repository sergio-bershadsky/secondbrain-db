package query

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/schema"
)

func setupSearchTest(t *testing.T) (*schema.Schema, string) {
	t.Helper()

	s, err := schema.Parse([]byte(testSchemaYAML))
	require.NoError(t, err)

	basePath := t.TempDir()
	docsDir := filepath.Join(basePath, s.DocsDir)
	recordsDir := filepath.Join(basePath, s.RecordsDir)

	os.MkdirAll(docsDir, 0o755)
	os.MkdirAll(recordsDir, 0o755)

	// Create markdown files (lowercase keywords for grep compatibility)
	os.WriteFile(filepath.Join(docsDir, "a.md"), []byte("# Alpha\n\ndeployment strategies for production.\n"), 0o644)
	os.WriteFile(filepath.Join(docsDir, "b.md"), []byte("# Beta\n\ncooking recipes and meal planning.\n"), 0o644)
	os.WriteFile(filepath.Join(docsDir, "c.md"), []byte("# Gamma\n\ndeployment pipeline with CI/CD.\n"), 0o644)

	// Create records
	records := []map[string]any{
		{"id": "a", "created": "2026-01-01", "status": "active", "file": "docs/notes/a.md"},
		{"id": "b", "created": "2026-02-01", "status": "active", "file": "docs/notes/b.md"},
		{"id": "c", "created": "2026-03-01", "status": "active", "file": "docs/notes/c.md"},
	}
	data, err := yaml.Marshal(records)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(recordsDir, "records.yaml"), data, 0o644))

	return s, basePath
}

func TestQuerySet_First(t *testing.T) {
	s, basePath := setupQueryTest(t)
	qs := NewQuerySet(s, basePath)

	doc, err := qs.OrderBy("id").First()
	require.NoError(t, err)
	require.NotNil(t, doc)
	assert.Equal(t, "a", doc.ID())
}

func TestQuerySet_First_Empty(t *testing.T) {
	s, basePath := setupQueryTest(t)
	qs := NewQuerySet(s, basePath)

	doc, err := qs.Filter(map[string]any{"status": "nonexistent"}).First()
	require.NoError(t, err)
	assert.Nil(t, doc, "First on empty result should return nil")
}

func TestQuerySet_Records(t *testing.T) {
	s, basePath := setupQueryTest(t)
	qs := NewQuerySet(s, basePath)

	records, err := qs.Filter(map[string]any{"status": "active"}).OrderBy("id").Records()
	require.NoError(t, err)
	assert.Len(t, records, 3)
	assert.Equal(t, "a", records[0]["id"])
}

func TestSearch_PureGo(t *testing.T) {
	s, basePath := setupSearchTest(t)

	results, err := Search(s, basePath, "deployment")
	require.NoError(t, err)
	require.True(t, len(results) >= 2, "should find 'deployment' in at least 2 files, got %d", len(results))

	for _, r := range results {
		assert.NotEmpty(t, r.File)
		assert.NotEmpty(t, r.Snippet)
	}
}

func TestSearch_NoResults(t *testing.T) {
	s, basePath := setupSearchTest(t)

	results, err := Search(s, basePath, "xyznonexistent")
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestFilenameToID(t *testing.T) {
	assert.Equal(t, "my-note", filenameToID("/path/to/my-note.md"))
	assert.Equal(t, "ADR-0001", filenameToID("ADR-0001.md"))
}

func TestExtractSnippetFromContent(t *testing.T) {
	content := "This is a long text about deployment strategies for production environments."
	snippet := extractSnippetFromContent(content, "deployment")
	assert.Contains(t, snippet, "deployment")
	assert.Contains(t, snippet, "...")
}
