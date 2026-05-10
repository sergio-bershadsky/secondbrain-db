---
name: secondbrain-db-edit
description: |
  Use when creating, updating, or deleting documents in an sbdb-managed
  knowledge base. Triggers on: "create a note", "add a discussion",
  "update this ADR", "edit the KB", "write a new page", "add content",
  "modify frontmatter", "create template", or any operation that writes
  to docs/ in a project with .sbdb.toml.
---

# KB Edit Skill — Direct edits with end-of-turn reconciliation

In sbdb v2 each document is a pair: `<id>.md` (content + frontmatter)
and a sibling `<id>.yaml` integrity sidecar. Both live under
`docs/<entity>/` and are committed to git. There is no `data/`
directory and no aggregate index.

## Default mode: edit `.md` files directly

**Use `Edit`, `Write`, and `MultiEdit` directly on any `.md` file under
`docs/`.** Treat it like any other markdown repo. A Stop hook reconciles
sidecars at end of turn via `sbdb doctor heal --since HEAD --i-meant-it`,
which:

- Recomputes virtual fields from the on-disk markdown.
- Re-signs sidecars in lockstep with the new content.
- Skips files outside any schema's `docs_dir` silently.

You don't run any "after-edit" command. You don't think about sidecars
during the session. The hook fires when your turn ends and prints a
one-line summary like:

> [sbdb] post-fix heal: 2 re-signed, 1 sidecar(s) recreated

### Creating a new document

```
Write to docs/notes/my-new-note.md:
---
id: my-new-note
created: 2026-05-10
status: active
---
# My note

Body here.
```

That's it. The Stop hook creates the `<id>.yaml` sidecar with fresh
hashes when the turn ends.

If you need to discover the schema's required frontmatter fields first:

```bash
sbdb schema show -s notes --format json
```

### Editing existing content

Just `Edit` the `.md`. Frontmatter changes, body changes, replacements —
all handled. No special command.

### Deleting a document

```bash
rm docs/notes/old-note.md docs/notes/old-note.yaml
```

Or use `sbdb delete -s notes --id old-note --yes` if you want a single
command that handles both files.

## When to use the `sbdb` CLI instead

The CLI is still the right tool when:

- **You want JSON I/O** — piping records between scripts:
  ```bash
  echo '{"id":"x","created":"2026-05-10","content":"# X"}' \
    | sbdb create -s notes --input -
  ```
- **Bulk frontmatter ops** — set a status across many docs:
  ```bash
  for id in $(sbdb query -s notes --filter status=draft --format json | jq -r '.data[].id'); do
    sbdb update -s notes --id "$id" --field status=published
  done
  ```
- **You're committing mid-conversation** and need pre-commit to pass
  immediately, not at end-of-turn.
- **Block mode is active** (see below).

## Recovering from drift / tamper

If `sbdb doctor check` reports issues — usually because edits happened
across multiple sessions or outside Claude Code — heal them:

```bash
sbdb doctor heal --i-meant-it           # heal everything dirty vs HEAD
sbdb doctor heal --i-meant-it --id foo  # heal one doc
sbdb doctor heal --i-meant-it --all     # heal everything in every schema
sbdb doctor heal --i-meant-it --since main  # heal everything dirty vs main
```

`heal` is sugar over `fix --recompute` + `sign --force`, with one
safety property: `--i-meant-it` is your acknowledgement that any
tampered files were edited intentionally. Without it, tamper exits 6
with an error message.

## Block mode (opt-in, strict guard)

Some KBs need real-time tamper detection — compliance ADRs, audit logs,
anything where "an unintentional edit slipped through" is a serious
problem. Activate by adding to `.sbdb.toml`:

```toml
[claude]
mode = "block"
```

In block mode:

- `Edit`, `Write`, `MultiEdit` under `docs/` are **denied** by a
  PreToolUse hook.
- A Stop hook actively **blocks** the agent from finishing with a dirty
  KB.
- All writes go through `sbdb create / update / delete`.

### Block-mode workflow

```bash
# Create (JSON on stdin)
echo '{"id":"ADR-0005","number":5,"title":"New Architecture","status":"draft","created":"2026-05-10","content":"# ADR-0005\n\n## Context\n..."}' \
  | sbdb create -s adr --input -

# Update fields
sbdb update -s adr --id ADR-0005 --field status=accepted

# Replace body from a file (the only ergonomic way to edit multi-line markdown
# in block mode — write the new body to /tmp/body.md, then pass it in)
sbdb update -s adr --id ADR-0005 --content-file /tmp/body.md

# Combined body + frontmatter via JSON
echo '{"content":"# updated\n\n...","status":"accepted"}' \
  | sbdb update -s adr --id ADR-0005 --input -

# Delete
sbdb delete -s adr --id ADR-0005 --yes
```

The PreToolUse hook's deny message lists these flags whenever you trip
it.

## Untracked files (both modes)

Files outside any schema's `docs_dir` (`docs/index.md`, `TEMPLATE.md`,
custom pages) aren't reconciled by the heal hook — `heal --since` skips
them with `outcome=skipped_no_schema`. Manage them via:

```bash
sbdb untracked sign <path>          # sign an existing file
sbdb untracked sign-all docs/       # sign every untracked .md under docs/
sbdb untracked create <path> --content-file <body>
```

## Rules

1. **Don't edit `<id>.yaml` sidecars directly.** They're integrity
   artefacts. The Stop hook regenerates them; touching them by hand
   only creates drift.
2. **In post-fix mode (the default), don't run `sbdb doctor check`
   reflexively after every edit.** The hook does it for you. Run it
   when something feels off, when reviewing a long session, or before
   merging a PR.
3. **Use `heal --i-meant-it` for recovery, not `sign --force --all`.**
   Heal recomputes virtuals before signing, which is the order that
   keeps record_sha consistent. Bare `sign --force` skips that step.
4. **In block mode, never use Edit/Write under `docs/`** — the hook
   denies it and the user has explicitly opted into that posture.
