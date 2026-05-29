---
name: secondbrain-db-sync
description: |
  Use when driving sbdb external sync — pushing docs to Confluence, Jira, Notion,
  or Slack canvas via MCP and recording results back to sbdb. Triggers when the
  user says "sync", "publish to confluence/jira/notion", "push my docs", "what's
  out of sync", or when a Stop-hook surfaces sbdb sync drift. Requires sbdb >= 2.4.0
  and a .sbdb/integrations/*.yaml config in the project.
---

# secondbrain-db-sync

This skill teaches you to act as the **integration runtime** for sbdb's
external-sync feature. sbdb owns the bookkeeping (configs, sidecars, drift
detection). You own the network: you talk to Confluence / Jira / Notion /
Slack via MCP servers, and report results back through `sbdb sync` commands.

## Mental model

- **sbdb is the bookkeeper.** It never makes a network call. Never edit a
  `<doc>.integrations.yaml` sidecar by hand — always go through
  `sbdb sync state set` so the slot invariants (`last_push` durable,
  `last_check`/`last_error` transient) are preserved.
- **You are the actor.** Every HTTP call to an external service goes through
  an MCP server. If no MCP server for the target service is available, stop
  and tell the user — do not improvise with raw curl unless they ask.
- **The user is in control.** Never push to an external service without
  explicit confirmation for that specific doc and integration. The Stop hook
  surfaces drift; you ask; the user decides.

## The loop

### Step 1 — Detect drift

```bash
sbdb sync check --format json
```

Output is raw JSON (not the `{version,data}` envelope other sbdb commands
use):

```json
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

Exit code **4** means drift is present; **0** means everything is in sync;
**1** means a config/validation error (fix the config, don't try to push).

### Step 2 — Branch on `result`

| `result` | Meaning | Action |
|---|---|---|
| `in_sync` | Doc unchanged since last push, remote unchanged | Nothing |
| `local_drift` | Doc edited since last push | Offer to re-push |
| `never_published` | Declares a target but no recorded push | Offer to publish |
| `remote_drift` | Remote advanced since last push (only you can observe this; sbdb never polls) | Offer to re-publish — local is source of truth and will overwrite |
| `both_drift` | Both changed | Same as `remote_drift`: local wins per design |

### Step 3 — Ask the user

Per doc, per integration. Do not batch silently.

> "Doc `notes/launch-checklist` has local changes not yet pushed to Confluence
> page 98765. Push now?"

### Step 4 — Fetch the payload sbdb assembled

```bash
sbdb sync state get -s notes --id launch-checklist --integration confluence
```

```json
{
  "integration": "confluence",
  "entity": "notes",
  "doc_path": "/abs/path/docs/notes/launch-checklist.md",
  "target_ref": "sync.confluence.pageId",
  "target_id": "98765",
  "status": "linked",
  "required": false,
  "payload": { "title": "Launch Checklist", "body": "# Launch Checklist\n…" },
  "drift_result": "local_drift",
  "current_doc_hash": "sha256:b71e…",
  "state": {
    "target_id": "98765",
    "last_push": { "doc_hash": "sha256:8a2f…", "at": "…", "remote_revision": "7", "actor": "claude-code" },
    "last_check": null,
    "last_error": null
  }
}
```

`payload` is the field-shape contract from the integration config. `status:
"unlinked"` means the doc has no target ID for this integration — don't push;
ask the user to add the back-ref first.

### Step 5 — Push via MCP

The payload is field-shaped; you map it onto the service's API. Discover the
exact MCP tool names in-session. Illustrative mappings:

- **Confluence**: `update_page(pageId=target_id, title=payload.title, body=payload.body)`. The MCP server converts markdown body → Confluence storage format.
- **Jira**: `update_issue(issueKey=target_id, summary=payload.summary, description=payload.description)`.
- **Notion**: `update_page(page_id=target_id, properties={...}, children=blocks(payload.body))`. Map flat payload keys onto Notion properties (`title`, `status`, `tags`) and convert the body to Notion blocks.
- **Slack canvas**: `update_canvas(canvas_id=target_id, content=payload.body)`.

`payload.body` (sourced from `rendered_markdown`) is plain markdown text. If
the MCP server doesn't convert it to the service's native format, do the
conversion before the call.

### Step 6 — Record the result

Capture the revision/version the service returned. On **success**:

```bash
sbdb sync state set \
  -s notes --id launch-checklist --integration confluence \
  --published-hash "sha256:b71e…" \
  --remote-revision "8" \
  --at "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
```

Use the `current_doc_hash` from step 4 as `--published-hash` — that's what you
actually pushed. This overwrites `last_push` and clears `last_error`.

On **failure** (do not corrupt the last good push record):

```bash
sbdb sync state set \
  -s notes --id launch-checklist --integration confluence \
  --error "Confluence API 401: token expired" \
  --stage push \
  --attempted-doc-hash "sha256:b71e…" \
  --at "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
```

### Step 7 — Observing remote drift (only when asked)

If the user asks "is Confluence ahead of us?", fetch the current revision via
MCP, compare to `state.last_push.remote_revision`, and record:

```bash
sbdb sync state set \
  -s notes --id launch-checklist --integration confluence \
  --check-result remote_drift \
  --remote-revision-observed "9" \
  --at "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
```

Never poll on a schedule. Only on explicit request.

## Hard rules

- Never edit `<doc>.integrations.yaml` directly. Always `sbdb sync state set`.
- Never push without an explicit "yes" for that specific doc and integration.
- Never change a doc's `sync.<integration>.<id>` frontmatter without the
  user's instruction — that value is their declaration of which external
  object the doc maps to. Use `sbdb update` if they do ask.
- Never call an external API without an MCP server. sbdb's role excludes HTTP.
- `--at` is required on every `state set`; generate with
  `date -u +%Y-%m-%dT%H:%M:%SZ`.

## Inspecting targets

```bash
sbdb sync targets -s notes --id launch-checklist
```

```json
{
  "confluence": { "target_ref": "sync.confluence.pageId", "target_id": "98765", "status": "linked",   "required": false },
  "notion":     { "target_ref": "sync.notion.pageId",     "target_id": "",      "status": "unlinked", "required": false }
}
```

Useful when onboarding a doc to a new integration — shows which back-refs are
still missing.

## Common error responses

- `integration "X" not configured` — `.sbdb/integrations/X.yaml` is missing.
  Don't push; tell the user.
- `--at is required` — every state-write needs a timestamp.
- `unknown drift result` — `--check-result` only accepts `in_sync`,
  `local_drift`, `remote_drift`, `both_drift`, `never_published`.
- `sync target validation failed` from `sbdb create`/`update` — an
  integration with `required: true` applies to this entity but the doc lacks
  the back-ref. Add `sync.<integration>.<field>` to the frontmatter.

## Cross-reference

For the broader sbdb model (schemas, docs, integrity, events, queries), see
the main `secondbrain-db` skill. For full command flags, see
`reference/cli-reference.md` in that skill, or run `sbdb sync --help`.
