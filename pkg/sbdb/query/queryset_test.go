package query

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/document"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/schema"
)

const testSchemaYAML = `
version: 1
entity: notes
docs_dir: docs/notes
filename: "{id}.md"
records_dir: data/notes
partition: none
id_field: id
integrity: off

fields:
  id:      { type: string, required: true }
  created: { type: date, required: true }
  status:  { type: enum, values: [active, archived], default: active }
  tags:    { type: list, items: { type: string } }
`

func setupQueryTest(t *testing.T) (*schema.Schema, string) {
	t.Helper()

	s, err := schema.Parse([]byte(testSchemaYAML))
	require.NoError(t, err)

	basePath := t.TempDir()
	docsDir := filepath.Join(basePath, "docs", "notes")
	require.NoError(t, os.MkdirAll(docsDir, 0o755))

	// Write markdown files with frontmatter so the walker can read them.
	docFixtures := []struct {
		id      string
		created string
		status  string
	}{
		{"a", "2026-01-01", "active"},
		{"b", "2026-02-01", "archived"},
		{"c", "2026-03-01", "active"},
		{"d", "2026-04-01", "active"},
	}
	for _, f := range docFixtures {
		doc := document.New(s, basePath)
		doc.Data = map[string]any{
			"id":      f.id,
			"created": f.created,
			"status":  f.status,
		}
		doc.Content = "# " + f.id + "\n"
		require.NoError(t, doc.Save(nil))
	}

	return s, basePath
}

func TestQuerySet_All(t *testing.T) {
	s, basePath := setupQueryTest(t)
	qs := NewQuerySet(s, basePath)

	docs, err := qs.All()
	require.NoError(t, err)
	assert.Len(t, docs, 4)
}

func TestQuerySet_FilterExact(t *testing.T) {
	s, basePath := setupQueryTest(t)
	qs := NewQuerySet(s, basePath)

	docs, err := qs.Filter(map[string]any{"status": "active"}).All()
	require.NoError(t, err)
	assert.Len(t, docs, 3)
}

func TestQuerySet_FilterGTE(t *testing.T) {
	s, basePath := setupQueryTest(t)
	qs := NewQuerySet(s, basePath)

	docs, err := qs.Filter(map[string]any{"created__gte": "2026-03-01"}).All()
	require.NoError(t, err)
	assert.Len(t, docs, 2) // c and d
}

func TestQuerySet_Exclude(t *testing.T) {
	s, basePath := setupQueryTest(t)
	qs := NewQuerySet(s, basePath)

	docs, err := qs.Exclude(map[string]any{"status": "archived"}).All()
	require.NoError(t, err)
	assert.Len(t, docs, 3)
}

func TestQuerySet_OrderBy(t *testing.T) {
	s, basePath := setupQueryTest(t)
	qs := NewQuerySet(s, basePath)

	docs, err := qs.OrderBy("-created").All()
	require.NoError(t, err)
	assert.Equal(t, "d", docs[0].ID())
	assert.Equal(t, "a", docs[3].ID())
}

func TestQuerySet_LimitOffset(t *testing.T) {
	s, basePath := setupQueryTest(t)
	qs := NewQuerySet(s, basePath)

	docs, err := qs.Limit(2).All()
	require.NoError(t, err)
	assert.Len(t, docs, 2)

	docs, err = qs.Offset(2).Limit(1).All()
	require.NoError(t, err)
	assert.Len(t, docs, 1)
}

func TestQuerySet_Get_Found(t *testing.T) {
	s, basePath := setupQueryTest(t)
	qs := NewQuerySet(s, basePath)

	doc, err := qs.Get(map[string]any{"id": "b"})
	require.NoError(t, err)
	assert.Equal(t, "b", doc.ID())
}

func TestQuerySet_Get_NotFound(t *testing.T) {
	s, basePath := setupQueryTest(t)
	qs := NewQuerySet(s, basePath)

	_, err := qs.Get(map[string]any{"id": "nonexistent"})
	require.Error(t, err)
	_, ok := err.(*document.NotFoundError)
	assert.True(t, ok)
}

func TestQuerySet_Count(t *testing.T) {
	s, basePath := setupQueryTest(t)
	qs := NewQuerySet(s, basePath)

	count, err := qs.Filter(map[string]any{"status": "active"}).Count()
	require.NoError(t, err)
	assert.Equal(t, 3, count)
}

func TestQuerySet_Exists(t *testing.T) {
	s, basePath := setupQueryTest(t)
	qs := NewQuerySet(s, basePath)

	exists, err := qs.Filter(map[string]any{"status": "archived"}).Exists()
	require.NoError(t, err)
	assert.True(t, exists)

	exists, err = qs.Filter(map[string]any{"status": "deleted"}).Exists()
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestQuerySet_Chaining(t *testing.T) {
	s, basePath := setupQueryTest(t)
	qs := NewQuerySet(s, basePath)

	docs, err := qs.
		Filter(map[string]any{"status": "active"}).
		OrderBy("-created").
		Limit(2).
		All()
	require.NoError(t, err)
	assert.Len(t, docs, 2)
	assert.Equal(t, "d", docs[0].ID())
	assert.Equal(t, "c", docs[1].ID())
}

func TestQuerySet_Immutable(t *testing.T) {
	s, basePath := setupQueryTest(t)
	qs := NewQuerySet(s, basePath)

	filtered := qs.Filter(map[string]any{"status": "active"})
	// Original should still return all
	allDocs, _ := qs.All()
	filteredDocs, _ := filtered.All()

	assert.Len(t, allDocs, 4)
	assert.Len(t, filteredDocs, 3)
}
