//go:build e2e

// Package e2e exercises the built sbdb binary against a temporary project,
// asserting CLI behavior end-to-end. Activated by `go test -tags=e2e`.
//
// The build tag keeps these tests out of the default `go test ./...` run
// because they need a freshly compiled binary and create on-disk fixtures.
// Run via `make test-e2e` or directly:
//
//	go test -tags=e2e -race ./e2e/...
package e2e

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// noteSchema is a minimal schema fixture used across tests.
const noteSchema = `version: 1
entity: note
docs_dir: docs/notes
filename: "{{slug}}.md"
records_dir: data/note
id_field: slug
fields:
  title: { type: string, required: true }
  slug:  { type: string, required: true }
`

// configMinimal is the .sbdb.toml fixture used when a test needs the
// events subsystem (i.e., a git repo + projection). Events are no longer
// configurable — they are projected from git history on demand.
const configMinimal = `default_schema = "note"
`

// project bundles a temp project root + the path to the built sbdb binary.
type project struct {
	root   string
	binary string
}

// newProject returns a fresh tempdir initialised with the note schema and
// .sbdb.toml. The binary is compiled once per test run via build().
// newProject creates a fresh tempdir initialized with the note schema and a
// minimal .sbdb.toml. The legacy `withEvents` parameter is preserved as a
// no-op so existing tests don't need rewriting; events are now always
// available via `sbdb events emit` and don't need configuration.
func newProject(t *testing.T, _ bool) *project {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "schemas"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "data", "note"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs", "notes"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "schemas", "note.yaml"),
		[]byte(noteSchema),
		0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "data", "note", "records.yaml"),
		[]byte("[]\n"),
		0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, ".sbdb.toml"),
		[]byte(configMinimal),
		0o644,
	))
	return &project{root: root, binary: ensureBinary(t)}
}

// run invokes the sbdb binary with the given args from the project root.
// Returns stdout, stderr, and exit code (no error for non-zero exit; just
// the code).
func (p *project) run(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	args = append([]string{"-b", p.root}, args...)
	cmd := exec.Command(p.binary, args...)
	cmd.Dir = p.root
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exit := 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			exit = ee.ExitCode()
		} else {
			t.Fatalf("running %v: %v", args, err)
		}
	}
	return stdout.String(), stderr.String(), exit
}

// runOK asserts exit 0 and returns stdout.
func (p *project) runOK(t *testing.T, args ...string) string {
	t.Helper()
	stdout, stderr, code := p.run(t, args...)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d for args=%v\nstdout: %s\nstderr: %s",
			code, args, stdout, stderr)
	}
	return stdout
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestCRUD_RoundTrip exercises create -> get -> update -> delete on a note.
func TestCRUD_RoundTrip(t *testing.T) {
	p := newProject(t, false)

	out := p.runOK(t,
		"create",
		"--field", "title=Hello",
		"--field", "slug=hello",
		"--content", "First note.",
		"--format", "json",
	)
	require.Contains(t, out, `"slug": "hello"`, "create output: %s", out)

	out = p.runOK(t, "get", "--id", "hello", "--format", "json")
	require.Contains(t, out, `"title": "Hello"`)

	out = p.runOK(t, "update", "--id", "hello", "--field", "title=Goodbye", "--format", "json")
	require.Contains(t, out, `"title": "Goodbye"`)

	_ = p.runOK(t, "delete", "--id", "hello", "--yes", "--format", "json")

	// Get after delete should fail (exit 2 = NOT_FOUND).
	_, _, code := p.run(t, "get", "--id", "hello", "--format", "json")
	require.Equal(t, 2, code, "expected NOT_FOUND exit code 2 after delete")
}

