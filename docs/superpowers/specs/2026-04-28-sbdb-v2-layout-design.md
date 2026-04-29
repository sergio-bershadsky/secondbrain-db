# sbdb v2 layout: drop `data/`, per-md sidecars, no builtin templates

**Status:** Proposed
**Author:** Sergey Bershadsky
**Created:** 2026-04-28
**Companion CLI version:** v2.0.0
**Companion plugin version:** secondbrain-db v1.4.0
**Supersedes:** issue #27 (template removal alone)

## 1. Problem

Two structural defects in the current layout block parallel-PR workflows
and couple the CLI to opinionated content shapes:

### 1.1 Aggregate write hotspots

Every `sbdb create | update | delete` rewrites two files per entity (or per
month, for `partition: monthly`):

- `data/<entity>/records.yaml` — full list of all records, in YAML
- `data/<entity>/.integrity.yaml` — full map of integrity entries keyed by id

Two PRs adding two different documents both rewrite these files with
non-overlapping but textually distinct content. Git's three-way merge
cannot reconcile the YAML rewrites cleanly: even though the additions are
semantically disjoint, they appear as overlapping line edits relative to
the common ancestor. Result: every parallel PR conflicts on aggregate
files, even when no two PRs touch the same record.

This kills:
- Multiple AI agents working in parallel branches.
- Background-refinement workflows that batch many small PRs.
- Branch-based exploration where contributors prepare independent additions.

### 1.2 Builtin entity templates

`sbdb init --template <name>` writes one of five hardcoded schemas
(`notes`, `blog`, `adr`, `discussion`, `task`) baked into
`cmd/init_cmd.go:templateSchema()`. Four of them are also mirrored as
standalone YAML at `schemas/*.yaml` in the CLI repo root.

The CLI is meant to be a structural engine (YAML schemas, integrity,
knowledge graph). Carrying opinionated entity templates inside the binary
couples the CLI to a content model it has no business owning, and creates
release-cadence drag (every template tweak is a CLI release).

## 2. Goals

- **Parallel-PR clean.** Two PRs each adding a different document under
  the same entity merge to `main` with zero git conflict.
- **Same `sbdb` CLI surface.** All user-facing commands (`create`,
  `update`, `delete`, `list`, `query`, `get`, `search`, `doctor`,
  `index`, `graph`) keep the same flags, JSON output shapes, and exit
  codes. Only the on-disk layout changes.
- **Single source of truth per document.** Each markdown file has exactly
  one sibling integrity sidecar; no aggregate state in git.
- **Content-agnostic CLI.** No builtin entity templates. The plugin
  carries reference schemas as copyable examples.
- **Smooth migration.** `sbdb doctor migrate` performs a one-shot
  conversion from v1 layout to v2 layout, idempotent.

## 3. Non-goals

- Replacing the SQLite knowledge-graph and semantic-index databases.
  These are already gitignored, already rebuilt locally, and are not in
  the merge path.
- Post-merge GitHub Actions to rebuild aggregate files. With `data/`
  gone there is nothing in git to rebuild.
- Changing schema semantics (field types, virtuals, integrity modes).
  Only deprecating two layout-related fields (`records_dir`, `partition`).
- Performance tuning for very large knowledge bases (>10k documents).
  An opt-in gitignored cache (`.sbdb-cache/`) is sketched as future work
  but not built in v2.0.0.

## 4. Architecture overview

### 4.1 Current layout (v1)

```
my-kb/
├── .sbdb.toml
├── schemas/
│   └── notes.yaml
├── docs/
│   └── notes/
│       ├── hello.md
│       └── world.md
└── data/                                 ← REMOVED in v2
    └── notes/
        ├── records.yaml                  ← aggregate, merge-hostile
        ├── .integrity.yaml               ← aggregate, merge-hostile
        ├── 2026-04.yaml                  ← if partition: monthly
        └── 2026-05.yaml
```

### 4.2 New layout (v2)

```
my-kb/
├── .sbdb.toml
├── schemas/
│   └── notes.yaml
└── docs/
    └── notes/
        ├── hello.md
        ├── hello.yaml                    ← NEW: per-doc sidecar
        ├── world.md
        └── world.yaml
```

