package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testSchema() *Schema {
	s, _ := Parse([]byte(testSchemaYAML))
	return s
}

func TestValidateRecord_Valid(t *testing.T) {
	s := testSchema()
	data := map[string]any{
		"id":      "test-note",
		"created": "2026-04-08",
		"status":  "active",
		"tags":    []any{"go", "orm"},
		"sources": []any{
			map[string]any{"type": "github", "link": "https://github.com/test"},
		},
	}

	errs := ValidateRecord(s, data)
	assert.Empty(t, errs)
}

func TestValidateRecord_MissingRequired(t *testing.T) {
	s := testSchema()
	data := map[string]any{
		"status": "active",
	}

	errs := ValidateRecord(s, data)
	assert.True(t, len(errs) >= 2) // id and created are required

	hasID := false
	hasCreated := false
	for _, e := range errs {
		if e.Path == "id" {
			hasID = true
		}
		if e.Path == "created" {
			hasCreated = true
		}
	}
	assert.True(t, hasID, "should report missing id")
	assert.True(t, hasCreated, "should report missing created")
}

func TestValidateRecord_InvalidEnum(t *testing.T) {
	s := testSchema()
	data := map[string]any{
		"id":      "test",
		"created": "2026-04-08",
		"status":  "invalid_status",
	}

	errs := ValidateRecord(s, data)
	require.Len(t, errs, 1)
	assert.Equal(t, "status", errs[0].Path)
	assert.Contains(t, errs[0].Message, "not in enum")
}

func TestValidateRecord_InvalidDate(t *testing.T) {
	s := testSchema()
	data := map[string]any{
		"id":      "test",
		"created": "not-a-date",
	}

	errs := ValidateRecord(s, data)
	hasDateErr := false
	for _, e := range errs {
		if e.Path == "created" {
			hasDateErr = true
		}
	}
	assert.True(t, hasDateErr)
}

func TestValidateRecord_NestedObjectValidation(t *testing.T) {
	s := testSchema()
	data := map[string]any{
		"id":      "test",
		"created": "2026-04-08",
		"sources": []any{
			map[string]any{"type": "github"}, // missing required "link"
		},
	}

	errs := ValidateRecord(s, data)
	hasLinkErr := false
	for _, e := range errs {
		if e.Path == "sources[0].link" {
			hasLinkErr = true
		}
	}
	assert.True(t, hasLinkErr, "should report missing sources[0].link")
}
