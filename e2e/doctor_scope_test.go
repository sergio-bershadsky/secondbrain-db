//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestE2E_DoctorScope_DefaultIsUncommittedOnly verifies that without --all,
// the doctor only audits files that differ from HEAD. A clean committed doc
// is skipped; an uncommitted (or modified) doc is found.
func TestE2E_DoctorScope_DefaultIsUncommittedOnly(t *testing.T) {
	p := newV2Project(t)

	gitInitInProject(t, p.root)
	p.runOK(t, "create", "-s", "note",
		"--field", "id=clean", "--field", "title=Clean", "--field", "status=active",
		"--content", "# Clean")
	p.runOK(t, "create", "-s", "note",
		"--field", "id=dirty", "--field", "title=Dirty", "--field", "status=active",
		"--content", "# Dirty")
	mustRunGit(t, p.root, "add", ".")
	mustRunGit(t, p.root, "commit", "-q", "-m", "add docs")

	// Tamper with the committed dirty.md (overwrite without the sidecar matching).
	dirtyMD := filepath.Join(p.root, "docs/note/dirty.md")
	require.NoError(t, os.WriteFile(dirtyMD,
		[]byte("---\nid: dirty\ntitle: Dirty\nstatus: active\n---\nTAMPERED"), 0o644))

	// Default scope — should detect the dirty drift but NOT report on clean.md
	stdout, stderr, code := p.run(t, "doctor", "check", "-s", "note")
	require.NotEqual(t, 0, code, "expected drift exit; stdout: %s; stderr: %s", stdout, stderr)
	combined := stdout + stderr
	assert.Contains(t, combined, "dirty",
		"default scope should examine dirty.md")
	// "clean.md" should NOT appear in the report (default scope skips clean committed docs).
	assert.NotContains(t, strings.ReplaceAll(combined, "cleanup", ""), "clean.md",
		"default scope should not examine committed clean docs")

	// --all also finds it.
	stdout2, _, code2 := p.run(t, "doctor", "check", "--all", "-s", "note")
	require.NotEqual(t, 0, code2)
	assert.Contains(t, stdout2, "dirty")
}

// TestE2E_DoctorScope_NonGit_FallsBackToAll verifies that doctor in a
// non-git directory falls back to --all and emits a stderr notice.
func TestE2E_DoctorScope_NonGit_FallsBackToAll(t *testing.T) {
	p := newV2Project(t)

	p.runOK(t, "create", "-s", "note",
		"--field", "id=hello", "--field", "title=Hello", "--field", "status=active",
		"--content", "# Hi")

	stdout, stderr, code := p.run(t, "doctor", "check", "-s", "note")
	require.Equal(t, 0, code, "stdout: %s; stderr: %s", stdout, stderr)
	assert.Contains(t, stderr, "not a git repo; falling back to --all")
}
