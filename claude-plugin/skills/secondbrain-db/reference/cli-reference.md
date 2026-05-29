# CLI Reference

## Global flags

| Flag | Short | Description |
|------|-------|-------------|
| `--schema-dir` | `-S` | Schemas directory (default: ./schemas) |
| `--schema` | `-s` | Schema name to use |
| `--base-path` | `-b` | Project root directory |
| `--format` | `-f` | Output: json, yaml, table (default: auto) |
| `--quiet` | | Suppress progress output |
| `--verbose` | | Increase logging |
| `--dry-run` | | Show what would change without writing |

## CRUD

### create
```bash
sbdb create --input -                        # JSON from stdin
sbdb create --field id=X --field status=Y    # field flags
sbdb create --field id=X --content-file body.md
```

### get
```bash
sbdb get --id <id>                           # full record + content
sbdb get --id <id> --no-content              # fields only
```

### list
```bash
sbdb list                                    # all records
sbdb list --limit 10 --order -created        # paginated, sorted
sbdb list --fields id,status                 # projection
```

### query
```bash
sbdb query --filter status=active                    # exact match
sbdb query --filter created__gte=2026-01-01          # date range
sbdb query --filter title__icontains=deploy          # case-insensitive search
sbdb query --filter status__in=active,draft           # in list
sbdb query --exclude status=archived                  # exclude
sbdb query --order -created --limit 5                 # sorted, limited
sbdb query --count                                    # count only
sbdb query --exists                                   # existence check
```

Lookup suffixes: `__gte`, `__lte`, `__gt`, `__lt`, `__in`, `__contains`, `__icontains`, `__startswith`

### update
```bash
sbdb update --id <id> --field status=archived        # set field
sbdb update --id <id> --field 'tags+=new-tag'        # append to list
sbdb update --id <id> --field 'tags-=old-tag'        # remove from list
sbdb update --id <id> --content-file new.md          # replace body
sbdb update --id <id> --input -                      # merge JSON from stdin
```

### delete
```bash
sbdb delete --id <id> --yes                          # hard delete
sbdb delete --id <id> --soft --yes                   # set status=archived
```

## search
```bash
sbdb search "phrase"                                 # grep full-text
sbdb search "phrase" --semantic --k 10               # vector similarity
sbdb search "phrase" --semantic --expand --depth 1   # + graph neighbors
```

## doctor
```bash
sbdb doctor check                   # check drift + tamper
sbdb doctor fix --recompute         # fix drift, recompute virtuals
sbdb doctor sign --force            # re-sign after intentional edits
sbdb doctor status                  # summary
sbdb doctor init-key                # generate HMAC key
```

## graph
```bash
sbdb graph incoming --id <id>       # edges pointing TO doc
sbdb graph outgoing --id <id>       # edges FROM doc
sbdb graph neighbors --id <id> --depth 2   # BFS traversal
sbdb graph export --export-format json     # for visualization
sbdb graph export --export-format mermaid  # for markdown
sbdb graph stats                    # node/edge counts
```

## index
```bash
sbdb index build                    # schema-mode index
sbdb index build --crawl            # crawl all .md files
sbdb index build --force            # re-index everything
sbdb index stats                    # index statistics
sbdb index drop --yes               # delete index
```

## schema
```bash
sbdb schema list                    # available schemas
sbdb schema show --format json      # active schema details
sbdb schema json-schema             # JSON Schema output
```

## sync
External-sync bookkeeping. sbdb never calls external services — these commands
let an integration runtime (Claude + MCP) read declared intent and record what
it did. Output is raw JSON (not the `{version,data}` envelope). See the
`secondbrain-db-sync` skill for the end-to-end workflow.

```bash
sbdb sync check --format json          # local-only drift report; exit 0=clean, 4=drift, 1=config error
sbdb sync targets -s notes --id hello  # resolved back-refs for one doc, all integrations
sbdb sync state get -s notes --id hello --integration confluence   # resolved payload + sidecar state
sbdb sync state set -s notes --id hello --integration confluence \
  --published-hash <h> --remote-revision <r> --at <ts> [--actor <a>]  # record successful push
sbdb sync state set ... --check-result <enum> --at <ts> [--remote-revision-observed <r>]  # record observation
sbdb sync state set ... --error "<msg>" --stage <push|check> --at <ts> [--attempted-doc-hash <h>]  # record failure
```

`check` results: `in_sync`, `local_drift`, `remote_drift`, `both_drift`, `never_published`.
`--at` is required on every `state set` (RFC3339 UTC, e.g. `date -u +%Y-%m-%dT%H:%M:%SZ`).
Integration configs live in `.sbdb/integrations/<name>.yaml`; per-doc state lives
in `<doc>.integrations.yaml` (managed by sbdb — never hand-edit).
