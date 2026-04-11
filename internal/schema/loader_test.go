package schema

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
  sources:
    type: list
    items:
      type: object
      fields:
        type: { type: string, required: true }
        link: { type: string, required: true }

virtuals:
  title:
    returns: string
    source: |
      def compute(content, fields):
          for line in content.splitlines():
              if line.startswith("# "):
                  return line.removeprefix("# ").strip()
          return fields["id"]
`

func TestParse_ValidSchema(t *testing.T) {
	s, err := Parse([]byte(testSchemaYAML))
	require.NoError(t, err)

	assert.Equal(t, "notes", s.Entity)
	assert.Equal(t, "docs/notes", s.DocsDir)
	assert.Equal(t, "{id}.md", s.Filename)
	assert.Equal(t, "id", s.IDField)
	assert.Equal(t, "strict", s.Integrity)

	assert.Contains(t, s.Fields, "id")
	assert.Equal(t, FieldTypeString, s.Fields["id"].Type)
	assert.True(t, s.Fields["id"].Required)

	assert.Contains(t, s.Fields, "status")
	assert.Equal(t, FieldTypeEnum, s.Fields["status"].Type)
	assert.Equal(t, []string{"active", "archived"}, s.Fields["status"].Values)

	assert.Contains(t, s.Fields, "tags")
	assert.Equal(t, FieldTypeList, s.Fields["tags"].Type)
	assert.NotNil(t, s.Fields["tags"].Items)

	assert.Contains(t, s.Fields, "sources")
	assert.Equal(t, FieldTypeList, s.Fields["sources"].Type)
	assert.Equal(t, FieldTypeObject, s.Fields["sources"].Items.Type)

	assert.Contains(t, s.Virtuals, "title")
	assert.Equal(t, "string", s.Virtuals["title"].Returns)
	assert.Contains(t, s.Virtuals["title"].Source, "compute")
}

func TestParse_MissingEntity(t *testing.T) {
	_, err := Parse([]byte(`version: 1
docs_dir: docs
filename: "{id}.md"
fields:
  id: { type: string, required: true }
`))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "entity is required")
}

func TestParse_InvalidFieldType(t *testing.T) {
	_, err := Parse([]byte(`version: 1
entity: test
docs_dir: docs
filename: "{id}.md"
fields:
  id: { type: string, required: true }
  bad: { type: foobar }
`))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown type")
}

func TestLoadFromDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.yaml")
	require.NoError(t, os.WriteFile(path, []byte(testSchemaYAML), 0o644))

	s, err := LoadFromDir(dir, "notes")
	require.NoError(t, err)
	assert.Equal(t, "notes", s.Entity)
}

func TestListSchemas(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "notes.yaml"), []byte(testSchemaYAML), 0o644)
	os.WriteFile(filepath.Join(dir, "blog.yaml"), []byte(testSchemaYAML), 0o644)
	os.WriteFile(filepath.Join(dir, "readme.md"), []byte("not a schema"), 0o644)

	names, err := ListSchemas(dir)
	require.NoError(t, err)
	assert.Len(t, names, 2)
	assert.Contains(t, names, "notes")
	assert.Contains(t, names, "blog")
}

func TestScalarAndComplexFields(t *testing.T) {
	s, err := Parse([]byte(testSchemaYAML))
	require.NoError(t, err)

	scalars := s.ScalarFields()
	assert.Contains(t, scalars, "id")
	assert.Contains(t, scalars, "created")
	assert.Contains(t, scalars, "status")
	assert.Contains(t, scalars, "title") // virtual scalar

	complex_ := s.ComplexFields()
	assert.Contains(t, complex_, "tags")
	assert.Contains(t, complex_, "sources")
}
