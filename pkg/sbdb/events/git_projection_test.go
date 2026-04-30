package events

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestProjector_RoundTrip drives the projector against a fresh git repo
// containing a known sequence of changes and asserts the JSONL output
// matches expected events.
func TestProjector_RoundTrip(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repo := t.TempDir()
	gitInit(t, repo)

	// Three commits, each touching docs/notes/* (mapped to bucket "note").
	writeAndCommit(t, repo, "docs/notes/foo.md", "# Foo\n", "alice@x.com", "add foo")
	writeAndCommit(t, repo, "docs/notes/foo.md", "# Foo updated\n", "bob@x.com", "update foo")
	writeAndCommit(t, repo, "docs/notes/bar.md", "# Bar\n", "alice@x.com", "add bar")
	deleteAndCommit(t, repo, "docs/notes/foo.md", "alice@x.com", "delete foo")

	// First commit's sha is our `from` (events are commits *after* this baseline).
	firstSHA := commitSHA(t, repo, "HEAD~3")

	p := &Projector{
		Repo: repo,
		PathToBucket: func(path string) string {
			if strings.HasPrefix(path, "docs/notes/") {
				return "note"
			}
			return ""
		},
	}

	var out bytes.Buffer
	require.NoError(t, p.Emit(&out, firstSHA, "HEAD"))

	lines := splitNonEmpty(out.String())
	// Three events: update foo, add bar, delete foo.
	require.Len(t, lines, 3, "got: %s", out.String())

	// Verify each line parses and has the expected structural fields.
	for _, line := range lines {
		var ev map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(line), &ev), line)
		require.Contains(t, []string{"note.updated", "note.created", "note.deleted"}, ev["type"])
		require.NotEmpty(t, ev["id"])
		require.NotEmpty(t, ev["op"])
		require.NotEmpty(t, ev["ts"])
	}

	// Specific structural assertions.
	parsed := make([]map[string]interface{}, 0, len(lines))
	for _, line := range lines {
		var ev map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(line), &ev))
		parsed = append(parsed, ev)
	}

	// In chronological order: updated, created, deleted.
	require.Equal(t, "note.updated", parsed[0]["type"])
	require.Equal(t, "docs/notes/foo.md", parsed[0]["id"])
	require.NotEmpty(t, parsed[0]["sha"], "updated must carry post-sha")
	require.NotEmpty(t, parsed[0]["prev"], "updated must carry pre-sha")
	require.NotEqual(t, parsed[0]["sha"], parsed[0]["prev"])

	require.Equal(t, "note.created", parsed[1]["type"])
	require.Equal(t, "docs/notes/bar.md", parsed[1]["id"])
	require.NotEmpty(t, parsed[1]["sha"])
	require.Empty(t, parsed[1]["prev"], "created must omit prev")

	require.Equal(t, "note.deleted", parsed[2]["type"])
	require.Equal(t, "docs/notes/foo.md", parsed[2]["id"])
	require.Empty(t, parsed[2]["sha"], "deleted must omit sha")
	require.NotEmpty(t, parsed[2]["prev"], "deleted must carry pre-sha")
}

// TestProjector_SkipsUnknownPaths asserts that files outside any known
// docs_dir produce no events.
func TestProjector_SkipsUnknownPaths(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	repo := t.TempDir()
	gitInit(t, repo)
	writeAndCommit(t, repo, "docs/notes/in.md", "in\n", "a@x.com", "in scope")
	writeAndCommit(t, repo, "README.md", "readme\n", "a@x.com", "out of scope")
	first := commitSHA(t, repo, "HEAD~1")

	p := &Projector{
		Repo: repo,
		PathToBucket: func(path string) string {
			if strings.HasPrefix(path, "docs/notes/") {
				return "note"
			}
			return ""
		},
	}
	var out bytes.Buffer
	require.NoError(t, p.Emit(&out, first, "HEAD"))
	require.Empty(t, splitNonEmpty(out.String()), "README.md should produce no events")
}

// TestProjector_BlobShaIsGitNative asserts the sha embedded in events is
// exactly what `git cat-file blob <sha>` would resolve. A worker can rely
// on this to retrieve content via plain git plumbing.
func TestProjector_BlobShaIsGitNative(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	repo := t.TempDir()
	gitInit(t, repo)
	writeAndCommit(t, repo, "docs/notes/x.md", "hello\n", "a@x.com", "add x")
	first := commitSHA(t, repo, "HEAD")

	// Make a second commit so we have a range to project.
	writeAndCommit(t, repo, "docs/notes/y.md", "world\n", "a@x.com", "add y")

	p := &Projector{
		Repo: repo,
		PathToBucket: func(path string) string {
			if strings.HasPrefix(path, "docs/notes/") {
				return "note"
			}
			return ""
		},
	}
	var out bytes.Buffer
	require.NoError(t, p.Emit(&out, first, "HEAD"))
	lines := splitNonEmpty(out.String())
	require.Len(t, lines, 1)

	var ev map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &ev))
	sha := ev["sha"].(string)

	// `git cat-file blob <sha>` must round-trip to "world\n".
	cmd := exec.Command("git", "-C", repo, "cat-file", "blob", sha)
	bytes, err := cmd.Output()
	require.NoError(t, err)
	require.Equal(t, "world\n", string(bytes))
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func gitInit(t *testing.T, dir string) {
	t.Helper()
	mustRun(t, dir, "git", "init", "-q", "-b", "main")
	mustRun(t, dir, "git", "config", "user.email", "test@example.com")
	mustRun(t, dir, "git", "config", "user.name", "Test")
	mustRun(t, dir, "git", "config", "commit.gpgsign", "false")
	// Ensure a baseline commit so HEAD~N references work.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitkeep"), []byte{}, 0o644))
	mustRun(t, dir, "git", "add", ".gitkeep")
	mustRun(t, dir, "git", "commit", "-q", "-m", "init")
}

func writeAndCommit(t *testing.T, dir, relPath, content, email, msg string) {
	t.Helper()
	full := filepath.Join(dir, relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	mustRun(t, dir, "git", "add", relPath)
	mustRunEnv(t, dir, []string{"GIT_AUTHOR_EMAIL=" + email, "GIT_COMMITTER_EMAIL=" + email}, "git", "commit", "-q", "-m", msg)
}

func deleteAndCommit(t *testing.T, dir, relPath, email, msg string) {
	t.Helper()
	mustRun(t, dir, "git", "rm", "-q", relPath)
	mustRunEnv(t, dir, []string{"GIT_AUTHOR_EMAIL=" + email, "GIT_COMMITTER_EMAIL=" + email}, "git", "commit", "-q", "-m", msg)
}

func commitSHA(t *testing.T, dir, ref string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "rev-parse", ref)
	out, err := cmd.Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}

func mustRun(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s %v: %s", name, args, out)
}

func mustRunEnv(t *testing.T, dir string, env []string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s %v: %s", name, args, out)
}

func splitNonEmpty(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