The `data/` directory is removed entirely. The single source of truth
for every document is the pair (`<id>.md`, `<id>.yaml`):

- `<id>.md` carries content + frontmatter (queryable scalar fields,
  unchanged from v1).
- `<id>.yaml` carries integrity hashes + signature.

`sbdb list/query/get` walks `docs/<entity>/` and parses frontmatter
directly, with concurrent file reads. There is no aggregate index file
in git.

Optional gitignored local cache for very large bases (deferred to a
future change):

```
.sbdb-cache/                              ← gitignored, never committed
└── records-<entity>.json
```

## 5. Sidecar specification

### 5.1 Naming

For a document at `docs/<entity>/<id>.md` the sidecar is at
`docs/<entity>/<id>.yaml` (same directory, same basename, `.yaml`
extension instead of `.md`).

Rationale for this naming:

- **Sorting:** `hello.md` and `hello.yaml` group adjacent in directory
  listings.
- **Discoverability:** not hidden — the sidecar appears in PR diffs and
  file trees as a first-class artifact.
- **Concise:** one extension, no double-suffix (`hello.md.sig.yaml`).

Collision concern: a schema author cannot name a record `<id>` if a
schema with `entity: <id>` also exists in the same directory tree. In
practice schemas live in `schemas/`, not under `docs/<entity>/`, so this
does not occur. The `sbdb doctor check` step verifies that every `.yaml`
under a schema's `docs_dir` has a matching `.md`; orphans are flagged.

### 5.2 Format

```yaml
version: 1
algo: sha256
hmac: true                                # false for integrity: warn|off modes
file: hello.md                            # basename, for rename detection
content_sha: 9f86d0...                    # SHA-256 of the markdown body bytes
frontmatter_sha: ab1c4d...                # SHA-256 of the canonical-yaml frontmatter
record_sha: 7d865e...                     # SHA-256 of the record projection (frontmatter + virtuals)
sig: 0a3b...                              # HMAC-SHA-256 over the three SHAs above
                                          # present iff hmac: true
updated_at: 2026-04-28T09:30:12Z
writer: secondbrain-db/2.0.0
```

`record_sha` ties the sidecar to the record-shape projection used by
`sbdb list/query`, so a frontmatter change that affects derived virtuals
shows up as drift.

### 5.3 Lifecycle

- **Create:** `sbdb create` writes `<id>.md` atomically (existing
  rename-into-place pattern), computes the three SHAs, computes HMAC
  if integrity mode is `strict`, writes `<id>.yaml` atomically.
- **Update:** Same as create, in place.
- **Delete:** `sbdb delete` removes both `<id>.md` and `<id>.yaml` in
  the same operation.
- **Drift detection (`sbdb doctor check`):** for each `<id>.md`, load
  `<id>.yaml`, recompute SHAs from the on-disk markdown, compare. Drift
  buckets:
  - `missing-sidecar`: `.md` exists, `.yaml` does not.
  - `missing-md`: `.yaml` exists, `.md` does not (orphan sidecar).
  - `content-drift`: `content_sha` mismatch (markdown body edited
    out-of-band).
  - `frontmatter-drift`: `frontmatter_sha` mismatch.
  - `bad-sig`: HMAC verification fails (tamper).
- **Repair (`sbdb doctor fix --recompute`):** rewrites sidecars from
  current on-disk state. With `--force` also re-signs.

### 5.4 Atomicity

Sidecars use the same temp-rename pattern as `.md` files:

```go
tmp, _ := os.CreateTemp(dir, ".sbdb-sidecar-*.yaml.tmp")
tmp.Write(yaml)
tmp.Close()
os.Rename(tmp.Name(), sidecarPath)        // atomic on POSIX
```

If the process crashes between the `.md` rename and the `.yaml` rename,
the doctor's `missing-sidecar` drift class catches it; `fix --recompute`
restores the sidecar. The opposite ordering (sidecar first, then md) is
not used because a stale sidecar pointing at a not-yet-written `.md`
can be misleading; a missing sidecar pointing at a written `.md` is the
recoverable case.

