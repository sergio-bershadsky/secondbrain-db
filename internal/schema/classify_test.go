package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifyFields(t *testing.T) {
	s := testSchema()

	scalar, complex_ := ClassifyFields(s)

	// Scalar: id, created, status
	assert.Contains(t, scalar, "id")
	assert.Contains(t, scalar, "created")
	assert.Contains(t, scalar, "status")

	// Complex: tags, sources
	assert.Contains(t, complex_, "tags")
	assert.Contains(t, complex_, "sources")
}

func TestBuildRecordData(t *testing.T) {
	s := testSchema()
	data := map[string]any{
		"id":      "test",
		"created": "2026-04-08",
		"status":  "active",
		"tags":    []any{"go", "test"}, // complex — should NOT be in record
	}
	virtuals := map[string]any{
		"title": "Test Title", // scalar virtual — should be in record
	}

	record := BuildRecordData(s, data, virtuals)

	assert.Equal(t, "test", record["id"])
	assert.Equal(t, "active", record["status"])
	assert.Equal(t, "Test Title", record["title"])
	_, hasTags := record["tags"]
	assert.False(t, hasTags, "complex fields should not be in record")
}

func TestBuildFrontmatterData(t *testing.T) {
	s := testSchema()
	data := map[string]any{
		"id":      "test",
		"created": "2026-04-08",
		"status":  "active",
		"tags":    []any{"go"},
	}
	virtuals := map[string]any{
		"title": "FM Title",
	}

	fm := BuildFrontmatterData(s, data, virtuals)

	// Frontmatter has everything
	assert.Equal(t, "test", fm["id"])
	assert.Equal(t, "active", fm["status"])
	assert.Equal(t, []any{"go"}, fm["tags"])
	assert.Equal(t, "FM Title", fm["title"])
}

func TestBuildRecordData_WithRefField(t *testing.T) {
	s, err := Parse([]byte(`
version: 1
entity: test
docs_dir: docs/test
filename: "{id}.md"
fields:
  id:     { type: string, required: true }
  parent: { type: ref, entity: test }
`))
	require.NoError(t, err)

	data := map[string]any{"id": "child", "parent": "parent-doc"}
	record := BuildRecordData(s, data, nil)

	// ref is scalar → should be in record
	assert.Equal(t, "parent-doc", record["parent"])
}
