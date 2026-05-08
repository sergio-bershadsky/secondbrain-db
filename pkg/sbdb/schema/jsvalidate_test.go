package schema

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompileAndValidateRecord_OK(t *testing.T) {
	src := []byte(`
x-entity: notes
x-storage: { docs_dir: docs/notes, filename: "{id}.md" }
x-id: id
type: object
required: [id]
properties:
  id:     { type: string }
  status: { enum: [active, archived] }
`)
	s, err := ParseJSONSchema(src)
	require.NoError(t, err)
	v, err := NewValidator(s)
	require.NoError(t, err)
	require.NoError(t, v.ValidateMap(map[string]any{"id": "x", "status": "active"}))
}

func TestCompileAndValidateRecord_RejectsBadEnum(t *testing.T) {
	src := []byte(`
x-entity: notes
x-storage: { docs_dir: docs/notes, filename: "{id}.md" }
x-id: id
type: object
required: [id]
properties:
  id:     { type: string }
  status: { enum: [active, archived] }
`)
	s, _ := ParseJSONSchema(src)
	v, _ := NewValidator(s)
	err := v.ValidateMap(map[string]any{"id": "x", "status": "bogus"})
	require.Error(t, err)
}

func TestCompileAndValidateRecord_RejectsMissingRequired(t *testing.T) {
	src := []byte(`
x-entity: notes
x-storage: { docs_dir: docs/notes, filename: "{id}.md" }
x-id: id
type: object
required: [id]
properties:
  id: { type: string }
`)
	s, _ := ParseJSONSchema(src)
	v, _ := NewValidator(s)
	err := v.ValidateMap(map[string]any{})
	require.Error(t, err)
}
