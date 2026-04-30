package integrity

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gitInit(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "t@t"},
		{"config", "user.name", "t"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		require.NoError(t, cmd.Run(), "git %v", args)
	}
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
}

func TestGitScope_NotARepo_ReturnsAll(t *testing.T) {
	dir := t.TempDir()
	scope, err := NewGitScope(dir)
	require.NoError(t, err)
	assert.False(t, scope.IsRepo)
}

func TestGitScope_CleanRepo_NoChanges(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte("hi"), 0o644))
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-q", "-m", "init")

	scope, err := NewGitScope(dir)
	require.NoError(t, err)
	assert.True(t, scope.IsRepo)
	assert.Empty(t, scope.ChangedPaths)
}

func TestGitScope_DetectsModifiedAndUntracked(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte("hi"), 0o644))
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-q", "-m", "init")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte("hello"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.md"), []byte("new"), 0o644))

	scope, err := NewGitScope(dir)
	require.NoError(t, err)
	assert.True(t, scope.IsRepo)
	assert.ElementsMatch(t,
		[]string{filepath.Join(dir, "a.md"), filepath.Join(dir, "b.md")},
		scope.ChangedPaths,
	)
}

func TestGitScope_PairScoping_MDPullsInSidecar(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte("hi"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("file: a.md"), 0o644))
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-q", "-m", "init")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte("modified"), 0o644))

	scope, err := NewGitScope(dir)
	require.NoError(t, err)
	paired := scope.PairScopedPaths()
	assert.ElementsMatch(t,
		[]string{filepath.Join(dir, "a.md"), filepath.Join(dir, "a.yaml")},
		paired,
	)
}

func TestGitScope_PairScoping_SidecarPullsInMD(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte("hi"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("file: a.md"), 0o644))
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-q", "-m", "init")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("file: a.md\nx: 1"), 0o644))

	scope, err := NewGitScope(dir)
	require.NoError(t, err)
	paired := scope.PairScopedPaths()
	assert.ElementsMatch(t,
		[]string{filepath.Join(dir, "a.md"), filepath.Join(dir, "a.yaml")},
		paired,
	)
}
