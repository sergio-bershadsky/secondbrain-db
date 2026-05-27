# External sync

sbdb keeps a *one-way* projection of your docs in external services like
Confluence, Jira, or Slack canvas. Your markdown stays the source of truth;
the external system holds a mirror that sbdb tracks but never updates on its
own.

sbdb itself never makes a network call. It just records intent and state. The
actual pushing is done by an *integration runtime* — in practice, Claude
driving MCP servers (Confluence, Jira, Slack) at your direction. You can also
drive the same commands yourself by hand; the runtime is just a typical
caller.

## Mental model

| Concern | Where it lives | Who owns it |
|---|---|---|
| Shape of the entity | `schemas/<entity>.yaml` | Schema author |
| Which entities publish to which service and how | `.sbdb/integrations/<name>.yaml` | Platform/ops |
| Per-doc target ID (Confluence page, Jira issue) | Doc frontmatter (`sync.<integration>.<field>`) | Doc author |
| Per-doc sync state (last push, last check, last error) | `<doc>.integrations.yaml` sidecar | sbdb (writes) / runtime (consumes) |

Schemas stay completely unaware of integrations. A team can add a new
integration tomorrow by dropping a file in `.sbdb/integrations/` without
touching any schema.

## 1. Declare an integration

Each external service is one YAML file under `.sbdb/integrations/`:

```yaml
# .sbdb/integrations/confluence.yaml
integration: confluence
applies_to:
  notes:                              # an entity name (x-entity in schemas/notes.yaml)
    target_ref: sync.confluence.pageId  # where the doc keeps the external ID
    required: false                     # if true, sbdb refuses to create docs without it
    payload:
      title: { from: frontmatter.title }
      body:  { from: rendered_markdown }
```

Payload sources allowed:

- `frontmatter.<dotted.path>` — pull a value out of the doc's frontmatter.
- `rendered_markdown` or `body` — the doc body (post-frontmatter), trimmed.
- `{ const: <value> }` — a literal value, useful for fixed labels.

Validation happens whenever `sbdb` loads the config (on every sync command
and during `sbdb doctor check`). Unknown entities, missing `target_ref`,
mutually-exclusive `from`/`const`, and unknown payload sources are all
caught at config load time.

## 2. Link a doc to an external object

The doc declares the *value* of the back-ref in its frontmatter:

```markdown
---
id: launch-checklist
created: 2026-05-27
title: Launch Checklist
sync:
  confluence:
    pageId: "98765"
  jira:
    issueKey: "ENG-42"
---

# Launch Checklist
…
```

If an integration is configured with `required: true`, `sbdb create` /
`sbdb update` refuses to write a doc whose frontmatter doesn't resolve the
back-ref path to a non-empty value:

```
$ echo '{"id":"orphan","title":"Orphan"}' | sbdb create -s runbooks --input -
ERROR sync target validation failed:
  - integration "confluence": required target_ref "sync.confluence.pageId" is missing
```

## 3. Detect drift

`sbdb sync check` is local-only. It walks every doc whose entity has an
integration declared, compares the doc's current hash to the sidecar's
`last_push.doc_hash`, and produces a JSON report.

```
$ sbdb sync check --format json
{
  "docs": [
    {
      "schema": "notes",
      "id": "launch-checklist",
      "integrations": {
        "confluence": {
          "result": "local_drift",
          "target_id": "98765",
          "last_push_hash": "sha256:8a2f…",
          "current_doc_hash": "sha256:b71e…"
        }
      }
    }
  ],
  "summary": { "local_drift": 1 }
}
```

Exit code is `0` when everything is `in_sync`, otherwise `4` — CI pipelines
can fail-fast on drift if you want them to. The command itself never blocks
your edits or commits.

The five possible `result` values:

| `result` | Meaning |
|---|---|
| `in_sync` | Doc unchanged since last push, remote unchanged |
| `local_drift` | Doc edited since last push, remote unchanged |
| `remote_drift` | Remote revision advanced since last push (only the runtime can observe this — sbdb never polls) |
| `both_drift` | Both changed |
| `never_published` | Doc declares a target but has no recorded push yet |

