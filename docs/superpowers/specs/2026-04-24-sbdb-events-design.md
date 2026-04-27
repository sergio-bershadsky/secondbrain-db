# sbdb Events — Design Specification

| Field         | Value                                                                              |
| ------------- | ---------------------------------------------------------------------------------- |
| Status        | Draft                                                                              |
| Spec version  | 2.0 (supersedes 1.0)                                                               |
| Date          | 2026-04-27                                                                         |
| Scope         | Normative spec for `sbdb events emit`: the on-demand projection of git history into a JSONL event stream |

> **Lead invariant.** Events are not stored. They are projected from git history on demand by `sbdb events emit`. The repo's git log IS the event log; every CRUD operation that produces a commit produces events, and the projection reads commit diffs to emit them. There is no events directory, no archive, no append path, no integrity manifest for events — git is the integrity manifest.

---

## 1. Scope

This spec defines a derived, on-demand event stream computed from git history. The stream is JSONL, written to stdout by the `sbdb events emit` command, and consumed by piping the output into a downstream worker.

**In scope:**

- The wire format (envelope shape, encoding, key order)
- The CLI command and its argument grammar
- The mapping from git diff status → event type
- The mapping from file path → bucket (schema-driven)
- Worker consumer contract

**Out of scope:**

- Storage. There is none. If you need durable event records for replay, your worker keeps them — the projection is stateless and fully replayable from git.
- Archival. Git already archives via its own pack files; sbdb does not duplicate this.
- Concurrency on append. There is no append. Concurrent invocations of `sbdb events emit` are independent reads from git, with the same lock-free guarantees git itself provides.

---

## 2. Wire format

Every event is exactly one JSON object on one line, with a trailing `\n`. Output is suitable for piping.

### 2.1 Envelope

| Field   | Required | Type   | Notes                                                                                                                                                                                                                                                                                                  |
| ------- | -------- | ------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `ts`    | yes      | string | RFC 3339 UTC, millisecond precision, `Z` suffix. Sourced from the commit's author-date.                                                                                                                                                                                                                |
| `type`  | yes      | string | `<bucket>.<verb>`. Closed catalog (§3).                                                                                                                                                                                                                                                                |
| `id`    | yes      | string | Repo-relative POSIX path of the affected file.                                                                                                                                                                                                                                                         |
| `sha`   | optional | string | Git blob hash of the file's content **after** the change. Same value `git hash-object <file>` would produce. Workers resolve content via `git cat-file blob <sha>`. Omitted on `*.deleted`.                                                                                                            |
| `prev`  | optional | string | Git blob hash before the change. Omitted on `*.created`.                                                                                                                                                                                                                                               |
| `op`    | optional | string | Commit hash. Groups all events emitted from one commit.                                                                                                                                                                                                                                                |
| `actor` | optional | string | Commit author email, or `git` if the commit has no author email recorded.                                                                                                                                                                                                                              |

Key order is fixed: `ts`, `type`, `id`, `sha`, `prev`, `op`, `actor`. Empty optionals are omitted (never `null`).

There is no `data` field. Anything a worker needs to know about a change comes from reading the file at `sha` or diffing against `prev`.

### 2.2 Forbidden patterns

- `null` values for any field.
- Any field added beyond the seven above. The envelope is closed.

---

## 3. Type catalog

Verbs are derived structurally from `git log --raw` status letters:

| Status | Verb       | Meaning                                |
| ------ | ---------- | -------------------------------------- |
| `A`    | `created`  | Path is new in this commit.            |
| `C`    | `created`  | Path is a copy of another file.        |
| `M`    | `updated`  | Path's content changed.                |
| `D`    | `deleted`  | Path was removed in this commit.       |

Other statuses (`T` type-change, `U` unmerged, `R` renames) are not emitted. Renames are projected with `--no-renames`, surfacing as paired `D` + `A` rows; consumers may collapse them by matching `sha` if they care about rename semantics, but the wire format stays structural.

Buckets are determined per-file by walking the project's loaded schemas: a file under a schema's `docs_dir` produces events under that schema's `bucket` (defaulting to `entity`). Files outside any schema's `docs_dir` produce no events.

The catalog is closed: there are no author-defined event types, no extension protocol, no `x.*` namespace. New verbs ship by adding emit-side logic; new buckets ship by adding schemas.

---

## 4. CLI

```
sbdb events emit <commit-from> [<commit-to>|latest]
```

- `<commit-from>`: any commit-ish recognized by git (sha, branch, tag, `HEAD~N`, `@{1.week.ago}`, …). Required.
- `<commit-to>`: optional, defaults to `HEAD`. The literal string `latest` is accepted as an alias for `HEAD`.
- The range is exclusive on `<commit-from>` and inclusive on `<commit-to>`, matching git's `<from>..<to>` semantics.
- Events are emitted in chronological order (oldest first).
- Merge commits are skipped (`--no-merges`); workers see only the line of first-parent history.

Exit codes: `0` on success, non-zero on a bad ref or unparseable git output.

### 4.1 Examples

```bash
# Last 50 commits' worth of events
sbdb events emit HEAD~50

# Events between two tags
sbdb events emit v1.0.0 v1.1.0

# Filter by type
sbdb events emit HEAD~7d | jq 'select(.type == "note.created")'

# Resume from a known cursor
sbdb events emit $LAST_SEEN_COMMIT
```

---

## 5. Worker contract

Workers consume the projection by piping the command's output:

```bash
sbdb events emit <last-seen-commit> | my-worker
```

Cursor management is the worker's responsibility. The natural cursor is the **commit hash** (`op` field) — workers persist the most recent commit they processed and pass it as `<commit-from>` on the next invocation. Re-running with the same `<commit-from>` produces the identical stream; the projection is deterministic and replayable.

There is no at-least-once / exactly-once distinction at this layer because there is no delivery — the projection is a pull. Workers that want exactly-once semantics implement them by keying side-effects on `(op, id)` (commit hash + path) and using their own idempotency table.

---

## 6. Implementation notes

The projection shells out to:

```
git -C <repo> log --reverse --raw --no-renames --no-merges -z \
    --format=COMMIT%x00%H%x00%ct%x00%aE \
    <from>..<to>
```

Output is parsed as a NUL-delimited token stream:

- `COMMIT` literal marker, followed by three metadata tokens: commit sha, unix-ts, author email.
- For each changed file: a `:<modes> <old-blob> <new-blob> <status>` row token, then a path token.

Per file, the projector emits one event with `sha` = post-image blob hash and `prev` = pre-image blob hash (each omitted if the corresponding side is git's all-zeros sentinel).

---

## 7. Compatibility

This spec replaces the v1.0 stored-events design entirely. Pre-existing `.sbdb/events/` directories are orphaned and may be `rm -rf`'d. Workers that previously tailed `.sbdb/events/*.jsonl` switch to piping `sbdb events emit | …`.
