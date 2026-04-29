# sbdb v2 layout — implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Drop the `data/` aggregate layer, replace integrity manifest with per-md sidecars (`<id>.yaml`), and remove builtin entity templates from `sbdb init` — all behind a feature flag, then flipped to default and legacy code deleted, all in one PR (sbdb v2.0.0).

**Architecture:** Each `<id>.md` gets a sibling `<id>.yaml` sidecar holding integrity hashes + HMAC sig. `sbdb list/query/get` walks `docs/<entity>/**.md` directly with concurrent frontmatter parsing — no `records.yaml`, no `.integrity.yaml`. Doctor scopes by default to working-tree changes (`git status` driven); `--all` for full audits. `sbdb doctor migrate` provides a one-shot v1 → v2 conversion.

**Tech Stack:** Go 1.23+, `gopkg.in/yaml.v3`, `github.com/stretchr/testify`, Cobra (existing CLI), `os/exec` for git-shellout. No new deps.

**Spec:** See `docs/superpowers/specs/2026-04-28-sbdb-v2-layout-design.md` for the full design rationale and risk register.

---

## File map

| Path | Action | Purpose |
|---|---|---|
| `internal/storage/walker.go` | CREATE | Concurrent walk of `docs_dir`; yields parsed `Doc{Path, ID, Frontmatter, Body}`. |
| `internal/storage/walker_test.go` | CREATE | Tests for walker. |
| `internal/integrity/sidecar.go` | CREATE | `Sidecar` struct, atomic write, drift detection, HMAC sign/verify. |
| `internal/integrity/sidecar_test.go` | CREATE | Tests for sidecar. |
| `internal/integrity/gitscope.go` | CREATE | Git-driven scope filter (working-tree changes vs HEAD). |
| `internal/integrity/gitscope_test.go` | CREATE | Tests for git scope. |
| `internal/document/save.go` | MODIFY | Branch on `useSidecar()`: write sidecar instead of records+manifest; same for delete. |
| `internal/document/save_test.go` | MODIFY | Cover both legacy and sidecar paths during transition; sidecar-only after flip. |
| `internal/query/queryset.go` | MODIFY | `loadRecords` branches on `useSidecar()`: walker path vs legacy. |
| `cmd/list.go`, `cmd/query.go`, `cmd/get.go` | MODIFY | Drop "reads records.yaml only" prose; otherwise unchanged (they go through QuerySet). |
| `cmd/doctor.go` | REWRITE | All subcommands operate on sidecars; new `migrate` subcommand; new `--all` flag and default uncommitted-only scope. |
| `cmd/doctor_test.go` | CREATE/EXTEND | Cover sidecar paths, scope filter, migrate. |
| `cmd/init_cmd.go` | MODIFY | Drop `--template` flag, drop `templateSchema()`, drop schema-write block, drop `default_schema` from `.sbdb.toml`. |
| `cmd/init_wizard.go` | MODIFY | Drop entity selection, drop per-entity branches; project name + 4 toggles only. |
| `internal/schema/loader.go` | MODIFY | Emit one-time deprecation warnings for `records_dir` and `partition`; ignore both. |
| `internal/storage/records.go` | DELETE | Replaced by walker. |
| `internal/storage/partition.go` | DELETE | Replaced by walker. |
| `internal/integrity/manifest.go` | DELETE | Replaced by sidecar. |
| `schemas/*.yaml` (root) | DELETE | Move to plugin in companion PR. |
| `e2e/v2_layout_test.go` | CREATE | Full CRUD scenario via CLI binary; asserts no `data/` ever created. |
| `e2e/migrate_test.go` | CREATE | v1 fixture → migrate → v2 layout + clean doctor. |
| `e2e/multi_pr_merge_test.go` | CREATE | Two parallel branches, conflict-free `git merge`. |
| `e2e/doctor_scope_test.go` | CREATE | Default scope = uncommitted only; `--all` overrides; non-git fallback. |
| `README.md`, `docs/guide.md` | MODIFY | Drop `--template` examples and `data/` references; add v2 layout diagram. |

---

## Task 1: Add `internal/storage/walker.go` (additive, behind no flag)

**Files:**
- Create: `internal/storage/walker.go`
- Test: `internal/storage/walker_test.go`

- [ ] **Step 1: Write failing tests**

`internal/storage/walker_test.go`:

```go
package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWalkDocs_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	docs, err := WalkDocsToSlice(dir)
	require.NoError(t, err)
	assert.Empty(t, docs)
}

func TestWalkDocs_FindsMDFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "alpha.md"),
		[]byte("---\nid: alpha\nstatus: active\n---\n# Alpha\n\nbody"),
		0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "beta.md"),
		[]byte("---\nid: beta\nstatus: archived\n---\n# Beta\n"),
		0o644,
	))

	docs, err := WalkDocsToSlice(dir)
	require.NoError(t, err)
	require.Len(t, docs, 2)

	byID := map[string]Doc{}
	for _, d := range docs {
		id, _ := d.Frontmatter["id"].(string)
		byID[id] = d
	}
	assert.Equal(t, "active", byID["alpha"].Frontmatter["status"])
	assert.Equal(t, "archived", byID["beta"].Frontmatter["status"])
	assert.Contains(t, byID["alpha"].Body, "body")
}

func TestWalkDocs_SkipsSidecarsAndNonMD(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "alpha.md"), []byte("---\nid: alpha\n---\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "alpha.yaml"), []byte("file: alpha.md\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.txt"), []byte("noise"), 0o644))

	docs, err := WalkDocsToSlice(dir)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "alpha", docs[0].Frontmatter["id"])
}

func TestWalkDocs_RecursesSubdirs(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "2026-04")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(sub, "alpha.md"),
		[]byte("---\nid: alpha\n---\n"),
		0o644,
	))

	docs, err := WalkDocsToSlice(dir)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, filepath.Join(sub, "alpha.md"), docs[0].Path)
}

func TestWalkDocs_MissingDirIsEmpty(t *testing.T) {
	docs, err := WalkDocsToSlice(filepath.Join(t.TempDir(), "missing"))
	require.NoError(t, err)
	assert.Empty(t, docs)
}

func TestWalkDocs_PropagatesParseError(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "bad.md"),
		[]byte("---\nthis: [is: not: valid: yaml\n---\nbody"),
		0o644,
	))
	_, err := WalkDocsToSlice(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bad.md")
}
```

- [ ] **Step 2: Run tests; verify FAIL**

```
go test ./internal/storage/ -run TestWalkDocs -v
```

Expected: build failure — `WalkDocsToSlice` and `Doc` undefined.

- [ ] **Step 3: Implement walker**

`internal/storage/walker.go`:

```go
package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// Doc is one parsed markdown document yielded by WalkDocs.
type Doc struct {
	Path        string         // absolute path to the .md file
	Frontmatter map[string]any // parsed YAML frontmatter (may be empty)
	Body        string         // markdown body after frontmatter
}

// WalkDocsToSlice walks docsDir recursively, parses every *.md file
// (skipping sidecar *.yaml and non-md files), and returns all Docs.
// Concurrency is bounded by SBDB_WALK_WORKERS (default GOMAXPROCS).
func WalkDocsToSlice(docsDir string) ([]Doc, error) {
	paths, err := collectMDPaths(docsDir)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, nil
	}

	workers := walkerWorkers()
	jobs := make(chan string, len(paths))
	for _, p := range paths {
		jobs <- p
	}
	close(jobs)

	results := make([]Doc, len(paths))
	errs := make([]error, len(paths))
	var wg sync.WaitGroup

	idx := 0
	idxMu := sync.Mutex{}
	nextIdx := func() int {
		idxMu.Lock()
		defer idxMu.Unlock()
		i := idx
		idx++
		return i
	}

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range jobs {
				i := nextIdx()
				fm, body, perr := ParseMarkdown(p)
				if perr != nil {
					errs[i] = fmt.Errorf("%s: %w", p, perr)
					continue
				}
				results[i] = Doc{Path: p, Frontmatter: fm, Body: body}
			}
		}()
	}
	wg.Wait()

	for _, e := range errs {
		if e != nil {
			return nil, e
		}
	}

	// Strip zero-value entries (parse errors leave them empty; we already
	// returned err in that case, so this is a no-op safety net).
	out := results[:0]
	for _, d := range results {
		if d.Path != "" {
			out = append(out, d)
		}
	}
	return out, nil
}

func collectMDPaths(docsDir string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(docsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) && path == docsDir {
				return filepath.SkipAll
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".md") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking %s: %w", docsDir, err)
	}
	return paths, nil
}

func walkerWorkers() int {
	if v := os.Getenv("SBDB_WALK_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return runtime.GOMAXPROCS(0)
}
```

- [ ] **Step 4: Run tests; verify PASS**

```
go test ./internal/storage/ -run TestWalkDocs -v
```

Expected: all 6 tests PASS.

- [ ] **Step 5: Commit**

```
git add internal/storage/walker.go internal/storage/walker_test.go
git commit -m "feat(storage): add concurrent docs walker

Adds WalkDocsToSlice for v2 layout — walks docs_dir, parses every .md
through the existing ParseMarkdown, returns Docs with frontmatter+body.
Skips sidecar *.yaml siblings. Concurrency bounded by SBDB_WALK_WORKERS
(default GOMAXPROCS). Additive; no call sites yet."
```

---

## Task 2: Add `internal/integrity/sidecar.go`

