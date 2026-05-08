package e2e

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestACLGitFilterRoundTrip simulates the full git-crypt-style flow:
// self-init -> unlock -> write doc -> acl set -> commit (clean fires) ->
// HEAD shows ciphertext -> wipe working tree -> checkout (smudge fires) ->
// working tree contains cleartext again.
func TestACLGitFilterRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	bin := buildSBDB(t)
	repo := newGitRepo(t)

	// Ensure the freshly-built sbdb binary is on PATH so the git filter
	// driver (`sbdb _filter clean ...`) can resolve it during git add.
	t.Setenv("PATH", filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))

	run(t, repo, bin, "keys", "self-init", "--name", "Alice", "--email", "a@x", "--nickname", "alice")
	run(t, repo, bin, "unlock")

	docsDir := filepath.Join(repo, "docs")
	require.NoError(t, os.MkdirAll(docsDir, 0o755))
	docPath := filepath.Join(docsDir, "secret.md")
	cleartext := []byte("# top secret\n\nbody\n")
	require.NoError(t, os.WriteFile(docPath, cleartext, 0o644))

	run(t, repo, bin, "acl", "set", docPath, "--readers", "alice")

	gitRun(t, repo, "add", "-A")
	gitRun(t, repo, "commit", "-m", "secret")

	// HEAD should hold the ciphertext envelope.
	head := gitOutput(t, repo, "show", "HEAD:docs/secret.md")
	require.True(t, strings.Contains(head, "BEGIN SBDB-ACL-ENVELOPE"),
		"committed form must be envelope, got:\n%s", head)

	// Wipe working tree copy and rematerialise via checkout (smudge runs).
	require.NoError(t, os.Remove(docPath))
	gitRun(t, repo, "checkout", "HEAD", "--", "docs/secret.md")

	got, err := os.ReadFile(docPath)
	require.NoError(t, err)
	require.Equal(t, cleartext, got, "smudge should restore cleartext for the reader")
}

func buildSBDB(t *testing.T) string {
	t.Helper()
	name := "sbdb"
	if runtime.GOOS == "windows" {
		name = "sbdb.exe"
	}
	out := filepath.Join(t.TempDir(), name)
	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = repoRootForTests(t)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build sbdb: %v\n%s", err, b)
	}
	return out
}

func newGitRepo(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	gitRun(t, d, "init", "-q", "-b", "main")
	gitRun(t, d, "config", "user.email", "test@x")
	gitRun(t, d, "config", "user.name", "Test")
	gitRun(t, d, "config", "commit.gpgsign", "false")
	return d
}

func run(t *testing.T, dir, bin string, args ...string) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s %v: %v\n%s", bin, args, err, out.String())
	}
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, b)
	}
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, b)
	}
	return string(b)
}

func repoRootForTests(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	for d := wd; d != "/"; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
	}
	t.Fatal("no go.mod found above " + wd)
	return ""
}
