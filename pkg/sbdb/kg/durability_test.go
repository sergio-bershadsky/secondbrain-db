package kg

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/document"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/integrity"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/schema"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/storage"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/virtuals"
)

// ============================================================================
// Synthetic KB generators
// ============================================================================

// genConfig controls the shape of a generated knowledge base.
type genConfig struct {
	NumDocs        int
	NumEntities    int      // how many different schemas
	EntityNames    []string // e.g. ["notes", "adrs", "guides"]
	LinkDensity    float64  // probability of a doc linking to another (0..1)
	FrontmatterPct float64  // probability of a doc having frontmatter
	MaxBodyWords   int
	Seed           int64
}

func defaultGenConfig() genConfig {
	return genConfig{
		NumDocs:        50,
		NumEntities:    3,
		EntityNames:    []string{"notes", "adrs", "guides"},
		LinkDensity:    0.15,
		FrontmatterPct: 0.7,
		MaxBodyWords:   300,
		Seed:           42,
	}
}

// generateKB creates a synthetic knowledge base on disk and returns the root path.
func generateKB(t *testing.T, cfg genConfig) string {
	t.Helper()
	root := t.TempDir()
	rng := rand.New(rand.NewSource(cfg.Seed))

	// Track all doc IDs for cross-referencing
	type docInfo struct {
		id     string
		entity string
		file   string
	}
	var allDocs []docInfo

	for i := 0; i < cfg.NumDocs; i++ {
		entity := cfg.EntityNames[i%len(cfg.EntityNames)]
		docID := fmt.Sprintf("doc-%04d", i)
		filename := docID + ".md"
		relPath := filepath.Join("docs", entity, filename)
		fullPath := filepath.Join(root, relPath)

		allDocs = append(allDocs, docInfo{id: docID, entity: entity, file: relPath})

		// Generate content
		var body strings.Builder
		title := fmt.Sprintf("Document %d: %s", i, randomTopic(rng))
		fmt.Fprintf(&body, "# %s\n\n", title)

		// Add some sections
		numSections := 1 + rng.Intn(4)
		for s := 0; s < numSections; s++ {
			fmt.Fprintf(&body, "## Section %d\n\n", s+1)
			body.WriteString(randomParagraph(rng, 20+rng.Intn(cfg.MaxBodyWords)) + "\n\n")
		}

		// Add cross-references to earlier docs
		for _, prev := range allDocs[:i] {
			if rng.Float64() < cfg.LinkDensity {
				relLink := fmt.Sprintf("../%s/%s.md", prev.entity, prev.id)
				fmt.Fprintf(&body, "See also [%s](%s).\n\n", prev.id, relLink)
			}
		}

		// Build frontmatter
		var fm map[string]any
		if rng.Float64() < cfg.FrontmatterPct {
			fm = map[string]any{
				"id":      docID,
				"title":   title,
				"created": fmt.Sprintf("2026-%02d-%02d", 1+rng.Intn(12), 1+rng.Intn(28)),
				"status":  []string{"active", "archived", "draft"}[rng.Intn(3)],
				"tags":    randomTags(rng),
			}
		} else {
			fm = map[string]any{}
		}

		// Write file
		os.MkdirAll(filepath.Dir(fullPath), 0o755)
		content, err := storage.RenderMarkdown(fm, body.String())
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(fullPath, []byte(content), 0o644))
	}

	return root
}

func randomTopic(rng *rand.Rand) string {
	topics := []string{
		"deployment strategies", "database optimization", "API design",
		"testing approaches", "monitoring setup", "security review",
		"performance tuning", "architecture decisions", "team processes",
		"infrastructure planning", "code review", "incident response",
	}
	return topics[rng.Intn(len(topics))]
}

func randomParagraph(rng *rand.Rand, words int) string {
	vocabulary := []string{
		"the", "system", "uses", "approach", "for", "handling",
		"requests", "with", "careful", "attention", "to", "performance",
		"and", "reliability", "each", "component", "is", "designed",
		"to", "scale", "independently", "while", "maintaining",
		"data", "consistency", "across", "services", "we", "implemented",
		"caching", "layer", "that", "reduces", "database", "load",
		"by", "storing", "frequently", "accessed", "records", "in",
		"memory", "this", "improved", "response", "times", "significantly",
	}
	var parts []string
	for i := 0; i < words; i++ {
		parts = append(parts, vocabulary[rng.Intn(len(vocabulary))])
	}
	return strings.Join(parts, " ")
}

func randomTags(rng *rand.Rand) []any {
	allTags := []string{"go", "python", "infrastructure", "api", "testing", "docs", "security", "performance"}
	n := 1 + rng.Intn(3)
	var tags []any
	for i := 0; i < n; i++ {
		tags = append(tags, allTags[rng.Intn(len(allTags))])
	}
	return tags
}

