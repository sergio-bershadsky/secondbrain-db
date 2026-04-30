package kg

import (
	"fmt"
	"math"
	"sort"
)

// SearchResult represents a semantic search match.
type SearchResult struct {
	DocID     string  `json:"doc_id"`
	Entity    string  `json:"entity"`
	Title     string  `json:"title"`
	ChunkText string  `json:"snippet"`
	Score     float32 `json:"score"` // cosine similarity (higher = more similar)
}

// Search performs semantic similarity search over the indexed chunks.
// Returns the top-k most similar chunks, deduplicated by document.
func (d *DB) Search(query string, k int) ([]SearchResult, error) {
	if d.embedder == nil {
		return nil, fmt.Errorf("semantic search requires an embedding provider — set SBDB_EMBED_API_KEY")
	}

	if k <= 0 {
		k = 10
	}

	// Embed the query
	vectors, err := d.embedder.Embed([]string{query})
	if err != nil {
		return nil, fmt.Errorf("embedding query: %w", err)
	}
	if len(vectors) == 0 {
		return nil, fmt.Errorf("embedder returned no vectors")
	}
	queryVec := vectors[0]

	// Brute-force cosine similarity over all chunks
	// (works well up to ~100k chunks; replace with sqlite-vec for larger scale)
	rows, err := d.db.Query(
		`SELECT c.doc_id, c.entity, c.text, c.embedding, COALESCE(n.title, c.doc_id)
		 FROM chunks c
		 LEFT JOIN nodes n ON n.id = c.doc_id
		 WHERE c.embedding IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("querying chunks: %w", err)
	}
	defer rows.Close()

	type scored struct {
		result SearchResult
		score  float32
	}
	var candidates []scored

	for rows.Next() {
		var docID, entity, text, title string
		var embBlob []byte

		if err := rows.Scan(&docID, &entity, &text, &embBlob, &title); err != nil {
			continue
		}

		chunkVec := bytesToFloat32(embBlob)
		sim := cosineSimilarity(queryVec, chunkVec)

		candidates = append(candidates, scored{
			result: SearchResult{
				DocID:     docID,
				Entity:    entity,
				Title:     title,
				ChunkText: truncate(text, 200),
				Score:     sim,
			},
			score: sim,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Sort by similarity (descending)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	// Deduplicate by doc_id (keep highest-scoring chunk per doc)
	seen := make(map[string]bool)
	var results []SearchResult
	for _, c := range candidates {
		if seen[c.result.DocID] {
			continue
		}
		seen[c.result.DocID] = true
		results = append(results, c.result)
		if len(results) >= k {
			break
		}
	}

	return results, nil
}

// SearchWithExpand performs semantic search, then expands results with graph neighbors.
func (d *DB) SearchWithExpand(query string, k, depth int) ([]SearchResult, []Edge, error) {
	results, err := d.Search(query, k)
	if err != nil {
		return nil, nil, err
	}

	// Collect all doc IDs from results
	var allEdges []Edge
	seen := make(map[string]bool)
	for _, r := range results {
		if seen[r.DocID] {
			continue
		}
		seen[r.DocID] = true

		neighbors, err := d.Neighbors(r.DocID, depth)
		if err != nil {
			continue
		}
		allEdges = append(allEdges, neighbors...)
	}

	return results, deduplicateEdges(allEdges), nil
}

// cosineSimilarity computes the cosine similarity between two vectors.
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}

	return float32(dotProduct / denom)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
