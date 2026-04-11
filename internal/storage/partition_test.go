package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordsPathForPartition_None(t *testing.T) {
	path, err := RecordsPathForPartition("/data/notes", "none", "", nil)
	require.NoError(t, err)
	assert.Equal(t, "/data/notes/records.yaml", path)
}

func TestRecordsPathForPartition_Monthly(t *testing.T) {
	record := map[string]any{"date": "2026-04-08"}
	path, err := RecordsPathForPartition("/data/meetings", "monthly", "date", record)
	require.NoError(t, err)
	assert.Equal(t, "/data/meetings/2026-04.yaml", path)
}

func TestRecordsPathForPartition_MissingDateField(t *testing.T) {
	_, err := RecordsPathForPartition("/data", "monthly", "date", map[string]any{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing date field")
}

func TestRecordsPathForPartition_UnknownMode(t *testing.T) {
	_, err := RecordsPathForPartition("/data", "weekly", "", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown partition")
}

func TestLoadAllPartitions_None(t *testing.T) {
	dir := t.TempDir()
	records := []map[string]any{{"id": "a"}, {"id": "b"}}
	require.NoError(t, SaveRecords(filepath.Join(dir, "records.yaml"), records))

	loaded, err := LoadAllPartitions(dir, "none")
	require.NoError(t, err)
	assert.Len(t, loaded, 2)
}

func TestLoadAllPartitions_Monthly(t *testing.T) {
	dir := t.TempDir()

	jan := []map[string]any{{"id": "jan-1"}, {"id": "jan-2"}}
	feb := []map[string]any{{"id": "feb-1"}}
	require.NoError(t, SaveRecords(filepath.Join(dir, "2026-01.yaml"), jan))
	require.NoError(t, SaveRecords(filepath.Join(dir, "2026-02.yaml"), feb))

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