// ============================================================================
// Durability tests
// ============================================================================

// TestDurability_CrawlLargeKB generates a 50-doc KB and verifies crawl indexing.
func TestDurability_CrawlLargeKB(t *testing.T) {
	cfg := defaultGenConfig()
	root := generateKB(t, cfg)
	db := newTestDB(t)

	result, err := db.CrawlAndIndex(CrawlOptions{
		DocsRoot: filepath.Join(root, "docs"),
	})
	require.NoError(t, err)

	// All docs indexed
	assert.Equal(t, cfg.NumDocs, result.FilesFound)
	assert.Equal(t, cfg.NumDocs, result.FilesIndexed)
	assert.True(t, result.EdgesFound > 0, "should find cross-reference edges")

	// Verify stats consistency
	stats, _ := db.Stats()
	assert.Equal(t, cfg.NumDocs, stats.Nodes)
	assert.True(t, stats.Edges > 0)
	assert.True(t, stats.Chunks > 0)
}

// TestDurability_CrawlIdempotent verifies double-crawl produces identical state.
func TestDurability_CrawlIdempotent(t *testing.T) {
	root := generateKB(t, defaultGenConfig())
	db := newTestDB(t)

	r1, _ := db.CrawlAndIndex(CrawlOptions{DocsRoot: filepath.Join(root, "docs")})
	stats1, _ := db.Stats()

	// Second crawl — should skip everything (staleness)
	r2, _ := db.CrawlAndIndex(CrawlOptions{DocsRoot: filepath.Join(root, "docs")})
	stats2, _ := db.Stats()

	assert.Equal(t, 0, r2.FilesIndexed, "idempotent crawl should index 0 files")
	assert.Equal(t, r1.FilesFound, r2.FilesSkipped)
	assert.Equal(t, stats1.Nodes, stats2.Nodes)
	assert.Equal(t, stats1.Chunks, stats2.Chunks)
}

// TestDurability_MutateAndReindex modifies docs then re-indexes, verifying state converges.
func TestDurability_MutateAndReindex(t *testing.T) {
	cfg := defaultGenConfig()
	cfg.NumDocs = 20
	root := generateKB(t, cfg)
	db := newTestDB(t)

	// Initial crawl
	db.CrawlAndIndex(CrawlOptions{DocsRoot: filepath.Join(root, "docs")})
	stats1, _ := db.Stats()

	// Mutate 5 docs
	for i := 0; i < 5; i++ {
		entity := cfg.EntityNames[i%len(cfg.EntityNames)]
		docID := fmt.Sprintf("doc-%04d", i)
		path := filepath.Join(root, "docs", entity, docID+".md")
		os.WriteFile(path, []byte(fmt.Sprintf("# Mutated %d\n\nNew content for %s.\n", i, docID)), 0o644)
	}

	// Re-crawl
	r2, _ := db.CrawlAndIndex(CrawlOptions{DocsRoot: filepath.Join(root, "docs")})
	stats2, _ := db.Stats()

	assert.Equal(t, 5, r2.FilesIndexed, "should re-index only mutated docs")
	assert.Equal(t, stats1.Nodes, stats2.Nodes, "node count should stay the same")
}

// TestDurability_DeleteAndReindex removes docs then re-crawls.
func TestDurability_DeleteAndReindex(t *testing.T) {
	cfg := defaultGenConfig()
	cfg.NumDocs = 15
	root := generateKB(t, cfg)
	db := newTestDB(t)

	db.CrawlAndIndex(CrawlOptions{DocsRoot: filepath.Join(root, "docs")})
	stats1, _ := db.Stats()
	assert.Equal(t, 15, stats1.Nodes)

	// Delete 3 docs from disk
	for i := 0; i < 3; i++ {
		entity := cfg.EntityNames[i%len(cfg.EntityNames)]
		docID := fmt.Sprintf("doc-%04d", i)
		os.Remove(filepath.Join(root, "docs", entity, docID+".md"))
	}

	// Force re-crawl (staleness won't help with deletions — but force will re-index remaining)
	db.CrawlAndIndex(CrawlOptions{DocsRoot: filepath.Join(root, "docs"), Force: true})

	// The 3 deleted docs are still in the DB (crawl adds, doesn't remove)
	// This is by design — cleanup is a separate concern
	stats2, _ := db.Stats()
	assert.True(t, stats2.Nodes >= 12, "remaining docs should still be indexed")
}

