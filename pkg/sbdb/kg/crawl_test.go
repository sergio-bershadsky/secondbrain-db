package kg

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupCrawlDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	docs := filepath.Join(root, "docs")

	// Schema-backed entity with frontmatter
	mkFile(t, filepath.Join(docs, "notes", "hello.md"), `---
title: Hello World
status: active
---

# Hello World

This is a [linked note](../guides/setup.md).
`)

	// Unstructured guide (no frontmatter)
	mkFile(t, filepath.Join(docs, "guides", "setup.md"), `# Setup Guide

Follow these steps to get started.

See also [architecture](../architecture/overview.md).
`)

	// Index page with Vue component (should still be indexed)
	mkFile(t, filepath.Join(docs, "notes", "index.md"), `# Notes

<NotesTable />

Browse all notes below.
`)

	// Architecture deep page
	mkFile(t, filepath.Join(docs, "architecture", "overview.md"), `# Architecture Overview

The system uses a microservices approach.

Related: [ADR-0001](../decisions/ADR-0001.md)
`)

	// ADR with frontmatter
	mkFile(t, filepath.Join(docs, "decisions", "ADR-0001.md"), `---
id: ADR-0001
status: accepted
author: Jane
---

# ADR-0001: Use Microservices

## Decision

We will use microservices.
`)

	// Excluded directory
	mkFile(t, filepath.Join(docs, ".vitepress", "config.ts"), `// this is not markdown`)
	mkFile(t, filepath.Join(docs, "node_modules", "pkg", "readme.md"), `# Should be excluded`)

	return docs
}

func mkFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestCrawlAndIndex_Basic(t *testing.T) {
	docs := setupCrawlDir(t)
	db := newTestDB(t)

	result, err := db.CrawlAndIndex(CrawlOptions{
		DocsRoot: docs,
	})
	require.NoError(t, err)

	assert.Equal(t, 5, result.FilesFound, "should find 5 .md files (excluding .vitepress and node_modules)")
	assert.Equal(t, 5, result.FilesIndexed)
	assert.True(t, result.EdgesFound >= 3, "should extract at least 3 markdown link edges")
}

func TestCrawlAndIndex_ExcludesDirs(t *testing.T) {
	docs := setupCrawlDir(t)
	db := newTestDBNoEmbed(t)

	result, err := db.CrawlAndIndex(CrawlOptions{DocsRoot: docs})
	require.NoError(t, err)

	// node_modules and .vitepress should be excluded
	nodes, _ := db.AllNodes("")
	for _, n := range nodes {
		assert.NotContains(t, n.File, "node_modules")
		assert.NotContains(t, n.File, ".vitepress")
	}
	assert.Equal(t, 5, result.FilesFound)
}

func TestCrawlAndIndex_DerivesEntity(t *testing.T) {
	docs := setupCrawlDir(t)
	db := newTestDBNoEmbed(t)

	db.CrawlAndIndex(CrawlOptions{DocsRoot: docs})

	nodes, _ := db.AllNodes("")
	entityMap := map[string]string{}
	for _, n := range nodes {
		entityMap[n.ID] = n.Entity
	}

	assert.Equal(t, "notes", entityMap["hello"])
	assert.Equal(t, "guides", entityMap["setup"])
	assert.Equal(t, "architecture", entityMap["overview"])
	assert.Equal(t, "decisions", entityMap["ADR-0001"])
}

func TestCrawlAndIndex_DerivesTitle(t *testing.T) {
	docs := setupCrawlDir(t)
	db := newTestDBNoEmbed(t)

	db.CrawlAndIndex(CrawlOptions{DocsRoot: docs})

	nodes, _ := db.AllNodes("")
	titleMap := map[string]string{}
	for _, n := range nodes {
		titleMap[n.ID] = n.Title
	}

	// Frontmatter title takes precedence
	assert.Equal(t, "Hello World", titleMap["hello"])
	// No frontmatter → first heading
	assert.Equal(t, "Setup Guide", titleMap["setup"])
	assert.Equal(t, "Architecture Overview", titleMap["overview"])
}