## 4. Inspect what a push would send

```
$ sbdb sync state get -s notes --id launch-checklist --integration confluence
{
  "integration": "confluence",
  "entity": "notes",
  "target_id": "98765",
  "status": "linked",
  "payload": {
    "title": "Launch Checklist",
    "body":  "# Launch Checklist\n…"
  },
  "current_doc_hash": "sha256:b71e…",
  "state": {
    "last_push":  { … },
    "last_check": null,
    "last_error": null
  }
}
```

This is the one call the integration runtime makes before pushing — it gets
the resolved payload, the target ID, and the current sidecar state in one
JSON blob.

## 5. Record what the runtime did

After a successful push, the runtime calls:

```
sbdb sync state set \
  -s notes --id launch-checklist --integration confluence \
  --published-hash "sha256:b71e…" \
  --remote-revision "8" \
  --at "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
```

This overwrites `last_push` and clears any `last_error`.

After a failed push, the runtime records the error *without* corrupting the
last good push record:

```
sbdb sync state set \
  -s notes --id launch-checklist --integration confluence \
  --error "Confluence API 401: token expired" \
  --stage push \
  --attempted-doc-hash "sha256:b71e…" \
  --at "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
```

The next successful push clears `last_error`. The next failure overwrites it.
`last_push` only ever moves forward on a confirmed success.

After observing the remote (e.g. fetching the current Confluence revision via
MCP):

```
sbdb sync state set \
  -s notes --id launch-checklist --integration confluence \
  --check-result remote_drift \
  --remote-revision-observed "9" \
  --at "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
```

This overwrites `last_check` only. `last_push` and `last_error` are
untouched.

## 6. Inspect targets across integrations

`sbdb sync targets` resolves back-refs for one doc across every configured
integration:

```
$ sbdb sync targets -s notes --id launch-checklist
{
  "confluence": { "target_ref": "sync.confluence.pageId", "target_id": "98765", "status": "linked",   "required": false },
  "jira":       { "target_ref": "sync.jira.issueKey",     "target_id": "ENG-42", "status": "linked",   "required": false },
  "slack":      { "target_ref": "sync.slack.canvasId",    "target_id": "",       "status": "unlinked", "required": false }
}
```

Useful when adding a new integration to existing docs — you can see at a
glance which ones still need a back-ref value.

## Sidecar file shape

`docs/notes/launch-checklist.integrations.yaml`:

```yaml
confluence:
  target_id: "98765"
  last_push:
    doc_hash: "sha256:b71e…"
    at: "2026-05-27T10:00:00Z"
    remote_revision: "8"
    actor: "claude-code"
  last_check:
    at: "2026-05-27T10:30:00Z"
    result: "in_sync"
    current_doc_hash: "sha256:b71e…"

jira:
  target_id: "ENG-42"
  last_push:
    doc_hash: "sha256:b71e…"
    at: "2026-05-27T10:01:00Z"
    remote_revision: "21"
    actor: "claude-code"
```

One file per doc, one top-level key per integration. sbdb manages it
atomically (write-temp-then-rename). You should not edit it by hand — the
runtime is the only thing that should mutate it, and it must do so via
`sbdb sync state set` so the slot invariants are preserved.

The sidecar is **not** covered by sbdb's integrity signing. It's operational
state that changes on every push and check; signing it would defeat the
purpose. Doc integrity (the `.md` file + its `.yaml` integrity sidecar) is
unaffected by sync activity.

## Required vs optional integrations

`required: true` on `applies_to.<entity>` enforces, at doc-validation time,
that every doc of that entity has a non-empty value at `target_ref`. This is
how you say "every runbook MUST live in Confluence." The doc cannot be
created or updated without it.

`required: false` means "this doc may or may not have an external mirror;
sync check only reports drift for docs that do."

## What sbdb deliberately does not do