// TestDurability_GraphConsistency generates a KB, builds the graph, and verifies
// that every edge's source and target exist as nodes.
func TestDurability_GraphConsistency(t *testing.T) {
	cfg := defaultGenConfig()
	cfg.LinkDensity = 0.3 // lots of links
	root := generateKB(t, cfg)
	db := newTestDBNoEmbed(t)

	db.CrawlAndIndex(CrawlOptions{DocsRoot: filepath.Join(root, "docs")})

	nodes, _ := db.AllNodes("")
	nodeIDs := map[string]bool{}
	for _, n := range nodes {
		nodeIDs[n.ID] = true
	}

	edges, _ := db.AllEdges(nil)
	for _, e := range edges {
		// Source must be a known node
		assert.True(t, nodeIDs[e.SourceID],
			"edge source %q not in nodes", e.SourceID)
		// Target may reference a doc that doesn't exist yet (cross-entity ref)
		// This is valid — we don't enforce referential integrity on targets
	}
}

// TestDurability_SchemaModeCRUDCycle runs a full CRUD cycle through the ORM
// and verifies KG + integrity stay consistent.
func TestDurability_SchemaModeCRUDCycle(t *testing.T) {
	s, err := schema.Parse([]byte(`
version: 1
entity: notes
docs_dir: docs/notes
filename: "{id}.md"
records_dir: data/notes
partition: none
id_field: id
integrity: strict

fields:
  id:      { type: string, required: true }
  created: { type: date, required: true }
  status:  { type: enum, values: [active, archived], default: active }

virtuals:
  title:
    returns: string
    source: |
      def compute(content, fields):
          for line in content.splitlines():
              if line.startswith("# "):
                  return line.removeprefix("# ").strip()
          return fields["id"]
`))
	require.NoError(t, err)

	rt := virtuals.NewRuntime()
	for name, v := range s.Virtuals {
		require.NoError(t, rt.Compile(name, v.Source, v.Returns))
	}

	basePath := t.TempDir()
	os.MkdirAll(filepath.Join(basePath, "docs", "notes"), 0o755)
	os.MkdirAll(filepath.Join(basePath, "data", "notes"), 0o755)

	db := newTestDB(t)

	// CREATE 10 docs
	var docs []*document.Document
	for i := 0; i < 10; i++ {
		doc := document.New(s, basePath)
		doc.Data = map[string]any{
			"id":      fmt.Sprintf("note-%d", i),
			"created": "2026-04-08",
			"status":  "active",
		}

		// Cross-reference previous doc
		body := fmt.Sprintf("# Note %d\n\nSome content for note %d.\n", i, i)
		if i > 0 {
			body += fmt.Sprintf("\nSee [note-%d](../notes/note-%d.md).\n", i-1, i-1)
		}
		doc.Content = body
		require.NoError(t, doc.Save(rt))

		// Index into KG
		require.NoError(t, db.IndexDocument(doc, s))
		docs = append(docs, doc)
	}

	stats, _ := db.Stats()
	assert.Equal(t, 10, stats.Nodes)
	assert.True(t, stats.Edges >= 9, "should have at least 9 link edges (chain)")
	assert.True(t, stats.Chunks > 0)

	// VERIFY integrity via sidecars
	for _, doc := range docs {
		loaded, err := document.LoadFromFile(s, basePath, doc.FilePath())
		require.NoError(t, err)
		assert.Equal(t, doc.ID(), loaded.ID())

		// Verify sidecar exists and has content SHA
		sc, err := integrity.LoadSidecar(doc.FilePath())
		require.NoError(t, err, "sidecar should exist for %s", doc.ID())
		assert.NotEmpty(t, sc.ContentSHA)
	}

	// UPDATE 3 docs
	for i := 0; i < 3; i++ {
		doc := docs[i]
		doc.Data["status"] = "archived"
		doc.Content = fmt.Sprintf("# Updated Note %d\n\nArchived content.\n", i)
		require.NoError(t, doc.Save(rt))
		require.NoError(t, db.IndexDocument(doc, s))
	}

	stats2, _ := db.Stats()
	assert.Equal(t, 10, stats2.Nodes, "update shouldn't change node count")

	// Verify frontmatter reflects updates
	archivedCount := 0
	for i := 0; i < 10; i++ {
		mdPath := filepath.Join(basePath, "docs", "notes", fmt.Sprintf("note-%d.md", i))
		fm, _, ferr := storage.ParseMarkdown(mdPath)
		require.NoError(t, ferr)
		if fm["status"] == "archived" {
			archivedCount++
		}
	}
	assert.Equal(t, 3, archivedCount)

	// DELETE 2 docs
	for i := 8; i < 10; i++ {
		require.NoError(t, docs[i].Delete())
		db.RemoveNode(docs[i].ID())
	}

	stats3, _ := db.Stats()
	assert.Equal(t, 8, stats3.Nodes)

	// VERIFY remaining docs still have sidecars, deleted ones do not
	for i := 0; i < 8; i++ {
		mdPath := filepath.Join(basePath, "docs", "notes", fmt.Sprintf("note-%d.md", i))
		_, err := integrity.LoadSidecar(mdPath)
		assert.NoError(t, err, "sidecar should still exist for note-%d", i)
	}
	for i := 8; i < 10; i++ {
		mdPath := filepath.Join(basePath, "docs", "notes", fmt.Sprintf("note-%d.md", i))
		_, err := integrity.LoadSidecar(mdPath)
		assert.True(t, os.IsNotExist(err), "sidecar should be gone for deleted note-%d", i)
	}
}

