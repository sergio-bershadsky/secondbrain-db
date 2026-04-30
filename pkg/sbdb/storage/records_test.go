package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadRecords_NonExistent(t *testing.T) {
	records, err := LoadRecords("/nonexistent/path/records.yaml")
	require.NoError(t, err)
	assert.Empty(t, records)
}
