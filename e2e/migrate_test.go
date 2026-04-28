//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestE2E_Migrate_V1ToV2 builds a v1-shaped fixture (data/<entity>/records.yaml
// + an .md file but no sidecar) and verifies that `sbdb doctor migrate`
// produces a clean v2 layout: data/ removed, sidecars written, doctor check
// passes. Idempotent — second run is a no-op.
func TestE2E_Migrate_V1ToV2(t *testing.T) {
	dir := t.TempDir()

	// V1 fixture: schema has records_dir + .md exists + records.yaml exists.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "schemas"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs/note"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "data/note"), 0o755))
	schemaYAML := `version: 1
entity: note
docs_dir: docs/note
filename: "{id}.md"
records_dir: data/note
id_field: id
integrity: off
fields:
  id: { type: string, required: true }
  title: { type: string, required: true }
  status: { type: string }
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "schemas/note.yaml"), []byte(schemaYAML), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".sbdb.toml"), []byte(`schema_dir = "./schemas"
base_path = "."
default_schema = "note"
[output]
format = "json"
[integrity]
key_source = "env"
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs/note/alpha.md"),
		[]byte("---\nid: alpha\ntitle: Alpha\nstatus: active\n---\n# Alpha\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "data/note/records.yaml"), []byte(`- id: alpha
  title: Alpha
  status: active
  file: docs/note/alpha.md
`), 0o644))

	bin := ensureBinary(t)

	runHere := func(args ...string) (string, int) {
		cmd := exec.Command(bin, args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				return string(out), ee.ExitCode()
			}
			t.Fatalf("run sbdb %v: %v", args, err)
		}
		return string(out), 0
	}

	// Migrate
	out, code := runHere("doctor", "migrate")
	require.Equal(t, 0, code, "migrate failed: %s", out)

	assert.NoDirExists(t, filepath.Join(dir, "data"))
	assert.FileExists(t, filepath.Join(dir, "docs/note/alpha.yaml"))

	// Idempotent
	_, code = runHere("doctor", "migrate")
	require.Equal(t, 0, code, "second migrate should be no-op")

	// Doctor check is clean
	_, code = runHere("doctor", "check", "--all", "-s", "note")
	require.Equal(t, 0, code, "post-migrate doctor check should be clean")
}