## 6. Read path: walker + frontmatter parsing

### 6.1 Walker package

A new package `internal/storage/walker.go` exposes:

```go
type Doc struct {
    Path        string                // absolute path to .md
    ID          string                // resolved from filename per schema
    Frontmatter map[string]any        // parsed YAML frontmatter
    Body        []byte                // markdown body after frontmatter
}

// WalkDocs yields every .md under docsDir whose name matches the
// schema's filename template. Sidecars (.yaml) and unrelated files
// are skipped. Concurrency is handled internally with a sized worker
// pool (default GOMAXPROCS, override via env SBDB_WALK_WORKERS).
func WalkDocs(docsDir string, schema *Schema) iter.Seq2[Doc, error]

// LoadDocByID resolves <id> through the schema's filename template
// and reads the resulting .md directly. Used by `sbdb get`.
func LoadDocByID(docsDir string, schema *Schema, id string) (*Doc, error)
```

### 6.2 Query path

`sbdb list -s notes`:

1. Resolve schema → `docs_dir`.
2. Stream `WalkDocs(docs_dir, schema)`.
3. For each `Doc`, project the frontmatter through the schema's record
   shape. All virtuals (scalar + complex) are already materialized into
   the frontmatter at write time by `BuildFrontmatterData`
   (unchanged from v1), so the read path does not invoke Starlark.
   `BuildRecordData` runs in-memory to filter the frontmatter map down
   to scalar fields + scalar virtuals — same projection as the v1
   `records.yaml` payload.
4. Emit JSON / table to stdout.

`sbdb query -s notes --filter status=active`:

Same as `list`, plus filter evaluation runs against `Doc.Frontmatter`
before emission.

`sbdb get -s notes --id hello`:

Direct: `LoadDocByID` → render schema's `filename` template with
`{id: "hello"}` → read that file → parse → emit. No directory walk.

### 6.3 Performance characteristics

- 1k documents × ~50 KiB each ≈ 50 MiB total reads. With 8-way
  concurrency and modern SSDs, complete walks finish in ~200 ms.
  Frontmatter parse is the dominant cost.
- 10k documents → ~2 seconds. Acceptable for CLI use; if it becomes a
  bottleneck, the gitignored cache lands as a separate change.
- `sbdb get` is O(1) — no walk.
- `sbdb list/query` are not currently cached between invocations within
  a session; each invocation walks. That matches v1 behavior (each
  invocation reads `records.yaml`).

## 7. CRUD flow diagrams

### 7.1 Create

```
sbdb create -s notes --input -                  # JSON on stdin
   │
   ├─► validate against schema (fields, types, required)
   ├─► compute virtuals (Starlark)
   ├─► render filename template → docs/notes/hello.md
   ├─► write hello.md (atomic rename)
   ├─► compute content_sha, frontmatter_sha, record_sha
   ├─► if integrity=strict: HMAC over (content+fm+record) shas
   ├─► serialize sidecar struct → docs/notes/hello.yaml (atomic rename)
   └─► emit JSON {action:create, id:hello, files:[hello.md, hello.yaml]}
```

### 7.2 Update

Identical to create, with the difference that the existing `.md` and
`.yaml` are overwritten via the rename-into-place pattern.

### 7.3 Delete

```
sbdb delete -s notes --id hello --yes
   │
   ├─► resolve filename: docs/notes/hello.md
   ├─► os.Remove(docs/notes/hello.md)
   ├─► os.Remove(docs/notes/hello.yaml)         # ignore ENOENT
   └─► emit JSON {action:delete, id:hello}
```

### 7.4 List / Query

```
sbdb list -s notes
   │
   ├─► resolve schema
   ├─► WalkDocs(docs/notes, schema) (concurrent)
   │     for each .md:
   │        parse frontmatter
   │        yield Doc
   ├─► (optional) apply --filter
   ├─► (optional) sort
   └─► emit JSON array / table
```

### 7.5 Get

```
sbdb get -s notes --id hello
   │
   ├─► render schema.Filename with {id: "hello"} → hello.md
   ├─► read docs/notes/hello.md
   ├─► parse frontmatter + body
   └─► emit JSON {id, frontmatter, body}
```