func TestCrawlAndIndex_ExtractsLinks(t *testing.T) {
	docs := setupCrawlDir(t)
	db := newTestDBNoEmbed(t)

	db.CrawlAndIndex(CrawlOptions{DocsRoot: docs})

	// hello.md links to setup.md
	outEdges, _ := db.Outgoing("hello")
	hasSetupLink := false
	for _, e := range outEdges {
		if e.TargetID == "setup" && e.EdgeType == "link" {
			hasSetupLink = true
		}
	}
	assert.True(t, hasSetupLink, "hello should link to setup")

	// setup.md links to overview.md
	outEdges, _ = db.Outgoing("setup")
	hasOverviewLink := false
	for _, e := range outEdges {
		if e.TargetID == "overview" && e.EdgeType == "link" {
			hasOverviewLink = true
		}
	}
	assert.True(t, hasOverviewLink, "setup should link to overview")
}

func TestCrawlAndIndex_Staleness(t *testing.T) {
	docs := setupCrawlDir(t)
	db := newTestDBNoEmbed(t)

	// First crawl
	r1, _ := db.CrawlAndIndex(CrawlOptions{DocsRoot: docs})
	assert.Equal(t, 5, r1.FilesIndexed)

	// Second crawl (no changes) — should skip all
	r2, _ := db.CrawlAndIndex(CrawlOptions{DocsRoot: docs})
	assert.Equal(t, 0, r2.FilesIndexed)
	assert.Equal(t, 5, r2.FilesSkipped)

	// Modify one file
	mkFile(t, filepath.Join(docs, "notes", "hello.md"), "# Updated Hello\n\nNew content.")

	// Third crawl — should re-index only the changed file
	r3, _ := db.CrawlAndIndex(CrawlOptions{DocsRoot: docs})
	assert.Equal(t, 1, r3.FilesIndexed)
	assert.Equal(t, 4, r3.FilesSkipped)
}

func TestCrawlAndIndex_Force(t *testing.T) {
	docs := setupCrawlDir(t)
	db := newTestDBNoEmbed(t)

	db.CrawlAndIndex(CrawlOptions{DocsRoot: docs})

	// Force re-index
	result, _ := db.CrawlAndIndex(CrawlOptions{DocsRoot: docs, Force: true})
	assert.Equal(t, 5, result.FilesIndexed, "force should re-index all")
}

func TestCrawlAndIndex_WithEmbeddings(t *testing.T) {
	docs := setupCrawlDir(t)
	db := newTestDB(t) // has stub embedder

	result, err := db.CrawlAndIndex(CrawlOptions{DocsRoot: docs})
	require.NoError(t, err)
	assert.Equal(t, 5, result.FilesIndexed)

	// Should have chunks
	stats, _ := db.Stats()
	assert.True(t, stats.Chunks > 0, "should have embedded chunks")
}

func TestDeriveEntity(t *testing.T) {
	assert.Equal(t, "notes", deriveEntity("notes/hello.md"))
	assert.Equal(t, "guides", deriveEntity("guides/setup/docker.md"))
	assert.Equal(t, "root", deriveEntity("index.md"))
	assert.Equal(t, "decisions", deriveEntity("decisions/ADR-0001.md"))
}

func TestDeriveTitle(t *testing.T) {
	// Frontmatter title wins
	assert.Equal(t, "FM Title", deriveTitle(map[string]any{"title": "FM Title"}, "# Heading", "fallback"))

	// Then heading
	assert.Equal(t, "My Heading", deriveTitle(map[string]any{}, "# My Heading\n\nBody.", "fallback"))

	// Then filename
	assert.Equal(t, "fallback", deriveTitle(map[string]any{}, "No heading here.", "fallback"))
}
