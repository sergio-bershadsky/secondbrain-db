// Package graphstore provides a bbolt-backed persistent graph store
// for the secondbrain-db knowledge graph.
package graphstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	bolt "go.etcd.io/bbolt"
)

var (
	bucketVertices = []byte("vertices")
	bucketEdges    = []byte("edges")
	bucketOutIdx   = []byte("edges_out")
	bucketInIdx    = []byte("edges_in")
)

// VertexData holds node metadata stored in bbolt.
type VertexData struct {
	ID         string            `json:"id"`
	Entity     string            `json:"entity"`
	Title      string            `json:"title"`
	File       string            `json:"file,omitempty"`
	ContentSHA string            `json:"content_sha,omitempty"`
	Attrs      map[string]string `json:"attrs,omitempty"`
}

// EdgeData holds edge metadata stored in bbolt.
type EdgeData struct {
	SourceID     string `json:"source_id"`
	SourceEntity string `json:"source_entity"`
	TargetID     string `json:"target_id"`
	TargetEntity string `json:"target_entity"`
	Type         string `json:"type"`
	Context      string `json:"context,omitempty"`
}

// Store is a persistent graph backed by bbolt.
type Store struct {
	db *bolt.DB
}

// Open opens or creates a bbolt graph store at the given path.
func Open(path string) (*Store, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating graph store directory: %w", err)
	}

	db, err := bolt.Open(path, 0o600, nil)
	if err != nil {
		return nil, fmt.Errorf("opening graph store: %w", err)
	}

	// Create buckets
	if err := db.Update(func(tx *bolt.Tx) error {
		for _, b := range [][]byte{bucketVertices, bucketEdges, bucketOutIdx, bucketInIdx} {
			if _, err := tx.CreateBucketIfNotExists(b); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		db.Close()
		return nil, err
	}

	return &Store{db: db}, nil
}

// OpenMemory opens an in-memory bbolt store (temp file, cleaned up on Close).
func OpenMemory() (*Store, error) {
	tmp, err := os.CreateTemp("", "sbdb-graph-*.db")
	if err != nil {
		return nil, err
	}
	path := tmp.Name()
	tmp.Close()

	s, err := Open(path)
	if err != nil {
		os.Remove(path)
		return nil, err
	}
	return s, nil
}

// Close closes the bbolt database.
func (s *Store) Close() error {
	return s.db.Close()
}

// --- Vertex operations ---

// PutVertex inserts or updates a vertex.
func (s *Store) PutVertex(v *VertexData) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketVertices).Put([]byte(v.ID), data)
	})
}

// GetVertex retrieves a vertex by ID. Returns nil if not found.
func (s *Store) GetVertex(id string) (*VertexData, error) {
	var v VertexData
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket(bucketVertices).Get([]byte(id))
		if data == nil {
			return fmt.Errorf("vertex %q not found", id)
		}
		return json.Unmarshal(data, &v)
	})
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// RemoveVertex deletes a vertex and all its edges.
func (s *Store) RemoveVertex(id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		// Delete vertex
		tx.Bucket(bucketVertices).Delete([]byte(id))

		// Delete all outgoing edges
		outBucket := tx.Bucket(bucketOutIdx)
		edgeBucket := tx.Bucket(bucketEdges)
		inBucket := tx.Bucket(bucketInIdx)

		prefix := []byte(id + ":")
		c := outBucket.Cursor()
		for k, _ := c.Seek(prefix); k != nil && hasPrefix(k, prefix); k, _ = c.Next() {
			targetID := string(k[len(prefix):])
			edgeKey := edgeKey(id, targetID)
			edgeBucket.Delete(edgeKey)
			inBucket.Delete([]byte(targetID + ":" + id))
			outBucket.Delete(k)
		}

		// Delete all incoming edges
		inPrefix := []byte(id + ":")
		c2 := inBucket.Cursor()
		for k, _ := c2.Seek(inPrefix); k != nil && hasPrefix(k, inPrefix); k, _ = c2.Next() {
			sourceID := string(k[len(inPrefix):])
			edgeKey := edgeKey(sourceID, id)
			edgeBucket.Delete(edgeKey)
			outBucket.Delete([]byte(sourceID + ":" + id))
			inBucket.Delete(k)
		}

		return nil
	})
}

// AllVertices returns all vertices.
func (s *Store) AllVertices(entity string) ([]VertexData, error) {
	var result []VertexData
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketVertices).ForEach(func(_, data []byte) error {
			var v VertexData
			if err := json.Unmarshal(data, &v); err != nil {
				return err
			}
			if entity == "" || v.Entity == entity {
				result = append(result, v)
			}
			return nil
		})
	})
	return result, err
}

// VertexCount returns the number of vertices.
func (s *Store) VertexCount() (int, error) {
	var count int
	err := s.db.View(func(tx *bolt.Tx) error {
		count = tx.Bucket(bucketVertices).Stats().KeyN
		return nil
	})
	return count, err
}

// --- Edge operations ---

