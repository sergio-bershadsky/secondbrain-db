package schema

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDetectDialect_Legacy(t *testing.T) {
	d, err := DetectDialect([]byte(`
version: 1
entity: notes
docs_dir: docs/notes
filename: "{id}.md"
fields:
  id: { type: string, required: true }
`))
	require.NoError(t, err)
	require.Equal(t, DialectLegacy, d)
}

func TestDetectDialect_New(t *testing.T) {
	d, err := DetectDialect([]byte(`
$schema: https://json-schema.org/draft/2020-12/schema
x-entity: notes
x-storage: { docs_dir: docs/notes, filename: "{id}.md" }
x-id: id
type: object
properties:
  id: { type: string }
`))
	require.NoError(t, err)
	require.Equal(t, DialectNew, d)
}

func TestDetectDialect_NewByXKey(t *testing.T) {
	d, err := DetectDialect([]byte(`
x-entity: notes
type: object
properties:
  id: { type: string }
`))
	require.NoError(t, err)
	require.Equal(t, DialectNew, d)
}

func TestDetectDialect_AmbiguousIsRejected(t *testing.T) {
	_, err := DetectDialect([]byte(`
$schema: https://json-schema.org/draft/2020-12/schema
entity: notes
fields:
  id: { type: string, required: true }
properties:
  id: { type: string }
`))
	require.Error(t, err)
}
