package cmd

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/storage"
)

const inProcessSchema = `version: 1
entity: notes
docs_dir: docs/notes
filename: "{id}.md"
id_field: id
integrity: off
fields:
  id:      { type: string, required: true }
  status:  { type: string }
  created: { type: date, required: true }
`

const inProcessConfig = `schema_dir = "./schemas"
base_path = "."
default_schema = "notes"
[output]
format = "json"
[integrity]
key_source = "env"
`

// setupInProcessProject builds a tempdir with a schemas/notes.yaml + .sbdb.toml.
// The test then runs in this tempdir via t.Chdir.
func setupInProcessProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "schemas"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "schemas/notes.yaml"), []byte(inProcessSchema), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".sbdb.toml"), []byte(inProcessConfig), 0o644))
	t.Chdir(dir)
	resetFlagsForTest()
	return dir
}

// runInProcess wires cobra with stdout/stderr buffers and executes args.
// Returns combined stdout and the error from Execute.
func runInProcess(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(context.Background())
	out := stdout.String()
	if out == "" {
		out = stderr.String()
	}
	return out, err
}

// readMD parses a .md file's frontmatter and body for assertions using the
// public storage.ParseMarkdown API.
func readMD(t *testing.T, path string) (map[string]any, string, error) {
	t.Helper()
	fm, body, err := storage.ParseMarkdown(path)
	if err != nil {
		return nil, "", err
	}
	return fm, body, nil
}

func TestInProcess_CreateAndGet(t *testing.T) {
	dir := setupInProcessProject(t)

	_, err := runInProcess(t,
		"create", "-s", "notes",
		"--field", "id=hello",
		"--field", "created=2026-04-28",
		"--field", "status=active",
		"--content", "# Hello")
	require.NoError(t, err)

	mdPath := filepath.Join(dir, "docs/notes/hello.md")
	assert.FileExists(t, mdPath)
	assert.FileExists(t, filepath.Join(dir, "docs/notes/hello.yaml"))

	fm, body, err := readMD(t, mdPath)
	require.NoError(t, err)
	assert.Equal(t, "hello", fm["id"])
	assert.Equal(t, "active", fm["status"])
	assert.Contains(t, body, "Hello")
}

func TestInProcess_CreateFromStdinJSON(t *testing.T) {
	dir := setupInProcessProject(t)

	// Write the JSON to a temp file and use --input <file> to avoid needing
	// to redirect os.Stdin (create.go reads os.Stdin directly for "--input -").
	payload := `{"id":"json-id","created":"2026-04-28","status":"active","content":"# From JSON"}`
	inputFile := filepath.Join(dir, "payload.json")
	require.NoError(t, os.WriteFile(inputFile, []byte(payload), 0o644))

	_, err := runInProcess(t,
		"create", "-s", "notes", "--input", inputFile)
	require.NoError(t, err)

	fm, body, err := readMD(t, filepath.Join(dir, "docs/notes/json-id.md"))
	require.NoError(t, err)
	assert.Equal(t, "json-id", fm["id"])
	assert.Contains(t, body, "From JSON")
}

func TestInProcess_UpdateMutation(t *testing.T) {
	dir := setupInProcessProject(t)

	_, err := runInProcess(t,
		"create", "-s", "notes",
		"--field", "id=alpha", "--field", "created=2026-04-28", "--field", "status=active",
		"--content", "# A")
	require.NoError(t, err)

	_, err = runInProcess(t,
		"update", "-s", "notes",
		"--id", "alpha",
		"--field", "status=archived")
	require.NoError(t, err)

	fm, _, err := readMD(t, filepath.Join(dir, "docs/notes/alpha.md"))
	require.NoError(t, err)
	assert.Equal(t, "archived", fm["status"])
}

func TestInProcess_GetExistingDoc(t *testing.T) {
	dir := setupInProcessProject(t)

	_, err := runInProcess(t,
		"create", "-s", "notes",
		"--field", "id=existing", "--field", "created=2026-04-28", "--field", "status=active",
		"--content", "# Existing")
	require.NoError(t, err)

	// get a doc that exists: contract is that it succeeds (no error).
	// Output goes to os.Stdout (not the cobra buffer), so we assert on the
	// filesystem side effect rather than captured output.
	_, err = runInProcess(t,
		"get", "-s", "notes", "--id", "existing")
	require.NoError(t, err)

	// Confirm the file is still intact (get is read-only).
	assert.FileExists(t, filepath.Join(dir, "docs/notes/existing.md"))
}

