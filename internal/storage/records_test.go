package storage

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadRecords_NonExistent(t *testing.T) {
	records, err := LoadRecords("/nonexistent/path/records.yaml")
	require.NoError(t, err)
	assert.Empty(t, records)
}

func TestSaveAndLoadRecords_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "records.yaml")

	records := []map[string]any{
		{"id": "note-1", "status": "active", "title": "First"},
		{"id": "note-2", "status": "archived", "title": "Second"},
	}

	require.NoError(t, SaveRecords(path, records))

	loaded, err := LoadRecords(path)
	require.NoError(t, err)
	require.Len(t, loaded, 2)

	assert.Equal(t, "note-1", loaded[0]["id"])
	assert.Equal(t, "active", loaded[0]["status"])
	assert.Equal(t, "note-2", loaded[1]["id"])
}

func TestUpsertRecord_Insert(t *testing.T) {
	records := []map[string]any{
		{"id": "a", "v": 1},
	}
	result := UpsertRecord(records, map[string]any{"id": "b", "v": 2}, "id")
	assert.Len(t, result, 2)
	assert.Equal(t, "b", result[1]["id"])
}

func TestUpsertRecord_Update(t *testing.T) {
	records := []map[string]any{
		{"id": "a", "v": 1},
		{"id": "b", "v": 2},
	}
	result := UpsertRecord(records, map[string]any{"id": "a", "v": 99}, "id")
	assert.Len(t, result, 2)
	assert.Equal(t, 99, result[0]["v"])
}

func TestRemoveRecord(t *testing.T) {
	records := []map[string]any{
		{"id": "a"}, {"id": "b"}, {"id": "c"},
	}

	result, removed := RemoveRecord(records, "id", "b")
	assert.True(t, removed)
	assert.Len(t, result, 2)
	assert.Equal(t, "a", result[0]["id"])
	assert.Equal(t, "c", result[1]["id"])

	result, removed = RemoveRecord(result, "id", "nonexistent")
	assert.False(t, removed)
	assert.Len(t, result, 2)
}