// PutEdge inserts or updates an edge.
func (s *Store) PutEdge(e *EdgeData) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	key := edgeKey(e.SourceID, e.TargetID)

	return s.db.Update(func(tx *bolt.Tx) error {
		if err := tx.Bucket(bucketEdges).Put(key, data); err != nil {
			return err
		}
		// Outgoing index
		if err := tx.Bucket(bucketOutIdx).Put([]byte(e.SourceID+":"+e.TargetID), nil); err != nil {
			return err
		}
		// Incoming index
		return tx.Bucket(bucketInIdx).Put([]byte(e.TargetID+":"+e.SourceID), nil)
	})
}

// RemoveEdgesFrom deletes all outgoing edges from a vertex.
func (s *Store) RemoveEdgesFrom(sourceID string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		outBucket := tx.Bucket(bucketOutIdx)
		edgeBucket := tx.Bucket(bucketEdges)
		inBucket := tx.Bucket(bucketInIdx)

		prefix := []byte(sourceID + ":")
		c := outBucket.Cursor()
		var toDelete [][]byte
		for k, _ := c.Seek(prefix); k != nil && hasPrefix(k, prefix); k, _ = c.Next() {
			targetID := string(k[len(prefix):])
			toDelete = append(toDelete, append([]byte{}, k...))
			edgeBucket.Delete(edgeKey(sourceID, targetID))
			inBucket.Delete([]byte(targetID + ":" + sourceID))
		}
		for _, k := range toDelete {
			outBucket.Delete(k)
		}
		return nil
	})
}

// Outgoing returns all edges from a vertex.
func (s *Store) Outgoing(sourceID string) ([]EdgeData, error) {
	var result []EdgeData
	err := s.db.View(func(tx *bolt.Tx) error {
		outBucket := tx.Bucket(bucketOutIdx)
		edgeBucket := tx.Bucket(bucketEdges)

		prefix := []byte(sourceID + ":")
		c := outBucket.Cursor()
		for k, _ := c.Seek(prefix); k != nil && hasPrefix(k, prefix); k, _ = c.Next() {
			targetID := string(k[len(prefix):])
			data := edgeBucket.Get(edgeKey(sourceID, targetID))
			if data != nil {
				var e EdgeData
				if err := json.Unmarshal(data, &e); err == nil {
					result = append(result, e)
				}
			}
		}
		return nil
	})
	return result, err
}

// Incoming returns all edges to a vertex.
func (s *Store) Incoming(targetID string) ([]EdgeData, error) {
	var result []EdgeData
	err := s.db.View(func(tx *bolt.Tx) error {
		inBucket := tx.Bucket(bucketInIdx)
		edgeBucket := tx.Bucket(bucketEdges)

		prefix := []byte(targetID + ":")
		c := inBucket.Cursor()
		for k, _ := c.Seek(prefix); k != nil && hasPrefix(k, prefix); k, _ = c.Next() {
			sourceID := string(k[len(prefix):])
			data := edgeBucket.Get(edgeKey(sourceID, targetID))
			if data != nil {
				var e EdgeData
				if err := json.Unmarshal(data, &e); err == nil {
					result = append(result, e)
				}
			}
		}
		return nil
	})
	return result, err
}

// AllEdges returns all edges, optionally filtered by type.
func (s *Store) AllEdges(edgeType string) ([]EdgeData, error) {
	var result []EdgeData
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketEdges).ForEach(func(_, data []byte) error {
			var e EdgeData
			if err := json.Unmarshal(data, &e); err != nil {
				return nil // skip malformed
			}
			if edgeType == "" || e.Type == edgeType {
				result = append(result, e)
			}
			return nil
		})
	})
	return result, err
}

// EdgeCount returns the number of edges.
func (s *Store) EdgeCount() (int, error) {
	var count int
	err := s.db.View(func(tx *bolt.Tx) error {
		count = tx.Bucket(bucketEdges).Stats().KeyN
		return nil
	})
	return count, err
}

// --- Traversal ---

// Neighbors returns all vertices reachable within `depth` hops via BFS.
// Returns the edges discovered during traversal (deduplicated).
func (s *Store) Neighbors(startID string, depth int) ([]EdgeData, error) {
	if depth < 1 {
		depth = 1
	}

	visited := map[string]bool{startID: true}
	frontier := []string{startID}
	var allEdges []EdgeData
	seen := map[string]bool{}

	for d := 0; d < depth; d++ {
		if len(frontier) == 0 {
			break
		}

		var nextFrontier []string
		for _, nodeID := range frontier {
			outEdges, err := s.Outgoing(nodeID)
			if err != nil {
				return nil, err
			}
			for _, e := range outEdges {
				key := e.SourceID + "|" + e.TargetID + "|" + e.Type
				if !seen[key] {
					seen[key] = true
					allEdges = append(allEdges, e)
				}
				if !visited[e.TargetID] {
					visited[e.TargetID] = true
					nextFrontier = append(nextFrontier, e.TargetID)
				}
			}

			inEdges, err := s.Incoming(nodeID)
			if err != nil {
				return nil, err
			}
			for _, e := range inEdges {
				key := e.SourceID + "|" + e.TargetID + "|" + e.Type
				if !seen[key] {
					seen[key] = true
					allEdges = append(allEdges, e)
				}
				if !visited[e.SourceID] {
					visited[e.SourceID] = true
					nextFrontier = append(nextFrontier, e.SourceID)
				}
			}
		}
		frontier = nextFrontier
	}

	return allEdges, nil
}

