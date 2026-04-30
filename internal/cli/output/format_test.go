package output

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFprintData_JSON(t *testing.T) {
	var buf bytes.Buffer
	data := map[string]any{"key": "value", "count": 42}

	err := FprintData(&buf, "json", data)
	require.NoError(t, err)

	var resp Response
	require.NoError(t, json.Unmarshal(buf.Bytes(), &resp))
	assert.Equal(t, 1, resp.Version)
	m, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "value", m["key"])
}

func TestFprintData_YAML(t *testing.T) {
	var buf bytes.Buffer
	data := map[string]any{"name": "test"}

	err := FprintData(&buf, "yaml", data)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "name: test")
}

func TestFprintData_Table_Records(t *testing.T) {
	var buf bytes.Buffer
	records := []map[string]any{
		{"id": "a", "status": "active"},
		{"id": "b", "status": "archived"},
	}

	err := FprintData(&buf, "table", records)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "id")
	assert.Contains(t, out, "status")
	assert.Contains(t, out, "active")
	assert.Contains(t, out, "archived")
}

func TestFprintData_Table_SingleRecord(t *testing.T) {
	var buf bytes.Buffer
	data := map[string]any{"id": "test", "status": "active"}

	err := FprintData(&buf, "table", data)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "id:")
	assert.Contains(t, out, "test")
}

func TestFprintData_Table_Empty(t *testing.T) {
	var buf bytes.Buffer
	err := FprintData(&buf, "table", []map[string]any{})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "no records")
}

func TestFprintData_DefaultsToJSON(t *testing.T) {
	var buf bytes.Buffer
	err := FprintData(&buf, "unknown", map[string]any{"k": "v"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), `"version"`)
}
