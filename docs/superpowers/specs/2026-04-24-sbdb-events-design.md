# sbdb Events — Design Specification

| Field | Value |
|---|---|
| Status | Draft |
| Spec version | 1.0 |
| Date | 2026-04-24 |
| Scope | Normative spec for sbdb's event log: wire format, type registry, extension protocol, archival, and concurrency guarantees |

> **Lead invariant.** Events are immutable, append-only facts. The only mutable artifact in the system is markdown file content, governed by git. Every other byte sbdb writes is write-once during routine operation. Type schemas evolve under strict additive rules; non-additive change requires a new type. The single exception is `sbdb event repair --truncate-partial`, an explicit user-initiated recovery from a crashed write (§7.10) — sbdb never auto-truncates.

---

## 1. Scope

This spec defines a durable, append-only event stream emitted by sbdb at every state-changing operation it performs. The stream lives inside the repo as plain JSONL files plus monthly gzipped archives. Workers consume the stream by reading the repo (no separate broker required).

**In scope:**
- The on-disk wire format (envelope, encoding, file layout)
- The naming convention for event types
- The built-in catalog of event types
- The extension protocol for new entity authors
- The worker consumer contract
- Concurrency, integrity, and conformance guarantees
- Archival to git or S3 under a 2-month sliding window

**Out of scope:**
- CLI ergonomics (`sbdb event append`, `sbdb doctor`, etc.) — covered by CLI reference docs
- Worker SDK / client libraries — language-agnostic; this spec defines the wire format only
- ACL or privacy policy — sbdb treats the repo's filesystem boundary as the trust boundary; all data inside the repo is in-scope for any consumer of that repo
- Cross-repo event federation — future spec

**Audience:**
- sbdb implementers (Go source)
- Authors writing entity schemas (`x.*` namespace)
- Worker authors consuming the event stream

---

## 1.5 Threat model

This spec's integrity guarantees are evaluated against this model:

**Primary adversary: AI / automation accidental corruption.** An LLM agent or automation script attempts to "fix" or "tidy" the events log, doctor it during a session, or write events directly via Edit/Write rather than through sbdb. This is the dominant failure mode for an LLM-mediated knowledge base and most of sbdb's defenses are aimed here.

**Secondary adversary: human supply-chain mistakes.** Force-pushes that rewrite event history, manual sed/grep edits to event files, accidental archival deletion. Detected by the integrity manifest's tail-hash chain plus git's own Merkle history.

**Out of scope:** state-actor attacks, cryptographic attacks on hash functions, hardware-level data corruption, malicious sbdb binaries.

The defenses prefer **detective controls** (drift/tamper detection by `sbdb doctor`) over **preventive cryptography** (signatures). Detection is sufficient when paired with git history as the immutable backstop.

---

## 2. Envelope

Every event is exactly one JSON object on one line, with a trailing `\n`.

### 2.1 Required fields

| Field | Type | Notes |
|---|---|---|
| `ts` | string | RFC 3339 UTC with millisecond precision and `Z` suffix. Example: `2026-04-24T14:32:01.123Z`. Never `+00:00`. |
| `type` | string | Dotted name (§3). MUST be present in the registry projection. |
| `id` | string | Stable identifier for the affected entity. For document events, the repo-relative POSIX path (forward slashes only). For non-entity events (`meta.*`), a name appropriate to the event (e.g. `2026-02` for `meta.archived`). |

### 2.2 Optional fields

| Field | Type | Notes |
|---|---|---|
| `sha` | string | Git blob hash of the affected file's content **after** the event — the same hex string `git hash-object` produces (`sha1("blob <len>\0" + bytes)`). This is git's native object identifier, so workers can resolve content directly via `git cat-file blob <sha>` and locate introducing commits via `git log --find-object=<sha>`. Required for document mutation events; omitted for events with no file (e.g. registry events). |
| `prev` | string | Git blob hash before the event. Used on `*.updated` events to anchor diffs. |
| `op` | string | ULID grouping events emitted from one logical operation. |
| `phase` | string | Sub-step within an `op` for ordered cascades. Free-form short identifier (e.g. `"graph"`, `"index"`). |
| `actor` | enum | `cli` \| `hook` \| `worker` \| `agent`. Closed enum in v1. |
| `data` | object | Type-specific payload. Bounded so the full line stays ≤ 4 KiB (§7). |

