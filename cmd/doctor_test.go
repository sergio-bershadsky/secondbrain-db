package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sergio-bershadsky/secondbrain-db/internal/integrity"
	schemapkg "github.com/sergio-bershadsky/secondbrain-db/internal/schema"
	"github.com/sergio-bershadsky/secondbrain-db/internal/storage"
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
		name := "sbdb"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		sbdbBinPath = filepath.Join(dir, name)
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

func setupV1KB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	schemaYAML := `version: 1
entity: notes
docs_dir: docs/notes
filename: "{id}.md"
records_dir: data/notes
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
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs/notes"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "data/notes"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "schemas/notes.yaml"), []byte(schemaYAML), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".sbdb.toml"), []byte(tomlContent), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs/notes/alpha.md"),
		[]byte("---\nid: alpha\ncreated: 2026-04-28\n---\n# Alpha"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "data/notes/records.yaml"), []byte(`- id: alpha
  created: 2026-04-28
  file: docs/notes/alpha.md
`), 0o644))
	return dir
}

func TestScopedDocPaths_AllFlag_WalksEverything(t *testing.T) {
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs/notes")
	require.NoError(t, os.MkdirAll(docs, 0o755))
	for _, id := range []string{"a", "b", "c"} {
		require.NoError(t, os.WriteFile(filepath.Join(docs, id+".md"),
			[]byte("---\nid: "+id+"\n---\n"), 0o644))
	}

	paths, err := scopedDocPaths(dir, docs, true)
	require.NoError(t, err)
	assert.Len(t, paths, 3)
}

func TestScopedDocPaths_NonGit_FallsBackToWalker(t *testing.T) {
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs/notes")
	require.NoError(t, os.MkdirAll(docs, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(docs, "a.md"),
		[]byte("---\nid: a\n---\n"), 0o644))

	paths, err := scopedDocPaths(dir, docs, false)
	require.NoError(t, err)
	assert.Len(t, paths, 1)
}

func TestCheckOneDoc_MissingSidecar(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "x.md")
	require.NoError(t, os.WriteFile(mdPath, []byte("---\nid: x\n---\n"), 0o644))

	s, err := schemapkg.Parse([]byte(`version: 1
entity: notes
docs_dir: .
filename: "{id}.md"
id_field: id
integrity: off
fields:
  id: { type: string, required: true }
`))
	require.NoError(t, err)

	report := checkOneDoc(s, dir, mdPath, nil)
	require.NotNil(t, report)
	assert.Equal(t, "missing-sidecar", report["drift"])
}

func TestCheckOneDoc_MissingMD(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "x.md")
	// Sidecar exists, .md does not
	sc := &integrity.Sidecar{Version: 1, Algo: "sha256", File: "x.md"}
	require.NoError(t, sc.Save(mdPath))

	s, _ := schemapkg.Parse([]byte(`version: 1
entity: notes
docs_dir: .
filename: "{id}.md"
id_field: id
integrity: off
fields:
  id: { type: string, required: true }
`))

	report := checkOneDoc(s, dir, mdPath, nil)
	require.NotNil(t, report)
	assert.Equal(t, "missing-md", report["drift"])
}

func TestCheckOneDoc_Clean_ReturnsNil(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "x.md")
	body := "---\nid: x\n---\nbody"
	require.NoError(t, os.WriteFile(mdPath, []byte(body), 0o644))

	s, _ := schemapkg.Parse([]byte(`version: 1
entity: notes
docs_dir: .
filename: "{id}.md"
id_field: id
integrity: off
fields:
  id: { type: string, required: true }
`))

	// Build a sidecar that matches the on-disk content.
	fm, body2, err := storage.ParseMarkdown(mdPath)
	require.NoError(t, err)
	rec := schemapkg.BuildRecordData(s, fm, nil)
	rec["file"] = "x.md"
	sc := &integrity.Sidecar{
		Version: 1, Algo: "sha256", File: "x.md",
		ContentSHA:     integrity.HashContent(body2),
		FrontmatterSHA: integrity.HashFrontmatter(fm),
		RecordSHA:      integrity.HashRecord(rec),
	}
	require.NoError(t, sc.Save(mdPath))

	report := checkOneDoc(s, dir, mdPath, nil)
	assert.Nil(t, report, "clean doc should produce no drift report")
}

func TestDoctorMigrate_ConvertsV1ToV2(t *testing.T) {
	dir := setupV1KB(t)
	out, code := runCLI(t, dir, nil, "doctor", "migrate")
	require.Equal(t, 0, code, "migrate failed: %s", out)

	assert.NoDirExists(t, filepath.Join(dir, "data"))
	assert.FileExists(t, filepath.Join(dir, "docs/notes/alpha.yaml"))

	// Idempotent
	_, code = runCLI(t, dir, nil, "doctor", "migrate")
	assert.Equal(t, 0, code, "second migrate should be no-op")

	// doctor check (v2 mode) is clean
	_, code = runCLI(t, dir, []string{"SBDB_USE_SIDECAR=1"},
		"doctor", "check", "--all", "-s", "notes")
	assert.Equal(t, 0, code, "post-migrate doctor check should be clean")
}
