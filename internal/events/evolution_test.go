package events

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEvolution_AddOptional_Allowed(t *testing.T) {
	old := &TypeSchema{Type: "x.recipe.cooked", Fields: []*FieldSchema{
		{Name: "date", Type: "date", Required: true},
	}}
	new := &TypeSchema{Type: "x.recipe.cooked", Fields: []*FieldSchema{
		{Name: "date", Type: "date", Required: true},
		{Name: "rating", Type: "int", Required: false},
	}}
	r := DiffSchema(old, new)
	require.True(t, r.Allowed)
	require.Equal(t, []string{"rating"}, r.AddedOptional)
}

func TestEvolution_AddRequired_Forbidden(t *testing.T) {
	old := &TypeSchema{Type: "x.r.c", Fields: []*FieldSchema{
		{Name: "date", Type: "date", Required: true},
	}}
	new := &TypeSchema{Type: "x.r.c", Fields: []*FieldSchema{
		{Name: "date", Type: "date", Required: true},
		{Name: "rating", Type: "int", Required: true},
	}}
	r := DiffSchema(old, new)
	require.False(t, r.Allowed)
	require.Contains(t, r.ForbiddenReason, "added as required")
}

func TestEvolution_RemoveField_Forbidden(t *testing.T) {
	old := &TypeSchema{Type: "x.r.c", Fields: []*FieldSchema{
		{Name: "date", Type: "date", Required: true},
		{Name: "rating", Type: "int"},
	}}
	new := &TypeSchema{Type: "x.r.c", Fields: []*FieldSchema{
		{Name: "date", Type: "date", Required: true},
	}}
	r := DiffSchema(old, new)
	require.False(t, r.Allowed)
	require.Contains(t, r.ForbiddenReason, "removed")
}

func TestEvolution_TypeChange_Forbidden(t *testing.T) {
	old := &TypeSchema{Type: "x.r.c", Fields: []*FieldSchema{
		{Name: "rating", Type: "int"},
	}}
	new := &TypeSchema{Type: "x.r.c", Fields: []*FieldSchema{
		{Name: "rating", Type: "string"},
	}}
	r := DiffSchema(old, new)
	require.False(t, r.Allowed)
	require.Contains(t, r.ForbiddenReason, "type changed")
}

func TestEvolution_RequiredFlip_Forbidden(t *testing.T) {
	old := &TypeSchema{Type: "x.r.c", Fields: []*FieldSchema{
		{Name: "date", Type: "date", Required: true},
	}}
	new := &TypeSchema{Type: "x.r.c", Fields: []*FieldSchema{
		{Name: "date", Type: "date", Required: false},
	}}
	r := DiffSchema(old, new)
	require.False(t, r.Allowed)
	require.Contains(t, r.ForbiddenReason, "required changed")
}

func TestEvolution_EnumAppend_Allowed(t *testing.T) {
	old := &TypeSchema{Type: "task.status_changed", Fields: []*FieldSchema{
		{Name: "status", Type: "enum", Required: true, EnumValues: []string{"open", "done"}},
	}}
	new := &TypeSchema{Type: "task.status_changed", Fields: []*FieldSchema{
		{Name: "status", Type: "enum", Required: true, EnumValues: []string{"open", "done", "blocked"}},
	}}
	r := DiffSchema(old, new)
	require.True(t, r.Allowed)
	require.Equal(t, []string{"blocked"}, r.AddedEnumValues["status"])
}

func TestEvolution_EnumRemove_Forbidden(t *testing.T) {
	old := &TypeSchema{Type: "task.status_changed", Fields: []*FieldSchema{
		{Name: "status", Type: "enum", EnumValues: []string{"open", "done", "blocked"}},
	}}
	new := &TypeSchema{Type: "task.status_changed", Fields: []*FieldSchema{
		{Name: "status", Type: "enum", EnumValues: []string{"open", "done"}},
	}}
	r := DiffSchema(old, new)
	require.False(t, r.Allowed)
	require.Contains(t, r.ForbiddenReason, "enum value")
	require.Contains(t, r.ForbiddenReason, "removed")
}

func TestEvolution_MaxLengthGrow_Allowed(t *testing.T) {
	old := &TypeSchema{Type: "x.r.c", Fields: []*FieldSchema{
		{Name: "title", Type: "string", MaxLength: 100},
	}}
	new := &TypeSchema{Type: "x.r.c", Fields: []*FieldSchema{
		{Name: "title", Type: "string", MaxLength: 200},
	}}
	r := DiffSchema(old, new)
	require.True(t, r.Allowed)
	require.Contains(t, r.WidenedFields, "title:max_length")
}

func TestEvolution_MaxLengthShrink_Forbidden(t *testing.T) {
	old := &TypeSchema{Type: "x.r.c", Fields: []*FieldSchema{
		{Name: "title", Type: "string", MaxLength: 200},
	}}
	new := &TypeSchema{Type: "x.r.c", Fields: []*FieldSchema{
		{Name: "title", Type: "string", MaxLength: 100},
	}}
	r := DiffSchema(old, new)
	require.False(t, r.Allowed)
	require.Contains(t, r.ForbiddenReason, "max_length tightened")
}

func TestEvolution_PatternRemove_Allowed(t *testing.T) {
	old := &TypeSchema{Type: "x.r.c", Fields: []*FieldSchema{
		{Name: "code", Type: "string", Pattern: "^[A-Z]{3}$"},
	}}
	new := &TypeSchema{Type: "x.r.c", Fields: []*FieldSchema{
		{Name: "code", Type: "string"},
	}}
	r := DiffSchema(old, new)
	require.True(t, r.Allowed)
	require.Contains(t, r.WidenedFields, "code:pattern")
}

func TestEvolution_PatternAdd_Forbidden(t *testing.T) {
	old := &TypeSchema{Type: "x.r.c", Fields: []*FieldSchema{
		{Name: "code", Type: "string"},
	}}
	new := &TypeSchema{Type: "x.r.c", Fields: []*FieldSchema{
		{Name: "code", Type: "string", Pattern: "^[A-Z]{3}$"},
	}}
	r := DiffSchema(old, new)
	require.False(t, r.Allowed)
	require.Contains(t, r.ForbiddenReason, "pattern added")
}
