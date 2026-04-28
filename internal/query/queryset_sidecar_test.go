package query

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sergio-bershadsky/secondbrain-db/internal/schema"
)

const sidecarSchemaYAML = `
version: 1
entity: notes
docs_dir: docs/notes
filename: "{id}.md"
records_dir: data/notes
partition: none
id_field: id
integrity: off

fields:
  id:      { type: string, required: true }
  status:  { type: string }
  created: { type: date }
`

func TestQuerySet_Records_SidecarMode_WalksMD(t *testing.T) {
	t.Setenv("SBDB_USE_SIDECAR", "1")

	s, err := schema.Parse([]byte(sidecarSchemaYAML))
	require.NoError(t, err)

	basePath := t.TempDir()
	docs := filepath.Join(basePath, "docs/notes")
	require.NoError(t, os.MkdirAll(docs, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(docs, "alpha.md"),
		[]byte("---\nid: alpha\nstatus: active\ncreated: 2026-04-28\n---\n# A"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(docs, "beta.md"),
		[]byte("---\nid: beta\nstatus: archived\ncreated: 2026-04-29\n---\n# B"), 0o644))

	qs := NewQuerySet(s, basePath)
	got, err := qs.Records()
	require.NoError(t, err)

	ids := []string{}
	for _, r := range got {
		ids = append(ids, r["id"].(string))
	}
	assert.ElementsMatch(t, []string{"alpha", "beta"}, ids)
}
