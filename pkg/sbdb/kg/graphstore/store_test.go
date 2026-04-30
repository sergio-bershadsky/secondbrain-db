package graphstore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenMemory()
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStore_VertexCRUD(t *testing.T) {
	s := newTestStore(t)

	// Put
	require.NoError(t, s.PutVertex(&VertexData{ID: "a", Entity: "notes", Title: "Note A"}))
	require.NoError(t, s.PutVertex(&VertexData{ID: "b", Entity: "adrs", Title: "ADR B"}))

	// Get
	v, err := s.GetVertex("a")
	require.NoError(t, err)
	assert.Equal(t, "Note A", v.Title)
	assert.Equal(t, "notes", v.Entity)

	// Not found
	_, err = s.GetVertex("nonexistent")
	assert.Error(t, err)

	// Count
	count, _ := s.VertexCount()
	assert.Equal(t, 2, count)

	// Update
	require.NoError(t, s.PutVertex(&VertexData{ID: "a", Entity: "notes", Title: "Updated A"}))
	v, _ = s.GetVertex("a")
	assert.Equal(t, "Updated A", v.Title)

	// All (no filter)
	all, _ := s.AllVertices("")
	assert.Len(t, all, 2)

	// All (entity filter)
	notes, _ := s.AllVertices("notes")
	assert.Len(t, notes, 1)

	// Remove
	require.NoError(t, s.RemoveVertex("a"))
	count, _ = s.VertexCount()
	assert.Equal(t, 1, count)
}

func TestStore_EdgeCRUD(t *testing.T) {
	s := newTestStore(t)

	s.PutVertex(&VertexData{ID: "a", Entity: "notes"})
	s.PutVertex(&VertexData{ID: "b", Entity: "notes"})
	s.PutVertex(&VertexData{ID: "c", Entity: "notes"})

	// Add edges
	require.NoError(t, s.PutEdge(&EdgeData{SourceID: "a", SourceEntity: "notes", TargetID: "b", TargetEntity: "notes", Type: "link", Context: "see also"}))
	require.NoError(t, s.PutEdge(&EdgeData{SourceID: "a", SourceEntity: "notes", TargetID: "c", TargetEntity: "notes", Type: "ref", Context: "parent"}))
	require.NoError(t, s.PutEdge(&EdgeData{SourceID: "b", SourceEntity: "notes", TargetID: "c", TargetEntity: "notes", Type: "link", Context: "related"}))

	// Count
	count, _ := s.EdgeCount()
	assert.Equal(t, 3, count)

	// Outgoing
	out, _ := s.Outgoing("a")
	assert.Len(t, out, 2)

	// Incoming
	inc, _ := s.Incoming("c")
	assert.Len(t, inc, 2) // from a and b

	// All edges
	all, _ := s.AllEdges("")
	assert.Len(t, all, 3)

	// Filter by type
	links, _ := s.AllEdges("link")
	assert.Len(t, links, 2)

	// Remove edges from a
	require.NoError(t, s.RemoveEdgesFrom("a"))
	out, _ = s.Outgoing("a")
	assert.Len(t, out, 0)

	// b→c still exists
	out, _ = s.Outgoing("b")
	assert.Len(t, out, 1)

	// Incoming to c should only have b now
	inc, _ = s.Incoming("c")
	assert.Len(t, inc, 1)
}

func TestStore_RemoveVertex_CleansEdges(t *testing.T) {
	s := newTestStore(t)

	s.PutVertex(&VertexData{ID: "a"})
	s.PutVertex(&VertexData{ID: "b"})
	s.PutVertex(&VertexData{ID: "c"})
	s.PutEdge(&EdgeData{SourceID: "a", TargetID: "b", Type: "link"})
	s.PutEdge(&EdgeData{SourceID: "c", TargetID: "a", Type: "ref"})

	// Remove a — should clean both outgoing (a→b) and incoming (c→a)
	require.NoError(t, s.RemoveVertex("a"))

	count, _ := s.EdgeCount()
	assert.Equal(t, 0, count)
}

