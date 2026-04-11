package kg

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sergio-bershadsky/secondbrain-db/internal/schema"
)

var markdownLinkRe = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+\.md)\)`)

// ExtractedEdge represents an edge found during content analysis.
type ExtractedEdge struct {
	TargetID     string
	TargetEntity string
	EdgeType     string // "link", "ref", "virtual"
	Context      string
}

// ExtractMarkdownLinks finds [text](path.md) links in markdown content
// and resolves them to document IDs.
func ExtractMarkdownLinks(content string, currentEntity string) []ExtractedEdge {
	matches := markdownLinkRe.FindAllStringSubmatch(content, -1)
	var edges []ExtractedEdge

	for _, m := range matches {
		linkText := m[1]
		linkPath := m[2]

		targetID := pathToDocID(linkPath)
		if targetID == "" {
			continue
		}

		edges = append(edges, ExtractedEdge{
			TargetID:     targetID,
			TargetEntity: currentEntity, // assume same entity unless path says otherwise
			EdgeType:     "link",
			Context:      linkText,
		})
	}

	return edges
}

// ExtractRefFields extracts edges from schema `ref` type fields.
func ExtractRefFields(s *schema.Schema, data map[string]any) []ExtractedEdge {
	var edges []ExtractedEdge

	for name, field := range s.Fields {
		if field.Type != schema.FieldTypeRef {
			continue
		}

		val, ok := data[name]
		if !ok || val == nil {
			continue
		}

		targetEntity := field.RefEntity
		if targetEntity == "" {
			targetEntity = s.Entity
		}

		edges = append(edges, ExtractedEdge{
			TargetID:     fmt.Sprintf("%v", val),
			TargetEntity: targetEntity,
			EdgeType:     "ref",
			Context:      name,
		})
	}

	// Also check list fields containing refs
	for name, field := range s.Fields {
		if field.Type != schema.FieldTypeList || field.Items == nil || field.Items.Type != schema.FieldTypeRef {
			continue
		}

		val, ok := data[name]
		if !ok || val == nil {
			continue
		}

		targetEntity := field.Items.RefEntity
		if targetEntity == "" {
			targetEntity = s.Entity
		}

		switch v := val.(type) {
		case []any:
			for _, item := range v {
				edges = append(edges, ExtractedEdge{
					TargetID:     fmt.Sprintf("%v", item),
					TargetEntity: targetEntity,
					EdgeType:     "ref",
					Context:      name,
				})
			}
		case []string:
			for _, item := range v {
				edges = append(edges, ExtractedEdge{
					TargetID:     item,
					TargetEntity: targetEntity,
					EdgeType:     "ref",
					Context:      name,
				})
			}
		}
	}

	return edges
}

// ExtractVirtualEdges extracts edges from virtual fields annotated with edge: true.
func ExtractVirtualEdges(s *schema.Schema, virtuals map[string]any) []ExtractedEdge {
	var edges []ExtractedEdge

	for name, v := range s.Virtuals {
		if !v.Edge {
			continue
		}

		val, ok := virtuals[name]
		if !ok || val == nil {
			continue
		}

		targetEntity := v.EdgeEntity
		if targetEntity == "" {
			targetEntity = s.Entity
		}

		switch vals := val.(type) {
		case []any:
			for _, item := range vals {
				edges = append(edges, ExtractedEdge{
					TargetID:     fmt.Sprintf("%v", item),
					TargetEntity: targetEntity,
					EdgeType:     "virtual",
					Context:      name,
				})
			}
		case []string:
			for _, item := range vals {
				edges = append(edges, ExtractedEdge{
					TargetID:     item,
					TargetEntity: targetEntity,
					EdgeType:     "virtual",
					Context:      name,
				})
			}
		case string:
			edges = append(edges, ExtractedEdge{
				TargetID:     vals,
				TargetEntity: targetEntity,
				EdgeType:     "virtual",
				Context:      name,
			})
		}
	}

	return edges
}

// pathToDocID converts a relative .md path to a document ID.
// e.g. "../notes/my-note.md" → "my-note"
func pathToDocID(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	if ext != ".md" {
		return ""
	}
	id := strings.TrimSuffix(base, ext)
	if id == "" {
		return ""
	}
	return id
}
