package schema

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func mustParse(t *testing.T, src string) *Schema {
	t.Helper()
	s, err := ParseJSONSchema([]byte(src))
	require.NoError(t, err)
	return s
}

func TestDiff_AdditiveOptionalField(t *testing.T) {
	old := mustParse(t, `
x-entity: notes
x-storage: { docs_dir: d, filename: "{id}.md" }
x-id: id
type: object
required: [id]
properties:
  id: { type: string }
`)
	newer := mustParse(t, `
x-entity: notes
x-storage: { docs_dir: d, filename: "{id}.md" }
x-id: id
type: object
required: [id]
properties:
  id:    { type: string }
  notes: { type: string }
`)
	d := Diff(old, newer)
	require.False(t, d.HasBreaking(), d.String())
}

func TestDiff_BreakingNewRequired(t *testing.T) {
	old := mustParse(t, `
x-entity: notes
x-storage: { docs_dir: d, filename: "{id}.md" }
x-id: id
type: object
required: [id]
properties:
  id: { type: string }
`)
	newer := mustParse(t, `
x-entity: notes
x-storage: { docs_dir: d, filename: "{id}.md" }
x-id: id
type: object
required: [id, created]
properties:
  id:      { type: string }
  created: { type: string, format: date }
`)
	d := Diff(old, newer)
	require.True(t, d.HasBreaking())
}

func TestDiff_BreakingTypeChange(t *testing.T) {
	old := mustParse(t, `
x-entity: notes
x-storage: { docs_dir: d, filename: "{id}.md" }
x-id: id
type: object
required: [id]
properties:
  id: { type: string }
`)
	newer := mustParse(t, `
x-entity: notes
x-storage: { docs_dir: d, filename: "{id}.md" }
x-id: id
type: object
required: [id]
properties:
  id: { type: integer }
`)
	d := Diff(old, newer)
	require.True(t, d.HasBreaking())
}

func TestDiff_RemovedFieldIsBreaking(t *testing.T) {
	old := mustParse(t, `
x-entity: notes
x-storage: { docs_dir: d, filename: "{id}.md" }
x-id: id
type: object
required: [id]
properties:
  id:   { type: string }
  note: { type: string }
`)
	newer := mustParse(t, `
x-entity: notes
x-storage: { docs_dir: d, filename: "{id}.md" }
x-id: id
type: object
required: [id]
properties:
  id: { type: string }
`)
	d := Diff(old, newer)
	require.True(t, d.HasBreaking())
}

func TestDiff_TightenedEnumIsBreaking(t *testing.T) {
	old := mustParse(t, `
x-entity: notes
x-storage: { docs_dir: d, filename: "{id}.md" }
x-id: id
type: object
required: [id]
properties:
  id:     { type: string }
  status: { enum: [active, archived, draft] }
`)
	newer := mustParse(t, `
x-entity: notes
x-storage: { docs_dir: d, filename: "{id}.md" }
x-id: id
type: object
required: [id]
properties:
  id:     { type: string }
  status: { enum: [active, archived] }
`)
	d := Diff(old, newer)
	require.True(t, d.HasBreaking())
}