## 8. Doctor commands

### 8.0 Default scope: uncommitted-only

`doctor check`, `doctor fix`, and `doctor sign` default to operating on
**files that differ from `HEAD` in the working tree** — the union of:

- Modified-but-unstaged files (`git diff --name-only`)
- Modified-and-staged files (`git diff --name-only --cached`)
- Untracked files under any schema's `docs_dir`
  (`git ls-files --others --exclude-standard`)

Premise: every commit already passed doctor checks (via the plugin's
PostToolUse hook, a pre-commit hook, or CI). Re-scanning thousands of
already-verified files on every invocation is wasteful. The committed
history is the trust boundary; doctor only needs to verify what the
user/agent has changed since then.

This makes interactive use nearly free even on large KBs (typical agent
turn touches 1–5 docs).

**`--all` flag** (added to `check`, `fix`, `sign`): bypasses the git
filter and walks every `<id>.md` + `<id>.yaml` under each schema's
`docs_dir`. Used for:

- Periodic full audits (CI, cron).
- Non-git working trees (no `.git` directory) — in that case `--all` is
  also the implicit default with a one-line stderr notice:
  `not a git repo; falling back to --all`.
- Recovery scenarios where the user suspects historical commits drifted
  (e.g., manual filesystem edits that bypassed `sbdb`).

**Pair scoping.** When the working-tree diff includes only `<id>.md` (or
only `<id>.yaml`), the matching pair file is automatically included in
the scope so drift between them is always detected. Example: editing
just `hello.md` triggers verification against `hello.yaml`.

**Migration scope.** `doctor migrate` is exempt from this rule — it
always walks the entire KB because v1 → v2 is a layout change, not a
content drift check.

### 8.1 `sbdb doctor check`

For each schema, iterate `WalkDocs` and additionally enumerate every
`*.yaml` under `docs_dir`. Cross-check:

- `<id>.md` present, `<id>.yaml` absent → `missing-sidecar`.
- `<id>.yaml` present, `<id>.md` absent → `missing-md` (orphan).
- Both present: load sidecar, recompute SHAs from `.md`, compare.
- HMAC mode: verify signature with the configured key.

Output: per-document drift report (JSON or table). Exit code 1 if any
drift, 0 if clean (matches v1 behavior).

### 8.2 `sbdb doctor fix --recompute`

For each `.md` whose sidecar is missing or has SHA mismatch:

- Recompute SHAs from current `.md`.
- Rewrite sidecar (preserving HMAC sig only if `--no-resign` is passed
  AND existing sig still verifies; otherwise drop sig and require
  `doctor sign --force` to re-sign).

For orphan `<id>.yaml` (no `.md`): delete the sidecar (with `--prune`
flag; default behavior is to flag and skip).

### 8.3 `sbdb doctor sign --force`

Walk all `.md`, write/overwrite HMAC sigs in the sidecars using the
configured key. Required after key rotation or after a `--no-resign`
fix.

### 8.4 `sbdb doctor migrate` (NEW)

One-shot v1 → v2 migration. Idempotent.

```
sbdb doctor migrate [--dry-run]
   │
   ├─► detect v1 layout: data/<entity>/records.yaml exists for some entity
   │     (if no v1 layout found: print "already v2", exit 0)
   ├─► for each entity:
   │     load records (records.yaml or YYYY-MM.yaml partition files)
   │     load integrity manifest (.integrity.yaml)
   │     for each record:
   │        resolve docs/<entity>/<id>.md (must exist; flag if not)
   │        construct sidecar struct from manifest entry + record
   │        write docs/<entity>/<id>.yaml
   ├─► verify: walk all .md, confirm each has a sidecar with valid SHAs
   ├─► (unless --dry-run) rm -rf data/
   └─► emit JSON summary
```

Failure modes:

- Records.yaml references an id whose `.md` is missing → migration
  errors out, leaves `data/` intact. User runs `sbdb doctor check` on
  v1 to repair, then re-runs migrate.
- HMAC key not available but integrity manifest has signatures → write
  sidecars with the existing signatures preserved (still valid against
  the same key).