func TestInProcess_DeleteRemovesPair(t *testing.T) {
	dir := setupInProcessProject(t)

	_, err := runInProcess(t,
		"create", "-s", "notes",
		"--field", "id=goner", "--field", "created=2026-04-28", "--field", "status=active",
		"--content", "# bye")
	require.NoError(t, err)

	mdPath := filepath.Join(dir, "docs/notes/goner.md")
	sidecarPath := filepath.Join(dir, "docs/notes/goner.yaml")
	require.FileExists(t, mdPath)
	require.FileExists(t, sidecarPath)

	_, err = runInProcess(t,
		"delete", "-s", "notes",
		"--id", "goner", "--yes")
	require.NoError(t, err)

	_, statErr := os.Stat(mdPath)
	assert.True(t, os.IsNotExist(statErr), "md should be gone")
	_, statErr = os.Stat(sidecarPath)
	assert.True(t, os.IsNotExist(statErr), "sidecar should be gone")
}

func TestInProcess_GetNoContent(t *testing.T) {
	dir := setupInProcessProject(t)

	_, err := runInProcess(t,
		"create", "-s", "notes",
		"--field", "id=nocontent", "--field", "created=2026-04-28",
		"--content", "# Body")
	require.NoError(t, err)

	resetFlagsForTest()
	_, err = runInProcess(t,
		"get", "-s", "notes", "--id", "nocontent", "--no-content")
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(dir, "docs/notes/nocontent.md"))
}

func TestInProcess_UpdateDryRun(t *testing.T) {
	dir := setupInProcessProject(t)

	_, err := runInProcess(t,
		"create", "-s", "notes",
		"--field", "id=drydoc", "--field", "created=2026-04-28", "--field", "status=active",
		"--content", "# Dry")
	require.NoError(t, err)

	resetFlagsForTest()
	_, err = runInProcess(t,
		"update", "-s", "notes",
		"--id", "drydoc",
		"--field", "status=pending",
		"--dry-run")
	require.NoError(t, err)

	// Dry-run: status must remain "active".
	fm, _, err := readMD(t, filepath.Join(dir, "docs/notes/drydoc.md"))
	require.NoError(t, err)
	assert.Equal(t, "active", fm["status"])
}

func TestInProcess_CreateDryRun(t *testing.T) {
	setupInProcessProject(t)
	resetFlagsForTest()

	_, err := runInProcess(t,
		"create", "-s", "notes",
		"--field", "id=dry", "--field", "created=2026-04-28",
		"--content", "# Dry",
		"--dry-run")
	require.NoError(t, err)
	// Dry-run: file must NOT be created.
	_, statErr := os.Stat("docs/notes/dry.md")
	assert.True(t, os.IsNotExist(statErr), "dry-run must not write file")
}

func TestInProcess_DeleteSoftArchives(t *testing.T) {
	dir := setupInProcessProject(t)

	_, err := runInProcess(t,
		"create", "-s", "notes",
		"--field", "id=softgone", "--field", "created=2026-04-28", "--field", "status=active",
		"--content", "# SG")
	require.NoError(t, err)

	_, err = runInProcess(t,
		"delete", "-s", "notes",
		"--id", "softgone", "--yes", "--soft")
	require.NoError(t, err)

	fm, _, err := readMD(t, filepath.Join(dir, "docs/notes/softgone.md"))
	require.NoError(t, err)
	assert.Equal(t, "archived", fm["status"])
}