**Files:**
- Create: `internal/integrity/sidecar.go`
- Test: `internal/integrity/sidecar_test.go`

- [ ] **Step 1: Write failing tests**

`internal/integrity/sidecar_test.go`:

```go
package integrity

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestSidecarPath(t *testing.T) {
	assert.Equal(t, "/x/docs/notes/hello.yaml", SidecarPath("/x/docs/notes/hello.md"))
	assert.Equal(t, "/x/docs/notes/2026-04/hello.yaml", SidecarPath("/x/docs/notes/2026-04/hello.md"))
}

func TestSidecar_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "hello.md")
	require.NoError(t, os.WriteFile(mdPath, []byte("# hi"), 0o644))

	sc := &Sidecar{
		Version:        1,
		Algo:           "sha256",
		HMAC:           false,
		File:           "hello.md",
		ContentSHA:     "aaa",
		FrontmatterSHA: "bbb",
		RecordSHA:      "ccc",
		UpdatedAt:      "2026-04-28T00:00:00Z",
		Writer:         "secondbrain-db/test",
	}
	require.NoError(t, sc.Save(mdPath))

	got, err := LoadSidecar(mdPath)
	require.NoError(t, err)
	assert.Equal(t, sc.ContentSHA, got.ContentSHA)
	assert.Equal(t, sc.File, got.File)
	assert.False(t, got.HMAC)
}

func TestSidecar_LoadMissingReturnsErrIfNotExist(t *testing.T) {
	_, err := LoadSidecar(filepath.Join(t.TempDir(), "missing.md"))
	require.Error(t, err)
	assert.True(t, os.IsNotExist(err))
}

func TestSidecar_HMAC_SignAndVerify(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "hello.md")
	require.NoError(t, os.WriteFile(mdPath, []byte("# hi"), 0o644))

	key := []byte("test-key-32-bytes-aaaaaaaaaaaaaaaa")
	sc := &Sidecar{
		Version: 1, Algo: "sha256", HMAC: true,
		File:           "hello.md",
		ContentSHA:     "aaa",
		FrontmatterSHA: "bbb",
		RecordSHA:      "ccc",
		UpdatedAt:      "2026-04-28T00:00:00Z",
		Writer:         "secondbrain-db/test",
	}
	sc.Sig = sc.SignWith(key)
	require.NoError(t, sc.Save(mdPath))

	got, err := LoadSidecar(mdPath)
	require.NoError(t, err)
	assert.True(t, got.VerifyWith(key))
	assert.False(t, got.VerifyWith([]byte("wrong-key-32-bytes-bbbbbbbbbbbbbb")))
}

func TestSidecar_Verify_DriftDetection(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "hello.md")
	body := []byte("# hi\nbody\n")
	require.NoError(t, os.WriteFile(mdPath, body, 0o644))

	sc := &Sidecar{
		Version: 1, Algo: "sha256", HMAC: false,
		File:           "hello.md",
		ContentSHA:     HashContent("# hi\nbody\n"),
		FrontmatterSHA: HashFrontmatter(map[string]any{}),
		RecordSHA:      HashRecord(map[string]any{}),
	}

	d, err := sc.Verify(mdPath, map[string]any{}, "# hi\nbody\n", map[string]any{}, nil)
	require.NoError(t, err)
	assert.False(t, d.Any())

	d2, err := sc.Verify(mdPath, map[string]any{}, "# hi\ntampered\n", map[string]any{}, nil)
	require.NoError(t, err)
	assert.True(t, d2.ContentDrift)
}

func TestSidecar_AtomicWrite(t *testing.T) {
	// Write twice; ensure no .tmp leftovers.
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "hello.md")
	require.NoError(t, os.WriteFile(mdPath, []byte("# hi"), 0o644))

	sc := &Sidecar{Version: 1, Algo: "sha256", File: "hello.md"}
	require.NoError(t, sc.Save(mdPath))
	require.NoError(t, sc.Save(mdPath))

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		assert.False(t, filepath.Ext(e.Name()) == ".tmp", "leftover tmp file: %s", e.Name())
	}
}

func TestSidecar_YAMLLayoutStable(t *testing.T) {
	sc := &Sidecar{
		Version: 1, Algo: "sha256", HMAC: true, File: "hello.md",
		ContentSHA: "a", FrontmatterSHA: "b", RecordSHA: "c", Sig: "s",
		UpdatedAt: "2026-04-28T00:00:00Z", Writer: "secondbrain-db/test",
	}
	out, err := yaml.Marshal(sc)
	require.NoError(t, err)
	assert.Contains(t, string(out), "content_sha: a")
	assert.Contains(t, string(out), "hmac: true")
}
```

- [ ] **Step 2: Run tests; verify FAIL**

```
go test ./internal/integrity/ -run TestSidecar -v
```

Expected: build failure — `Sidecar`, `SidecarPath`, `LoadSidecar`, etc. undefined.

- [ ] **Step 3: Implement sidecar**

`internal/integrity/sidecar.go`:

```go
package integrity

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/sergio-bershadsky/secondbrain-db/internal/version"
)

// Sidecar is the per-doc integrity manifest stored next to the .md file.
// File path: replaces the .md extension with .yaml (e.g. hello.md → hello.yaml).
type Sidecar struct {
	Version        int    `yaml:"version"`
	Algo           string `yaml:"algo"`
	HMAC           bool   `yaml:"hmac"`
	File           string `yaml:"file"`
	ContentSHA     string `yaml:"content_sha"`
	FrontmatterSHA string `yaml:"frontmatter_sha"`
	RecordSHA      string `yaml:"record_sha"`
	Sig            string `yaml:"sig,omitempty"`
	UpdatedAt      string `yaml:"updated_at,omitempty"`
	Writer         string `yaml:"writer,omitempty"`
}

// SidecarPath returns the sidecar path for a given .md file:
// "<dir>/<basename>.yaml". For non-.md inputs, the extension is replaced anyway.
func SidecarPath(mdPath string) string {
	ext := filepath.Ext(mdPath)
	if ext == "" {
		return mdPath + ".yaml"
	}
	return strings.TrimSuffix(mdPath, ext) + ".yaml"
}

// LoadSidecar reads the sidecar for the given .md path. Returns os.IsNotExist
// errors unwrapped so callers can detect "no sidecar" with errors.Is/IsNotExist.
func LoadSidecar(mdPath string) (*Sidecar, error) {
	path := SidecarPath(mdPath)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sc Sidecar
	if err := yaml.Unmarshal(data, &sc); err != nil {
		return nil, fmt.Errorf("parsing sidecar %s: %w", path, err)
	}
	return &sc, nil
}

// Save writes the sidecar atomically (temp + rename).
func (s *Sidecar) Save(mdPath string) error {
	if s.UpdatedAt == "" {
		s.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if s.Writer == "" {
		s.Writer = "secondbrain-db/" + version.Version
	}
	if s.Algo == "" {
		s.Algo = "sha256"
	}
	if s.Version == 0 {
		s.Version = 1
	}

	data, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Errorf("marshaling sidecar: %w", err)
	}

	path := SidecarPath(mdPath)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating sidecar directory: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".sbdb-sidecar-*.yaml.tmp")
	if err != nil {
		return fmt.Errorf("creating sidecar temp: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("writing sidecar: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming sidecar: %w", err)
	}
	return nil
}

// SignWith returns the HMAC-SHA-256 sig over the three SHAs concatenated.
// Caller assigns result to s.Sig.
func (s *Sidecar) SignWith(key []byte) string {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(s.ContentSHA))
	h.Write([]byte("\n"))
	h.Write([]byte(s.FrontmatterSHA))
	h.Write([]byte("\n"))
	h.Write([]byte(s.RecordSHA))
	return hex.EncodeToString(h.Sum(nil))
}

// VerifyWith checks the stored Sig against the three SHAs and the given key.
func (s *Sidecar) VerifyWith(key []byte) bool {
	if s.Sig == "" {
		return false
	}
	expected := s.SignWith(key)
	return hmac.Equal([]byte(expected), []byte(s.Sig))
}

// Drift describes mismatches between a sidecar and the on-disk doc.
type Drift struct {
	MissingSidecar    bool
	MissingMD         bool
	ContentDrift      bool
	FrontmatterDrift  bool
	RecordDrift       bool
	BadSig            bool
}

// Any returns true if any drift bit is set.
func (d Drift) Any() bool {
	return d.MissingSidecar || d.MissingMD || d.ContentDrift ||
		d.FrontmatterDrift || d.RecordDrift || d.BadSig
}

// Verify recomputes hashes from the supplied frontmatter/body/record and
// compares against the sidecar. If key is non-nil and sidecar has HMAC,
// also verifies the signature.
func (s *Sidecar) Verify(mdPath string, fm map[string]any, body string, record map[string]any, key []byte) (Drift, error) {
	var d Drift
	if HashContent(body) != s.ContentSHA {
		d.ContentDrift = true
	}
	if HashFrontmatter(fm) != s.FrontmatterSHA {
		d.FrontmatterDrift = true
	}
	if HashRecord(record) != s.RecordSHA {
		d.RecordDrift = true
	}
	if s.HMAC && key != nil && !s.VerifyWith(key) {
		d.BadSig = true
	}
	return d, nil
}
```

- [ ] **Step 4: Run tests; verify PASS**

```
go test ./internal/integrity/ -run TestSidecar -v
```