- Talk to Confluence, Jira, Slack, or any external API.
- Store credentials, tokens, or URLs beyond the target ID you put in
  frontmatter.
- Render markdown into service-specific formats (Confluence storage XML,
  Jira ADF, Slack mrkdwn). The runtime owns that conversion.
- Detect remote drift on its own. Only the runtime can observe what the
  external service currently looks like.
- Run scheduled jobs or pre-commit hooks that block your work. All sync
  actions are explicit and on-demand.

## Common workflows

### "Push my unsynced docs"

```
sbdb sync check --format json
# Look at results; for each local_drift / never_published:
sbdb sync state get … --integration X     # fetch resolved payload
# … push via MCP / curl / whatever …
sbdb sync state set … --published-hash … --remote-revision …
```

The integration runtime (e.g. Claude with the `secondbrain-db-sync` skill —
shipping in a follow-up release) automates the loop and asks you for
confirmation per doc.

### "Did Confluence get edited behind my back?"

This needs a remote read, which sbdb cannot do. The runtime queries the
remote revision, compares with `state.last_push.remote_revision`, and
records the result via `--check-result remote_drift`. sbdb just stores
what it's told.

### "Block CI when a doc has unsynced changes"

```
sbdb sync check
echo $?   # 0 = clean, 4 = drift, 1 = config/validation error
```

Plug into your CI gate of choice. No flag needed — the exit code is the
contract.

## Worked examples

### Confluence

#### A. Minimal page mirror

```yaml
# .sbdb/integrations/confluence.yaml
integration: confluence
applies_to:
  notes:
    target_ref: sync.confluence.pageId
    required: false
    payload:
      title: { from: frontmatter.title }
      body:  { from: rendered_markdown }
```

Doc frontmatter:

```yaml
sync:
  confluence:
    pageId: "12345"
```

The runtime gets a payload of `{title, body}` and calls (illustrative) the
Confluence MCP server's `update_page(pageId="12345", title=…, body=…)`. The
MCP server is responsible for converting markdown body → Confluence storage
format.

#### B. Pages with labels mapped from tags

```yaml
integration: confluence
applies_to:
  notes:
    target_ref: sync.confluence.pageId
    payload:
      title:  { from: frontmatter.title }
      body:   { from: rendered_markdown }
      labels: { from: frontmatter.tags }
```

A doc with `tags: [intro, demo]` produces `labels: ["intro", "demo"]` in the
payload. The runtime hands the list to Confluence's labels API.

#### C. One integration, multiple entities with different policies

```yaml
integration: confluence
applies_to:
  notes:
    target_ref: sync.confluence.pageId
    required: false
    payload:
      title: { from: frontmatter.title }
      body:  { from: rendered_markdown }

  runbooks:
    target_ref: sync.confluence.pageId
    required: true                       # every runbook MUST be in Confluence
    payload:
      title:  { from: frontmatter.title }
      body:   { from: rendered_markdown }
      labels: { const: ["runbook", "ops"] }   # fixed labels for the runbook class

  adrs:
    target_ref: sync.confluence.pageId
    required: true
    payload:
      title:  { from: frontmatter.title }
      body:   { from: rendered_markdown }
      labels: { const: ["adr", "architecture"] }
```

One integration file binds three entity types. Runbooks and ADRs cannot be
created without a Confluence page; notes are optional. Constant labels
distinguish the class of doc inside the same Confluence space.

#### D. Per-environment Confluence spaces

If you mirror to a dev space in development and prod in production, keep
that out of sbdb — sbdb is environment-neutral. The runtime resolves the
actual base URL / space from its own configuration (or from a per-machine
env var) and only consumes `pageId` from sbdb. The same `pageId` ought to
be valid against whichever space the runtime is currently targeted at.

### Notion

Notion's data model is "pages with structured properties, inside databases."
sbdb's payload mapping is flat — one key per logical field — but Notion
properties slot in naturally because each property is identified by name.

#### A. Page in a Notion database