func TestStore_Neighbors_BFS(t *testing.T) {
	s := newTestStore(t)

	// Chain: a → b → c → d
	for _, id := range []string{"a", "b", "c", "d"} {
		s.PutVertex(&VertexData{ID: id, Entity: "notes"})
	}
	s.PutEdge(&EdgeData{SourceID: "a", TargetID: "b", Type: "link"})
	s.PutEdge(&EdgeData{SourceID: "b", TargetID: "c", Type: "link"})
	s.PutEdge(&EdgeData{SourceID: "c", TargetID: "d", Type: "link"})

	// Depth 1: a→b only
	edges, err := s.Neighbors("a", 1)
	require.NoError(t, err)
	assert.Len(t, edges, 1)

	// Depth 2: a→b, b→c
	edges, _ = s.Neighbors("a", 2)
	assert.Len(t, edges, 2)

	// Depth 3: all 3 edges
	edges, _ = s.Neighbors("a", 3)
	assert.Len(t, edges, 3)
}

func TestStore_Neighbors_NoDuplicates(t *testing.T) {
	s := newTestStore(t)

	// Triangle: a→b, b→c, c→a
	for _, id := range []string{"a", "b", "c"} {
		s.PutVertex(&VertexData{ID: id})
	}
	s.PutEdge(&EdgeData{SourceID: "a", TargetID: "b", Type: "link"})
	s.PutEdge(&EdgeData{SourceID: "b", TargetID: "c", Type: "link"})
	s.PutEdge(&EdgeData{SourceID: "c", TargetID: "a", Type: "link"})

	edges, _ := s.Neighbors("a", 5)
	assert.Len(t, edges, 3, "triangle has exactly 3 edges regardless of depth")
}

func TestStore_ShortestPath(t *testing.T) {
	s := newTestStore(t)

	// a→b→c→d and a→d (shortcut)
	for _, id := range []string{"a", "b", "c", "d"} {
		s.PutVertex(&VertexData{ID: id})
	}
	s.PutEdge(&EdgeData{SourceID: "a", TargetID: "b", Type: "link"})
	s.PutEdge(&EdgeData{SourceID: "b", TargetID: "c", Type: "link"})
	s.PutEdge(&EdgeData{SourceID: "c", TargetID: "d", Type: "link"})
	s.PutEdge(&EdgeData{SourceID: "a", TargetID: "d", Type: "link"}) // shortcut

	path, err := s.ShortestPath("a", "d")
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "d"}, path, "should take the direct shortcut")

	// No path
	path, _ = s.ShortestPath("d", "a")
	assert.Nil(t, path, "no reverse path in directed graph")

	// Same node
	path, _ = s.ShortestPath("a", "a")
	assert.Equal(t, []string{"a"}, path)
}

func TestStore_ExportMermaid(t *testing.T) {
	s := newTestStore(t)
	s.PutEdge(&EdgeData{SourceID: "a", TargetID: "b", Type: "link", Context: "see also"})

	out, err := s.ExportMermaid()
	require.NoError(t, err)
	assert.Contains(t, out, "graph LR")
	assert.Contains(t, out, "a -->|see also| b")
}

func TestStore_ExportDOT(t *testing.T) {
	s := newTestStore(t)
	s.PutEdge(&EdgeData{SourceID: "a", TargetID: "b", Type: "link"})

	out, err := s.ExportDOT()
	require.NoError(t, err)
	assert.Contains(t, out, "digraph KnowledgeGraph")
	assert.Contains(t, out, `"a" -> "b"`)
}

func TestStore_ExportJSON(t *testing.T) {
	s := newTestStore(t)
	s.PutVertex(&VertexData{ID: "a", Entity: "notes", Title: "Note A"})
	s.PutEdge(&EdgeData{SourceID: "a", TargetID: "b", TargetEntity: "adrs", Type: "ref"})

	g, err := s.ExportGraphJSON()
	require.NoError(t, err)

	// Should have 2 nodes (a from vertex, b as placeholder from edge)
	assert.Len(t, g.Nodes, 2)
	assert.Len(t, g.Edges, 1)
	assert.Equal(t, "a", g.Edges[0].Source)
	assert.Equal(t, "b", g.Edges[0].Target)
}

func TestStore_Drop(t *testing.T) {
	s := newTestStore(t)
	s.PutVertex(&VertexData{ID: "a"})
	s.PutEdge(&EdgeData{SourceID: "a", TargetID: "b", Type: "link"})

	require.NoError(t, s.Drop())

	vc, _ := s.VertexCount()
	ec, _ := s.EdgeCount()
	assert.Equal(t, 0, vc)
	assert.Equal(t, 0, ec)
}
