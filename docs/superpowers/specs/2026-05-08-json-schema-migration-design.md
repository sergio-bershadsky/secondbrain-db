# Design: JSON Schema 2020-12 with `x-*` extensions

- **Issue:** [#46](https://github.com/sergio-bershadsky/secondbrain-db/issues/46)
- **Date:** 2026-05-08
- **Status:** Draft for review
- **Type:** `feat`

## Summary

Replace sbdb's hand-rolled YAML schema dialect with standard **JSON Schema 2020-12**. Sbdb-specific concepts (entity name, storage layout, id field, partitioning, integrity policy, event bucket, computed fields) become bare `x-*` extension keywords — the conventional OpenAPI / JSON Schema vendor-extension namespace. Foreign-key-style references collapse onto JSON Schema's native `$ref`. Virtuals become regular properties with `readOnly: true` plus an `x-compute` block.

The loader auto-migrates legacy schemas on read, so no existing knowledge base breaks. `sbdb schema migrate` rewrites legacy YAML in place; `sbdb schema lint` validates against embedded sbdb meta-schemas. Validation swaps to `github.com/santhosh-tekuri/jsonschema/v6`.

## Goals

1. Schema files are valid, drop-in JSON Schema 2020-12. A stock validator (e.g. `ajv`) loads them and ignores the `x-*` keys per spec.
2. Editor LSPs (yaml-language-server, IntelliJ, VS Code) provide autocomplete and diagnostics out of the box, given a `# yaml-language-server: $schema=...` directive or `.json` extension.
3. Existing knowledge bases continue to load without changes; users opt into the new on-disk form via `sbdb schema migrate` when ready.
4. Less code: replace the custom validator with a battle-tested library; carry only the sbdb-specific cross-document checks ourselves.
5. Foreign-key references reuse JSON Schema's `$ref`. The link graph derives entity edges from the `$ref` URIs themselves — no separate metadata channel.

## Non-goals (v1)

- Publishing the meta-schemas to a real HTTPS host. The `$id` URLs are reserved (`https://schemas.sbdb.dev/2026-05/...`) but they are loaded from the binary's embedded copy. Hosting is a follow-up.
- OpenAPI 3.1 spec auto-generation from sbdb schemas. Now trivially possible — out of scope here.
- JSON Schema → TypeScript / Python / Go type generation.
- Renaming the project's own dialect docs to refer exclusively to JSON Schema. Some user-facing docs will keep the legacy form alongside the new for one release.

## Reserved keywords

After this work the only sbdb-specific schema keywords are:

| Keyword | Where | Replaces (legacy) | Type / shape |
|---|---|---|---|
| `x-schema-version` | top | `version` | integer (currently always 1) |
| `x-entity` | top | `entity` | string (entity name) |
| `x-storage` | top | `docs_dir`, `records_dir`, `filename` | object: `{docs_dir, records_dir?, filename}` |
| `x-id` | top | `id_field` | string (property name) |
| `x-partition` | top | `partition`, `date_field` | object: `{mode: none|monthly, field?: string}` |
| `x-integrity` | top | `integrity` | enum: `strict | warn | off` |
| `x-events` | top | `bucket`, `event_types` | object: `{bucket?: string, types: {...}}` |
| `x-compute` | per-property | `virtuals.*` | object: `{source: string, edge?: bool, edge_entity?: string}` |

Everything else is core JSON Schema 2020-12. There is no `x-ref`: references use `$ref`.

## On-disk shape (canonical)

```yaml
$schema: "https://json-schema.org/draft/2020-12/schema"
$id: "sbdb://notes"
x-schema-version: 1
x-entity: notes
x-storage:
  docs_dir: docs/notes
  filename: "{id}.md"
x-id: id
x-integrity: strict

type: object
required: [id, created]
properties:
  id:
    type: string
    pattern: "^[a-z0-9-]+$"
  created:
    type: string
    format: date
  status:
    enum: [active, archived]
    default: active
  tags:
    type: array
    items: { type: string }

  # Foreign key — pure $ref. Validator inherits the target's id schema.
  parent:
    $ref: "sbdb://notes#/properties/id"

  # Polymorphic foreign key — JSON Schema's oneOf.
  related_to:
    oneOf:
      - $ref: "sbdb://notes#/properties/id"
      - $ref: "sbdb://discussions#/properties/id"

  # Virtual: regular property + readOnly + x-compute.
  title:
    type: string
    readOnly: true
    x-compute:
      source: |
        def compute(content, fields):
            for line in content.splitlines():
                if line.startswith("# "):
                    return line.removeprefix("# ").strip()
            return fields["id"]

  # Virtual that produces graph edges.
  related_topics:
    type: array
    items:
      $ref: "sbdb://topics#/properties/id"
    readOnly: true
    x-compute:
      source: |
        def compute(content, fields):
            ...
      edge: true
```

Schemas may also be authored as `.json`. The loader accepts either extension.

## Foreign-key model

Sbdb's link graph is derived, not declared. The loader walks the resolved `$ref` graph for every property in every schema. A `$ref` of the form `sbdb://<entity>#/properties/<id-field-of-entity>` is treated as an edge from the containing schema's entity to `<entity>`. The id field is read from the target schema's `x-id`.

Validation of *existence* (does the referenced doc actually exist?) stays a cross-document concern in `pkg/sbdb/kg`, run on `doctor check` and on insert/update. JSON Schema cannot express it and we do not try to.

## Virtuals

Virtuals are properties with `readOnly: true` and an `x-compute` block:

```yaml
title:
  type: string
  readOnly: true
  x-compute:
    source: "def compute(content, fields): ..."
    edge: false                 # default
    edge_entity: ""             # ignored unless edge=true
```

`type` participates in normal validation: when the Starlark function returns a value that does not match the declared type, the schema layer reports it as a validation error against the *computed* record. This is the same behavior we have today.

`readOnly: true` is JSON-Schema-native and means "the value is set by the owning authority, not by the data submitter." That matches the sbdb semantics exactly.

`x-compute.source` holds the Starlark code. We deliberately do not yet wrap it in a `{lang: starlark, code: ...}` envelope — the implementation reserves the right to introduce that shape later if we add other compute languages, but YAGNI for now. If we add it, the bare-string form continues to be accepted as syntactic sugar.

`x-compute.edge: true` marks the property's values as graph edges. `x-compute.edge_entity` defaults to the entity referenced by the property's own `$ref` / `items.$ref` (so it usually doesn't need to be set explicitly).

## Meta-schemas

Two YAML files embedded in the binary, written to `.sbdb/cache/meta/` on `sbdb init`:

- `sbdb.schema.json` — extends `https://json-schema.org/draft/2020-12/schema`. Adds:
  - Required: `x-entity`, `x-storage` (with required `docs_dir`, `filename`), `x-id`.
  - Optional with strict types: `x-schema-version`, `x-partition`, `x-integrity`, `x-events`.
  - For every property under `properties`, allows `x-compute` and validates it via the second meta-schema.
- `sbdb.compute.schema.json` — types the `x-compute` block: `source` required string, `edge` optional boolean, `edge_entity` optional string.

Files in `.sbdb/cache/meta/` are gitignored — they are a per-clone convenience for editor tooling. Users may also reference the canonical URL in their `# yaml-language-server: $schema=...` directive once the URL is published.

## Loader

`pkg/sbdb/schema/loader.go` becomes a small dispatcher:

```
Load(path) []byte →
  parse YAML or JSON to map[string]any
  detect dialect:
    has "$schema" or any "x-*" key or "properties" → new
    has "entity" + "fields" + no "$schema"          → legacy
  legacy → normalise to new shape (in memory)
  new    → pass through
  parse into internal Schema struct
```

The internal `Schema` struct is reshaped so the new keyword names are the canonical names. The legacy normaliser is a one-way translator with full coverage of every legacy field — so the codepath after normalisation is identical for both inputs. There are not two parsers, just one parser plus a key-rewriter.

Detection is conservative: a file with no `$schema` and no `x-*` keys but with `properties` is treated as new (since legacy uses `fields`). A file with `entity:` at the top level and `fields:` is legacy. Ambiguity (both shapes present) is rejected with an explanatory error.

## Validation

We adopt `github.com/santhosh-tekuri/jsonschema/v6`:

- It supports draft 2020-12 natively.
- It allows custom keywords via the `Vocabulary` API. We register the sbdb keywords (`x-storage`, `x-integrity`, etc.) so they participate in `lint` and so unknown variants are caught early.
- It resolves `$ref` across files, including `sbdb://` URIs (we plug a custom loader that resolves `sbdb://<entity>` against the registered schema set).

Cross-document checks remain in `pkg/sbdb/schema/validate.go` and `pkg/sbdb/kg`:

- Foreign-key existence (does the referenced doc exist?).
- Virtual return-type vs declared `type` of the property.
- Partition consistency (`x-partition.field` must reference a `format: date` or `format: date-time` property when mode is `monthly`).
- Filename pattern variables resolve to declared properties (`{id}` → there must be a property named `id`).

These run after JSON Schema validation succeeds; failures are reported with the same diagnostic shape as today.

## CLI surface

```
sbdb schema lint <path>...                # validate file(s) against meta-schemas
sbdb schema migrate <path>... [--check] [--in-place|-o <dir>]
                                          # rewrite legacy → new shape
sbdb schema diff <old> <new>              # classify additive vs breaking deltas
sbdb schema check [--against <git-ref>]   # validate every existing doc against current schema
sbdb schema show <entity>                 # already exists; updated to emit new shape
```

`migrate --check` exits non-zero if any input file is still legacy (CI-friendly). Default writes alongside as `<name>.new.yaml`; `--in-place` rewrites the original.

`show` prints the in-memory schema in the new on-disk form, so it is useful as both an inspection tool and an emergency migrator (`sbdb schema show notes > schemas/notes.yaml`).

## Backwards compatibility

Dual-dialect parsing is permanent in the loader. The cost is a constant-time key sniff per file; not worth ripping out. No deprecation warning is emitted by default; users who want a warning set `SBDB_WARN_LEGACY_SCHEMA=1`. Documented in the user guide section we add as part of this work.

`sbdb doctor check` does not flag legacy schemas. `sbdb schema lint` does (with a clear "this is legacy, run `sbdb schema migrate`" message).

## Schema evolution guardrails

Schema files describe the shape of every existing document. Editing a schema changes what counts as valid retroactively. Without guardrails an additive-looking edit (a new required field, a tightened enum) silently invalidates the entire knowledge base on the next `doctor check`. The pre-commit hook is where the user feels the change first, so that is where we enforce.

### What "breaking" means

| Change | Class |
|---|---|
| Add optional property | additive |
| Add property to `required` | breaking |
| Remove a property when `additionalProperties: false` | breaking |
| Tighten an `enum`, `pattern`, `minLength`, `minimum`, `maxLength`, `maximum` | breaking |
| Change `type` | breaking |
| Loosen a constraint | additive |
| Add or remove an entity / schema file | additive (independent of existing docs) |
| Rename a property | breaking (modelled as remove + add) |
| Edit `x-compute.source` | additive (only changes computed values, not stored data) |
| Edit `x-storage.docs_dir` / `x-storage.filename` | breaking (changes where docs live; needs migration) |

### Three commands

- **`sbdb schema diff <old> <new>`** — pure schema comparison. Walks both schemas, classifies each delta as `additive` or `breaking`, prints both lists. Exit code `0` if additive-only, `1` if any breaking. No docs needed; runs in milliseconds.
- **`sbdb schema check [--against <git-ref>]`** — empirical compatibility check. Runs the new schema against every existing doc on disk. With `--against HEAD~1`, diffs the working-tree schema against docs that were valid under the previous committed schema. Tells the truth regardless of how subtle the schema edit was. Default exit `0` only if every doc still validates.
- **`sbdb schema lint <path>...`** — already in the spec; validates a schema file against the meta-schemas. Catches malformed `x-*` blocks before they reach the validator.

### Pre-commit hook (the gate)

Sbdb ships a local pre-commit hook entry in the project's `.pre-commit-config.yaml`. The hook fires on commits that modify any file under `schemas/` or any file matching `*.schema.{yaml,yml,json}`:

```yaml
- repo: local
  hooks:
    - id: sbdb-schema-validate
      name: sbdb schema validate
      entry: scripts/schema-precommit.sh
      language: script
      files: '^(schemas/.*\.(ya?ml|json)|.*\.schema\.(ya?ml|json))$'
      pass_filenames: true
```

`scripts/schema-precommit.sh` runs three checks on every staged schema file:

1. `sbdb schema lint <file>` — meta-schema validation.
2. `sbdb schema diff HEAD:<file> <file>` — classify the change.
3. If diff reports `breaking`, run `sbdb schema check --against HEAD` and refuse the commit unless `x-schema-version` major component has been bumped *and* every existing doc still validates against the new schema (i.e. user has already migrated docs in the same commit).

Override is intentional friction: pass `SBDB_ALLOW_BREAKING=1` in the environment to skip the breaking check (useful in mid-rebase or merge-conflict states). The override does not skip `lint` or `diff` — only the failing-docs check.

### Schema version convention

`x-schema-version` becomes a meaningful number, not always `1`:

- Major bump (1 → 2): a breaking change has been made. Required when `schema diff` reports breaking; the pre-commit hook enforces.
- Optional minor sub-component (e.g. `x-schema-version: "1.3"`): additive-only. Not enforced; convention only.

This does not introduce multi-version coexistence — at any moment the schema file in `main` is the single source of truth and every doc must validate against it. The version number signals to operators that a migration occurred between two points in history; release-please can also surface it in changelog entries.

### Doc migration story (manual, v1)

When the user knowingly makes a breaking change:

1. Edit schema.
2. `sbdb schema check` lists docs that would fail and why.
3. User edits the docs (manually, or via batched `sbdb update` calls) in the same commit.
4. User bumps `x-schema-version` major component.
5. `git commit` — pre-commit hook re-runs `schema check`, now passes.

Declarative `x-migrations` blocks (transformations applied to docs at load time) are explicitly out of scope for v1. Revisit if real user need surfaces.

## Risks and mitigations

- **Behaviour drift between hand-rolled validator and library.** Mitigation: an exhaustive port of the existing validator test suite to run against the new path, plus a phase where both validators run side-by-side in tests and we assert results agree on every existing testdata fixture.
- **Library performance regression.** Mitigation: micro-benchmark on a 1000-doc fixture; the current validator is fast because it's narrow; the library is fast because it compiles schemas once. Compare before/after in CI; if regression is real, cache compiled schemas in the runtime.
- **`$ref` to non-existent target.** Mitigation: catch at schema-set load time (we know all entities up front), report once, refuse to start. Today's behaviour is roughly the same for legacy `ref` types — no regression.
- **Multi-schema `$ref` resolution edge cases.** Mitigation: pin the resolver to a registered in-memory schema set; never reach the network. `sbdb://` URIs are the only cross-schema form sbdb emits.
- **Legacy schemas that exercise edge cases the normaliser misses.** Mitigation: add a fixture-driven test that walks `testdata/` and asserts every legacy schema migrates to a new-shape schema that re-parses identically. CI gate.

## Open questions

None blocking. Implementation will pin (a) whether `x-compute.source` accepts the `{lang, code}` envelope from day one (leaning no — YAGNI; introduce when we have a second compute language), and (b) the exact `sbdb://` URI scheme registration in the resolver (one entity per schema; the entity is the path).

## Acceptance criteria

- Spec landed and reviewed.
- Implementation plan landed.
- New loader + normaliser + validator integration; old hand-rolled validator deleted.
- Two meta-schemas embedded in the binary; written to `.sbdb/cache/meta/` on `sbdb init`.
- `sbdb schema lint` rejects malformed `x-*` blocks (e.g. wrong type for `x-integrity`, missing `x-storage.docs_dir`).
- `sbdb schema diff` classifies a curated set of edits correctly (table in the Schema Evolution Guardrails section).
- `sbdb schema check` reports failing docs accurately; pre-commit hook refuses commits that would break the KB unless `x-schema-version` major is bumped and docs are migrated in the same commit.
- Pre-commit hook entry exists in `.pre-commit-config.yaml`; `scripts/schema-precommit.sh` runs lint + diff + check; override via `SBDB_ALLOW_BREAKING=1` works.
- `sbdb schema migrate` round-trips: migrate(legacy) parses cleanly under the new loader; in-memory `Schema` is identical to the legacy parse.
- An external JSON Schema validator (added as a test dep, e.g. `ajv` driven from a small Node test harness, or another Go validator) accepts every migrated schema file with no errors (ignoring `x-*` per spec).
- All existing testdata fixtures pass `sbdb doctor check` after migration.
- User guide section added covering the migration, the new shape, and editor LSP setup.