```yaml
# .sbdb/integrations/notion.yaml
integration: notion
applies_to:
  notes:
    target_ref: sync.notion.pageId
    required: false
    payload:
      title:  { from: frontmatter.title }
      body:   { from: rendered_markdown }
      status: { from: frontmatter.status }
      tags:   { from: frontmatter.tags }
```

Doc frontmatter:

```yaml
sync:
  notion:
    pageId: "8a2f7c91-1b3d-4e5f-9a8b-12345abcdef0"
```

Notion page IDs are UUIDs. sbdb treats them as opaque strings — no
validation, just round-trip via `target_ref`.

The runtime maps the payload to Notion's API shape:

- `title` → the page's title property (always exists on a database page)
- `body` → page children (the runtime converts markdown to Notion blocks)
- `status` → a `Status` select property on the database
- `tags` → a `Tags` multi-select property

sbdb doesn't know which payload key is a property vs. a block — that's the
runtime's domain knowledge of Notion. The mapping is just "expose these
named values; runtime decides where they go."

#### B. Read-only fields that should never drift

You may have a Notion property the runtime should write but you never want
to *vary by doc* — e.g. "Source: secondbrain-db" as a fixed marker so
Notion users know where the page came from.

```yaml
integration: notion
applies_to:
  notes:
    target_ref: sync.notion.pageId
    payload:
      title:  { from: frontmatter.title }
      body:   { from: rendered_markdown }
      source: { const: "secondbrain-db" }   # always this literal value
```

Every push asserts `source = "secondbrain-db"` on the Notion page,
overwriting any manual change in that one field.

#### C. Mapping a derived virtual field

If your schema computes a virtual field (e.g. via Starlark), that value is
in the frontmatter at write time and is therefore available to the mapping:

```yaml
# schemas/notes.yaml (excerpt)
properties:
  ticket_refs:
    type: array
    items: { type: string }
    readOnly: true
    x-compute:
      lang: starlark
      from: body
      expr: re_find_all("[A-Z]+-[0-9]+", body)
```

```yaml
# .sbdb/integrations/notion.yaml
applies_to:
  notes:
    target_ref: sync.notion.pageId
    payload:
      tickets: { from: frontmatter.ticket_refs }
```

The Notion page gets a `tickets` property populated from whatever tickets
sbdb extracted from the body. Edit the doc body → next push refreshes the
property.

#### D. Multiple Notion databases mapped from different entities

```yaml
integration: notion
applies_to:
  meetings:
    target_ref: sync.notion.pageId
    payload:
      title:        { from: frontmatter.title }
      body:         { from: rendered_markdown }
      participants: { from: frontmatter.attendees }
      date:         { from: frontmatter.date }

  adrs:
    target_ref: sync.notion.pageId
    payload:
      title:    { from: frontmatter.title }
      body:     { from: rendered_markdown }
      status:   { from: frontmatter.status }
      decision: { from: frontmatter.decision }
```

The two entity types live in different Notion databases (the runtime infers
the database from the page itself or from MCP configuration). sbdb just
emits the right payload shape per entity.

### Combining services

Nothing in sbdb is exclusive — a single doc can publish to several services
at once by stacking back-refs in frontmatter and dropping multiple
integration files in `.sbdb/integrations/`:

```yaml
# docs/notes/q3-roadmap.md
---
id: q3-roadmap
created: 2026-05-27
title: Q3 Roadmap
sync:
  confluence: { pageId: "98765" }
  notion:     { pageId: "8a2f7c91-…" }
  slack:      { canvasId: "F12345ABC" }
---
```

The sidecar carries three top-level keys (`confluence:`, `notion:`,
`slack:`), each with independent `last_push` / `last_check` / `last_error`
state. Pushing to one service has no effect on the others — drift,
failures, and successes are tracked per integration.

## See also

- `sbdb sync --help`, `sbdb sync check --help`, `sbdb sync state set --help`
  for the full flag reference.
- `docs/guide/schemas.md` for how schemas declare entities.
- The README's "External sync" section for the design rationale and the
  bookkeeper-vs-runtime split.
