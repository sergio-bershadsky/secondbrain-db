package kg

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubEmbedder returns deterministic vectors for testing (no API calls).
type stubEmbedder struct {
	dim int
}

func (s *stubEmbedder) Embed(texts []string) ([][]float32, error) {
	vectors := make([][]float32, len(texts))
	for i, text := range texts {
		vec := make([]float32, s.dim)
		// Deterministic hash: each char contributes to a dimension
		for j, ch := range text {
			vec[j%s.dim] += float32(ch) / 1000.0
		}
		// Normalize
		var norm float32
		for _, v := range vec {
			norm += v * v
		}
		if norm > 0 {
			for j := range vec {
				vec[j] /= norm
			}
		}
		vectors[i] = vec
	}
	return vectors, nil
}

func (s *stubEmbedder) ModelID() string { return "stub-model" }
func (s *stubEmbedder) Dim() int        { return s.dim }

func newTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := OpenMemory(&stubEmbedder{dim: 8})
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func newTestDBNoEmbed(t *testing.T) *DB {
	t.Helper()
	db, err := OpenMemory(nil)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

// --- DB tests ---

func TestDB_OpenAndMigrate(t *testing.T) {
	db := newTestDB(t)
	stats, err := db.Stats()
	require.NoError(t, err)
	assert.Equal(t, 0, stats.Nodes)
	assert.Equal(t, 0, stats.Edges)
	assert.Equal(t, 0, stats.Chunks)
}

func TestDB_Meta(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, db.SetMeta("key1", "value1"))
	val, err := db.GetMeta("key1")
	require.NoError(t, err)
	assert.Equal(t, "value1", val)

	// Missing key
	val, err = db.GetMeta("nonexistent")
	require.NoError(t, err)
	assert.Equal(t, "", val)

	// Overwrite
	require.NoError(t, db.SetMeta("key1", "updated"))
	val, _ = db.GetMeta("key1")
	assert.Equal(t, "updated", val)
}

func TestDB_Drop(t *testing.T) {
	db := newTestDB(t)
	db.UpsertNode("a", "notes", "Title A", "docs/a.md", "sha1")
	db.AddEdge("a", "notes", "b", "notes", "link", "context")

	require.NoError(t, db.Drop())

	stats, _ := db.Stats()
	assert.Equal(t, 0, stats.Nodes)
	assert.Equal(t, 0, stats.Edges)
}

// --- Graph tests ---

func TestGraph_UpsertAndRemoveNode(t *testing.T) {
	db := newTestDBNoEmbed(t)

	require.NoError(t, db.UpsertNode("note-1", "notes", "First Note", "docs/notes/note-1.md", "sha1"))
	require.NoError(t, db.UpsertNode("note-2", "notes", "Second Note", "docs/notes/note-2.md", "sha2"))

	nodes, err := db.AllNodes("")
	require.NoError(t, err)
	assert.Len(t, nodes, 2)

	require.NoError(t, db.RemoveNode("note-1"))
	nodes, _ = db.AllNodes("")
	assert.Len(t, nodes, 1)
	assert.Equal(t, "note-2", nodes[0].ID)
}

func TestGraph_AddAndQueryEdges(t *testing.T) {
	db := newTestDBNoEmbed(t)

	db.UpsertNode("a", "notes", "A", "", "")
	db.UpsertNode("b", "notes", "B", "", "")
	db.UpsertNode("c", "notes", "C", "", "")

	db.AddEdge("a", "notes", "b", "notes", "link", "see also")
	db.AddEdge("a", "notes", "c", "notes", "ref", "parent")
	db.AddEdge("b", "notes", "c", "notes", "link", "related")

	// Outgoing from A
	out, err := db.Outgoing("a")
	require.NoError(t, err)
	assert.Len(t, out, 2)

	// Incoming to C
	inc, err := db.Incoming("c")
	require.NoError(t, err)
	assert.Len(t, inc, 2) // from A (ref) and B (link)

	// Remove edges for A
	require.NoError(t, db.RemoveEdgesForDoc("a"))
	out, _ = db.Outgoing("a")
	assert.Len(t, out, 0)

	// B→C still exists
	out, _ = db.Outgoing("b")
	assert.Len(t, out, 1)
}

func TestGraph_Neighbors_BFS(t *testing.T) {
	db := newTestDBNoEmbed(t)

	// A → B → C → D (chain)
	for _, id := range []string{"a", "b", "c", "d"} {
		db.UpsertNode(id, "notes", id, "", "")
	}
	db.AddEdge("a", "notes", "b", "notes", "link", "")
	db.AddEdge("b", "notes", "c", "notes", "link", "")
	db.AddEdge("c", "notes", "d", "notes", "link", "")

	// Depth 1 from A: should find A→B
	edges, err := db.Neighbors("a", 1)
	require.NoError(t, err)
	assert.Len(t, edges, 1)
	assert.Equal(t, "b", edges[0].TargetID)

	// Depth 2 from A: should find A→B, B→C
	edges, err = db.Neighbors("a", 2)
	require.NoError(t, err)
	assert.Len(t, edges, 2)

	// Depth 3 from A: should find all 3 edges
	edges, err = db.Neighbors("a", 3)
	require.NoError(t, err)
	assert.Len(t, edges, 3)
}