func TestInProcess_UpdateWithAppendAndRemove(t *testing.T) {
	dir := setupInProcessProject(t)

	// Use a tags list field — update schema in place for this test.
	const schemaWithTags = `version: 1
entity: notes
docs_dir: docs/notes
filename: "{id}.md"
id_field: id
integrity: off
fields:
  id:      { type: string, required: true }
  status:  { type: string }
  created: { type: date, required: true }
  tags:    { type: list, items: { type: string } }
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "schemas/notes.yaml"), []byte(schemaWithTags), 0o644))

	resetFlagsForTest()
	_, err := runInProcess(t,
		"create", "-s", "notes",
		"--field", "id=tagged", "--field", "created=2026-04-28",
		"--field", `tags=["go","test"]`,
		"--content", "# Tagged")
	require.NoError(t, err)

	resetFlagsForTest()
	_, err = runInProcess(t,
		"update", "-s", "notes",
		"--id", "tagged",
		"--field", "tags+=extra")
	require.NoError(t, err)

	resetFlagsForTest()
	_, err = runInProcess(t,
		"update", "-s", "notes",
		"--id", "tagged",
		"--field", "tags-=go")
	require.NoError(t, err)

	resetFlagsForTest()
	_, err = runInProcess(t,
		"update", "-s", "notes",
		"--id", "tagged",
		"--field", "status~=")
	require.NoError(t, err)

	fm, _, err := readMD(t, filepath.Join(dir, "docs/notes/tagged.md"))
	require.NoError(t, err)
	_, hasStatus := fm["status"]
	assert.False(t, hasStatus, "~= operator should have deleted status key")
}

func TestInProcess_UpdateWithInputFile(t *testing.T) {
	dir := setupInProcessProject(t)

	_, err := runInProcess(t,
		"create", "-s", "notes",
		"--field", "id=updinput", "--field", "created=2026-04-28", "--field", "status=draft",
		"--content", "# Draft")
	require.NoError(t, err)

	// Write a JSON input for the update.
	payload := `{"status":"published"}`
	inputFile := filepath.Join(dir, "update.json")
	require.NoError(t, os.WriteFile(inputFile, []byte(payload), 0o644))

	resetFlagsForTest()
	_, err = runInProcess(t,
		"update", "-s", "notes",
		"--id", "updinput",
		"--input", inputFile)
	require.NoError(t, err)

	fm, _, err := readMD(t, filepath.Join(dir, "docs/notes/updinput.md"))
	require.NoError(t, err)
	assert.Equal(t, "published", fm["status"])
}

func TestInProcess_UpdateWithContentFile(t *testing.T) {
	dir := setupInProcessProject(t)

	_, err := runInProcess(t,
		"create", "-s", "notes",
		"--field", "id=updcontent", "--field", "created=2026-04-28",
		"--content", "# Old Body")
	require.NoError(t, err)

	newBodyFile := filepath.Join(dir, "newbody.md")
	require.NoError(t, os.WriteFile(newBodyFile, []byte("# New Body\n\nReplaced."), 0o644))

	resetFlagsForTest()
	_, err = runInProcess(t,
		"update", "-s", "notes",
		"--id", "updcontent",
		"--content-file", newBodyFile)
	require.NoError(t, err)

	_, body, err := readMD(t, filepath.Join(dir, "docs/notes/updcontent.md"))
	require.NoError(t, err)
	assert.Contains(t, body, "New Body")
}

func TestInProcess_CreateWithContentFile(t *testing.T) {
	dir := setupInProcessProject(t)

	contentFile := filepath.Join(dir, "body.md")
	require.NoError(t, os.WriteFile(contentFile, []byte("# From File\n\nContent from disk."), 0o644))

	resetFlagsForTest()
	_, err := runInProcess(t,
		"create", "-s", "notes",
		"--field", "id=fromfile", "--field", "created=2026-04-28",
		"--content-file", contentFile)
	require.NoError(t, err)

	_, body, err := readMD(t, filepath.Join(dir, "docs/notes/fromfile.md"))
	require.NoError(t, err)
	assert.Contains(t, body, "From File")
}

// moduleRoot returns the absolute path to the repo module root, found by
// locating the go.mod file relative to this test file.
func moduleRoot(t *testing.T) string {
	t.Helper()
	// Walk up from the current working directory to find go.mod.
	// The cmd package is at <root>/cmd, so the root is one level up.
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find go.mod")
		}
		dir = parent
	}
}

func TestInProcess_GetUnknownIDReturnsError(t *testing.T) {
	// get calls os.Exit(2) when the id is not found; we use a subprocess to
	// avoid terminating the test process. We build the binary from the module
	// root so go.mod is always found regardless of t.Chdir state.
	root := moduleRoot(t)
	projDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projDir, "schemas"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projDir, "schemas/notes.yaml"), []byte(inProcessSchema), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projDir, ".sbdb.toml"), []byte(inProcessConfig), 0o644))

	binName := "sbdb-test"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(t.TempDir(), binName)
	build := exec.Command("go", "build", "-o", binPath, ".")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build sbdb: %v\n%s", err, out)
	}

	run := exec.Command(binPath, "get", "-s", "notes", "--id", "noexist")
	run.Dir = projDir
	err := run.Run()
	if exitErr, ok := err.(*exec.ExitError); ok {
		assert.NotEqual(t, 0, exitErr.ExitCode(), "get on missing id should exit non-zero")
		return
	}
	// If we got here with no error, the command unexpectedly succeeded.
	assert.Fail(t, "expected non-zero exit for missing id, got success")
}
