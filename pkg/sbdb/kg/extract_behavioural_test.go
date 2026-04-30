package kg

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/schema"
)

// TestExtractRefFields_SingleRef: schema with a "ref" field; record with a
// matching id; one edge produced.
func TestExtractRefFields_SingleRef(t *testing.T) {
	s := &schema.Schema{
		Entity: "tasks",
		Fields: map[string]*schema.Field{
			"blocks": {Type: schema.FieldTypeRef, RefEntity: "tasks"},
		},
	}
	edges := ExtractRefFields(s, map[string]any{"blocks": "task-42"})
	assert.Len(t, edges, 1)
	assert.Equal(t, "task-42", edges[0].TargetID)
	assert.Equal(t, "tasks", edges[0].TargetEntity)
}

func TestExtractRefFields_NoRefFieldsYieldsEmpty(t *testing.T) {
	s := &schema.Schema{
		Entity: "notes",
		Fields: map[string]*schema.Field{
			"id":    {Type: schema.FieldTypeString},
			"title": {Type: schema.FieldTypeString},
		},
	}
	edges := ExtractRefFields(s, map[string]any{"id": "x", "title": "y"})
	assert.Empty(t, edges)
}

func TestExtractRefFields_NilValueSkipped(t *testing.T) {
	s := &schema.Schema{
		Entity: "tasks",
		Fields: map[string]*schema.Field{
			"blocks": {Type: schema.FieldTypeRef, RefEntity: "tasks"},
		},
	}
	edges := ExtractRefFields(s, map[string]any{"blocks": nil})
	assert.Empty(t, edges)
}

// TestExtractRefFields_RefEntityDefault: when RefEntity is empty, the edge
// target entity defaults to the schema's own entity.
func TestExtractRefFields_RefEntityDefault(t *testing.T) {
	s := &schema.Schema{
		Entity: "notes",
		Fields: map[string]*schema.Field{
			"related": {Type: schema.FieldTypeRef},
		},
	}
	edges := ExtractRefFields(s, map[string]any{"related": "abc"})
	assert.Len(t, edges, 1)
	assert.Equal(t, "notes", edges[0].TargetEntity)
}

// TestExtractRefFields_ListRefAnySlice: a list field whose items are refs
// produces one edge per element ([]any slice path).
func TestExtractRefFields_ListRefAnySlice(t *testing.T) {
	s := &schema.Schema{
		Entity: "tasks",
		Fields: map[string]*schema.Field{
			"deps": {
				Type:  schema.FieldTypeList,
				Items: &schema.Field{Type: schema.FieldTypeRef, RefEntity: "tasks"},
			},
		},
	}
	edges := ExtractRefFields(s, map[string]any{"deps": []any{"t1", "t2"}})
	assert.Len(t, edges, 2)
	assert.Equal(t, "t1", edges[0].TargetID)
	assert.Equal(t, "t2", edges[1].TargetID)
	assert.Equal(t, "tasks", edges[0].TargetEntity)
}

// TestExtractRefFields_ListRefStringSlice: list field with []string items.
func TestExtractRefFields_ListRefStringSlice(t *testing.T) {
	s := &schema.Schema{
		Entity: "tasks",
		Fields: map[string]*schema.Field{
			"deps": {
				Type:  schema.FieldTypeList,
				Items: &schema.Field{Type: schema.FieldTypeRef, RefEntity: "tasks"},
			},
		},
	}
	edges := ExtractRefFields(s, map[string]any{"deps": []string{"x", "y", "z"}})
	assert.Len(t, edges, 3)
	assert.Equal(t, "x", edges[0].TargetID)
}

// TestExtractVirtualEdges_StringValue: a virtual field with edge:true and a
// string value produces one edge.
func TestExtractVirtualEdges_StringValue(t *testing.T) {
	s := &schema.Schema{
		Entity: "notes",
		Virtuals: map[string]*schema.Virtual{
			"parent": {Edge: true, EdgeEntity: "notes"},
		},
	}
	edges := ExtractVirtualEdges(s, map[string]any{"parent": "note-99"})
	assert.Len(t, edges, 1)
	assert.Equal(t, "note-99", edges[0].TargetID)
	assert.Equal(t, "virtual", edges[0].EdgeType)
}

// TestExtractVirtualEdges_NoEdgeFlagSkipped: virtual fields without edge:true
// are ignored.
func TestExtractVirtualEdges_NoEdgeFlagSkipped(t *testing.T) {
	s := &schema.Schema{
		Entity: "notes",
		Virtuals: map[string]*schema.Virtual{
			"computed": {Edge: false},
		},
	}
	edges := ExtractVirtualEdges(s, map[string]any{"computed": "value"})
	assert.Empty(t, edges)
}

// TestExtractVirtualEdges_AnySlice: virtual field with edge:true and []any value.
func TestExtractVirtualEdges_AnySlice(t *testing.T) {
	s := &schema.Schema{
		Entity: "notes",
		Virtuals: map[string]*schema.Virtual{
			"links": {Edge: true, EdgeEntity: "notes"},
		},
	}
	edges := ExtractVirtualEdges(s, map[string]any{"links": []any{"a", "b"}})
	assert.Len(t, edges, 2)
}

// TestExtractVirtualEdges_StringSlice: virtual field with edge:true and []string value.
func TestExtractVirtualEdges_StringSlice(t *testing.T) {
	s := &schema.Schema{
		Entity: "notes",
		Virtuals: map[string]*schema.Virtual{
			"refs": {Edge: true, EdgeEntity: "notes"},
		},
	}
	edges := ExtractVirtualEdges(s, map[string]any{"refs": []string{"x", "y"}})
	assert.Len(t, edges, 2)
	assert.Equal(t, "x", edges[0].TargetID)
}

// TestExtractVirtualEdges_EntityDefaultsToOwn: EdgeEntity empty falls back to schema entity.
func TestExtractVirtualEdges_EntityDefaultsToOwn(t *testing.T) {
	s := &schema.Schema{
		Entity: "tasks",
		Virtuals: map[string]*schema.Virtual{
			"dep": {Edge: true},
		},
	}
	edges := ExtractVirtualEdges(s, map[string]any{"dep": "task-7"})
	assert.Len(t, edges, 1)
	assert.Equal(t, "tasks", edges[0].TargetEntity)
}