Expected: all 7 tests PASS.

- [ ] **Step 5: Commit**

```
git add internal/integrity/sidecar.go internal/integrity/sidecar_test.go
git commit -m "feat(integrity): add per-doc sidecar (Sidecar struct, atomic save, drift, HMAC)

Per-md sidecar file written at <id>.yaml (sibling of <id>.md). Holds
content/frontmatter/record SHAs + optional HMAC sig. Atomic write via
temp+rename. Verify reports drift bits: missing-sidecar / missing-md /
content / frontmatter / record / bad-sig. Additive; no call sites yet."
```

---

## Task 3: Add `internal/integrity/gitscope.go` (working-tree filter)

**Files:**
- Create: `internal/integrity/gitscope.go`
- Test: `internal/integrity/gitscope_test.go`

- [ ] **Step 1: Write failing tests**

`internal/integrity/gitscope_test.go`:

```go
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

	// Modify committed file
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte("hello"), 0o644))
	// Add untracked file
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
```

- [ ] **Step 2: Run tests; verify FAIL**

```
go test ./internal/integrity/ -run TestGitScope -v
```

Expected: build failure — `NewGitScope` undefined.

- [ ] **Step 3: Implement git scope**

`internal/integrity/gitscope.go`:

```go
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
	root, err := gitTopLevel(dir)
	if err != nil {
		return &GitScope{IsRepo: false}, nil
	}

	porcelain, err := gitPorcelain(dir)
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
	cmd := exec.Command("git", "-C", dir, "status", "--porcelain", "-z")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git status failed: %w", err)
	}
	// -z uses NUL separators; convert back to lines for the simple parser
	// above. We split on NUL and re-join with \n, which is fine for
	// non-rename entries; rename entries contain "old\0new" which our
	// caller already handles via the " -> " sentinel — but with -z that
	// sentinel does not appear. Drop -z and re-fetch without it for
	// simplicity.
	cmd2 := exec.Command("git", "-C", dir, "status", "--porcelain")
	out.Reset()
	cmd2.Stdout = &out
	if err := cmd2.Run(); err != nil {
		return "", fmt.Errorf("git status failed: %w", err)
	}
	return out.String(), nil
}
```

- [ ] **Step 4: Run tests; verify PASS**

```
go test ./internal/integrity/ -run TestGitScope -v
```

Expected: all 5 tests PASS.

- [ ] **Step 5: Commit**

```
git add internal/integrity/gitscope.go internal/integrity/gitscope_test.go
git commit -m "feat(integrity): add GitScope for working-tree-change filter

Shells out to \`git status --porcelain\` to enumerate
modified+staged+untracked paths. Treats non-git dirs as IsRepo=false
with no error. PairScopedPaths pulls in the matching sidecar/.md so
drift between paired files is always detected. Used by doctor for
default uncommitted-only scope; --all bypasses."
```

---

## Task 4: Feature flag + sidecar write path in `Document.Save`/`Document.Delete`

**Files:**
- Modify: `internal/document/save.go`
- Test: `internal/document/save_test.go` (extend existing)

The flag is `SBDB_USE_SIDECAR=1`. While the flag is the opt-in, both write paths
run (legacy records.yaml + manifest AND sidecar). On the read side (Task 5), the
flag chooses one path. After Task 9 the legacy is removed entirely.

- [ ] **Step 1: Write failing test for sidecar write**

Append to `internal/document/save_test.go`:

```go
func TestSave_WritesSidecar_WhenFlagSet(t *testing.T) {
	t.Setenv("SBDB_USE_SIDECAR", "1")
	d, basePath := setupSaveDoc(t) // existing helper that builds a notes Document
	d.Data["id"] = "alpha"
	d.Data["created"] = "2026-04-28"
	d.Content = "# Alpha"

	require.NoError(t, d.Save(nil))

	mdPath := filepath.Join(basePath, "docs/notes/alpha.md")
	sidecarPath := filepath.Join(basePath, "docs/notes/alpha.yaml")
	assert.FileExists(t, mdPath)
	assert.FileExists(t, sidecarPath)

	sc, err := integrity.LoadSidecar(mdPath)
	require.NoError(t, err)
	assert.Equal(t, "alpha.md", sc.File)
	assert.NotEmpty(t, sc.ContentSHA)
}

func TestDelete_RemovesSidecar_WhenFlagSet(t *testing.T) {
	t.Setenv("SBDB_USE_SIDECAR", "1")
	d, basePath := setupSaveDoc(t)
	d.Data["id"] = "alpha"
	d.Data["created"] = "2026-04-28"
	d.Content = "# Alpha"
	require.NoError(t, d.Save(nil))

	require.NoError(t, d.Delete())

	assert.NoFileExists(t, filepath.Join(basePath, "docs/notes/alpha.md"))
	assert.NoFileExists(t, filepath.Join(basePath, "docs/notes/alpha.yaml"))
}
```

If `setupSaveDoc` does not exist, create it at the top of the test file:

```go
func setupSaveDoc(t *testing.T) (*Document, string) {
	t.Helper()
	basePath := t.TempDir()
	s := &schema.Schema{
		Version: 1, Entity: "notes",
		DocsDir:    "docs/notes",
		Filename:   "{id}.md",
		RecordsDir: "data/notes",
		IDField:    "id",
		Integrity:  "off",
		Fields: schema.FieldMap{
			"id":      {Type: "string", Required: true},
			"created": {Type: "date", Required: true},
		},
	}
	d := New(s, basePath)
	return d, basePath
}
```

- [ ] **Step 2: Run; verify FAIL**

```
go test ./internal/document/ -run TestSave_WritesSidecar -v
```

Expected: assertion fails — sidecar not written.

- [ ] **Step 3: Plumb sidecar into Save**

In `internal/document/save.go`, replace `Save` with:

```go
func (d *Document) Save(rt *virtuals.Runtime) error {
	if rt != nil && len(d.Schema.Virtuals) > 0 {
		vResults, err := rt.EvaluateAll(d.Content, d.Data)
		if err != nil {
			return fmt.Errorf("evaluating virtuals: %w", err)
		}
		d.SetVirtuals(vResults)
	}

	fmData := schema.BuildFrontmatterData(d.Schema, d.Data, d.virtuals)
	recordData := schema.BuildRecordData(d.Schema, d.Data, d.virtuals)
	recordData["file"] = d.RelativeFilePath()

	mdPath := d.FilePath()
	if err := storage.WriteMarkdown(mdPath, fmData, d.Content); err != nil {
		return fmt.Errorf("writing markdown: %w", err)
	}

	if useSidecar() {
		if err := d.writeSidecar(mdPath, fmData, recordData); err != nil {
			return fmt.Errorf("writing sidecar: %w", err)
		}
	} else {
		if err := d.writeLegacyRecordsAndManifest(fmData, recordData); err != nil {
			return err
		}
	}

	if d.OnSave != nil {
		if err := d.OnSave(d); err != nil {
			fmt.Fprintf(os.Stderr, "warning: post-save hook failed for %s: %v\n", d.ID(), err)
		}
	}
	return nil
}

func useSidecar() bool {
	return os.Getenv("SBDB_USE_SIDECAR") == "1"
}

func (d *Document) writeSidecar(mdPath string, fmData, recordData map[string]any) error {
	sc := &integrity.Sidecar{
		Version:        1,
		Algo:           "sha256",
		File:           filepath.Base(mdPath),
		ContentSHA:     integrity.HashContent(d.Content),
		FrontmatterSHA: integrity.HashFrontmatter(fmData),
		RecordSHA:      integrity.HashRecord(recordData),
	}
	key, err := integrity.LoadKey()
	if err != nil {
		return fmt.Errorf("loading integrity key: %w", err)
	}
	if key != nil {
		sc.HMAC = true
		sc.Sig = sc.SignWith(key)
	}
	return sc.Save(mdPath)
}

func (d *Document) writeLegacyRecordsAndManifest(fmData, recordData map[string]any) error {
	recordsPath, err := storage.RecordsPathForPartition(
		d.RecordsDir(), d.Schema.Partition, d.Schema.DateField, d.Data,
	)
	if err != nil {
		return fmt.Errorf("resolving records path: %w", err)
	}
	records, err := storage.LoadRecords(recordsPath)
	if err != nil {
		return fmt.Errorf("loading records: %w", err)
	}
	records = storage.UpsertRecord(records, recordData, d.Schema.IDField)
	if err := storage.SaveRecords(recordsPath, records); err != nil {
		return fmt.Errorf("saving records: %w", err)
	}
	if err := d.updateManifest(fmData, recordData); err != nil {
		return fmt.Errorf("updating manifest: %w", err)
	}
	return nil
}
```

Add `path/filepath` to imports. Replace `Delete` with:

```go
func (d *Document) Delete() error {
	id := d.ID()
	mdPath := d.FilePath()
	if err := removeIfExists(mdPath); err != nil {
		return fmt.Errorf("deleting markdown file: %w", err)
	}

	if useSidecar() {
		if err := removeIfExists(integrity.SidecarPath(mdPath)); err != nil {
			return fmt.Errorf("deleting sidecar: %w", err)
		}
	} else {
		recordsPath, err := storage.RecordsPathForPartition(
			d.RecordsDir(), d.Schema.Partition, d.Schema.DateField, d.Data,
		)
		if err != nil {
			return fmt.Errorf("resolving records path: %w", err)
		}
		records, err := storage.LoadRecords(recordsPath)
		if err != nil {
			return fmt.Errorf("loading records: %w", err)
		}
		records, _ = storage.RemoveRecord(records, d.Schema.IDField, id)
		if err := storage.SaveRecords(recordsPath, records); err != nil {
			return fmt.Errorf("saving records after delete: %w", err)
		}
		manifest, err := integrity.LoadManifest(d.RecordsDir())
		if err != nil {
			return fmt.Errorf("loading manifest: %w", err)
		}
		manifest.RemoveEntry(id)
		if err := manifest.Save(d.RecordsDir()); err != nil {
			return fmt.Errorf("saving manifest: %w", err)
		}
	}

	if d.OnDelete != nil {
		if err := d.OnDelete(id); err != nil {
			fmt.Fprintf(os.Stderr, "warning: post-delete hook failed for %s: %v\n", id, err)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run; verify PASS**

```
go test ./internal/document/ -v
```

Expected: existing tests still pass; new `TestSave_WritesSidecar` and `TestDelete_RemovesSidecar` PASS.

- [ ] **Step 5: Commit**

```
git add internal/document/save.go internal/document/save_test.go
git commit -m "feat(document): plumb sidecar into Save/Delete behind SBDB_USE_SIDECAR=1

When the flag is set, Save writes <id>.yaml next to <id>.md and skips
records.yaml + .integrity.yaml; Delete removes the sidecar instead of
mutating the aggregates. Default behavior unchanged. Both paths share
filesystem effects on .md so the markdown file is always the same."
```

---

## Task 5: Walker-based read path in `QuerySet.loadRecords` (behind flag)

**Files:**
- Modify: `internal/query/queryset.go`
- Test: `internal/query/queryset_sidecar_test.go` (new)

- [ ] **Step 1: Write failing test**

`internal/query/queryset_sidecar_test.go`:

```go
package query

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sergio-bershadsky/secondbrain-db/internal/schema"
)

func TestQuerySet_Records_SidecarMode_WalksMD(t *testing.T) {
	t.Setenv("SBDB_USE_SIDECAR", "1")
	basePath := t.TempDir()
	docs := filepath.Join(basePath, "docs/notes")
	require.NoError(t, os.MkdirAll(docs, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(docs, "alpha.md"),
		[]byte("---\nid: alpha\nstatus: active\ncreated: 2026-04-28\n---\n# A"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(docs, "beta.md"),
		[]byte("---\nid: beta\nstatus: archived\ncreated: 2026-04-29\n---\n# B"), 0o644))

	s := &schema.Schema{
		Entity: "notes", DocsDir: "docs/notes", IDField: "id",
		Filename: "{id}.md",
		Fields: schema.FieldMap{
			"id":      {Type: "string", Required: true},
			"status":  {Type: "string"},
			"created": {Type: "date"},
		},
	}

	qs := NewQuerySet(s, basePath)
	got, err := qs.Records()
	require.NoError(t, err)

	ids := []string{}
	for _, r := range got {
		ids = append(ids, r["id"].(string))
	}
	assert.ElementsMatch(t, []string{"alpha", "beta"}, ids)
}
```

- [ ] **Step 2: Run; verify FAIL**

```
go test ./internal/query/ -run TestQuerySet_Records_SidecarMode -v
```

Expected: 0 records returned (legacy path reads empty `data/notes/records.yaml`).

- [ ] **Step 3: Branch `loadRecords` on flag**

In `internal/query/queryset.go`, replace `loadRecords`:

```go
func (qs *QuerySet) loadRecords() ([]map[string]any, error) {
	if os.Getenv("SBDB_USE_SIDECAR") == "1" {
		return qs.loadRecordsViaWalker()
	}
	recordsDir := filepath.Join(qs.basePath, qs.schema.RecordsDir)
	return storage.LoadAllPartitions(recordsDir, qs.schema.Partition)
}

func (qs *QuerySet) loadRecordsViaWalker() ([]map[string]any, error) {
	docsDir := filepath.Join(qs.basePath, qs.schema.DocsDir)
	docs, err := storage.WalkDocsToSlice(docsDir)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(docs))
	for _, d := range docs {
		// Project frontmatter through the schema's record shape.
		rec := schema.BuildRecordData(qs.schema, d.Frontmatter, nil)
		// File path relative to base, like the legacy records had.
		if rel, err := filepath.Rel(qs.basePath, d.Path); err == nil {
			rec["file"] = rel
		}
		out = append(out, rec)
	}
	return out, nil
}
```

Add imports: `"os"`, `"path/filepath"`, `"github.com/sergio-bershadsky/secondbrain-db/internal/schema"`.

- [ ] **Step 4: Run; verify PASS**

```
go test ./internal/query/ -v
```

Expected: existing tests pass; new sidecar-mode test PASSES.

- [ ] **Step 5: Commit**

```
git add internal/query/queryset.go internal/query/queryset_sidecar_test.go
git commit -m "feat(query): walker-based loadRecords behind SBDB_USE_SIDECAR=1

When the flag is set, QuerySet.loadRecords walks docs_dir, parses
each .md frontmatter, and projects through BuildRecordData — same
shape as the legacy records.yaml payload. file path is relative to
basePath. Default path (records.yaml) unchanged."
```

---

## Task 6: Doctor — sidecar-aware `check` with default uncommitted scope and `--all`

**Files:**
- Modify: `cmd/doctor.go` (rewrite the `check` subcommand)
- Test: `cmd/doctor_test.go` (new file)

The legacy doctor path stays in place (untouched) until Task 9 flips the
default. This task adds a new `runDoctorCheckV2` invoked when the flag is set.

- [ ] **Step 1: Write failing test**

`cmd/doctor_test.go`:

```go
package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeMD(t *testing.T, path, fm, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	full := "---\n" + fm + "\n---\n" + body
	require.NoError(t, os.WriteFile(path, []byte(full), 0o644))
}

func TestDoctorCheck_SidecarMode_CleanRepo(t *testing.T) {
	t.Setenv("SBDB_USE_SIDECAR", "1")
	dir := setupV2KB(t) // creates schemas/notes.yaml + .sbdb.toml + one valid <id>.md+<id>.yaml

	out, code := runCLI(t, dir, "doctor", "check", "--all", "-s", "notes")
	require.Equal(t, 0, code, "stdout: %s", out)
}

func TestDoctorCheck_SidecarMode_DetectsContentDrift(t *testing.T) {
	t.Setenv("SBDB_USE_SIDECAR", "1")
	dir := setupV2KB(t)
	mdPath := filepath.Join(dir, "docs/notes/alpha.md")
	require.NoError(t, os.WriteFile(mdPath, []byte("---\nid: alpha\ncreated: 2026-04-28\n---\nTAMPERED"), 0o644))

	out, code := runCLI(t, dir, "doctor", "check", "--all", "-s", "notes")
	require.NotEqual(t, 0, code)
	assert.Contains(t, out, "content_sha mismatch")
}
```

`runCLI` and `setupV2KB` are helpers; add them at top of `cmd/doctor_test.go`:

```go
func runCLI(t *testing.T, dir string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(sbdbBin(t), args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return string(out), exitErr.ExitCode()
		}
		t.Fatalf("running sbdb: %v", err)
	}
	return string(out), 0
}

func sbdbBin(t *testing.T) string {
	// Build once per package run.
	t.Helper()
	bin := filepath.Join(t.TempDir(), "sbdb")
	cmd := exec.Command("go", "build", "-o", bin, "github.com/sergio-bershadsky/secondbrain-db")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "build sbdb: %s", out)
	return bin
}

func setupV2KB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	schemaYAML := `version: 1
entity: notes
docs_dir: docs/notes
filename: "{id}.md"
id_field: id
integrity: off
fields:
  id: { type: string, required: true }
  created: { type: date, required: true }
`
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "schemas"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "schemas/notes.yaml"), []byte(schemaYAML), 0o644))
	tomlContent := `schema_dir = "./schemas"
base_path = "."
[output]
format = "json"
[integrity]
key_source = "env"
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".sbdb.toml"), []byte(tomlContent), 0o644))

	// Use the binary to create one doc so its sidecar is consistent.
	cmd := exec.Command(sbdbBin(t), "create", "-s", "notes",
		"--field", "id=alpha",
		"--field", "created=2026-04-28",
		"--content", "# Alpha")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "SBDB_USE_SIDECAR=1")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "create: %s", out)
	return dir
}
```

Add imports `"os/exec"` to the test file.

- [ ] **Step 2: Run; verify FAIL**

```
go test ./cmd/ -run TestDoctorCheck_SidecarMode -v
```

Expected: tests fail because doctor still reads the manifest (or because `--all` flag is unknown).

- [ ] **Step 3: Implement v2 doctor check**

In `cmd/doctor.go`, add `--all` flag and branch on `useSidecar()`:

```go
var doctorAll bool

func init() {
	// (existing init body remains)
	doctorCheckCmd.Flags().BoolVar(&doctorAll, "all", false, "audit all docs, not just uncommitted changes")
}
```

Inside `runDoctorCheck` (or wherever the existing entry point lives), prepend:

```go
if os.Getenv("SBDB_USE_SIDECAR") == "1" {
	return runDoctorCheckV2(cmd, args)
}
```

Add:

```go
func runDoctorCheckV2(cmd *cobra.Command, _ []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}
	s, err := loadSchema(cfg)
	if err != nil {
		return err
	}
	docsDir := filepath.Join(cfg.BasePath, s.DocsDir)

	paths, err := scopedDocPaths(cfg.BasePath, docsDir, doctorAll)
	if err != nil {
		return err
	}

	key, _ := integrity.LoadKey()
	var drifts []map[string]any
	for _, mdPath := range paths {
		report := checkOneDoc(s, cfg.BasePath, mdPath, key)
		if report != nil {
			drifts = append(drifts, report)
		}
	}

	format := outputFormat(cfg)
	_ = output.PrintData(format, map[string]any{
		"action": "doctor.check",
		"scope":  scopeLabel(doctorAll),
		"drifts": drifts,
	})
	if len(drifts) > 0 {
		os.Exit(1)
	}
	return nil
}

func scopedDocPaths(basePath, docsDir string, all bool) ([]string, error) {
	if all {
		return collectMDsFromWalker(docsDir)
	}
	scope, err := integrity.NewGitScope(basePath)
	if err != nil {
		return nil, err
	}
	if !scope.IsRepo {
		fmt.Fprintln(os.Stderr, "not a git repo; falling back to --all")
		return collectMDsFromWalker(docsDir)
	}
	var out []string
	for _, p := range scope.PairScopedPaths() {
		if strings.HasSuffix(p, ".md") && strings.HasPrefix(p, docsDir+string(filepath.Separator)) {
			out = append(out, p)
		}
	}
	return out, nil
}

func collectMDsFromWalker(docsDir string) ([]string, error) {
	docs, err := storage.WalkDocsToSlice(docsDir)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(docs))
	for _, d := range docs {
		out = append(out, d.Path)
	}
	return out, nil
}

func scopeLabel(all bool) string {
	if all {
		return "all"
	}
	return "uncommitted"
}

func checkOneDoc(s *schema.Schema, basePath, mdPath string, key []byte) map[string]any {
	mdExists := fileExists(mdPath)
	sidecarPath := integrity.SidecarPath(mdPath)
	sc, err := integrity.LoadSidecar(mdPath)

	switch {
	case !mdExists && err == nil:
		return map[string]any{"file": relPath(basePath, mdPath), "drift": "missing-md"}
	case mdExists && os.IsNotExist(err):
		return map[string]any{"file": relPath(basePath, mdPath), "drift": "missing-sidecar"}
	case err != nil:
		return map[string]any{"file": relPath(basePath, mdPath), "drift": "sidecar-parse-error", "error": err.Error()}
	}

	fm, body, perr := storage.ParseMarkdown(mdPath)
	if perr != nil {
		return map[string]any{"file": relPath(basePath, mdPath), "drift": "md-parse-error", "error": perr.Error()}
	}
	rec := schema.BuildRecordData(s, fm, nil)
	if rel, e := filepath.Rel(basePath, mdPath); e == nil {
		rec["file"] = rel
	}
	d, _ := sc.Verify(mdPath, fm, body, rec, key)
	if !d.Any() {
		return nil
	}
	driftLabels := []string{}
	if d.ContentDrift {
		driftLabels = append(driftLabels, "content_sha mismatch")
	}
	if d.FrontmatterDrift {
		driftLabels = append(driftLabels, "frontmatter_sha mismatch")
	}
	if d.RecordDrift {
		driftLabels = append(driftLabels, "record_sha mismatch")
	}
	if d.BadSig {
		driftLabels = append(driftLabels, "bad_sig")
	}
	_ = sidecarPath
	return map[string]any{
		"file":   relPath(basePath, mdPath),
		"drift":  "tamper",
		"causes": driftLabels,
	}
}

func relPath(base, path string) string {
	if rel, err := filepath.Rel(base, path); err == nil {
		return rel
	}
	return path
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
```

Add imports: `"strings"`, `"github.com/sergio-bershadsky/secondbrain-db/internal/storage"`, etc. as needed.

- [ ] **Step 4: Run; verify PASS**

```
go build ./... && go test ./cmd/ -run TestDoctorCheck_SidecarMode -v
```

Expected: both tests PASS.

- [ ] **Step 5: Commit**

```
git add cmd/doctor.go cmd/doctor_test.go
git commit -m "feat(doctor): sidecar-aware check with --all and uncommitted-only default

When SBDB_USE_SIDECAR=1, doctor check walks the configured scope
(default: working-tree changes via GitScope; --all overrides) and
verifies each .md against its <id>.yaml sidecar. Reports
missing-md / missing-sidecar / content / frontmatter / record / bad-sig.
Non-git fallback to --all with stderr notice."
```

---

## Task 7: Doctor — sidecar-aware `fix` and `sign`

**Files:**
- Modify: `cmd/doctor.go` (extend)
- Test: `cmd/doctor_test.go` (extend)

- [ ] **Step 1: Write failing tests**

```go
func TestDoctorFix_SidecarMode_RewritesSidecar(t *testing.T) {
	t.Setenv("SBDB_USE_SIDECAR", "1")
	dir := setupV2KB(t)
	// Edit md out of band
	mdPath := filepath.Join(dir, "docs/notes/alpha.md")
	require.NoError(t, os.WriteFile(mdPath, []byte("---\nid: alpha\ncreated: 2026-04-28\n---\nNEW"), 0o644))

	out, code := runCLI(t, dir, "doctor", "fix", "--recompute", "--all", "-s", "notes")
	require.Equal(t, 0, code, "stdout: %s", out)

	// Now check should pass.
	_, code2 := runCLI(t, dir, "doctor", "check", "--all", "-s", "notes")
	require.Equal(t, 0, code2)
}
```

- [ ] **Step 2: Run; verify FAIL**

```
go test ./cmd/ -run TestDoctorFix_SidecarMode -v
```

Expected: FAIL — fix is still legacy.

- [ ] **Step 3: Branch `fix` and `sign` on flag**

In `cmd/doctor.go`, near the existing `runDoctorFix`, add:

```go
func runDoctorFixV2(cmd *cobra.Command, _ []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}
	s, err := loadSchema(cfg)
	if err != nil {
		return err
	}
	docsDir := filepath.Join(cfg.BasePath, s.DocsDir)
	paths, err := scopedDocPaths(cfg.BasePath, docsDir, doctorAll)
	if err != nil {
		return err
	}
	key, _ := integrity.LoadKey()
	fixed := 0
	for _, mdPath := range paths {
		fm, body, err := storage.ParseMarkdown(mdPath)
		if err != nil {
			continue
		}
		rec := schema.BuildRecordData(s, fm, nil)
		if rel, e := filepath.Rel(cfg.BasePath, mdPath); e == nil {
			rec["file"] = rel
		}
		sc := &integrity.Sidecar{
			Version: 1, Algo: "sha256",
			File:           filepath.Base(mdPath),
			ContentSHA:     integrity.HashContent(body),
			FrontmatterSHA: integrity.HashFrontmatter(fm),
			RecordSHA:      integrity.HashRecord(rec),
		}
		if key != nil {
			sc.HMAC = true
			sc.Sig = sc.SignWith(key)
		}
		if err := sc.Save(mdPath); err != nil {
			return err
		}
		fixed++
	}
	return output.PrintData(outputFormat(cfg), map[string]any{
		"action": "doctor.fix",
		"fixed":  fixed,
		"scope":  scopeLabel(doctorAll),
	})
}
```

Add an early-exit branch in the existing `runDoctorFix` and the existing `runDoctorSign`:

```go
if os.Getenv("SBDB_USE_SIDECAR") == "1" {
	return runDoctorFixV2(cmd, args)
}
```

`runDoctorSignV2` is structurally identical to `runDoctorFixV2` but only writes when `key != nil`; if no key is configured, return an error `"sign requires an HMAC key; run sbdb doctor init-key"`.

- [ ] **Step 4: Run; verify PASS**

```
go test ./cmd/ -run TestDoctorFix_SidecarMode -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```
git add cmd/doctor.go cmd/doctor_test.go
git commit -m "feat(doctor): sidecar-aware fix and sign