// TestEventsEmit_FromGitHistory asserts the `sbdb events emit` projection
// produces correct JSONL events for a sequence of git commits that touch
// files under a known schema's docs_dir. Replaces the older on-disk
// "events emitted on CRUD" check — events are no longer stored anywhere;
// the projection is computed from the git log on demand.
func TestEventsEmit_FromGitHistory(t *testing.T) {
	p := newProject(t, false)

	// Initialize the project as a git repo and make commits with both
	// schema-relevant and irrelevant files.
	gitInitInProject(t, p.root)
	gitCommitFile(t, p.root, "docs/notes/foo.md", "# Foo\n", "add foo")
	gitCommitFile(t, p.root, "docs/notes/foo.md", "# Foo updated\n", "update foo")
	gitCommitFile(t, p.root, "README.md", "readme\n", "out-of-scope file")
	gitCommitDelete(t, p.root, "docs/notes/foo.md", "delete foo")

	// Project events from the very first commit (the init one) onwards.
	out := p.runOK(t, "events", "emit", "HEAD~4", "HEAD")

	lines := splitLines(out)
	// Three in-scope events: created, updated, deleted. README should be skipped.
	require.Len(t, lines, 3, "got: %s", out)

	types := make([]string, 0, 3)
	for _, line := range lines {
		var ev struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}
		require.NoError(t, json.Unmarshal([]byte(line), &ev), line)
		require.Equal(t, "docs/notes/foo.md", ev.ID)
		types = append(types, ev.Type)
	}
	require.Equal(t, []string{"note.created", "note.updated", "note.deleted"}, types)
}

// gitInitInProject initializes a git repo in the project root with a
// baseline commit that includes the schemas + .sbdb.toml fixtures so HEAD~N
// references work for events emit.
func gitInitInProject(t *testing.T, dir string) {
	t.Helper()
	mustRunGit(t, dir, "init", "-q", "-b", "main")
	mustRunGit(t, dir, "config", "user.email", "test@example.com")
	mustRunGit(t, dir, "config", "user.name", "Test")
	mustRunGit(t, dir, "config", "commit.gpgsign", "false")
	mustRunGit(t, dir, "add", ".")
	mustRunGit(t, dir, "commit", "-q", "-m", "init")
}

func gitCommitFile(t *testing.T, dir, relPath, content, msg string) {
	t.Helper()
	full := filepath.Join(dir, relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	mustRunGit(t, dir, "add", relPath)
	mustRunGit(t, dir, "commit", "-q", "-m", msg)
}

func gitCommitDelete(t *testing.T, dir, relPath, msg string) {
	t.Helper()
	mustRunGit(t, dir, "rm", "-q", relPath)
	mustRunGit(t, dir, "commit", "-q", "-m", msg)
}

func mustRunGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
}

func splitLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// TestVersion_Output asserts the `version` command prints something.
func TestVersion_Output(t *testing.T) {
	p := newProject(t, false)
	out := p.runOK(t, "version")
	require.NotEmpty(t, strings.TrimSpace(out))
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

var (
	binaryOnce sync.Once
	binaryPath string
	binaryErr  error
)

// ensureBinary builds sbdb once per test process and returns the path. The
// binary lives in os.TempDir() and is reused across every TestX call.
func ensureBinary(t *testing.T) string {
	t.Helper()
	binaryOnce.Do(func() {
		dir, err := os.MkdirTemp("", "sbdb-e2e-bin-")
		if err != nil {
			binaryErr = err
			return
		}
		name := "sbdb"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		binaryPath = filepath.Join(dir, name)
		// Build from repo root (parent of this package).
		repoRoot, err := repoRoot()
		if err != nil {
			binaryErr = err
			return
		}
		cmd := exec.Command("go", "build", "-o", binaryPath, ".")
		cmd.Dir = repoRoot
		cmd.Env = append(os.Environ(), "CGO_ENABLED=1")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			binaryErr = fmt.Errorf("go build: %v\n%s", err, stderr.String())
			return
		}
	})
	if binaryErr != nil {
		t.Fatalf("building sbdb: %v", binaryErr)
	}
	return binaryPath
}

// repoRoot walks up from the e2e package dir until it finds go.mod.
func repoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := cwd
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		dir = filepath.Dir(dir)
	}
	return "", fmt.Errorf("go.mod not found searching up from %s", cwd)
}