## 9. Init: bare scaffold + reference schemas

`sbdb init` creates:

```
.sbdb.toml          # schema_dir = "./schemas", base_path = "."
                    # [output] format = "auto"
                    # [integrity] key_source = "env"
schemas/            # empty
docs/               # empty
```

No `data/`. No starter schema. The `--template` flag is removed.

`.sbdb.toml` no longer carries `default_schema` (a remnant of the
single-builtin-template assumption). The CLI infers schema selection
from `-s <name>` or, when omitted, errors with
`use -s to select a schema; available: <list-from-schemas-dir>`.

Reference schemas (the four removed from CLI repo root) ship in the
`secondbrain-db` Claude Code plugin under
`skills/secondbrain-db/reference/schemas/`. The plugin's SKILL.md
documents the copy-from-reference flow.

## 10. Schema YAML deprecations

Two schema fields become deprecated:

- `records_dir`: ignored. Records no longer have a separate directory.
  Loader emits a one-time warning per schema:
  `schemas/notes.yaml: 'records_dir' is deprecated and ignored in v2; remove it`.
- `partition`: ignored at the records-storage level. Loader emits a
  one-time warning if `partition: monthly` is present:
  `'partition' is deprecated; v2 has no aggregate records to partition.
  If you want monthly directory layout under docs_dir, organize the
  filenames yourself (e.g., id values like 2026-04/hello)`. Filename
  templating in v2 is unchanged from v1 (plain `{field}` substitution);
  date-formatted templates are out of scope here.

These deprecations get a v3 removal note. Existing v1 schema files
continue to load without modification (warnings only).

## 11. Multi-PR merge property

Central success criterion. With v2:

```
PR-A (branched from main):
  + docs/notes/alpha.md
  + docs/notes/alpha.yaml

PR-B (branched from main):
  + docs/notes/beta.md
  + docs/notes/beta.yaml

main + PR-A + PR-B:
  docs/notes/alpha.md, alpha.yaml, beta.md, beta.yaml
```

Git's merge sees four file additions across two PRs, no shared file
edited by both. Three-way merge succeeds with zero conflict.

Compare to v1 where both PRs would have rewritten
`data/notes/records.yaml` and `data/notes/.integrity.yaml`, producing
two conflict hunks per merge.

Verified by an automated e2e test (`e2e/multi_pr_merge_test.go`) that:

1. Initializes a temp git repo, runs `sbdb init`, drops in a notes
   schema, commits.
2. Branches twice. On each branch, runs `sbdb create` for a different
   id and commits.
3. Merges both branches into main via `git merge --no-ff`.
4. Asserts `git status` shows clean and `sbdb doctor check` passes.

## 12. Implementation strategy: feature flag → flip → cleanup

Single PR; staged commits inside it. Each commit leaves the test suite
green. The flag is `SBDB_USE_SIDECAR=1` at the env level.

| Commit | Scope |
|--------|-------|
| A | Add `internal/storage/walker.go` + tests. No call sites yet. |
| B | Add `internal/integrity/sidecar.go` + tests. No call sites yet. |
| C | Wire `internal/document/save.go` to write sidecars when flag set, in addition to the legacy manifest. Reads still use legacy `records.yaml`. |
| D | Wire `cmd/list.go`, `cmd/query.go`, `cmd/get.go` to use the walker when flag set; legacy reads otherwise. |
| E | Implement `cmd/doctor.go migrate` subcommand. Test on a fixture KB. |
| F | Flip default: flag becomes opt-out (`SBDB_USE_LEGACY=1`). All E2E tests run on the new path. |
| G | Delete legacy code: `internal/storage/records.go`, `internal/storage/partition.go`, `internal/integrity/manifest.go`. Remove the opt-out flag. |
| H | Remove builtin templates: simplify `cmd/init_cmd.go`, `cmd/init_wizard.go`. Delete CLI repo root `schemas/*.yaml` (already done on the discarded branch — re-apply). |
| I | Docs sweep: `README.md`, `docs/guide.md`, this spec is the canonical reference for the behavior. |
| J | Add `e2e/multi_pr_merge_test.go`. |

