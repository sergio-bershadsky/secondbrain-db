package kg

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/storage"
)

// CrawlResult holds stats from a crawl-mode index build.
type CrawlResult struct {
	FilesFound   int `json:"files_found"`
	FilesIndexed int `json:"files_indexed"`
	FilesSkipped int `json:"files_skipped"`
	EdgesFound   int `json:"edges_found"`
}

// CrawlOptions controls crawl behavior.
type CrawlOptions struct {
	DocsRoot string   // root directory to walk (e.g. "docs/")
	Exclude  []string // glob patterns to exclude (e.g. "node_modules", ".vitepress")
	Force    bool     // re-index even if content hasn't changed
}

// DefaultExcludes are directories skipped during crawl.
var DefaultExcludes = []string{
	"node_modules",
	".vitepress",
	".git",
	"public",
	"dist",
}

// CrawlAndIndex walks a docs directory, indexing every .md file it finds.
// For files with YAML frontmatter, it extracts fields. For files without,
// it derives metadata from the filename and path.
func (d *DB) CrawlAndIndex(opts CrawlOptions) (*CrawlResult, error) {
	result := &CrawlResult{}

	if opts.DocsRoot == "" {
		return nil, fmt.Errorf("crawl: docs_root is required")
	}

	if len(opts.Exclude) == 0 {
		opts.Exclude = DefaultExcludes
	}

	err := filepath.Walk(opts.DocsRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable paths
		}

		// Skip excluded directories
		if info.IsDir() {
			base := filepath.Base(path)
			for _, excl := range opts.Exclude {
				if base == excl {
					return filepath.SkipDir
				}
			}
			return nil
		}

		// Only process .md files
		if filepath.Ext(path) != ".md" {
			return nil
		}

		result.FilesFound++

		// Parse the file
		fm, body, parseErr := storage.ParseMarkdown(path)
		if parseErr != nil {
			result.FilesSkipped++
			return nil
		}

		// Derive metadata
		relPath, _ := filepath.Rel(opts.DocsRoot, path)
		if relPath == "" {
			relPath = path
		}

		docID := pathToDocID(path)
		entity := deriveEntity(relPath)
		title := deriveTitle(fm, body, docID)
		contentSHA := storage.CanonicalBodyHash(body)

		// Check staleness
		if !opts.Force && !d.IsStale(docID, contentSHA) {
			result.FilesSkipped++
			return nil
		}

		// Upsert node
		if err := d.UpsertNode(docID, entity, title, relPath, contentSHA); err != nil {
			result.FilesSkipped++
			return nil
		}

		// Remove old edges from this doc
		d.RemoveEdgesForDoc(docID)

		// Extract edges from markdown links
		linkEdges := ExtractMarkdownLinks(body, entity)
		for _, e := range linkEdges {
			d.AddEdge(docID, entity, e.TargetID, e.TargetEntity, e.EdgeType, e.Context)
			result.EdgesFound++
		}

		// Extract edges from frontmatter refs (if any ref-like fields exist)
		for key, val := range fm {
			if s, ok := val.(string); ok && looksLikeDocRef(s) {
				d.AddEdge(docID, entity, s, entity, "frontmatter_ref", key)
				result.EdgesFound++
			}
		}

		// Chunk and embed (if embedder available)
		if d.embedder != nil {
			chunks := ChunkMarkdown(docID, body, contentSHA, DefaultMaxTokens)
			if len(chunks) > 0 {
				if err := d.indexChunksRaw(docID, entity, chunks); err != nil {
					// Log but don't fail the crawl
					Logger.Warn("embedding failed", "file", relPath, "error", err)
				}
			}
		}

		result.FilesIndexed++
		return nil
	})

	return result, err
}

// indexChunksRaw indexes pre-built chunks (used by crawl mode).
func (d *DB) indexChunksRaw(docID, entity string, chunks []Chunk) error {
	modelID := d.embedder.ModelID()

	// Remove old chunks
	d.db.Exec(`DELETE FROM chunks WHERE doc_id = ? AND model_id = ?`, docID, modelID)

	// Batch embed
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Text
	}

	vectors, err := d.embedder.Embed(texts)
	if err != nil {
		return err
	}
	if len(vectors) != len(chunks) {
		return fmt.Errorf("embedder returned %d vectors for %d chunks", len(vectors), len(chunks))
	}

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
		_, err := stmt.Exec(chunk.DocID, entity, chunk.ChunkIndex, chunk.Text,
			chunk.ContentSHA, modelID, float32ToBytes(vectors[i]))
		if err != nil {
			return err
		}
	}

	tx.Exec(`INSERT INTO meta (key, value) VALUES ('last_build_at', ?) ON CONFLICT(key) DO UPDATE SET value = ?`,
		nowRFC3339(), nowRFC3339())

	return tx.Commit()
}

// deriveEntity infers the entity type from the relative path.
// e.g. "discussions/2026-01-05-sync.md" → "discussions"
// e.g. "guides/setup/docker.md" → "guides"
func deriveEntity(relPath string) string {
	parts := strings.Split(filepath.ToSlash(relPath), "/")
	if len(parts) >= 2 {
		return parts[0]
	}
	return "root"
}

// deriveTitle extracts a title from frontmatter, first heading, or filename.
func deriveTitle(fm map[string]any, body, fallbackID string) string {
	// 1. Frontmatter title
	if t, ok := fm["title"]; ok {
		return fmt.Sprintf("%v", t)
	}

	// 2. First # heading
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimPrefix(trimmed, "# ")
		}
	}

	// 3. Filename
	return fallbackID
}

// looksLikeDocRef returns true if a string value looks like a document reference.
// Matches patterns like "ADR-0012", "note-slug", etc.
var docRefRe = regexp.MustCompile(`^[a-zA-Z][\w-]+$`)

func looksLikeDocRef(s string) bool {
	if len(s) < 2 || len(s) > 100 {
		return false
	}
	// Must look like an identifier, not prose
	return docRefRe.MatchString(s) && !strings.Contains(s, " ")
}