### 2.3 Forbidden patterns

- `null` values for any field (omit instead).
- Nested arrays-of-objects deeper than 1 level inside `data`.
- Base64 blobs of any kind. Reference by `sha` instead.
- Any field whose value would push the line past 4 KiB.

### 2.4 Field key namespacing inside `data`

Within `data`, top-level keys belong to the type's owner.

- For built-in types (no `x.` prefix on the type name), top-level `data` keys are owned by sbdb maintainers.
- For author-extended types (`x.*` prefix), top-level `data` keys are owned by the author.
- **Any author who needs to add fields to a type they do not own** (i.e. extending a built-in) MUST place those fields inside a nested object at `data.x`. Concretely, the `data` object MAY contain a key literally named `"x"` whose value is an object holding all author-added fields. Author-added fields are NOT JSON keys with literal dots like `"x.author_team"`; they are normal keys inside the `x` sub-object. The `x` sub-object inside `data` is the only path through which non-owners may extend a type.

Example — a built-in `note.updated` event extended with an author-added field:

```json
{"ts":"2026-04-24T14:32:01.123Z","type":"note.updated","id":"notes/foo.md","sha":"def012","data":{"changed_keys":["title"],"x":{"author_team":"docs"}}}
```

`changed_keys` is owned by sbdb. `x.author_team` is owned by whatever schema declared it.

---

## 3. Type naming convention

### 3.1 Format

`<bucket>[.<verb>]` for built-ins; `x.<bucket>.<verb>` for author types.

- Both `bucket` and `verb` use lowercase ASCII, snake_case.
- Verbs prefer past-tense and concrete (`created`, `updated`, `deleted`, `signed`, `tamper_detected`, `accepted`, `superseded`).
- Vague verbs (`changed`, `modified`) without a qualifier are discouraged.

### 3.2 Reserved buckets (built-in only)

`note`, `task`, `adr`, `discussion`, `graph`, `kb`, `records`, `integrity`, `review`, `freshness`, `meta`, `search`.

These names MUST NOT be claimed by author schemas.

### 3.3 Reserved namespace prefix

The `sbdb.*` prefix is reserved for future built-in buckets. Authors MUST NOT claim a bucket beginning with `sbdb.`. This prevents future expansion of built-ins from retroactively colliding with author claims.

### 3.4 Author types

Author types take the form `x.<bucket>.<verb>` with three dotted segments minimum. The leading `x` is the namespace marker; everything below it is owned by the author who registers it.

Examples:
- `x.recipe.created`
- `x.recipe.cooked`
- `x.book.read`
- `x.meeting.scheduled`

Workers detect author types by checking whether the type name starts with `x.`.

### 3.5 Versioning suffix

Type-name evolution is forbidden (§6.3). When a breaking change is required, register a new type entirely. The convention for the new name is to append a numeric suffix:

- `note.created` → `note.created2`
- `x.recipe.cooked` → `x.recipe.cooked2`

The old type stays in the registry forever, marked deprecated. Both names are emitted in parallel during any transition; consumers choose which they trust.

---

## 4. Built-in event catalog

Each entry below names the type, when sbdb emits it, mandatory `data` keys, and worker idempotency notes.

Idempotency convention for all events: `(year, month, seq)` is globally unique; `(type, id, sha)` is content-keyed. Workers MUST handle at-least-once delivery.

### 4.1 Document lifecycle

For each entity bucket (`note`, `task`, `adr`, `discussion`):

| Type | When | Required `data` |
|---|---|---|
| `<bucket>.created` | sbdb creates a new file under the entity's data directory | none |
| `<bucket>.updated` | sbdb writes content changes to an existing file | `changed_keys: [string...]` (frontmatter keys whose values changed; `["__body__"]` if body changed) |
| `<bucket>.deleted` | sbdb removes a file from the entity's data directory | none |

Domain-specific verbs added by built-ins:

| Type | Notes |
|---|---|
| `task.status_changed` | `data: { from: string, to: string }` — both values from the task status enum |
| `task.completed` | shortcut for `status_changed` to a terminal status; emitted in addition |
| `adr.proposed` | status transition; `data: { previous: string }` |
| `adr.accepted` | status transition; `data: { previous: string }` |
| `adr.superseded` | `data: { superseded_by: string }` (id of the superseding ADR) |
| `adr.rejected` | `data: { previous: string }` |
| `discussion.action_added` | `data: { action_id: string, owner: string }` |
| `discussion.action_resolved` | `data: { action_id: string, resolution: string }` |

