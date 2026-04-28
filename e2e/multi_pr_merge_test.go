//go:build e2e

package e2e

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestE2E_MultiPR_NoMergeConflict creates two branches, each adds a different
// note via `sbdb create`, merges both back to main without git conflict, and
// confirms doctor check is clean afterwards. This is the central success
// criterion for v2 layout — the reason data/ aggregates were dropped.
func TestE2E_MultiPR_NoMergeConflict(t *testing.T) {
	p := newV2Project(t)

	// gitInitInProject creates the initial commit from the project fixtures.
	gitInitInProject(t, p.root)

	// Branch A
	mustRunGit(t, p.root, "checkout", "-q", "-b", "pr-a")
	p.runOK(t, "create", "-s", "note",
		"--field", "id=alpha", "--field", "title=Alpha", "--field", "status=active",
		"--content", "# Alpha")
	mustRunGit(t, p.root, "add", ".")
	mustRunGit(t, p.root, "commit", "-q", "-m", "alpha")

	// Branch B from main
	mustRunGit(t, p.root, "checkout", "-q", "main")
	mustRunGit(t, p.root, "checkout", "-q", "-b", "pr-b")
	p.runOK(t, "create", "-s", "note",
		"--field", "id=beta", "--field", "title=Beta", "--field", "status=active",
		"--content", "# Beta")
	mustRunGit(t, p.root, "add", ".")
	mustRunGit(t, p.root, "commit", "-q", "-m", "beta")

	// Merge both into main
	mustRunGit(t, p.root, "checkout", "-q", "main")
	mustRunGit(t, p.root, "merge", "--no-ff", "-q", "-m", "merge a", "pr-a")
	mustRunGit(t, p.root, "merge", "--no-ff", "-q", "-m", "merge b", "pr-b")

	// Working tree must be clean.
	cmd := exec.Command("git", "-C", p.root, "status", "--porcelain")
	out, err := cmd.Output()
	require.NoError(t, err)
	require.Empty(t, strings.TrimSpace(string(out)),
		"unexpected dirty status: %s", out)

	// doctor check is clean (use --all so it audits everything).
	p.runOK(t, "doctor", "check", "--all", "-s", "note")
}
