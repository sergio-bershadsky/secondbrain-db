package schema

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseJSONSchema_BasicNotes(t *testing.T) {
	src := []byte(`
$schema: https://json-schema.org/draft/2020-12/schema
$id: sbdb://notes
x-entity: notes
x-storage: { docs_dir: docs/notes, filename: "{id}.md" }
x-id: id
x-integrity: strict

type: object
required: [id, created]
properties:
  id:      { type: string }
  created: { type: string, format: date }
  status:  { enum: [active, archived], default: active }
  tags:    { type: array, items: { type: string } }
`)
	s, err := ParseJSONSchema(src)
	require.NoError(t, err)
	require.Equal(t, "notes", s.Entity)
	require.Equal(t, "docs/notes", s.DocsDir)
	require.Equal(t, "{id}.md", s.Filename)
	require.Equal(t, "id", s.IDField)
	require.Equal(t, "strict", s.Integrity)

	require.Contains(t, s.Fields, "id")
	require.Equal(t, FieldTypeString, s.Fields["id"].Type)
	require.True(t, s.Fields["id"].Required)

	require.Equal(t, FieldTypeDate, s.Fields["created"].Type)
	require.Equal(t, FieldTypeEnum, s.Fields["status"].Type)
	require.ElementsMatch(t, []string{"active", "archived"}, s.Fields["status"].Values)

	require.Equal(t, FieldTypeList, s.Fields["tags"].Type)
	require.NotNil(t, s.Fields["tags"].Items)
	require.Equal(t, FieldTypeString, s.Fields["tags"].Items.Type)
}

func TestParseJSONSchema_VirtualWithCompute(t *testing.T) {
	src := []byte(`
x-entity: notes
x-storage: { docs_dir: docs/notes, filename: "{id}.md" }
x-id: id
type: object
properties:
  id: { type: string }
  title:
    type: string
    readOnly: true
    x-compute:
      source: "def compute(content, fields): return 'x'"
`)
	s, err := ParseJSONSchema(src)
	require.NoError(t, err)
	require.Contains(t, s.Virtuals, "title")
	require.Equal(t, "string", s.Virtuals["title"].Returns)
	require.Contains(t, s.Virtuals["title"].Source, "def compute")
	require.NotContains(t, s.Fields, "title", "virtual must not appear as a regular field")
}

func TestParseJSONSchema_RefBecomesRefField(t *testing.T) {
	src := []byte(`
x-entity: notes
x-storage: { docs_dir: docs/notes, filename: "{id}.md" }
x-id: id
type: object
properties:
  id: { type: string }
  parent:
    $ref: "sbdb://notes#/properties/id"
`)
	s, err := ParseJSONSchema(src)
	require.NoError(t, err)
	require.Equal(t, FieldTypeRef, s.Fields["parent"].Type)
	require.Equal(t, "notes", s.Fields["parent"].RefEntity)
}
