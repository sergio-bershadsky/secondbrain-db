package integrity

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// GitScope captures the set of working-tree paths that differ from HEAD.
// Used by `sbdb doctor` to limit work to uncommitted changes by default.
type GitScope struct {
	IsRepo       bool
	Root         string
	ChangedPaths []string // absolute paths
}

// NewGitScope detects whether dir is inside a git work tree, and if so,
// enumerates the union of: modified-unstaged, modified-staged, untracked.
// If dir is not a git repo, returns IsRepo=false with no error.
func NewGitScope(dir string) (*GitScope, error) {
	// Resolve symlinks so git's output (which uses real paths) can be
	// mapped back to the caller's dir form. Handles macOS /var↔/private/var
	// and Windows 8.3 short names (e.g. RUNNER~1↔runneradmin).
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		realDir = dir
	}

	realRoot, err := gitTopLevel(realDir)
	if err != nil {
		return &GitScope{IsRepo: false}, nil
	}
	// Git emits forward slashes even on Windows; normalise to OS-native.
	realRoot = filepath.FromSlash(realRoot)

	// Derive the caller-visible root via path-relative arithmetic. This
	// works regardless of slash style or short/long Windows names because
	// filepath.Rel is OS-aware (case-insensitive on Windows).
	rel, err := filepath.Rel(realDir, realRoot)
	if err != nil {
		rel = "."
	}
	root := filepath.Clean(filepath.Join(dir, rel))

	porcelain, err := gitPorcelain(realDir)
	if err != nil {
		return nil, err
	}

	scope := &GitScope{IsRepo: true, Root: root}
	for _, line := range strings.Split(porcelain, "\n") {
		if len(line) < 4 {
			continue
		}
		// `git status --porcelain` format: "XY <path>"
		// X = staged status, Y = unstaged status. Untracked: "??"
		path := line[3:]
		// Renames look like "R  old -> new"; take the new path.
		if i := strings.Index(path, " -> "); i >= 0 {
			path = path[i+4:]
		}
		path = strings.Trim(path, `"`)
		scope.ChangedPaths = append(scope.ChangedPaths, filepath.Join(root, path))
	}
	return scope, nil
}

// PairScopedPaths returns ChangedPaths plus the matching pair file
// (sidecar for an .md, .md for a sidecar) so drift between pair members
// is always detected.
func (g *GitScope) PairScopedPaths() []string {
	seen := map[string]bool{}
	for _, p := range g.ChangedPaths {
		seen[p] = true
		if strings.HasSuffix(p, ".md") {
			seen[SidecarPath(p)] = true
		} else if strings.HasSuffix(p, ".yaml") {
			seen[mdForSidecar(p)] = true
		}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	return out
}

func mdForSidecar(sidecarPath string) string {
	return strings.TrimSuffix(sidecarPath, ".yaml") + ".md"
}

func gitTopLevel(dir string) (string, error) {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("not a git repo: %w", err)
	}
	return strings.TrimSpace(out.String()), nil
}

func gitPorcelain(dir string) (string, error) {
	cmd := exec.Command("git", "-C", dir, "status", "--porcelain")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git status failed: %w", err)
	}
	return out.String(), nil
}