**Note.** Renames are NOT emitted as a single event. A file move from `a` to `b` emits `<bucket>.deleted{id:"a", sha:"X"}` followed by `<bucket>.created{id:"b", sha:"X"}`. Workers that build human activity feeds may collapse adjacent delete+create pairs with matching `sha` into a "renamed" presentation.

### 4.2 Knowledge graph

| Type | When | Required `data` |
|---|---|---|
| `graph.node_added` | new node inserted | `entity: string` |
| `graph.node_removed` | node removed | none |
| `graph.edge_added` | edge inserted | `from: string, to: string, edge_type: string` |
| `graph.edge_removed` | edge removed | `from: string, to: string, edge_type: string` |
| `graph.reindexed` | full graph rebuild completed | `nodes: int, edges: int, took_ms: int` |

### 4.3 Index / embeddings

| Type | When | Required `data` |
|---|---|---|
| `kb.indexed` | full reindex completed | `chunks_added: int, chunks_removed: int, embeddings_updated: int, took_ms: int` |
| `kb.chunk_added` | chunk created during incremental indexing | `chunk_id: string` |
| `kb.chunk_removed` | chunk removed | `chunk_id: string` |
| `kb.embedding_updated` | re-embedded due to content change | `chunk_id: string, model_id: string` |
| `kb.model_changed` | embedding model rotation; forces re-embed | `from: string, to: string` |

`kb.indexed` is emitted only on successful completion. Crashed reindex emits no event; doctor detects drift and retries.

### 4.4 Records

| Type | When | Required `data` |
|---|---|---|
| `records.upserted` | scalar projection updated | `entity: string` |
| `records.removed` | record removed | `entity: string` |
| `records.partition_rotated` | monthly shard split or merged | `entity: string, partition: string` |

### 4.5 Integrity

| Type | When | Required `data` |
|---|---|---|
| `integrity.signed` | new sign of one or more files | `count: int` |
| `integrity.recomputed` | virtual fields recomputed (Starlark) | `count: int` |
| `integrity.drift_detected` | doctor found drift | `drift_count: int` |
| `integrity.tamper_detected` | doctor found tamper | `tamper_count: int` |

### 4.6 Review / freshness

| Type | When | Required `data` |
|---|---|---|
| `review.stamped` | review stamp applied | `reviewer: string` |
| `freshness.stale_flagged` | content marked stale by freshness check | none |

### 4.7 Meta

