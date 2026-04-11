package kg

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/sergio-bershadsky/secondbrain-db/internal/document"
	"github.com/sergio-bershadsky/secondbrain-db/internal/integrity"
	"github.com/sergio-bershadsky/secondbrain-db/internal/schema"
)

// IndexDocument indexes a single document: chunks, embeds, extracts edges, updates the graph.
func (d *DB) IndexDocument(doc *document.Document, s *schema.Schema) error {
	if err := doc.EnsureLoaded(); err != nil {
		return fmt.Errorf("loading document for indexing: %w", err)
	}

	id := doc.ID()
	contentSHA := integrity.HashContent(doc.Content)

	// Check staleness
	if !d.IsStale(id, contentSHA) {
		return nil // already indexed with same content
	}

	// Get title from data or virtuals
	title := id
	if t, ok := doc.Get("title"); ok {
		title = fmt.Sprintf("%v", t)
	}

	// 1. Upsert node
	if err := d.UpsertNode(id, s.Entity, title, doc.RelativeFilePath(), contentSHA); err != nil {
		return fmt.Errorf("upserting node: %w", err)
	}

	// 2. Remove old edges from this doc (will re-extract)
	if err := d.RemoveEdgesForDoc(id); err != nil {
		return fmt.Errorf("removing old edges: %w", err)
	}

	// 3. Extract and add edges
	// From markdown links
	linkEdges := ExtractMarkdownLinks(doc.Content, s.Entity)
	for _, e := range linkEdges {
		d.AddEdge(id, s.Entity, e.TargetID, e.TargetEntity, e.EdgeType, e.Context)
	}

	// From ref fields
	refEdges := ExtractRefFields(s, doc.Data)
	for _, e := range refEdges {
		d.AddEdge(id, s.Entity, e.TargetID, e.TargetEntity, e.EdgeType, e.Context)
	}

	// From virtual fields with edge: true
	virtualEdges := ExtractVirtualEdges(s, doc.Virtuals())
	for _, e := range virtualEdges {
		d.AddEdge(id, s.Entity, e.TargetID, e.TargetEntity, e.EdgeType, e.Context)
	}

	// 4. Chunk and embed (only if embedder is configured)
	if d.embedder != nil {
		if err := d.indexChunks(doc, s, contentSHA); err != nil {
			return fmt.Errorf("indexing chunks: %w", err)
		}
	}

	return nil
}

func (d *DB) indexChunks(doc *document.Document, s *schema.Schema, contentSHA string) error {
	id := doc.ID()
	modelID := d.embedder.ModelID()

	// Remove old chunks for this doc+model
	_, err := d.db.Exec(`DELETE FROM chunks WHERE doc_id = ? AND model_id = ?`, id, modelID)
	if err != nil {
		return err
	}

	// Chunk the content
	chunks := ChunkMarkdown(id, doc.Content, contentSHA, DefaultMaxTokens)
	if len(chunks) == 0 {
		return nil
	}

	// Collect texts for batch embedding
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Text
	}

	// Call embedder
	vectors, err := d.embedder.Embed(texts)
	if err != nil {
		return fmt.Errorf("embedding %d chunks: %w", len(chunks), err)
	}

	if len(vectors) != len(chunks) {
		return fmt.Errorf("embedder returned %d vectors for %d chunks", len(vectors), len(chunks))
	}

	// Insert chunks + embeddings
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(
		`INSERT INTO chunks (doc_id, entity, chunk_index, text, content_sha, model_id, embedding)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for i, chunk := range chunks {
		embBlob := float32ToBytes(vectors[i])
		_, err := stmt.Exec(chunk.DocID, s.Entity, chunk.ChunkIndex, chunk.Text,
			chunk.ContentSHA, modelID, embBlob)
		if err != nil {
			return fmt.Errorf("inserting chunk %d: %w", i, err)
		}
	}

	// Update metadata
	tx.Exec(`INSERT INTO meta (key, value) VALUES ('model_id', ?) ON CONFLICT(key) DO UPDATE SET value = ?`,
		modelID, modelID)
	tx.Exec(`INSERT INTO meta (key, value) VALUES ('last_build_at', ?) ON CONFLICT(key) DO UPDATE SET value = ?`,
		nowRFC3339(), nowRFC3339())

	return tx.Commit()
}

// RemoveDocument removes a document from the index and graph.
func (d *DB) RemoveDocument(docID string) error {
	return d.RemoveNode(docID)
}

// IsStale returns true if the document needs re-indexing.
func (d *DB) IsStale(docID, contentSHA string) bool {
	var existing string
	err := d.db.QueryRow(`SELECT content_sha FROM nodes WHERE id = ?`, docID).Scan(&existing)
	if err == sql.ErrNoRows {
		return true
	}
	if err != nil {
		return true
	}
	return existing != contentSHA
}

// BuildAll indexes all provided documents.
func (d *DB) BuildAll(docs []*document.Document, s *schema.Schema, force bool) (indexed int, err error) {
	for _, doc := range docs {
		if err := doc.EnsureLoaded(); err != nil {
			continue
		}

		if !force {
			contentSHA := integrity.HashContent(doc.Content)
			if !d.IsStale(doc.ID(), contentSHA) {
				continue
			}
		}

		if err := d.IndexDocument(doc, s); err != nil {
			return indexed, fmt.Errorf("indexing %s: %w", doc.ID(), err)
		}
		indexed++
	}
	return indexed, nil
}

// float32ToBytes converts a float32 slice to a byte slice (little-endian).
func float32ToBytes(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// bytesToFloat32 converts a byte slice back to float32 slice.
func bytesToFloat32(b []byte) []float32 {
	n := len(b) / 4
	v := make([]float32, n)
	for i := 0; i < n; i++ {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}
