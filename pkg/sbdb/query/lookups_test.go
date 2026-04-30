package query

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseLookup_ExactMatch(t *testing.T) {
	l := ParseLookup("status", "active")
	assert.Equal(t, "status", l.Field)
	assert.Equal(t, "", l.Operator)
	assert.Equal(t, "active", l.Value)
}

func TestParseLookup_GTE(t *testing.T) {
	l := ParseLookup("created__gte", "2026-01-01")
	assert.Equal(t, "created", l.Field)
	assert.Equal(t, "gte", l.Operator)
}

func TestParseLookup_Contains(t *testing.T) {
	l := ParseLookup("title__contains", "deploy")
	assert.Equal(t, "title", l.Field)
	assert.Equal(t, "contains", l.Operator)
}

func TestParseLookup_IContains(t *testing.T) {
	l := ParseLookup("title__icontains", "Deploy")
	assert.Equal(t, "title", l.Field)
	assert.Equal(t, "icontains", l.Operator)
}

func TestParseLookup_In(t *testing.T) {
	l := ParseLookup("status__in", "active,archived")
	assert.Equal(t, "status", l.Field)
	assert.Equal(t, "in", l.Operator)
}

func TestParseLookup_Startswith(t *testing.T) {
	l := ParseLookup("id__startswith", "adr-")
	assert.Equal(t, "id", l.Field)
	assert.Equal(t, "startswith", l.Operator)
}

func TestLookup_Match_Exact(t *testing.T) {
	l := Lookup{Field: "status", Operator: "", Value: "active"}
	assert.True(t, l.Match(map[string]any{"status": "active"}))
	assert.False(t, l.Match(map[string]any{"status": "archived"}))
	assert.False(t, l.Match(map[string]any{"other": "active"}))
}

func TestLookup_Match_GTE(t *testing.T) {
	l := Lookup{Field: "created", Operator: "gte", Value: "2026-03-01"}
	assert.True(t, l.Match(map[string]any{"created": "2026-04-01"}))
	assert.True(t, l.Match(map[string]any{"created": "2026-03-01"}))
	assert.False(t, l.Match(map[string]any{"created": "2026-02-28"}))
}

func TestLookup_Match_LTE(t *testing.T) {
	l := Lookup{Field: "count", Operator: "lte", Value: 10}
	assert.True(t, l.Match(map[string]any{"count": 5}))
	assert.True(t, l.Match(map[string]any{"count": 10}))
	assert.False(t, l.Match(map[string]any{"count": 15}))
}

func TestLookup_Match_Contains(t *testing.T) {
	l := Lookup{Field: "title", Operator: "contains", Value: "deploy"}
	assert.True(t, l.Match(map[string]any{"title": "How to deploy apps"}))
	assert.False(t, l.Match(map[string]any{"title": "How to Deploy apps"})) // case-sensitive
}

func TestLookup_Match_IContains(t *testing.T) {
	l := Lookup{Field: "title", Operator: "icontains", Value: "deploy"}
	assert.True(t, l.Match(map[string]any{"title": "How to Deploy apps"}))
	assert.True(t, l.Match(map[string]any{"title": "DEPLOY everything"}))
}

func TestLookup_Match_Startswith(t *testing.T) {
	l := Lookup{Field: "id", Operator: "startswith", Value: "adr-"}
	assert.True(t, l.Match(map[string]any{"id": "adr-0001"}))
	assert.False(t, l.Match(map[string]any{"id": "note-1"}))
}

func TestLookup_Match_In_CSV(t *testing.T) {
	l := Lookup{Field: "status", Operator: "in", Value: "active,draft"}
	assert.True(t, l.Match(map[string]any{"status": "active"}))
	assert.True(t, l.Match(map[string]any{"status": "draft"}))
	assert.False(t, l.Match(map[string]any{"status": "archived"}))
}

func TestLookup_Match_In_Slice(t *testing.T) {
	l := Lookup{Field: "status", Operator: "in", Value: []any{"active", "draft"}}
	assert.True(t, l.Match(map[string]any{"status": "active"}))
	assert.False(t, l.Match(map[string]any{"status": "archived"}))
}

func TestLookup_Match_NumericComparison(t *testing.T) {
	l := Lookup{Field: "priority", Operator: "gt", Value: 5}
	assert.True(t, l.Match(map[string]any{"priority": 10}))
	assert.False(t, l.Match(map[string]any{"priority": 3}))
	assert.False(t, l.Match(map[string]any{"priority": 5}))
}