| Type | When | Required `data` |
|---|---|---|
| `meta.archived` | doctor archived a month | `range: string` (YYYY-MM), `target: "git"\|"s3"`, `uri: string` (s3://... or path), `gz_sha256: string`, `line_count: int` |
| `meta.event_type_registered` | new type added to registry | `owner: "builtin"\|<schema-path>`, `schema_version: int` |
| `meta.event_type_evolved` | additive change to existing type | `schema_version: int, added_optional: [string...], added_enum_values: { field: [string...] }, widened_constraints: [string...]` |
| `meta.event_type_deprecated` | type marked deprecated (still emitted) | `reason: string, successor: string\|""` |
| `meta.config_changed` | `.sbdb.toml` modified | `keys_changed: [string...]` |

### 4.8 Search (opt-in)

| Type | When | Required `data` |
|---|---|---|
| `search.queried` | a search query ran | `query_sha: string` (SHA-256 of the query string; the literal query is not emitted) |

Disabled by default. Enable in `.sbdb.toml` under `[events] emit_search_queried = true`.

### 4.9 Events sbdb does NOT emit

- **Hook activity** — `meta.edit_blocked` and similar are intentionally not emitted. Hooks block at the tool boundary; their activity is observable through Claude Code's transcript and the integrity layer's drift/tamper detection. The events stream is reserved for sbdb-mediated state changes.
- **Doctor lifecycle** — there is no `meta.doctor_started` / `meta.doctor_completed`. Doctor emits events only for actual state changes (`integrity.signed`, `meta.archived`, `meta.event_type_evolved`).
- **Read operations** — search queries are opt-in; reads of the KB never emit.

---

## 5. Worker contract

### 5.1 Cursor format

Workers track position as `(year, month, seq)`. This tuple is stable across daily file rotation, monthly archival, and rebases. Workers MUST NOT use file paths or byte offsets as durable cursors.

### 5.2 Ordering

- **Within a month**: strict total order by `seq`. The kernel's O_APPEND atomicity guarantees `seq` matches the order events landed.
- **Across months**: `meta.archived` events fence cleanly. A consumer that has processed everything up to a `meta.archived` event for month M is guaranteed to have seen every event in month M.
- **Within an `op`**: the optional `phase` field allows downstream consumers to reason about sub-step ordering inside one logical operation. Without `phase`, treat as a set.

### 5.3 Delivery semantics

**At-least-once.** Workers MUST tolerate duplicate delivery and key downstream effects on `(type, id, sha)` or `(year, month, seq)`. There is no exactly-once guarantee.

### 5.4 Forward compatibility

Workers MUST:

- Ignore unknown `data` keys (additive evolution, §6.3).
- Tolerate unknown enum values, routing them to a default branch.
- Skip (with log) any event whose `type` is not in the worker's known set, never crash.

This is required because schema evolution is additive and registry rebuilds are eventual on the consumer side.

### 5.5 Replay

Replay-from-zero is supported by design but not optimized for. Cold workers SHOULD prefer to bootstrap from a snapshot of the underlying MD files and then resume from the latest event. The event stream is an audit channel, not an event-sourcing primary store.

### 5.6 Performance contract

Append latency MUST be ≤ 5 ms p99 on local POSIX filesystems for events ≤ 4 KiB. Registry validation runs against an in-memory projection loaded once per process; reload only on `meta.event_type_*` events seen during the same process's lifetime.

---

## 6. Extension protocol — author-defined types

### 6.1 The `x.` namespace

Authors who want to track new entity types (`recipe`, `book`, `meeting`, etc.) declare them in entity schema files. All author-owned identifiers live under the `x.` prefix at every level:

| Surface | Built-in | Author |
|---|---|---|
| Type name | `note.created` | `x.recipe.created` |
| Bucket name | `note` | `x.recipe` |
| `data` field on a built-in type | `data.changed_keys` | `data.x.author_team` |
| Enum value extension on a built-in field | (cannot be done by authors; create a new type) | author types' enums are author-owned |

The `x.` prefix is the universal namespace marker. Workers can detect any author-owned identifier by string-prefix match on `x.`.

### 6.2 Schema declaration

Author entity schemas declare their event types in a dedicated block:

```yaml
# schemas/recipe.yaml
entity: x.recipe
bucket: x.recipe
event_types:
  - name: created
    data:
      fields:
        - { name: title, type: string, required: true }
        - { name: source, type: string, required: false }
  - name: updated
    data:
      fields:
        - { name: changed_keys, type: list[string], required: true }
  - name: deleted
    data: {}
  - name: cooked
    data:
      fields:
        - { name: date, type: date, required: true }
        - { name: rating, type: int, required: false }
emits_on:
  create: x.recipe.created
  update: x.recipe.updated
  delete: x.recipe.deleted
```

### 6.3 Schema evolution matrix

A schema change to a registered type is **allowed** if every event valid under the old schema remains valid under the new, AND every event valid under the new schema would also be valid under the old schema's "ignore unknown fields" mode.

| Change | Allowed |
|---|---|
| Add a new optional field | yes |
| Add documentation / examples / `description` | yes |
| Mark a field `deprecated: true` (advisory) | yes |
| Add a new value to an enum (append-only) | yes |
| Loosen a constraint (`max_length` grows, `pattern` broadens) | **yes** (reverse-compatible) |
| Add a new required field | no |
| Rename a field | no |
| Remove a field | no |
| Change a field's type | no |
| Required → optional | no |
| Optional → required | no |
| Tighten a constraint | no |

Disallowed changes require minting a new type (§3.5).

### 6.4 Doctor enforcement

`sbdb doctor check` on every invocation:

1. Loads old schema state from the registry projection.
2. Loads new schema state from `schemas/*.yaml`.
3. Computes diff per type.
4. Validates diff against §6.3.
5. **Disallowed diff** → exit non-zero with a clear message naming the offending type and field, refusing to advance.
6. **Allowed diff** → emit `meta.event_type_evolved` to today's events file with the additive change recorded.
7. Regenerate `internal/events/registry.yaml` projection from the event log; verify byte-equal to a fresh rebuild.

The check is deterministic; a clean repo + a clean schema produces zero registry drift.

### 6.5 Bucket conflict resolution

If two schemas claim the same bucket, the **first registration wins** (earliest `meta.event_type_registered` for that bucket in the event log). Subsequent schemas with the same bucket are rejected by doctor with a clear remediation: rename your bucket. Once registered, a bucket is permanently bound to its first owner; even if that schema is later removed, the bucket remains "retired" and cannot be reclaimed.

### 6.6 Type deprecation

A type may be marked `deprecated: true` in the registering schema. Doctor emits `meta.event_type_deprecated` once on the first observation of the new flag. Deprecated types continue to be valid for emission and consumption indefinitely; deprecation is a documentation signal only. Deletion of a type is forbidden under the immutability invariant.

---

## 7. Wire format

### 7.1 Encoding

- UTF-8, no BOM.
- Paths in `id` MUST be NFC-normalized; sbdb rejects non-NFC at append time.
- Line terminator `\n` only; never `\r\n`.

### 7.2 JSON serialization

- One object per line; no pretty-printing; no trailing comma.
- Output key order: `ts`, `type`, `id`, `sha`, `prev`, `op`, `phase`, `actor`, `data`. Stable for diff readability; consumers MUST NOT depend on key order.
- Empty optional fields are **omitted**, not present-as-`null`.

### 7.3 Per-line size

Total byte length including the trailing `\n` MUST be ≤ 4 KiB. The append API MUST reject events that would exceed this size before any filesystem syscall.

### 7.4 File layout

```
.sbdb/events/
  2026-03-01.jsonl          # previous full month, daily files
  2026-03-02.jsonl
  ...
  2026-03-31.jsonl
  2026-04-01.jsonl          # current month, partial
  2026-04-24.jsonl          # today
  2026-04-24.001.jsonl      # rotation slice (>5000 lines)
  archive/
    2025.MANIFEST.yaml
    2026.MANIFEST.yaml
    2026-02.jsonl.gz        # immutable, gzipped
    2026-01.jsonl.gz
    2025-12.jsonl.gz
    ...
```

### 7.5 Window invariant

At any moment, only the **current month** (partial) and the **immediately previous month** (full) exist as `.jsonl`. Everything older lives in `archive/` as `.gz`. Doctor enforces this; appearing daily files older than two months is reported as drift by `sbdb doctor check`.

### 7.6 Daily rotation

Within a day, a file is rotated once it crosses 5,000 lines:

- `2026-04-24.jsonl` → 5,000 lines max
- `2026-04-24.001.jsonl` → next 5,000 lines
- `2026-04-24.002.jsonl` → next 5,000 lines

Workers walk `<date>*.jsonl` in lex order to read a day in sequence.

### 7.7 Archival

`sbdb doctor fix` archives expired months:

1. Wait until end-of-month + 7 days settle period.
2. Concatenate all `YYYY-MM-*.jsonl` files in lex order → `gzip -9` → `archive/YYYY-MM.jsonl.gz`.
3. Verify line count of decompressed gz equals `wc -l` of inputs.
4. Verify tail-hash chain end-to-end.
5. Update `archive/YYYY.MANIFEST.yaml` with line count, tail sha, file size.
6. Atomic git commit: add `.gz`, remove daily files, update manifest.
7. Append `meta.archived` event to today's daily file.

Idempotent: re-running on an already-archived month is a no-op.

### 7.8 Archive target — git or S3

Archive destination is configured in `.sbdb.toml`:

```toml
[events]
window_months = 2
rotation_lines = 5000

[events.archive]
target = "git"             # "git" | "s3" | "both"

[events.archive.s3]
bucket = "my-sbdb-archive"
prefix = "secondbrain/events/"
region = "us-east-1"
storage_class = "STANDARD_IA"
sse = "AES256"
auth = "env"               # env | profile | instance | irsa
```

When `target = "s3"` (or `"both"`), the repo retains an immutable pointer file per archived month:

```yaml
# .sbdb/events/archive/2026-02.pointer.yaml
month: 2026-02
line_count: 4823
sha256: 7f3a...e91d
gz_sha256: 1ab2...c4d5
gz_bytes: 612340
target: s3
s3_uri: s3://my-sbdb-archive/secondbrain/events/2026-02.jsonl.gz
sealed_at: 2026-04-08T00:00:01Z
sealed_by: sbdb@1.4.2
```

Pointer files are mandatory — they preserve the audit chain in git even when blobs live in S3.

### 7.9 S3 archival flow

`sbdb doctor fix` for an expired month with `target = "s3"`:

1. Concatenate dailies → gzip → tempfile.
2. Verify locally (line count, tail hash). Fail before any S3 call on local error.
3. Upload to `s3://bucket/prefix/YYYY-MM.jsonl.gz` with:
   - `Content-MD5` header (S3 verifies on receipt)
   - `x-amz-meta-line-count`, `x-amz-meta-sha256`, `x-amz-meta-sbdb-version` for self-describing blobs
4. Re-fetch HEAD; verify `ETag` and metadata match.
5. Write pointer YAML, delete daily files, update year manifest, commit.
6. Emit `meta.archived` with `target: "s3"` and the `s3_uri`.

If upload fails, nothing is removed locally. Idempotent re-run picks up; if S3 already has the blob with matching hash, upload is skipped.

### 7.10 Crash recovery

A partially-written final line (e.g. a process crash during append) is **not** auto-repaired. `sbdb doctor check` reports it as a corruption signal and refuses to advance until the user runs `sbdb event repair --truncate-partial` with explicit consent. The system never silently mutates an event file.

---

## 8. Examples

### 8.1 Create a note

User runs `sbdb note create`. sbdb writes the file, indexes it, and emits a sequence of events under one `op`:

```jsonl
{"ts":"2026-04-24T14:32:01.123Z","type":"note.created","id":"notes/2026/04/architecture.md","sha":"abc123","op":"01HW3R8M","actor":"cli"}
{"ts":"2026-04-24T14:32:01.124Z","type":"records.upserted","id":"notes/2026/04/architecture.md","op":"01HW3R8M","actor":"cli","data":{"entity":"note"}}
{"ts":"2026-04-24T14:32:01.125Z","type":"graph.node_added","id":"notes/2026/04/architecture.md","op":"01HW3R8M","phase":"graph","actor":"cli","data":{"entity":"note"}}
{"ts":"2026-04-24T14:32:01.126Z","type":"kb.chunk_added","id":"notes/2026/04/architecture.md","op":"01HW3R8M","phase":"index","actor":"cli","data":{"chunk_id":"abc123:0"}}
{"ts":"2026-04-24T14:32:01.220Z","type":"kb.embedding_updated","id":"notes/2026/04/architecture.md","op":"01HW3R8M","phase":"index","actor":"cli","data":{"chunk_id":"abc123:0","model_id":"text-embedding-3-small"}}
{"ts":"2026-04-24T14:32:01.221Z","type":"integrity.signed","id":"notes/2026/04/architecture.md","sha":"abc123","op":"01HW3R8M","actor":"cli","data":{"count":1}}
```

### 8.2 Reorganize 200 notes into a new directory

Each rename emits two events — a `note.deleted` and a `note.created` with matching `sha`. 400 events total. Workers building human-readable feeds may collapse adjacent matched-sha pairs into a "renamed" presentation; the wire format stays pure.

### 8.3 Doctor archives February 2026

```jsonl
{"ts":"2026-04-08T00:00:01.000Z","type":"meta.archived","id":"2026-02","actor":"cli","data":{"range":"2026-02","target":"s3","uri":"s3://my-sbdb-archive/secondbrain/events/2026-02.jsonl.gz","gz_sha256":"1ab2c4d5","line_count":4823}}
```

### 8.4 Author registers a new entity

A user adds `schemas/recipe.yaml` and runs `sbdb doctor check`. Doctor emits:

```jsonl
{"ts":"2026-04-24T15:00:00.000Z","type":"meta.event_type_registered","id":"x.recipe.created","actor":"cli","data":{"owner":"schemas/recipe.yaml","schema_version":1}}
{"ts":"2026-04-24T15:00:00.001Z","type":"meta.event_type_registered","id":"x.recipe.updated","actor":"cli","data":{"owner":"schemas/recipe.yaml","schema_version":1}}
{"ts":"2026-04-24T15:00:00.002Z","type":"meta.event_type_registered","id":"x.recipe.deleted","actor":"cli","data":{"owner":"schemas/recipe.yaml","schema_version":1}}
{"ts":"2026-04-24T15:00:00.003Z","type":"meta.event_type_registered","id":"x.recipe.cooked","actor":"cli","data":{"owner":"schemas/recipe.yaml","schema_version":1}}
```

### 8.5 Author adds an optional field

Editing `schemas/recipe.yaml` to add an optional `serves` field to `x.recipe.cooked`:

```jsonl
{"ts":"2026-05-01T10:00:00.000Z","type":"meta.event_type_evolved","id":"x.recipe.cooked","actor":"cli","data":{"schema_version":2,"added_optional":["serves"]}}
```

Old archived events without `serves` remain valid. New events may include it.

### 8.6 Author adds an enum value

Editing `schemas/task.yaml` ... wait, `task` is built-in. Authors cannot add enum values to built-in types — they can only register their own types with their own enums. If the built-in maintainer adds an enum value, the same event applies:

```jsonl
{"ts":"2026-05-15T10:00:00.000Z","type":"meta.event_type_evolved","id":"task.status_changed","actor":"cli","data":{"schema_version":3,"added_enum_values":{"status":["blocked"]}}}
```

Workers seeing `"status":"blocked"` in subsequent events MUST handle it gracefully (default branch, log, etc.).

---

## 9. Registry projection

### 9.1 Source of truth

The event log is the source of truth for type registration. The registry file (`internal/events/registry.yaml`) is a **projection** regenerated from the log.

### 9.2 Format

```yaml
version: 1
generated_at: 2026-04-24T15:30:00Z
generated_from_seq:
  2026: { 04: 12345 }   # last seq processed per month-bucket
buckets:
  note:
    owner: builtin
    types: [created, updated, deleted, status_changed]   # task example
    deprecated: []
  x.recipe:
    owner: schemas/recipe.yaml
    registered_at: 2026-04-24T15:00:00Z
    types: [created, updated, deleted, cooked]
    deprecated: []
  task:
    owner: builtin
    types: [created, updated, deleted, status_changed, completed]
    deprecated: []
    enums:
      status: [open, in_progress, done, blocked]
```

### 9.3 Doctor's invariant

`sbdb doctor check` regenerates the projection fresh from the event log and asserts byte-equality with the on-disk file. Any drift = registry tampering.

---

## 10. Versioning, deprecation, governance

### 10.1 Spec versioning

This document carries `Spec version: X.Y` in the header. Minor versions are additive (new event types, new optional fields, new built-in buckets). Major versions are reserved for breaking changes to the envelope, ordering, or wire format itself; major bumps require a deprecation cycle ≥ 1 year and dual-emit infrastructure.

### 10.2 Registry versioning

`registry.yaml` carries `version: <int>`. Bumped only on breaking changes to the projection format. Currently `1`.

### 10.3 Type evolution

See §6.3 (matrix) and §6.4 (enforcement). Summary:

- Reverse-compatible changes only — defined by the §6.3 matrix. This includes adding optional fields, adding enum values, loosening constraints, and adding documentation. It does NOT include any change that invalidates events valid under the prior schema.
- Type-name changes forbidden; new name + deprecation of old.
- Deletion forbidden ever.

### 10.4 Built-in changes

Any change to a built-in type or bucket requires a PR to this spec, with the `events:` label and a migration note. Author types are owned by their schema.

---

## 11. Concurrency conformance tests

The lock-free append claim — that `O_APPEND` plus single-syscall writes ≤ 4 KiB cannot interleave or corrupt — is enforced by the test suite below. All tests live in `internal/events/concurrency_test.go` and `internal/events/concurrency_subprocess_test.go`. CI runs them with `go test -count=100 -race`. Zero flakes required.

### 11.1 Test inventory

| # | Name | Setup | Action | Assertions | Proves |
|---|---|---|---|---|---|
| 1 | Concurrent goroutine append | empty events file | 64 goroutines × 1,000 events | exact line count; every line valid JSON; per-writer seqs monotonic | kernel append atomicity within one process |
| 2 | Concurrent subprocess append | empty events file | 16 subprocesses × 5,000 events | identical to 1; all subprocesses exit 0 | per-FD atomicity across processes (production case) |
| 3 | Mixed reader / writer | empty events file | 8 writers (40k total lines) + 4 readers tailing | every read line valid JSON; no torn reads; reader counts strictly monotonic; union of reads = union of writes | reader/writer coexist without locks |
| 4 | Size boundary | empty events file | events at 100B, 1KB, 3.9KB, 4KB, 4.1KB | ≤ 4KB succeeds; > 4KB rejected at API; file unchanged on rejection | atomicity precondition enforced |
| 5 | Crash during append | empty events file | subprocess appends in tight loop, parent SIGKILLs at random delay; 50 iterations | file always ends in `\n`; every line valid JSON; no gaps below highest seq | kernel write is all-or-nothing for ≤ 4 KiB |
| 6 | No-buffered-writer regression | events file with one writer | append event with unique sentinel; immediately read via second FD | sentinel present byte-identical; trailing `\n` present | no user-space buffering between API and kernel |
| 7 (static) | Source-grep guard | build-time AST walk | reject `bufio.NewWriter` / `bufio.Writer` / wrappers in the path from `Append()` to `Write()` | direct path verified | structural complement to test 6 |
| 8 (static) | Non-POSIX rejection | runtime check | refuse event-append on NFS/SMB/CIFS unless `events.allow_non_posix = true` | mock NFS rejected; local accepted | documented limitation enforced |

### 11.2 What the suite does NOT prove

- **Durability under OS crash.** sbdb deliberately does not `fsync` per event; an audit log accepts last-second loss.
- **Cross-host correctness on networked filesystems.** Documented unsupported; refused at runtime.
- **Behavior on Windows.** Non-goal in v1.

---

## 12. Deliverables

| Artifact | Path | Notes |
|---|---|---|
| This spec | `docs/superpowers/specs/2026-04-24-sbdb-events-design.md` | normative |
| User-facing guide | `docs/EVENTS.md` (sbdb-db source repo) | derived from this spec; written during implementation |
| Type registry | `internal/events/registry.yaml` | projection of the event log |
| Conformance corpus | `internal/events/conformance_corpus/*.jsonl` | golden event files per catalog entry |
| Concurrency tests | `internal/events/concurrency_test.go`, `_subprocess_test.go` | §11 |
| Sync-check script | `scripts/check-events-sync.py` | CI guard that doc and registry agree |
| Example author schema | `schemas/_example_recipe.yaml` | referenced from §6.2 |
| Plugin version bump | `plugins/secondbrain-db/.claude-plugin/plugin.json` | `0.1.0` → `0.2.0` |

---

## Appendix A — Reserved buckets

These bucket names MUST NOT be claimed by author schemas:

- `note`, `task`, `adr`, `discussion`
- `graph`, `kb`, `records`
- `integrity`, `review`, `freshness`
- `meta`, `search`
- `sbdb.*` (entire prefix reserved for future built-ins)

---

## Appendix B — Schema evolution decision tree

```
Schema change to type T?
├─ Did you change a field's name or type? ──────────────────────► forbidden → new type
├─ Did you add a required field? ───────────────────────────────► forbidden → new type
├─ Did you change required ↔ optional in either direction? ─────► forbidden → new type
├─ Did you tighten any constraint (max_length down, etc.)? ─────► forbidden → new type
├─ Did you remove a field or remove an enum value? ─────────────► forbidden → new type
└─ Otherwise (added optional field, loosened constraint, added
   enum value, added documentation, marked deprecated): ────────► allowed → meta.event_type_evolved
```

---

## Appendix C — Open items deferred to later specs

These are out of scope for v1.0 and tracked here so they aren't lost:

- **Conformance test suite for non-Go implementations.** Currently the tests live in Go alongside sbdb's source. A language-agnostic harness (golden corpus + black-box runner) is a future deliverable.
- **Cross-repo event federation.** One sbdb instance subscribing to another's event stream. Future spec.
- **Compaction policy beyond monthly archival.** E.g., quarterly merge of monthly gz archives. Out of scope; current per-month files work indefinitely.
