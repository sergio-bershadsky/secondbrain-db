package kg

import (
	"database/sql"
	"fmt"
	"strings"
)

// Edge represents a directed relationship between two documents.
type Edge struct {
	SourceID     string `json:"source_id"`
	SourceEntity string `json:"source_entity"`
	TargetID     string `json:"target_id"`
	TargetEntity string `json:"target_entity"`
	EdgeType     string `json:"edge_type"` // "ref", "link", "virtual", "backlink"
	Context      string `json:"context"`
}

// Node represents a document in the knowledge graph.
type Node struct {
	ID         string `json:"id"`
	Entity     string `json:"entity"`
	Title      string `json:"title"`
	File       string `json:"file"`
	ContentSHA string `json:"content_sha"`
}

// GraphFilter controls which nodes/edges to include in exports.
type GraphFilter struct {
	Entities []string // filter by entity type
	EdgeType string   // filter by edge type
}

// UpsertNode inserts or updates a node in the graph.
func (d *DB) UpsertNode(id, entity, title, file, contentSHA string) error {
	_, err := d.db.Exec(
		`INSERT INTO nodes (id, entity, title, file, content_sha, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   entity=?, title=?, file=?, content_sha=?, updated_at=?`,
		id, entity, title, file, contentSHA, nowRFC3339(),
		entity, title, file, contentSHA, nowRFC3339(),
	)
	return err
}

// RemoveNode deletes a node and all its edges.
func (d *DB) RemoveNode(id string) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	tx.Exec(`DELETE FROM nodes WHERE id = ?`, id)
	tx.Exec(`DELETE FROM edges WHERE source_id = ? OR target_id = ?`, id, id)
	tx.Exec(`DELETE FROM chunks WHERE doc_id = ?`, id)

	return tx.Commit()
}

// AddEdge adds a directed edge between two documents.
func (d *DB) AddEdge(source, sourceEntity, target, targetEntity, edgeType, context string) error {
	_, err := d.db.Exec(
		`INSERT INTO edges (source_id, source_entity, target_id, target_entity, edge_type, context)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(source_id, target_id, edge_type) DO UPDATE SET
		   context=?, source_entity=?, target_entity=?`,
		source, sourceEntity, target, targetEntity, edgeType, context,
		context, sourceEntity, targetEntity,
	)
	return err
}

// RemoveEdgesForDoc removes all edges where the doc is the source.
func (d *DB) RemoveEdgesForDoc(docID string) error {
	_, err := d.db.Exec(`DELETE FROM edges WHERE source_id = ?`, docID)
	return err
}

