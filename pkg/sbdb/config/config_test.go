package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_Defaults(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(dir)
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(dir, "schemas"), cfg.SchemaDir)
	assert.Equal(t, dir, cfg.BasePath)
	assert.Equal(t, "", cfg.DefaultSchema)
	assert.Equal(t, "auto", cfg.Output.Format)
	assert.Equal(t, "env", cfg.Integrity.KeySource)
	assert.Equal(t, false, cfg.KnowledgeGraph.Enabled)
	assert.Equal(t, "data/.sbdb.db", cfg.KnowledgeGraph.DBPath)
	assert.Equal(t, "openai", cfg.KnowledgeGraph.Embeddings.Provider)
	assert.Equal(t, 1536, cfg.KnowledgeGraph.Embeddings.Dimension)
	assert.Equal(t, true, cfg.KnowledgeGraph.Graph.AutoIndex)
	assert.Equal(t, true, cfg.KnowledgeGraph.Graph.ExtractLinks)
	assert.Equal(t, "post-fix", cfg.Claude.Mode, "post-fix is the default Claude mode")
}

func TestLoad_ClaudeMode_FromTOML(t *testing.T) {
	dir := t.TempDir()
	toml := `[claude]
mode = "block"
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".sbdb.toml"), []byte(toml), 0o644))

	cfg, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, "block", cfg.Claude.Mode)
}

func TestLoad_FromTOML(t *testing.T) {
	dir := t.TempDir()
	toml := `
schema_dir = "./custom-schemas"
base_path = "."
default_schema = "recipes"

[output]
format = "json"

[integrity]
key_source = "file"

[knowledge_graph]
enabled = true
db_path = "custom.db"

[knowledge_graph.embeddings]
model = "text-embedding-3-large"
dimension = 3072

[knowledge_graph.graph]
auto_index = false
extract_links = false
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".sbdb.toml"), []byte(toml), 0o644))

	cfg, err := Load(dir)
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(dir, "custom-schemas"), cfg.SchemaDir)
	assert.Equal(t, "recipes", cfg.DefaultSchema)
	assert.Equal(t, "json", cfg.Output.Format)
	assert.Equal(t, "file", cfg.Integrity.KeySource)
	assert.Equal(t, true, cfg.KnowledgeGraph.Enabled)
	assert.Equal(t, "custom.db", cfg.KnowledgeGraph.DBPath)
	assert.Equal(t, "text-embedding-3-large", cfg.KnowledgeGraph.Embeddings.Model)
	assert.Equal(t, 3072, cfg.KnowledgeGraph.Embeddings.Dimension)
	assert.Equal(t, false, cfg.KnowledgeGraph.Graph.AutoIndex)
	assert.Equal(t, false, cfg.KnowledgeGraph.Graph.ExtractLinks)
}

func TestLoad_EnvOverrides(t *testing.T) {
	dir := t.TempDir()

	t.Setenv("SBDB_DEFAULT_SCHEMA", "env-schema")

	cfg, err := Load(dir)
	require.NoError(t, err)

	assert.Equal(t, "env-schema", cfg.DefaultSchema)
}

func TestResolveFormat_Auto(t *testing.T) {
	// When running in tests, stdout is not a TTY → should resolve to "json"
	assert.Equal(t, "json", ResolveFormat("auto"))
	assert.Equal(t, "json", ResolveFormat(""))
}

func TestResolveFormat_Explicit(t *testing.T) {
	assert.Equal(t, "json", ResolveFormat("json"))
	assert.Equal(t, "yaml", ResolveFormat("yaml"))
	assert.Equal(t, "table", ResolveFormat("table"))
}
