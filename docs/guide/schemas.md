# Schemas

`sbdb` schemas are valid **JSON Schema 2020-12** documents with a small set of `x-*` extension keywords for sbdb-specific concepts. A stock JSON Schema validator (e.g. `ajv`) accepts them; editor LSPs (yaml-language-server, IntelliJ, VS Code) provide autocomplete and diagnostics out of the box.

## Anatomy of a schema

```yaml
$schema: https://json-schema.org/draft/2020-12/schema
$id: sbdb://notes
x-schema-version: 1
x-entity: notes
x-storage: { docs_dir: docs/notes, filename: "{id}.md" }
x-id: id
x-integrity: strict

type: object
required: [id, created]
properties:
  id:      { type: string, pattern: "^[a-z0-9-]+$" }
  created: { type: string, format: date }
  status:  { enum: [active, archived], default: active }
  tags:    { type: array, items: { type: string } }
  parent:  { $ref: "sbdb://notes#/properties/id" }   # foreign key
  title:                                             # virtual
    type: string
    readOnly: true
    x-compute:
      source: |
        def compute(content, fields):
            ...
```

## Reserved keywords

| Keyword | Where | Meaning |
|---|---|---|
| `x-schema-version` | top | integer (or "major.minor") tracking schema evolution |
| `x-entity` | top | entity name (slug) |
| `x-storage` | top | `{docs_dir, filename, records_dir?}` |
| `x-id` | top | name of the id property |
| `x-integrity` | top | `strict | warn | off` |
| `x-partition` | top | `{mode: none|monthly, field?: string}` |
| `x-events` | top | event bucket + types |
| `x-compute` | per-property | virtual computation block |

Foreign-key references are pure JSON Schema `$ref` pointing at `sbdb://<entity>#/properties/<id>`. The link graph derives entity edges from these URIs.

## Editor support

Add this directive at the top of any schema YAML:

```
# yaml-language-server: $schema=.sbdb/cache/meta/sbdb.schema.json
```

`sbdb init` writes the meta-schemas into `.sbdb/cache/meta/` (forthcoming follow-up).

## Migration from the legacy dialect

```bash
sbdb schema migrate --in-place schemas/notes.yaml
```

Or `--check` to fail CI if any legacy schemas remain.

## Schema evolution guardrails

| Command | Purpose |
|---|---|
| `sbdb schema lint <file>` | Validate against the meta-schema |
| `sbdb schema diff <old> <new>` | Classify deltas as additive vs breaking |
| `sbdb schema check` | Run schemas against every existing doc |

The pre-commit hook (`sbdb-schema-validate` in `.pre-commit-config.yaml`) runs all three and refuses commits that would invalidate existing docs unless `x-schema-version` major is bumped or `SBDB_ALLOW_BREAKING=1` is set.
