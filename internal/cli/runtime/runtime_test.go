package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testProjectTOML = `schema_dir = "./schemas"
base_path = "."
[output]
format = "json"
`

const testSchema = `version: 1
entity: notes
docs_dir: docs/notes
filename: "{id}.md"
id_field: id
integrity: off
fields:
  id: { type: string, required: true }
`

// setupTempProject creates a tempdir with .sbdb.toml + schemas/notes.yaml
// and returns the absolute path. Tests use this as the project root.
func setupTempProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "schemas"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "schemas/notes.yaml"),
		[]byte(testSchema),
		0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".sbdb.toml"),
		[]byte(testProjectTOML),
		0o644,
	))
	return dir
}

func TestResolveConfig_UsesFlagBasePath(t *testing.T) {
	dir := setupTempProject(t)
	cfg, err := ResolveConfig(dir, "", "", "")
	require.NoError(t, err)
	assert.Equal(t, dir, cfg.BasePath)
	assert.Equal(t, "json", cfg.Output.Format)
}

func TestResolveConfig_FlagSchemaDirOverride(t *testing.T) {
	dir := setupTempProject(t)
	cfg, err := ResolveConfig(dir, "custom-schemas", "", "")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "custom-schemas"), cfg.SchemaDir)
}

func TestResolveConfig_FlagFormatOverride(t *testing.T) {
	dir := setupTempProject(t)
	cfg, err := ResolveConfig(dir, "", "", "table")
	require.NoError(t, err)
	assert.Equal(t, "table", cfg.Output.Format)
}

func TestResolveConfig_FlagSchemaOverride(t *testing.T) {
	dir := setupTempProject(t)
	cfg, err := ResolveConfig(dir, "", "decisions", "")
	require.NoError(t, err)
	assert.Equal(t, "decisions", cfg.DefaultSchema)
}

func TestOutputFormat_DefaultsToAuto(t *testing.T) {
	assert.Equal(t, "auto", OutputFormat(nil))
}

func TestOpenDB_ReturnsHandle(t *testing.T) {
	dir := setupTempProject(t)
	db, cfg, err := OpenDB(context.Background(), dir, "", "", "")
	require.NoError(t, err)
	defer db.Close()
	require.NotNil(t, db)
	require.NotNil(t, cfg)
	assert.Equal(t, dir, cfg.BasePath)
}

func TestPrintData_RoundTripsViaOutputFormat(t *testing.T) {
	// Smoke-test that PrintData can be invoked. Doesn't assert on stdout
	// (would require redirecting os.Stdout globally — fragile); just
	// confirms no panic and no error for a JSON-format payload.
	dir := setupTempProject(t)
	cfg, err := ResolveConfig(dir, "", "", "json")
	require.NoError(t, err)
	err = PrintData(cfg, map[string]any{"ok": true})
	assert.NoError(t, err)
}