When SBDB_USE_SIDECAR=1, fix --recompute walks the scoped paths and
rewrites each sidecar from the on-disk markdown. sign --force does
the same, requiring an HMAC key. Both honour --all and the
default-uncommitted scope from Task 6."
```

---

## Task 8: Doctor — `migrate` v1 → v2 subcommand

**Files:**
- Modify: `cmd/doctor.go`
- Test: `cmd/doctor_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestDoctorMigrate_ConvertsV1ToV2(t *testing.T) {
	dir := setupV1KB(t) // creates a v1 layout: docs + data/<entity>/records.yaml + .integrity.yaml
	t.Setenv("SBDB_USE_SIDECAR", "1") // migrate is independent of flag, but post-checks need it

	out, code := runCLI(t, dir, "doctor", "migrate")
	require.Equal(t, 0, code, "stdout: %s", out)

	assert.NoDirExists(t, filepath.Join(dir, "data"))
	assert.FileExists(t, filepath.Join(dir, "docs/notes/alpha.yaml"))

	// Idempotent
	_, code2 := runCLI(t, dir, "doctor", "migrate")
	assert.Equal(t, 0, code2)

	// doctor check is clean
	_, code3 := runCLI(t, dir, "doctor", "check", "--all", "-s", "notes")
	assert.Equal(t, 0, code3)
}
```

`setupV1KB` creates the legacy structure manually (no need to invoke a v1 binary):

```go
func setupV1KB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	schemaYAML := `version: 1
entity: notes
docs_dir: docs/notes
filename: "{id}.md"
records_dir: data/notes
id_field: id
integrity: off
fields:
  id: { type: string, required: true }
  created: { type: date, required: true }
`
	tomlContent := `schema_dir = "./schemas"
base_path = "."
[output]
format = "json"
[integrity]
key_source = "env"
`
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "schemas"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs/notes"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "data/notes"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "schemas/notes.yaml"), []byte(schemaYAML), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".sbdb.toml"), []byte(tomlContent), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs/notes/alpha.md"),
		[]byte("---\nid: alpha\ncreated: 2026-04-28\n---\n# Alpha"), 0o644))

	// Pre-existing aggregate records + manifest:
	require.NoError(t, os.WriteFile(filepath.Join(dir, "data/notes/records.yaml"),
		[]byte(`- id: alpha
  created: 2026-04-28
  file: docs/notes/alpha.md
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "data/notes/.integrity.yaml"),
		[]byte(`version: 1
algo: sha256
hmac: false
entries:
  alpha:
    file: docs/notes/alpha.md
    content_sha: PLACEHOLDER
    frontmatter_sha: PLACEHOLDER
    record_sha: PLACEHOLDER
    updated_at: 2026-04-28T00:00:00Z
    writer: secondbrain-db/v1
`), 0o644))
	return dir
}
```

- [ ] **Step 2: Run; verify FAIL**

```
go test ./cmd/ -run TestDoctorMigrate -v
```

Expected: FAIL — `migrate` subcommand does not exist.

- [ ] **Step 3: Implement migrate**

Add to `cmd/doctor.go`:

```go
var doctorMigrateDryRun bool

var doctorMigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate v1 (data/) layout to v2 (per-md sidecars)",
	RunE:  runDoctorMigrate,
}

func init() {
	doctorMigrateCmd.Flags().BoolVar(&doctorMigrateDryRun, "dry-run", false, "report planned changes without writing")
	doctorCmd.AddCommand(doctorMigrateCmd)
}

func runDoctorMigrate(cmd *cobra.Command, _ []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}
	schemas, err := loadAllSchemas(cfg) // helper that returns all schemas under schema_dir
	if err != nil {
		return err
	}

	dataDir := filepath.Join(cfg.BasePath, "data")
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		return output.PrintData(outputFormat(cfg), map[string]any{
			"action": "doctor.migrate",
			"result": "already-v2",
		})
	}

	migrated := 0
	for _, s := range schemas {
		recordsDir := filepath.Join(cfg.BasePath, s.RecordsDir)
		if recordsDir == "" {
			continue
		}
		// Load aggregate records (handles partition: monthly transparently)
		records, err := storage.LoadAllPartitions(recordsDir, s.Partition)
		if err != nil {
			return fmt.Errorf("loading legacy records for %s: %w", s.Entity, err)
		}
		manifest, err := integrity.LoadManifest(recordsDir)
		if err != nil {
			return fmt.Errorf("loading legacy manifest for %s: %w", s.Entity, err)
		}
		for _, rec := range records {
			id := fmt.Sprintf("%v", rec[s.IDField])
			file, _ := rec["file"].(string)
			if file == "" {
				continue
			}
			mdPath := filepath.Join(cfg.BasePath, file)
			entry := manifest.Entries[id]
			sc := buildSidecarFromV1(rec, entry, mdPath)
			if doctorMigrateDryRun {
				continue
			}
			if err := sc.Save(mdPath); err != nil {
				return fmt.Errorf("writing sidecar for %s: %w", id, err)
			}
			migrated++
		}
	}

	if !doctorMigrateDryRun {
		if err := os.RemoveAll(dataDir); err != nil {
			return fmt.Errorf("removing legacy data/: %w", err)
		}
	}

	return output.PrintData(outputFormat(cfg), map[string]any{
		"action":   "doctor.migrate",
		"migrated": migrated,
		"dry_run":  doctorMigrateDryRun,
	})
}

func buildSidecarFromV1(rec map[string]any, entry *integrity.Entry, mdPath string) *integrity.Sidecar {
	sc := &integrity.Sidecar{
		Version: 1, Algo: "sha256",
		File: filepath.Base(mdPath),
	}
	if entry != nil {
		sc.ContentSHA = entry.ContentSHA
		sc.FrontmatterSHA = entry.FrontmatterSHA
		sc.RecordSHA = entry.RecordSHA
		sc.Sig = entry.Sig
		sc.HMAC = entry.Sig != ""
		sc.UpdatedAt = entry.UpdatedAt
		sc.Writer = entry.Writer
	} else {
		// No manifest entry — recompute from disk.
		fm, body, err := storage.ParseMarkdown(mdPath)
		if err == nil {
			sc.ContentSHA = integrity.HashContent(body)
			sc.FrontmatterSHA = integrity.HashFrontmatter(fm)
			sc.RecordSHA = integrity.HashRecord(rec)
		}
	}
	return sc
}
```

`loadAllSchemas` helper (add to `cmd/config.go` or near `loadSchema`):

```go
func loadAllSchemas(cfg *config.Config) ([]*schema.Schema, error) {
	dir := filepath.Join(cfg.BasePath, cfg.SchemaDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []*schema.Schema
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		s, err := schema.LoadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		out = append(out, s)
	}
	return out, nil
}
```

- [ ] **Step 4: Run; verify PASS**

```
go test ./cmd/ -run TestDoctorMigrate -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```
git add cmd/doctor.go cmd/doctor_test.go cmd/config.go
git commit -m "feat(doctor): add migrate subcommand for v1 → v2 layout

Reads each entity's legacy records.yaml (or YYYY-MM.yaml partitions)
and .integrity.yaml, writes a per-doc <id>.yaml sidecar next to each
.md, then removes data/. Idempotent: a no-op on already-v2 trees.
--dry-run reports planned actions without writing."
```

---

## Task 9: Flip the default — sidecar is on, legacy is dead code

**Files:**
- Modify: many call sites (remove the flag check, keep only the v2 path)
- Delete: `internal/storage/records.go`, `internal/storage/partition.go`, `internal/integrity/manifest.go`, related tests

- [ ] **Step 1: Run the suite end-to-end with the flag set, confirm green**

```
SBDB_USE_SIDECAR=1 go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 2: Remove the flag check from `Document.Save/Delete`**

In `internal/document/save.go`: drop `if useSidecar()` branches; keep only the sidecar path. Delete `useSidecar()`, `writeLegacyRecordsAndManifest`. Delete `updateManifest`.

- [ ] **Step 3: Remove the flag check from `QuerySet.loadRecords`**

In `internal/query/queryset.go`: replace `loadRecords` with the body of `loadRecordsViaWalker`. Delete the helper.

- [ ] **Step 4: Remove the flag check from doctor**

In `cmd/doctor.go`: drop the `if os.Getenv("SBDB_USE_SIDECAR")...` branch in each subcommand and have them directly call the V2 implementations. Drop the V1 implementations entirely (`runDoctorCheck`, `runDoctorFix`, `runDoctorSign` legacy bodies). Rename `runDoctorCheckV2` → `runDoctorCheck`, etc.

- [ ] **Step 5: Delete legacy storage and integrity files**

```
git rm internal/storage/records.go internal/storage/records_test.go
git rm internal/storage/partition.go internal/storage/partition_test.go
git rm internal/integrity/manifest.go internal/integrity/manifest_test.go
```

If anything else still references `storage.LoadRecords`, `storage.SaveRecords`, `storage.LoadAllPartitions`, `storage.RecordsPathForPartition`, `storage.RemoveRecord`, `storage.UpsertRecord`, or `integrity.Manifest*`/`integrity.LoadManifest`, follow the compile errors and remove or replace.

- [ ] **Step 6: Run full suite**

```
go build ./... && go test ./... -count=1
```

Expected: green. If anything in `cmd/index.go`, `cmd/doctor.go`, `internal/kg/`, or e2e still calls a deleted function, fix the call site (KG indexer should walk via `storage.WalkDocsToSlice` — same iteration as `loadRecordsViaWalker`).

- [ ] **Step 7: Commit**

```
git add -A
git commit -m "refactor: flip sidecar default, delete legacy data/ + manifest paths

Removes the SBDB_USE_SIDECAR opt-in. Document.Save/Delete writes
sidecars unconditionally; QuerySet walks docs_dir; doctor verifies
sidecars. Deletes internal/storage/records.go, partition.go,
integrity/manifest.go and their tests. Any remaining legacy callers
(KG indexer, doctor, e2e) move to walker."
```

---

## Task 10: Schema loader — deprecate `records_dir` and `partition`

**Files:**
- Modify: `internal/schema/loader.go`
- Test: `internal/schema/loader_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestLoad_DeprecationWarnings(t *testing.T) {
	yamlData := `version: 1
entity: notes
docs_dir: docs/notes
filename: "{id}.md"
id_field: id
integrity: off
records_dir: data/notes
partition: monthly
date_field: created
fields:
  id: { type: string, required: true }
  created: { type: date, required: true }
`
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.yaml")
	require.NoError(t, os.WriteFile(path, []byte(yamlData), 0o644))

	stderr := captureStderr(t, func() {
		_, err := schema.LoadFile(path)
		require.NoError(t, err)
	})
	assert.Contains(t, stderr, "records_dir")
	assert.Contains(t, stderr, "partition")
	assert.Contains(t, stderr, "deprecated")
}
```

`captureStderr` is a small helper using `os.Pipe`; if it doesn't exist add it in the same test file.

- [ ] **Step 2: Run; verify FAIL**

```
go test ./internal/schema/ -run TestLoad_DeprecationWarnings -v
```

Expected: FAIL.

- [ ] **Step 3: Add warnings in loader**

In `internal/schema/loader.go`, after the YAML unmarshal but before validation:

```go
if s.RecordsDir != "" {
	fmt.Fprintf(os.Stderr, "%s: 'records_dir' is deprecated and ignored in v2; remove it\n", path)
}
if s.Partition != "" && s.Partition != "none" {
	fmt.Fprintf(os.Stderr, "%s: 'partition' is deprecated; v2 has no aggregate records to partition. If you want monthly directory layout under docs_dir, organize the filenames yourself (e.g., id values like 2026-04/hello)\n", path)
}
```

(Use `os.Stderr` since the loader currently has no logger.)

- [ ] **Step 4: Run; verify PASS**

```
go test ./internal/schema/ -v
```

Expected: PASS. The warning duplication across multiple loads is acceptable for v2.0.0 (a one-shot upgrade).

- [ ] **Step 5: Commit**

```
git add internal/schema/loader.go internal/schema/loader_test.go
git commit -m "feat(schema): deprecate records_dir and partition in loader

Both fields are still accepted in YAML for graceful upgrade but
ignored. Loader emits a one-time stderr notice per schema instructing
users to remove them. Final removal targeted for v3."
```

---

## Task 11: Init — bare scaffold, no `--template`

**Files:**
- Modify: `cmd/init_cmd.go`, `cmd/init_wizard.go`
- Delete: `schemas/adr.yaml`, `schemas/discussion.yaml`, `schemas/notes.yaml`, `schemas/task.yaml`

- [ ] **Step 1: Replace `cmd/init_cmd.go` body with the simplified version**

```go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/sergio-bershadsky/secondbrain-db/internal/output"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new secondbrain-db project",
	RunE:  runInit,
}

func init() { rootCmd.AddCommand(initCmd) }

func runInit(cmd *cobra.Command, _ []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}
	format := outputFormat(cfg)
	basePath := cfg.BasePath

	if initInteractive {
		return runInteractiveInit(basePath, format)
	}

	for _, d := range []string{"schemas", "docs"} {
		if err := os.MkdirAll(filepath.Join(basePath, d), 0o755); err != nil {
			return fmt.Errorf("creating %s: %w", d, err)
		}
	}

	tomlContent := `schema_dir = "./schemas"
base_path = "."

[output]
format = "auto"

[integrity]
key_source = "env"
`
	tomlPath := filepath.Join(basePath, ".sbdb.toml")
	if err := os.WriteFile(tomlPath, []byte(tomlContent), 0o644); err != nil {
		return err
	}

	return output.PrintData(format, map[string]any{
		"action":  "init",
		"config":  tomlPath,
		"schemas": filepath.Join(basePath, "schemas"),
		"next":    "Add a schema under schemas/<entity>.yaml — see the secondbrain-db plugin for reference schemas.",
	})
}
```

- [ ] **Step 2: Replace `cmd/init_wizard.go`**

Use the file content from the discarded local commit (template-removal-only branch). Project name + GitHub/VitePress/Integrity/KG toggles, no entity selection. (The full content was committed locally on `feat/drop-builtin-templates` — copy it back. If that commit is gone, refer to the snippet at the end of this task.)

- [ ] **Step 3: Delete root `schemas/*.yaml`**

```
git rm schemas/adr.yaml schemas/discussion.yaml schemas/notes.yaml schemas/task.yaml
```

- [ ] **Step 4: Update README and guide**

`README.md` — replace each occurrence of `sbdb init --template notes` with `sbdb init`; remove the "ships with three built-in templates" sentence; update the layout diagram to drop `data/` and add the sidecar pair.

`docs/guide.md` — update the "Project setup" tree, drop `default_schema = "notes"` from the `.sbdb.toml` example, drop any reference to `data/` or `records.yaml`/`.integrity.yaml`. Add a paragraph pointing at the plugin's reference schemas.

- [ ] **Step 5: Run build + targeted test**

```
go build ./... && go test ./cmd/ -run TestInit -v
```

Expected: PASS (or no `TestInit*` matches, which is also fine — covered end-to-end in Task 13).

- [ ] **Step 6: Commit**

```
git add -A
git commit -m "feat(init)!: remove builtin entity templates; sbdb init produces bare scaffold

Drops --template flag and the five hardcoded starter schemas.
sbdb init now writes only .sbdb.toml + empty schemas/, docs/.
Wizard simplifies to project name + GitHub/VitePress/Integrity/KG
toggles. Reference schemas (notes/adr/discussion/task) move to the
secondbrain-db Claude Code plugin under
skills/secondbrain-db/reference/schemas/.

BREAKING CHANGE: sbdb init --template <name> is removed; sbdb init
produces an empty scaffold."
```

---

## Task 12: e2e — full v2 CRUD scenario

**Files:**
- Create: `e2e/v2_layout_test.go`

- [ ] **Step 1: Write the test**

```go
package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestE2E_V2_FullCRUD_NoDataDir(t *testing.T) {
	bin := buildSBDB(t)
	dir := t.TempDir()

	run := func(args ...string) (string, int) {
		cmd := exec.Command(bin, args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				return string(out), ee.ExitCode()
			}
			t.Fatalf("%v: %v", args, err)
		}
		return string(out), 0
	}

	_, code := run("init")
	require.Equal(t, 0, code)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "schemas/notes.yaml"), []byte(`version: 1
entity: notes
docs_dir: docs/notes
filename: "{id}.md"
id_field: id
integrity: off
fields:
  id: { type: string, required: true }
  created: { type: date, required: true }
`), 0o644))

	_, code = run("create", "-s", "notes", "--field", "id=hello",
		"--field", "created=2026-04-28", "--content", "# Hi")
	require.Equal(t, 0, code)

	assert.FileExists(t, filepath.Join(dir, "docs/notes/hello.md"))
	assert.FileExists(t, filepath.Join(dir, "docs/notes/hello.yaml"))
	assert.NoDirExists(t, filepath.Join(dir, "data"))

	_, code = run("doctor", "check", "--all", "-s", "notes")
	assert.Equal(t, 0, code)

	_, code = run("update", "-s", "notes", "--id", "hello", "--field", "created=2026-04-29")
	require.Equal(t, 0, code)
	_, code = run("doctor", "check", "--all", "-s", "notes")
	assert.Equal(t, 0, code)

	_, code = run("delete", "-s", "notes", "--id", "hello", "--yes")
	require.Equal(t, 0, code)
	assert.NoFileExists(t, filepath.Join(dir, "docs/notes/hello.md"))
	assert.NoFileExists(t, filepath.Join(dir, "docs/notes/hello.yaml"))
}

func buildSBDB(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "sbdb")
	cmd := exec.Command("go", "build", "-o", bin, "github.com/sergio-bershadsky/secondbrain-db")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "build sbdb: %s", out)
	return bin
}
```

- [ ] **Step 2: Run**

```
go test ./e2e/ -run TestE2E_V2_FullCRUD -v
```

Expected: PASS.

- [ ] **Step 3: Commit**

```
git add e2e/v2_layout_test.go
git commit -m "test(e2e): full v2 CRUD scenario, asserts no data/ ever created"
```

---

## Task 13: e2e — multi-PR conflict-free merge

**Files:**
- Create: `e2e/multi_pr_merge_test.go`

- [ ] **Step 1: Write the test**

```go
package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestE2E_MultiPR_NoMergeConflict(t *testing.T) {
	bin := buildSBDB(t)
	dir := t.TempDir()

	gitInit := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	sbdb := func(args ...string) {
		cmd := exec.Command(bin, args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "sbdb %v: %s", args, out)
	}

	gitInit("init", "-q", "-b", "main")
	gitInit("config", "user.email", "t@t")
	gitInit("config", "user.name", "t")
	sbdb("init")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "schemas/notes.yaml"), []byte(`version: 1
entity: notes
docs_dir: docs/notes
filename: "{id}.md"
id_field: id
integrity: off
fields:
  id: { type: string, required: true }
  created: { type: date, required: true }
`), 0o644))
	gitInit("add", ".")
	gitInit("commit", "-q", "-m", "init")

	gitInit("checkout", "-q", "-b", "pr-a")
	sbdb("create", "-s", "notes", "--field", "id=alpha",
		"--field", "created=2026-04-28", "--content", "# A")
	gitInit("add", ".")
	gitInit("commit", "-q", "-m", "alpha")

	gitInit("checkout", "-q", "main")
	gitInit("checkout", "-q", "-b", "pr-b")
	sbdb("create", "-s", "notes", "--field", "id=beta",
		"--field", "created=2026-04-28", "--content", "# B")
	gitInit("add", ".")
	gitInit("commit", "-q", "-m", "beta")

	gitInit("checkout", "-q", "main")
	gitInit("merge", "--no-ff", "-q", "-m", "merge a", "pr-a")
	gitInit("merge", "--no-ff", "-q", "-m", "merge b", "pr-b")

	cmd := exec.Command("git", "-C", dir, "status", "--porcelain")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err)
	require.Empty(t, string(out), "unexpected dirty status: %s", out)

	cmd = exec.Command(bin, "doctor", "check", "--all", "-s", "notes")
	cmd.Dir = dir
	out, err = cmd.CombinedOutput()
	require.NoError(t, err, "doctor: %s", out)
}
```

- [ ] **Step 2: Run**

```
go test ./e2e/ -run TestE2E_MultiPR -v
```

Expected: PASS — both branches merge cleanly, doctor is clean.

- [ ] **Step 3: Commit**

```
git add e2e/multi_pr_merge_test.go
git commit -m "test(e2e): multi-PR conflict-free merge property

Two branches each add a different note via sbdb create, both merge
to main without git conflict, doctor check is clean afterwards.
This is the central success criterion for v2 layout."
```

---

## Task 14: e2e — doctor scope (uncommitted-only default and `--all`)

**Files:**
- Create: `e2e/doctor_scope_test.go`

- [ ] **Step 1: Write the test**

```go
package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestE2E_DoctorScope_DefaultIsUncommittedOnly(t *testing.T) {
	bin := buildSBDB(t)
	dir := t.TempDir()
	gitInit := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	sbdb := func(args ...string) string {
		cmd := exec.Command(bin, args...)
		cmd.Dir = dir
		out, _ := cmd.CombinedOutput()
		return string(out)
	}

	gitInit("init", "-q", "-b", "main")
	gitInit("config", "user.email", "t@t")
	gitInit("config", "user.name", "t")
	sbdb("init")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "schemas/notes.yaml"), []byte(`version: 1
entity: notes
docs_dir: docs/notes
filename: "{id}.md"
id_field: id
integrity: off
fields:
  id: { type: string, required: true }
  created: { type: date, required: true }
`), 0o644))
	sbdb("create", "-s", "notes", "--field", "id=clean",
		"--field", "created=2026-04-28", "--content", "# C")
	sbdb("create", "-s", "notes", "--field", "id=dirty",
		"--field", "created=2026-04-28", "--content", "# D")
	gitInit("add", ".")
	gitInit("commit", "-q", "-m", "init")

	// Tamper one md after committing.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs/notes/dirty.md"),
		[]byte("---\nid: dirty\ncreated: 2026-04-28\n---\nTAMPERED"), 0o644))

	// Default scope = uncommitted only -> finds drift on dirty.md
	cmd := exec.Command(bin, "doctor", "check", "-s", "notes")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	assert.Error(t, err)
	assert.Contains(t, string(out), "dirty")
	assert.NotContains(t, string(out), "clean.md", "default scope should not examine committed clean docs")

	// --all also finds it (and would report any other historical drift)
	cmd = exec.Command(bin, "doctor", "check", "--all", "-s", "notes")
	cmd.Dir = dir
	out, _ = cmd.CombinedOutput()
	assert.Contains(t, string(out), "dirty")
}

func TestE2E_DoctorScope_NonGit_FallsBackToAll(t *testing.T) {
	bin := buildSBDB(t)
	dir := t.TempDir()
	cmd := exec.Command(bin, "init")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	require.NoError(t, os.WriteFile(filepath.Join(dir, "schemas/notes.yaml"), []byte(`version: 1
entity: notes
docs_dir: docs/notes
filename: "{id}.md"
id_field: id
integrity: off
fields:
  id: { type: string, required: true }
  created: { type: date, required: true }
`), 0o644))

	cmd = exec.Command(bin, "create", "-s", "notes", "--field", "id=hello",
		"--field", "created=2026-04-28", "--content", "# Hi")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	cmd = exec.Command(bin, "doctor", "check", "-s", "notes")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "out: %s", out)
	assert.Contains(t, string(out), "not a git repo; falling back to --all")
}
```

- [ ] **Step 2: Run**

```
go test ./e2e/ -run TestE2E_DoctorScope -v
```

Expected: PASS.

- [ ] **Step 3: Commit**

```
git add e2e/doctor_scope_test.go
git commit -m "test(e2e): doctor default uncommitted-only scope and --all override

Dirty working-tree change is found by default; clean committed docs
are skipped. Non-git tempdir falls back to --all with stderr notice."
```

---

## Task 15: e2e — migrate

**Files:**
- Create: `e2e/migrate_test.go`

- [ ] **Step 1: Write the test**

```go
package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestE2E_Migrate_V1ToV2(t *testing.T) {
	bin := buildSBDB(t)
	dir := t.TempDir()

	// Build a v1-shaped fixture by hand (no v1 binary needed).
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "schemas"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs/notes"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "data/notes"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "schemas/notes.yaml"), []byte(`version: 1
entity: notes
docs_dir: docs/notes
filename: "{id}.md"
records_dir: data/notes
id_field: id
integrity: off
fields:
  id: { type: string, required: true }
  created: { type: date, required: true }
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".sbdb.toml"), []byte(`schema_dir = "./schemas"
base_path = "."
[output]
format = "json"
[integrity]
key_source = "env"
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs/notes/alpha.md"),
		[]byte("---\nid: alpha\ncreated: 2026-04-28\n---\n# Alpha"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "data/notes/records.yaml"), []byte(`- id: alpha
  created: 2026-04-28
  file: docs/notes/alpha.md
`), 0o644))

	cmd := exec.Command(bin, "doctor", "migrate")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "migrate: %s", out)

	assert.NoDirExists(t, filepath.Join(dir, "data"))
	assert.FileExists(t, filepath.Join(dir, "docs/notes/alpha.yaml"))

	// Idempotent
	cmd = exec.Command(bin, "doctor", "migrate")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	cmd = exec.Command(bin, "doctor", "check", "--all", "-s", "notes")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())
}
```

- [ ] **Step 2: Run**

```
go test ./e2e/ -run TestE2E_Migrate -v
```

Expected: PASS.

- [ ] **Step 3: Commit**

```
git add e2e/migrate_test.go
git commit -m "test(e2e): doctor migrate v1 → v2 layout, idempotent, post-clean"
```

---

## Final task: open the PR

- [ ] **Step 1: Open the umbrella issue (if not already open)**

```
gh issue create \
  --title "feat: drop data/ layer; per-md sidecars; remove builtin templates (sbdb v2.0.0)" \
  --label "feat,breaking-change" \
  --body-file <(cat <<'EOF'
[issue body — see spec at docs/superpowers/specs/2026-04-28-sbdb-v2-layout-design.md]
EOF
)
```

Note the issue number; close #27 with a comment pointing at this issue.

- [ ] **Step 2: Push the branch**

```
git push -u origin feat/sbdb-v2-layout
```

- [ ] **Step 3: Open the PR**

```
gh pr create \
  --title "feat!: drop data/ layer; per-md sidecars; remove builtin templates" \
  --body "$(cat <<'EOF'
## Summary
- Drops the data/ aggregate tree (records.yaml, .integrity.yaml) — per-md
  <id>.yaml sidecars replace it. Two PRs adding two different docs now
  merge with zero git conflict (verified by e2e/multi_pr_merge_test.go).
- sbdb list/query/get walk docs_dir directly with concurrent frontmatter
  parsing. CLI surface (commands, flags, JSON shapes, exit codes) is
  preserved.
- sbdb init removes the --template flag; produces a bare scaffold.
  Reference schemas relocate to the secondbrain-db Claude Code plugin
  in a follow-up PR.
- New: sbdb doctor migrate (idempotent v1 → v2). New: --all flag and
  default uncommitted-only scope on doctor (committed history is the
  trust boundary).

## Spec
docs/superpowers/specs/2026-04-28-sbdb-v2-layout-design.md

## Test plan
- [ ] go test ./... -count=1 — full suite green
- [ ] e2e/v2_layout_test.go — full CRUD; data/ never created
- [ ] e2e/multi_pr_merge_test.go — two branches merge clean
- [ ] e2e/doctor_scope_test.go — default uncommitted-only; --all override; non-git fallback
- [ ] e2e/migrate_test.go — v1 fixture migrates idempotently

## Migration
Existing users run `sbdb doctor migrate` once on each KB.

## Companion plugin
Plugin v1.4.0 in a follow-up PR in the ai repo.

Closes #<umbrella-issue-number>
EOF
)"
```

- [ ] **Step 4: Wait for CI green; merge as squash**

After merge, release-please drafts v2.0.0; merge that PR.

---

## Self-review pass

Quick checklist run on the plan above:

- **Spec coverage:** §1–§17 of the spec each map to one or more tasks
  here. §11 (multi-PR property) → Task 13. §6 (walker) → Task 1. §5
  (sidecar) → Task 2. §8.0 (uncommitted scope) → Tasks 3 + 6 + 14. §8.4
  (migrate) → Tasks 8 + 15. §9 (init) → Task 11. §10 (deprecations) →
  Task 10. §12 (staged execution) → Tasks 1–9 mirror commits A–G; H, I,
  J → Tasks 11, README/guide edits inside Task 11, and Tasks 12–15.
- **Placeholders:** none — all code is concrete; only the issue-body
  `<<EOF>>` in the final task references the spec by path rather than
  inlining it (acceptable, not a placeholder).
- **Type consistency:** `Sidecar` fields match between `sidecar.go`
  definition (Task 2), the document write path (Task 4), the doctor
  check (Task 6), and the migrate command (Task 8). `Doc` fields match
  between `walker.go` (Task 1) and `loadRecordsViaWalker` (Task 5).
- **Spec sidecar naming:** `<id>.yaml` sibling of `<id>.md` — consistent
  throughout.
