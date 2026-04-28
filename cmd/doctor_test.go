package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	sbdbBinOnce sync.Once
	sbdbBinPath string
)

func sbdbBin(t *testing.T) string {
	t.Helper()
	sbdbBinOnce.Do(func() {
		dir, err := os.MkdirTemp("", "sbdb-test-bin-*")
		require.NoError(t, err)
		sbdbBinPath = filepath.Join(dir, "sbdb")
		cmd := exec.Command("go", "build", "-o", sbdbBinPath, "github.com/sergio-bershadsky/secondbrain-db")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "build sbdb: %s", out)
	})
	return sbdbBinPath
}

func runCLI(t *testing.T, dir string, env []string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(sbdbBin(t), args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return string(out), exitErr.ExitCode()
		}
		t.Fatalf("run sbdb %v: %v", args, err)
	}
	return string(out), 0
}

func setupV2KB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	schemaYAML := `version: 1
entity: notes
docs_dir: docs/notes
filename: "{id}.md"
id_field: id
integrity: off
fields:
  id: { type: string, required: true }
  created: { type: date, required: true }
`
	tomlContent := `schema_dir = "./schemas"
base_path = "."
[output]
format = "json"
[integrity]
key_source = "env"
`
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "schemas"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "schemas/notes.yaml"), []byte(schemaYAML), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".sbdb.toml"), []byte(tomlContent), 0o644))

	// Create one valid doc via the v2 path so its sidecar is consistent.
	out, code := runCLI(t, dir, []string{"SBDB_USE_SIDECAR=1"},
		"create", "-s", "notes",
		"--field", "id=alpha",
		"--field", "created=2026-04-28",
		"--content", "# Alpha")
	require.Equal(t, 0, code, "create failed: %s", out)
	return dir
}

func TestDoctorCheck_SidecarMode_CleanWithAllFlag(t *testing.T) {
	dir := setupV2KB(t)
	out, code := runCLI(t, dir, []string{"SBDB_USE_SIDECAR=1"},
		"doctor", "check", "--all", "-s", "notes")
	require.Equal(t, 0, code, "stdout: %s", out)
}

func TestDoctorCheck_SidecarMode_DetectsContentDrift(t *testing.T) {
	dir := setupV2KB(t)
	mdPath := filepath.Join(dir, "docs/notes/alpha.md")
	require.NoError(t, os.WriteFile(mdPath,
		[]byte("---\nid: alpha\ncreated: 2026-04-28\n---\nTAMPERED"), 0o644))

	out, code := runCLI(t, dir, []string{"SBDB_USE_SIDECAR=1"},
		"doctor", "check", "--all", "-s", "notes")
	require.NotEqual(t, 0, code, "expected drift exit; out: %s", out)
	assert.Contains(t, out, "content_sha mismatch")
	_ = fmt.Sprintf // keep import
}

func TestDoctorFix_SidecarMode_RewritesSidecar(t *testing.T) {
	dir := setupV2KB(t)
	mdPath := filepath.Join(dir, "docs/notes/alpha.md")
	require.NoError(t, os.WriteFile(mdPath,
		[]byte("---\nid: alpha\ncreated: 2026-04-28\n---\nNEW BODY"), 0o644))

	out, code := runCLI(t, dir, []string{"SBDB_USE_SIDECAR=1"},
		"doctor", "fix", "--recompute", "--all", "-s", "notes")
	require.Equal(t, 0, code, "fix failed: %s", out)

	_, code = runCLI(t, dir, []string{"SBDB_USE_SIDECAR=1"},
		"doctor", "check", "--all", "-s", "notes")
	require.Equal(t, 0, code, "check after fix should be clean")
}

func TestDoctorSign_SidecarMode_RequiresKey(t *testing.T) {
	dir := setupV2KB(t)
	out, code := runCLI(t, dir, []string{"SBDB_USE_SIDECAR=1"},
		"doctor", "sign", "--force", "--all", "-s", "notes")
	require.NotEqual(t, 0, code, "sign without key should fail")
	assert.Contains(t, out, "key")
}