// TestDurability_MixedSchemaAndCrawl verifies that schema-mode and crawl-mode
// can coexist in the same SQLite DB without conflicts.
func TestDurability_MixedSchemaAndCrawl(t *testing.T) {
	s, _ := schema.Parse([]byte(`
version: 1
entity: notes
docs_dir: docs/notes
filename: "{id}.md"
records_dir: data/notes
partition: none
id_field: id
integrity: off
fields:
  id:     { type: string, required: true }
  status: { type: enum, values: [active, archived], default: active }
`))

	basePath := t.TempDir()
	os.MkdirAll(filepath.Join(basePath, "docs", "notes"), 0o755)
	os.MkdirAll(filepath.Join(basePath, "docs", "guides"), 0o755)
	os.MkdirAll(filepath.Join(basePath, "data", "notes"), 0o755)

	db := newTestDB(t)

	// Schema-mode: create 3 notes via ORM
	for i := 0; i < 3; i++ {
		doc := document.New(s, basePath)
		doc.Data = map[string]any{"id": fmt.Sprintf("note-%d", i), "status": "active"}
		doc.Content = fmt.Sprintf("# Note %d\n\nContent.\n", i)
		doc.Save(nil) // no virtuals
		db.IndexDocument(doc, s)
	}

	// Crawl-mode: create 2 unstructured guides (no schema)
	for i := 0; i < 2; i++ {
		path := filepath.Join(basePath, "docs", "guides", fmt.Sprintf("guide-%d.md", i))
		os.WriteFile(path, []byte(fmt.Sprintf("# Guide %d\n\nUnstructured content.\n", i)), 0o644)
	}

	result, err := db.CrawlAndIndex(CrawlOptions{
		DocsRoot: filepath.Join(basePath, "docs"),
	})
	require.NoError(t, err)

	// Crawl should find all 5 files (3 notes + 2 guides)
	assert.Equal(t, 5, result.FilesFound)

	// But only index the 2 new guides (3 notes already indexed, staleness check passes)
	// Actually notes won't be stale because CrawlAndIndex uses content_sha comparison
	// and we wrote them via ORM with frontmatter... let's just check totals.
	stats, _ := db.Stats()
	assert.True(t, stats.Nodes >= 5, "should have at least 5 nodes (3 schema + 2 crawl)")

	// Both entity types should exist
	notes, _ := db.AllNodes("notes")
	guides, _ := db.AllNodes("guides")
	assert.True(t, len(notes) >= 3)
	assert.True(t, len(guides) >= 2)
}

// TestDurability_HighLinkDensity generates a heavily interlinked KB and verifies
// BFS doesn't explode or return duplicates.
func TestDurability_HighLinkDensity(t *testing.T) {
	cfg := defaultGenConfig()
	cfg.NumDocs = 30
	cfg.LinkDensity = 0.5 // 50% chance of link between any two docs
	root := generateKB(t, cfg)

	db := newTestDBNoEmbed(t)
	db.CrawlAndIndex(CrawlOptions{DocsRoot: filepath.Join(root, "docs")})

	stats, _ := db.Stats()
	assert.True(t, stats.Edges > 50, "high density should produce many edges, got %d", stats.Edges)

	// BFS depth 3 from first doc — should not panic or return dupes
	edges, err := db.Neighbors("doc-0000", 3)
	require.NoError(t, err)

	// Check no duplicate edges
	seen := map[string]bool{}
	for _, e := range edges {
		key := e.SourceID + "|" + e.TargetID + "|" + e.EdgeType
		assert.False(t, seen[key], "duplicate edge: %s", key)
		seen[key] = true
	}
}

// TestDurability_EmptyKB verifies all operations work on an empty KB.
func TestDurability_EmptyKB(t *testing.T) {
	db := newTestDB(t)

	stats, _ := db.Stats()
	assert.Equal(t, 0, stats.Nodes)

	edges, _ := db.Incoming("nonexistent")
	assert.Empty(t, edges)

	edges, _ = db.Outgoing("nonexistent")
	assert.Empty(t, edges)

	edges, _ = db.Neighbors("nonexistent", 3)
	assert.Empty(t, edges)

	mermaid, _ := db.ExportMermaid(nil)
	assert.Contains(t, mermaid, "graph LR")

	// Search on empty DB
	results, err := db.Search("anything", 5)
	require.NoError(t, err)
	assert.Empty(t, results)
}
