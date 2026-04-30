package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// writeRecords is a test helper that serialises records to a YAML file.
func writeRecords(t *testing.T, path string, records []map[string]any) {
	t.Helper()
	data, err := yaml.Marshal(records)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

func TestLoadAllPartitions_None(t *testing.T) {
	dir := t.TempDir()
	records := []map[string]any{{"id": "a"}, {"id": "b"}}
	writeRecords(t, filepath.Join(dir, "records.yaml"), records)

	loaded, err := LoadAllPartitions(dir, "none")
	require.NoError(t, err)
	assert.Len(t, loaded, 2)
}

func TestLoadAllPartitions_Monthly(t *testing.T) {
	dir := t.TempDir()

	jan := []map[string]any{{"id": "jan-1"}, {"id": "jan-2"}}
	feb := []map[string]any{{"id": "feb-1"}}
	writeRecords(t, filepath.Join(dir, "2026-01.yaml"), jan)
	writeRecords(t, filepath.Join(dir, "2026-02.yaml"), feb)

	// Also a non-partition file that should be ignored
	os.WriteFile(filepath.Join(dir, "notes.yaml"), []byte("ignored"), 0o644)

	loaded, err := LoadAllPartitions(dir, "monthly")
	require.NoError(t, err)
	assert.Len(t, loaded, 3) // 2 jan + 1 feb
}

func TestLoadAllPartitions_EmptyDir(t *testing.T) {
	loaded, err := LoadAllPartitions("/nonexistent", "monthly")
	require.NoError(t, err)
	assert.Empty(t, loaded)
}
