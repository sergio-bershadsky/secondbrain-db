package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runHeal is a thin wrapper around runCLI scoped to `doctor heal` so each
// case reads as a single line.
func runHeal(t *testing.T, dir string, args ...string) (string, int) {
	t.Helper()
	full := append([]string{"doctor", "heal", "-s", "notes"}, args...)
	return runCLI(t, dir, []string{"SBDB_USE_SIDECAR=1"}, full...)
}

// tamperContent rewrites a doc's body to a different string without
// touching its sidecar — a content_sha mismatch results.
func tamperContent(t *testing.T, mdPath, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(mdPath,
		[]byte("---\nid: alpha\ncreated: 2026-04-28\n---\n"+body), 0o644))
}

// removeSidecar deletes the sidecar of a doc — produces a missing-sidecar
// drift state, which is heal's "fixable without ack" path.
func removeSidecar(t *testing.T, mdPath string) {
	t.Helper()
	dir, base := filepath.Split(mdPath)
	id := strings.TrimSuffix(base, ".md")
	require.NoError(t, os.Remove(filepath.Join(dir, id+".yaml")))
}

// parseHealJSON returns the inner `data` payload — the wrapper envelope
// (`{"version":1,"data":...}`) is uninteresting for assertions. Tamper
// runs append a stderr line after the JSON; we only consume the first
// object on the stream.
func parseHealJSON(t *testing.T, out string) map[string]any {
	t.Helper()
	idx := strings.IndexByte(out, '{')
	require.GreaterOrEqual(t, idx, 0, "no JSON in output: %q", out)
	dec := json.NewDecoder(strings.NewReader(out[idx:]))
	var envelope map[string]any
	require.NoError(t, dec.Decode(&envelope), "parsing %q", out[idx:])
	data, ok := envelope["data"].(map[string]any)
	require.True(t, ok, "expected `data` key in envelope: %v", envelope)
	return data
}

func TestDoctorHeal_NoChange_ReturnsCleanReport(t *testing.T) {
	dir := setupV2KB(t)

	out, code := runHeal(t, dir, "--all")
	require.Equal(t, 0, code, "heal on clean KB should succeed: %s", out)

	m := parseHealJSON(t, out)
	require.Equal(t, "doctor.heal", m["action"])
	results, _ := m["results"].([]any)
	require.Len(t, results, 1)
	first := results[0].(map[string]any)
	assert.Equal(t, "no_change", first["outcome"])
}

func TestDoctorHeal_MissingSidecar_FixedWithoutAck(t *testing.T) {
	dir := setupV2KB(t)
	mdPath := filepath.Join(dir, "docs/notes/alpha.md")
	removeSidecar(t, mdPath)

	out, code := runHeal(t, dir, "--all")
	require.Equal(t, 0, code, "missing-sidecar repair should not require ack: %s", out)

	m := parseHealJSON(t, out)
	results := m["results"].([]any)
	first := results[0].(map[string]any)
	assert.Equal(t, "drift_fixed", first["outcome"])

	// And the file should now pass check.
	_, checkCode := runCLI(t, dir, []string{"SBDB_USE_SIDECAR=1"},
		"doctor", "check", "--all", "-s", "notes")
	require.Equal(t, 0, checkCode, "post-heal check should be clean")
}

func TestDoctorHeal_Tamper_WithoutAck_Exits6(t *testing.T) {
	dir := setupV2KB(t)
	mdPath := filepath.Join(dir, "docs/notes/alpha.md")
	tamperContent(t, mdPath, "TAMPERED-BODY")

	out, code := runHeal(t, dir, "--all")
	require.Equal(t, 6, code, "tamper without --i-meant-it should exit 6: %s", out)

	assert.Contains(t, out,
		"Tamper detected on 1 file(s). If your edits were intentional, re-run with --i-meant-it")
	m := parseHealJSON(t, out)
	results := m["results"].([]any)
	first := results[0].(map[string]any)
	assert.Equal(t, "tamper_unacked", first["outcome"])
}

func TestDoctorHeal_Tamper_WithAck_ReSigns(t *testing.T) {
	dir := setupV2KB(t)
	mdPath := filepath.Join(dir, "docs/notes/alpha.md")
	tamperContent(t, mdPath, "INTENTIONAL-EDIT")

	out, code := runHeal(t, dir, "--all", "--i-meant-it")
	require.Equal(t, 0, code, "tamper with ack should heal: %s", out)

	m := parseHealJSON(t, out)
	results := m["results"].([]any)
	first := results[0].(map[string]any)
	assert.Equal(t, "re_signed", first["outcome"])

	// Post-heal must check clean.
	_, checkCode := runCLI(t, dir, []string{"SBDB_USE_SIDECAR=1"},
		"doctor", "check", "--all", "-s", "notes")
	require.Equal(t, 0, checkCode, "post-heal check should be clean")
}

func TestDoctorHeal_DryRun_DoesNotWrite(t *testing.T) {
	dir := setupV2KB(t)
	mdPath := filepath.Join(dir, "docs/notes/alpha.md")
	tamperContent(t, mdPath, "DRY-RUN-CHECK")
	sidecarPath := filepath.Join(dir, "docs/notes/alpha.yaml")
	beforeSidecar, err := os.ReadFile(sidecarPath)
	require.NoError(t, err)

	out, code := runHeal(t, dir, "--all", "--i-meant-it", "--dry-run")
	require.Equal(t, 0, code, "dry-run heal should succeed: %s", out)

	m := parseHealJSON(t, out)
	results := m["results"].([]any)
	first := results[0].(map[string]any)
	assert.Equal(t, "would_re_sign", first["outcome"])

	afterSidecar, err := os.ReadFile(sidecarPath)
	require.NoError(t, err)
	assert.Equal(t, beforeSidecar, afterSidecar, "dry-run must not modify sidecar")
}