func TestGraph_ExportMermaid(t *testing.T) {
	db := newTestDBNoEmbed(t)
	db.AddEdge("a", "notes", "b", "notes", "link", "see also")
	db.AddEdge("b", "notes", "c", "notes", "ref", "parent")

	out, err := db.ExportMermaid(nil)
	require.NoError(t, err)
	assert.Contains(t, out, "graph LR")
	assert.Contains(t, out, "a -->|see also| b")
	assert.Contains(t, out, "b -->|parent| c")
}

func TestGraph_ExportDOT(t *testing.T) {
	db := newTestDBNoEmbed(t)
	db.AddEdge("a", "notes", "b", "notes", "link", "")

	out, err := db.ExportDOT(nil)
	require.NoError(t, err)
	assert.Contains(t, out, "digraph KnowledgeGraph")
	assert.Contains(t, out, `"a" -> "b"`)
}

// --- Chunker tests ---

func TestChunkMarkdown_Basic(t *testing.T) {
	content := "# Hello\n\nSome paragraph.\n\n## Section 2\n\nMore content here."
	chunks := ChunkMarkdown("doc1", content, "sha1", 500)

	require.NotEmpty(t, chunks)
	assert.Equal(t, "doc1", chunks[0].DocID)
	assert.Equal(t, "sha1", chunks[0].ContentSHA)
}

func TestChunkMarkdown_SplitsOnHeadings(t *testing.T) {
	// Create content with enough text to force multiple chunks
	content := "# Part 1\n\n" + longText(200) + "\n\n## Part 2\n\n" + longText(200)
	chunks := ChunkMarkdown("doc1", content, "sha1", 100) // low max to force splitting

	assert.True(t, len(chunks) >= 2, "should split into multiple chunks, got %d", len(chunks))
}

func TestChunkMarkdown_Empty(t *testing.T) {
	chunks := ChunkMarkdown("doc1", "", "sha1", 500)
	assert.Nil(t, chunks)

	chunks = ChunkMarkdown("doc1", "   \n  \n  ", "sha1", 500)
	assert.Nil(t, chunks)
}

func TestChunkMarkdown_PreservesDocID(t *testing.T) {
	chunks := ChunkMarkdown("my-doc", "# Title\n\nContent.", "sha1", 500)
	for _, c := range chunks {
		assert.Equal(t, "my-doc", c.DocID)
	}
}

// --- Extract tests ---

func TestExtractMarkdownLinks(t *testing.T) {
	content := `See [deployment guide](../guides/deployment.md) and [ADR-12](../../adrs/adr-0012.md).
Also a non-md link [website](https://example.com) which should be ignored.`

	edges := ExtractMarkdownLinks(content, "notes")
	assert.Len(t, edges, 2)
	assert.Equal(t, "deployment", edges[0].TargetID)
	assert.Equal(t, "link", edges[0].EdgeType)
	assert.Equal(t, "deployment guide", edges[0].Context)
	assert.Equal(t, "adr-0012", edges[1].TargetID)
}

func TestExtractMarkdownLinks_NoLinks(t *testing.T) {
	edges := ExtractMarkdownLinks("No links here.", "notes")
	assert.Empty(t, edges)
}

// --- Search tests (with stub embedder) ---

func TestSearch_Basic(t *testing.T) {
	db := newTestDB(t)

	// Manually insert nodes and chunks with embeddings
	db.UpsertNode("doc-1", "notes", "Deployment Guide", "docs/doc-1.md", "sha1")
	db.UpsertNode("doc-2", "notes", "Cooking Recipes", "docs/doc-2.md", "sha2")

	// Index chunks
	texts := []string{"How to deploy applications to production", "Best pasta recipes for weeknights"}
	vecs, _ := db.embedder.Embed(texts)

	db.db.Exec(`INSERT INTO chunks (doc_id, entity, chunk_index, text, content_sha, model_id, embedding)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, "doc-1", "notes", 0, texts[0], "sha1", "stub-model", float32ToBytes(vecs[0]))
	db.db.Exec(`INSERT INTO chunks (doc_id, entity, chunk_index, text, content_sha, model_id, embedding)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, "doc-2", "notes", 0, texts[1], "sha2", "stub-model", float32ToBytes(vecs[1]))

	// Search — with stub embedder, just verify it returns results and deduplicates by doc
	results, err := db.Search("deploy to production", 5)
	require.NoError(t, err)
	require.Len(t, results, 2, "should return both docs (one chunk each)")

	// Both docs should appear (order depends on stub embedder, not semantically meaningful)
	docIDs := map[string]bool{}
	for _, r := range results {
		docIDs[r.DocID] = true
		assert.NotEmpty(t, r.Title)
		assert.NotEmpty(t, r.ChunkText)
	}
	assert.True(t, docIDs["doc-1"])
	assert.True(t, docIDs["doc-2"])
}

func TestSearch_NoEmbedder(t *testing.T) {
	db := newTestDBNoEmbed(t)
	_, err := db.Search("anything", 5)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "embedding provider")
}

// --- Staleness tests ---

func TestIsStale(t *testing.T) {
	db := newTestDBNoEmbed(t)

	// New doc is always stale
	assert.True(t, db.IsStale("new-doc", "sha1"))

	// After upserting node, matching SHA is not stale
	db.UpsertNode("doc-1", "notes", "Title", "", "sha1")
	assert.False(t, db.IsStale("doc-1", "sha1"))

	// Different SHA is stale
	assert.True(t, db.IsStale("doc-1", "sha2"))
}

// --- Helpers ---

func longText(words int) string {
	var result string
	for i := 0; i < words; i++ {
		if i > 0 {
			result += " "
		}
		result += "word"
	}
	return result
}