The PR is opened after commit J. The umbrella issue replaces #27 (which
gets closed as superseded with a comment).

## 13. Testing strategy

Unit tests:

- `internal/storage/walker_test.go`: directory walk, filename-template
  matching, frontmatter parse, concurrency stress test.
- `internal/integrity/sidecar_test.go`: serialize/parse, atomic write,
  HMAC sign/verify, drift detection (each bucket).

Integration tests:

- `internal/document/save_test.go`: round-trip create/update/delete on
  a tempdir; assert exact filesystem state after each op.

End-to-end:

- `e2e/v2_layout_test.go`: full CRUD scenario through the CLI binary,
  asserts no `data/` is ever created.
- `e2e/migrate_test.go`: builds a fixture v1 KB, runs `sbdb doctor
  migrate`, verifies v2 layout + clean doctor check.
- `e2e/doctor_scope_test.go`: in a git repo with N committed clean docs
  + 1 modified working-tree doc, asserts default `doctor check` only
  examines the modified doc; `--all` examines all N+1; non-git tempdir
  falls back to `--all` with the stderr notice.
- `e2e/multi_pr_merge_test.go`: parallel-branch merge property
  (described in §11).

Backward compatibility:

- Test that v1 schema files (with `records_dir` and `partition`) load
  with deprecation warnings and produce identical query results when
  the underlying KB has been migrated.

## 14. Risk register

| Risk | Likelihood | Impact | Mitigation |
|------|---|---|---|
| Walk performance on >10k docs feels slow | Medium | Medium | Document the SBDB_WALK_WORKERS knob; ship gitignored cache as follow-up |
| Migration fails partway, leaves mixed v1/v2 state | Low | High | `migrate` is idempotent; run-twice-safe; `--dry-run` for preview |
| Users edit sidecars directly, break HMAC | Medium | Low | `doctor check` catches; plugin's `guard-docs.py` blocks direct sidecar edits |
| Filename collisions between `<id>.yaml` sidecar and a user's hand-written yaml in `docs_dir` | Low | Low | Doctor enumerates and flags orphan sidecars; user is alerted |
| HMAC key absent during migration (lost key) | Low | Medium | Migration falls back to writing sidecars without `sig`; user runs `doctor init-key` + `doctor sign --force` after |
| Default uncommitted-only scope misses drift introduced before the workflow adopted v2 | Low | Medium | `--all` flag + documented periodic full audit (e.g., CI cron); non-git fallback to `--all` |
| User runs `doctor check` outside a git repo and gets misleading "clean" result | Low | Low | Fallback to `--all` is automatic, with a stderr notice |
| Disk-full mid-CRUD leaves `.md` without `.yaml` | Low | Low | Same as v1's mid-write risk; doctor `fix --recompute` restores |

## 15. Out of scope (deferred)

- Gitignored `.sbdb-cache/` for query speedup on large KBs.
- Schema-level changes (field types, virtuals, partitioning beyond the
  filename-template trick described in §10).
- Plugin (ai repo) work — see companion plan in §16.
- Replacement of the SQLite KG / semantic-index databases.

## 16. Companion plugin work (separate PR in ai repo)

After CLI v2.0.0 tags:

1. Open issue `feat: support sbdb v2 layout in plugin`.
2. Update `hooks/guard-docs.py` to block direct edits to `*.yaml`
   under any `docs_dir` declared in `schemas/*.yaml` (route through
   `sbdb create/update/delete`).
3. Add reference schemas under
   `skills/secondbrain-db/reference/schemas/{notes,adr,discussion,task}.yaml`.
4. Update `SKILL.md`: replace `sbdb init --template notes` with two
   steps (init + copy reference). Add a "Reference schemas" section.
5. Update `commands/sbdb-init.md`: drop the `--template` argument hint.
6. Bump `plugin.json` and `marketplace.json` to `1.4.0`.
7. Add `RELEASES/1.4.0.md`: documents the v2 layout, sidecar guard,
   reference-schema flow.

## 17. Open questions

None at time of writing. All design decisions are made in this document.
If review surfaces ambiguity it is fixed inline before the
implementation plan.