// Incoming returns all edges pointing TO the given document.
func (d *DB) Incoming(docID string) ([]Edge, error) {
	rows, err := d.db.Query(
		`SELECT source_id, source_entity, target_id, target_entity, edge_type, COALESCE(context,'')
		 FROM edges WHERE target_id = ?`, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEdges(rows)
}

// Outgoing returns all edges FROM the given document.
func (d *DB) Outgoing(docID string) ([]Edge, error) {
	rows, err := d.db.Query(
		`SELECT source_id, source_entity, target_id, target_entity, edge_type, COALESCE(context,'')
		 FROM edges WHERE source_id = ?`, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEdges(rows)
}

// Neighbors returns all edges within `depth` hops of the given document (BFS).
func (d *DB) Neighbors(docID string, depth int) ([]Edge, error) {
	if depth < 1 {
		depth = 1
	}

	visited := map[string]bool{docID: true}
	frontier := []string{docID}
	var allEdges []Edge

	for d_i := 0; d_i < depth; d_i++ {
		if len(frontier) == 0 {
			break
		}

		placeholders := make([]string, len(frontier))
		args := make([]any, len(frontier)*2)
		for i, id := range frontier {
			placeholders[i] = "?"
			args[i] = id
			args[len(frontier)+i] = id
		}
		ph := strings.Join(placeholders, ",")

		query := fmt.Sprintf(
			`SELECT source_id, source_entity, target_id, target_entity, edge_type, COALESCE(context,'')
			 FROM edges WHERE source_id IN (%s) OR target_id IN (%s)`, ph, ph)

		rows, err := d.db.Query(query, args...)
		if err != nil {
			return nil, err
		}

		edges, err := scanEdges(rows)
		rows.Close()
		if err != nil {
			return nil, err
		}

		var nextFrontier []string
		for _, e := range edges {
			allEdges = append(allEdges, e)
			if !visited[e.SourceID] {
				visited[e.SourceID] = true
				nextFrontier = append(nextFrontier, e.SourceID)
			}
			if !visited[e.TargetID] {
				visited[e.TargetID] = true
				nextFrontier = append(nextFrontier, e.TargetID)
			}
		}
		frontier = nextFrontier
	}

	return deduplicateEdges(allEdges), nil
}

// AllNodes returns all nodes, optionally filtered by entity.
func (d *DB) AllNodes(entity string) ([]Node, error) {
	query := `SELECT id, entity, COALESCE(title,''), COALESCE(file,''), COALESCE(content_sha,'') FROM nodes`
	var args []any
	if entity != "" {
		query += ` WHERE entity = ?`
		args = append(args, entity)
	}
	query += ` ORDER BY id`

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []Node
	for rows.Next() {
		var n Node
		if err := rows.Scan(&n.ID, &n.Entity, &n.Title, &n.File, &n.ContentSHA); err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

// AllEdges returns all edges, optionally filtered.
func (d *DB) AllEdges(filter *GraphFilter) ([]Edge, error) {
	query := `SELECT source_id, source_entity, target_id, target_entity, edge_type, COALESCE(context,'') FROM edges`
	var conditions []string
	var args []any

	if filter != nil {
		if filter.EdgeType != "" {
			conditions = append(conditions, "edge_type = ?")
			args = append(args, filter.EdgeType)
		}
		if len(filter.Entities) > 0 {
			ph := make([]string, len(filter.Entities))
			for i, e := range filter.Entities {
				ph[i] = "?"
				args = append(args, e)
			}
			conditions = append(conditions, fmt.Sprintf("source_entity IN (%s)", strings.Join(ph, ",")))
		}
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEdges(rows)
}

// ExportMermaid generates a Mermaid diagram of the graph.
func (d *DB) ExportMermaid(filter *GraphFilter) (string, error) {
	edges, err := d.AllEdges(filter)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("graph LR\n")

	for _, e := range edges {
		label := e.EdgeType
		if e.Context != "" {
			label = e.Context
		}
		fmt.Fprintf(&b, "    %s -->|%s| %s\n", e.SourceID, label, e.TargetID)
	}

	return b.String(), nil
}

// ExportDOT generates a Graphviz DOT diagram.
func (d *DB) ExportDOT(filter *GraphFilter) (string, error) {
	edges, err := d.AllEdges(filter)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("digraph KnowledgeGraph {\n")
	b.WriteString("    rankdir=LR;\n")
	b.WriteString("    node [shape=box];\n\n")

	for _, e := range edges {
		label := e.EdgeType
		if e.Context != "" {
			label = e.Context
		}
		fmt.Fprintf(&b, "    \"%s\" -> \"%s\" [label=\"%s\"];\n", e.SourceID, e.TargetID, label)
	}

	b.WriteString("}\n")
	return b.String(), nil
}

// GraphJSON is the visualization-ready JSON export format.
// Compatible with D3.js force graphs, Cytoscape.js, React Flow, Sigma.js.
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

// ExportJSON returns the graph in a format ready for visualization libraries.
func (d *DB) ExportJSON(filter *GraphFilter) (*GraphJSON, error) {
	nodes, err := d.AllNodes("")
	if err != nil {
		return nil, err
	}

	edges, err := d.AllEdges(filter)
	if err != nil {
		return nil, err
	}

	g := &GraphJSON{}

	// Collect node IDs that appear in edges for filtering
	edgeNodeIDs := map[string]bool{}
	for _, e := range edges {
		edgeNodeIDs[e.SourceID] = true
		edgeNodeIDs[e.TargetID] = true
	}

	for _, n := range nodes {
		g.Nodes = append(g.Nodes, NodeJSON{
			ID:     n.ID,
			Entity: n.Entity,
			Title:  n.Title,
			File:   n.File,
		})
	}

	// Add placeholder nodes for targets that aren't in the DB yet
	for _, e := range edges {
		if !nodeExists(nodes, e.TargetID) {
			g.Nodes = append(g.Nodes, NodeJSON{
				ID:     e.TargetID,
				Entity: e.TargetEntity,
				Title:  e.TargetID,
			})
		}
	}

	for _, e := range edges {
		label := e.EdgeType
		if e.Context != "" {
			label = e.Context
		}
		g.Edges = append(g.Edges, EdgeJSON{
			Source: e.SourceID,
			Target: e.TargetID,
			Type:   e.EdgeType,
			Label:  label,
		})
	}

	return g, nil
}

func nodeExists(nodes []Node, id string) bool {
	for _, n := range nodes {
		if n.ID == id {
			return true
		}
	}
	return false
}

func scanEdges(rows *sql.Rows) ([]Edge, error) {
	var edges []Edge
	for rows.Next() {
		var e Edge
		if err := rows.Scan(&e.SourceID, &e.SourceEntity, &e.TargetID, &e.TargetEntity, &e.EdgeType, &e.Context); err != nil {
			return nil, err
		}
		edges = append(edges, e)
	}
	return edges, rows.Err()
}

func deduplicateEdges(edges []Edge) []Edge {
	seen := make(map[string]bool)
	var result []Edge
	for _, e := range edges {
		key := e.SourceID + "|" + e.TargetID + "|" + e.EdgeType
		if !seen[key] {
			seen[key] = true
			result = append(result, e)
		}
	}
	return result
}
