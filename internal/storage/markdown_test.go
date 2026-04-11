package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMarkdownBytes_WithFrontmatter(t *testing.T) {
	input := []byte(`---
title: Hello
tags:
  - a
  - b
---

# Hello World

Some content here.
`)
	fm, body, err := ParseMarkdownBytes(input)
	require.NoError(t, err)

	assert.Equal(t, "Hello", fm["title"])
	assert.Equal(t, []any{"a", "b"}, fm["tags"])
	assert.Equal(t, "# Hello World\n\nSome content here.\n", body)
}

func TestParseMarkdownBytes_NoFrontmatter(t *testing.T) {
	input := []byte("# Just markdown\n\nNo frontmatter here.\n")
	fm, body, err := ParseMarkdownBytes(input)
	require.NoError(t, err)

	assert.Empty(t, fm)
	assert.Equal(t, "# Just markdown\n\nNo frontmatter here.\n", body)
}

func TestParseMarkdownBytes_EmptyFrontmatter(t *testing.T) {
	input := []byte("---\n---\n\nBody here.\n")
	fm, body, err := ParseMarkdownBytes(input)
	require.NoError(t, err)
	assert.Empty(t, fm)
	assert.Contains(t, body, "Body here.")
}

func TestWriteMarkdown_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")

	fm := map[string]any{
		"title":  "Test",
		"status": "active",
	}
	body := "# Test\n\nSome content.\n"

	err := WriteMarkdown(path, fm, body)
	require.NoError(t, err)

	// Read back
	readFM, readBody, err := ParseMarkdown(path)
	require.NoError(t, err)

	assert.Equal(t, "Test", readFM["title"])
	assert.Equal(t, "active", readFM["status"])
	assert.Contains(t, readBody, "# Test")
	assert.Contains(t, readBody, "Some content.")
}

func TestWriteMarkdown_AtomicReplace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")

	// Write initial
	require.NoError(t, WriteMarkdown(path, map[string]any{"v": 1}, "body1"))
	// Overwrite
	require.NoError(t, WriteMarkdown(path, map[string]any{"v": 2}, "body2"))

	fm, body, err := ParseMarkdown(path)
	require.NoError(t, err)
	assert.Equal(t, 2, fm["v"])
	assert.Contains(t, body, "body2")

	// No temp files left
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		assert.False(t, filepath.Ext(e.Name()) == ".tmp", "temp file left behind: %s", e.Name())
	}
}

func TestWriteMarkdown_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "dir", "test.md")

	err := WriteMarkdown(path, map[string]any{"k": "v"}, "body")
	require.NoError(t, err)

	_, err = os.Stat(path)
	assert.NoError(t, err)
}