// ShortestPath finds the shortest path between two vertices using BFS.
// Returns the sequence of vertex IDs, or nil if no path exists.
func (s *Store) ShortestPath(fromID, toID string) ([]string, error) {
	if fromID == toID {
		return []string{fromID}, nil
	}

	type queueItem struct {
		id   string
		path []string
	}

	visited := map[string]bool{fromID: true}
	queue := []queueItem{{id: fromID, path: []string{fromID}}}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		outEdges, err := s.Outgoing(current.id)
		if err != nil {
			return nil, err
		}

		for _, e := range outEdges {
			if e.TargetID == toID {
				return append(current.path, toID), nil
			}
			if !visited[e.TargetID] {
				visited[e.TargetID] = true
				newPath := make([]string, len(current.path)+1)
				copy(newPath, current.path)
				newPath[len(current.path)] = e.TargetID
				queue = append(queue, queueItem{id: e.TargetID, path: newPath})
			}
		}
	}

	return nil, nil // no path
}

// Drop removes all data from the graph store.
func (s *Store) Drop() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		for _, name := range [][]byte{bucketVertices, bucketEdges, bucketOutIdx, bucketInIdx} {
			if err := tx.DeleteBucket(name); err != nil {
				return err
			}
			if _, err := tx.CreateBucket(name); err != nil {
				return err
			}
		}
		return nil
	})
}

// --- Export ---

// ExportMermaid generates a Mermaid diagram.
func (s *Store) ExportMermaid() (string, error) {
	edges, err := s.AllEdges("")
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("graph LR\n")
	for _, e := range edges {
		label := e.Type
		if e.Context != "" {
			label = e.Context
		}
		fmt.Fprintf(&b, "    %s -->|%s| %s\n", e.SourceID, label, e.TargetID)
	}
	return b.String(), nil
}

// ExportDOT generates a Graphviz DOT diagram.
func (s *Store) ExportDOT() (string, error) {
	edges, err := s.AllEdges("")
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("digraph KnowledgeGraph {\n")
	b.WriteString("    rankdir=LR;\n")
	b.WriteString("    node [shape=box];\n\n")
	for _, e := range edges {
		label := e.Type
		if e.Context != "" {
			label = e.Context
		}
		fmt.Fprintf(&b, "    \"%s\" -> \"%s\" [label=\"%s\"];\n", e.SourceID, e.TargetID, label)
	}
	b.WriteString("}\n")
	return b.String(), nil
}

// ExportJSON returns the graph as a visualization-ready JSON structure.
type GraphJSON struct {
	Nodes []NodeJSON `json:"nodes"`
	Edges []EdgeJSON `json:"edges"`
}

// NodeJSON is a node in the export format.
type NodeJSON struct {
	ID     string `json:"id"`
	Entity string `json:"entity"`
	Title  string `json:"title"`
	File   string `json:"file,omitempty"`
}

// EdgeJSON is an edge in the export format.
type EdgeJSON struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"`
	Label  string `json:"label,omitempty"`
}

func (s *Store) ExportGraphJSON() (*GraphJSON, error) {
	vertices, err := s.AllVertices("")
	if err != nil {
		return nil, err
	}
	edges, err := s.AllEdges("")
	if err != nil {
		return nil, err
	}

	g := &GraphJSON{}

	nodeSet := map[string]bool{}
	for _, v := range vertices {
		nodeSet[v.ID] = true
		g.Nodes = append(g.Nodes, NodeJSON{
			ID: v.ID, Entity: v.Entity, Title: v.Title, File: v.File,
		})
	}

	for _, e := range edges {
		// Add placeholder nodes for targets not yet in DB
		if !nodeSet[e.TargetID] {
			nodeSet[e.TargetID] = true
			g.Nodes = append(g.Nodes, NodeJSON{ID: e.TargetID, Entity: e.TargetEntity, Title: e.TargetID})
		}
		label := e.Type
		if e.Context != "" {
			label = e.Context
		}
		g.Edges = append(g.Edges, EdgeJSON{
			Source: e.SourceID, Target: e.TargetID, Type: e.Type, Label: label,
		})
	}

	return g, nil
}

// --- Helpers ---

func edgeKey(sourceID, targetID string) []byte {
	return []byte(sourceID + ":" + targetID)
}

func hasPrefix(key, prefix []byte) bool {
	if len(key) < len(prefix) {
		return false
	}
	for i := range prefix {
		if key[i] != prefix[i] {
			return false
		}
	}
	return true
}