func TestDoctorHeal_ID_Repeatable(t *testing.T) {
	dir := setupV2KB(t)
	// Add a second doc.
	out, code := runCLI(t, dir, []string{"SBDB_USE_SIDECAR=1"},
		"create", "-s", "notes",
		"--field", "id=beta",
		"--field", "created=2026-04-29",
		"--content", "# Beta")
	require.Equal(t, 0, code, "create beta: %s", out)

	// Tamper both, then heal both via repeated --id.
	tamperContent(t, filepath.Join(dir, "docs/notes/alpha.md"), "FIRST")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs/notes/beta.md"),
		[]byte("---\nid: beta\ncreated: 2026-04-29\n---\nSECOND"), 0o644))

	out, code = runHeal(t, dir, "--id", "alpha", "--id", "beta", "--i-meant-it")
	require.Equal(t, 0, code, "id-scoped heal: %s", out)

	m := parseHealJSON(t, out)
	results := m["results"].([]any)
	require.Len(t, results, 2)
	for _, r := range results {
		assert.Equal(t, "re_signed", r.(map[string]any)["outcome"])
	}
}

func TestDoctorHeal_ID_NotFound_Errors(t *testing.T) {
	dir := setupV2KB(t)
	out, code := runHeal(t, dir, "--id", "does-not-exist", "--i-meant-it")
	require.NotEqual(t, 0, code, "missing id should error: %s", out)
	assert.Contains(t, out, "does-not-exist")
}

func TestDoctorHeal_MutuallyExclusiveFlags(t *testing.T) {
	dir := setupV2KB(t)
	out, code := runHeal(t, dir, "--all", "--id", "alpha")
	require.NotEqual(t, 0, code, "expected mutual-exclusion error: %s", out)
	assert.Contains(t, out, "mutually exclusive")

	out, code = runHeal(t, dir, "--all", "--since", "HEAD")
	require.NotEqual(t, 0, code)
	assert.Contains(t, out, "mutually exclusive")
}

func TestDoctorHeal_Since_GitDiff(t *testing.T) {
	dir := setupV2KB(t)

	// Initialise git so --since has something to diff against.
	gitInit(t, dir)
	gitAddAll(t, dir, "initial commit")

	// Edit alpha.md content (without sidecar update — pre-commit guard not in play here).
	mdPath := filepath.Join(dir, "docs/notes/alpha.md")
	tamperContent(t, mdPath, "POST-INITIAL-COMMIT")

	out, code := runHeal(t, dir, "--since", "HEAD", "--i-meant-it")
	require.Equal(t, 0, code, "since-scoped heal: %s", out)

	m := parseHealJSON(t, out)
	scope, _ := m["scope"].(string)
	assert.Equal(t, "since:HEAD", scope)
	results := m["results"].([]any)
	// At least one result, and the alpha.md result should be re_signed.
	require.NotEmpty(t, results)
	var sawAlpha bool
	for _, r := range results {
		rm := r.(map[string]any)
		if file, _ := rm["file"].(string); strings.HasSuffix(file, "alpha.md") {
			sawAlpha = true
			assert.Equal(t, "re_signed", rm["outcome"])
		}
	}
	assert.True(t, sawAlpha, "alpha.md should appear in --since results")
}

func TestDoctorHeal_Since_SkipsNonSchemaPaths(t *testing.T) {
	dir := setupV2KB(t)
	gitInit(t, dir)

	// Add an unrelated .md outside any schema before the initial commit so
	// it's tracked; then modify it after the commit so git diff HEAD will
	// surface it. The heal command must skip it silently rather than error,
	// since hooks pass arbitrary diff output.
	outside := filepath.Join(dir, "notes-outside/random.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(outside), 0o755))
	require.NoError(t, os.WriteFile(outside, []byte("# v1"), 0o644))

	gitAddAll(t, dir, "initial commit")

	require.NoError(t, os.WriteFile(outside, []byte("# v2 — modified"), 0o644))

	out, code := runHeal(t, dir, "--since", "HEAD")
	require.Equal(t, 0, code, "non-schema paths must not break heal: %s", out)

	m := parseHealJSON(t, out)
	results := m["results"].([]any)
	var sawSkipped bool
	for _, r := range results {
		rm := r.(map[string]any)
		if rm["outcome"] == "skipped_no_schema" {
			sawSkipped = true
			file, _ := rm["file"].(string)
			assert.Contains(t, file, "notes-outside")
		}
	}
	assert.True(t, sawSkipped, "non-schema path should yield skipped_no_schema")
}

// gitInit and gitAddAll bootstrap a minimal repo for --since tests. We
// can't reuse helpers from outside this package without exporting them,
// and the existing doctor tests don't need git, so these stay local.
func gitInit(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
		{"config", "commit.gpgsign", "false"},
	} {
		c := exec.Command("git", args...)
		c.Dir = dir
		require.NoError(t, c.Run(), "git %v", args)
	}
}

func gitAddAll(t *testing.T, dir, msg string) {
	t.Helper()
	for _, args := range [][]string{
		{"add", "-A"},
		{"commit", "-q", "-m", msg, "--no-verify"},
	} {
		c := exec.Command("git", args...)
		c.Dir = dir
		out, err := c.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
}
