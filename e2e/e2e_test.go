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
	"strings"
	"sync"
	"testing"
	"time"

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

// configWithEvents enables the events feature in the .sbdb.toml fixture.
const configWithEvents = `default_schema = "note"

[events]
enabled = true
`

// project bundles a temp project root + the path to the built sbdb binary.
type project struct {
	root   string
	binary string
}

// newProject returns a fresh tempdir initialised with the note schema and
// .sbdb.toml. The binary is compiled once per test run via build().
func newProject(t *testing.T, withEvents bool) *project {
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
	if withEvents {
		require.NoError(t, os.WriteFile(
			filepath.Join(root, ".sbdb.toml"),
			[]byte(configWithEvents),
			0o644,
		))
	} else {
		require.NoError(t, os.WriteFile(
			filepath.Join(root, ".sbdb.toml"),
			[]byte("default_schema = \"note\"\n"),
			0o644,
		))
	}
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

// TestEventsCRUD_EmitsEvents asserts that CRUD on documents emits the
// matching note.created / note.updated / note.deleted events when
// events.enabled = true.
func TestEventsCRUD_EmitsEvents(t *testing.T) {
	p := newProject(t, true)

	_ = p.runOK(t,
		"create",
		"--field", "title=Hello",
		"--field", "slug=hello",
		"--content", "First.",
		"--format", "json",
	)
	_ = p.runOK(t, "update", "--id", "hello", "--field", "title=Updated", "--format", "json")
	_ = p.runOK(t, "delete", "--id", "hello", "--yes", "--format", "json")

	lines := readEvents(t, p.root)
	require.GreaterOrEqual(t, len(lines), 3, "expected >=3 events, got %d", len(lines))

	types := make([]string, 0, len(lines))
	for _, line := range lines {
		var ev struct {
			Type string `json:"type"`
		}
		require.NoError(t, json.Unmarshal([]byte(line), &ev), "invalid event JSON: %s", line)
		types = append(types, ev.Type)
	}
	require.Contains(t, types, "note.created")
	require.Contains(t, types, "note.updated")
	require.Contains(t, types, "note.deleted")
}

// TestEventTypes_ListsBuiltins verifies `sbdb event types` includes every
// expected built-in bucket from the spec catalog.
func TestEventTypes_ListsBuiltins(t *testing.T) {
	p := newProject(t, true)
	out := p.runOK(t, "event", "types")

	for _, expected := range []string{
		"note.created",
		"note.updated",
		"note.deleted",
		"task.created",
		"task.status_changed",
		"adr.proposed",
		"adr.accepted",
		"discussion.action_added",
		"graph.node_added",
		"graph.edge_added",
		"kb.indexed",
		"kb.embedding_updated",
		"records.upserted",
		"integrity.signed",
		"meta.archived",
		"meta.event_type_registered",
	} {
		require.Contains(t, out, expected, "missing built-in type %q", expected)
	}
}

// TestEventAppend_Show exercises the programmatic append + show pipeline.
func TestEventAppend_Show(t *testing.T) {
	p := newProject(t, true)

	_ = p.runOK(t,
		"event", "append",
		"--type", "note.created",
		"--id", "notes/test.md",
		"--sha", "abc123",
	)

	out := p.runOK(t, "event", "show", "1")
	require.Contains(t, out, `"type":"note.created"`)
	require.Contains(t, out, `"id":"notes/test.md"`)
	require.Contains(t, out, `"sha":"abc123"`)
}

// TestDoctor_WindowViolation verifies that an old daily-events file
// triggers exit 4 on `doctor check`, and that `doctor fix` archives it
// and clears the violation.
func TestDoctor_WindowViolation(t *testing.T) {
	p := newProject(t, true)

	// Plant an event from January (well outside live window).
	eventsDir := filepath.Join(p.root, ".sbdb", "events")
	require.NoError(t, os.MkdirAll(eventsDir, 0o755))
	stale := filepath.Join(eventsDir, "2026-01-15.jsonl")
	require.NoError(t, os.WriteFile(
		stale,
		[]byte(`{"ts":"2026-01-15T10:00:00.000Z","type":"note.created","id":"notes/old.md"}`+"\n"),
		0o644,
	))

	// doctor check should report exit 4 (drift / window violation).
	stdout, _, code := p.run(t, "doctor", "check", "--format", "json")
	require.Equal(t, 4, code, "expected exit 4 on window violation, got %d\n%s", code, stdout)
	require.Contains(t, stdout, "event_window")

	// doctor fix should archive January cleanly.
	_ = p.runOK(t, "doctor", "fix", "--format", "json")

	// Stale file gone, archive present.
	_, err := os.Stat(stale)
	require.True(t, os.IsNotExist(err), "stale daily file still present")

	gz := filepath.Join(p.root, ".sbdb", "events", "archive", "2026-01.jsonl.gz")
	_, err = os.Stat(gz)
	require.NoError(t, err, "archive gz missing")

	// doctor check now exits 0.
	_, _, code = p.run(t, "doctor", "check", "--format", "json")
	require.Equal(t, 0, code, "doctor check should be clean after fix")
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
		binaryPath = filepath.Join(dir, "sbdb")
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

// readEvents collects all event JSONL lines from the project's live event
// files in lex order.
func readEvents(t *testing.T, root string) []string {
	t.Helper()
	dir := filepath.Join(root, ".sbdb", "events")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("reading events dir: %v", err)
	}
	// Wait briefly for filesystem flush on slow CI runners.
	time.Sleep(20 * time.Millisecond)
	var lines []string
	for _, ent := range entries {
		if !strings.HasSuffix(ent.Name(), ".jsonl") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, ent.Name()))
		if err != nil {
			t.Fatalf("reading %s: %v", ent.Name(), err)
		}
		for _, line := range strings.Split(string(data), "\n") {
			if line != "" {
				lines = append(lines, line)
			}
		}
	}
	return lines
}
