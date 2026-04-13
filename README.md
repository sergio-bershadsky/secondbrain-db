# secondbrain-db

[![CI](https://github.com/sergio-bershadsky/secondbrain-db/actions/workflows/ci.yml/badge.svg)](https://github.com/sergio-bershadsky/secondbrain-db/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Report Card](https://goreportcard.com/badge/github.com/sergio-bershadsky/secondbrain-db)](https://goreportcard.com/report/github.com/sergio-bershadsky/secondbrain-db)

A file-backed knowledge base ORM. Define schemas in YAML, compute virtual fields with Starlark, query with a chainable filter API, and verify integrity with SHA-256 + HMAC signing. Single static binary, designed as an AI-agent API layer.

## Why this exists

Knowledge lives in markdown files. Teams write ADRs, meeting notes, guides, and architecture docs as `.md` files with YAML frontmatter, then serve them through VitePress, Docusaurus, Obsidian, or Jekyll. This works until it doesn't.

The problems start quietly. A frontmatter field says `status: active` but the YAML index says `status: draft`. Someone edits a file by hand and breaks the frontmatter format. A new team member creates a discussion note in the wrong directory. An AI agent rewrites a doc and silently drops three metadata fields. Six months in, nobody trusts the data, searches return stale results, and the knowledge base has become a knowledge graveyard.

The root cause is that **markdown files are treated as dumb text when they're actually structured data**. They have schemas (frontmatter fields), relationships (links between docs), computed properties (title from heading, word count, ticket references), and lifecycle states (draft, active, archived). But nothing enforces this structure. Nothing detects when it breaks. Nothing connects the dots between documents.

`secondbrain-db` exists because knowledge bases deserve the same guarantees that databases take for granted:

- **Schema validation** — a note with `status: invalid` is rejected, not silently accepted
- **Integrity signing** — every file is hashed; hand-edits are detected, not lost in the noise
- **Computed fields** — the title is extracted from the `# heading` once, not maintained by hand in two places
- **Queryable indexes** — filtering 1,000 records reads one YAML file, not 1,000 markdown files
- **Relationship tracking** — when doc A links to doc B, that relationship is a first-class edge in a knowledge graph, not a string buried in prose
- **Two-tier tracking** — structured entities get full ORM treatment; unstructured pages (templates, index pages, guides) still get integrity signing and graph inclusion

The tool is deliberately a **single static binary** (`sbdb`) that operates on **plain files on disk**. No database server. No lock-in. Your docs stay as markdown files that any tool can read. `sbdb` layers structure, integrity, and intelligence on top — and gets out of the way when you don't need it.

It's also designed as an **API layer for AI agents**. Every command outputs structured JSON. Exit codes are stable and semantic. Schema introspection is self-describing. A Claude Code plugin ships with integrity hooks that automatically detect and fix drift after every edit. The premise is simple: if an AI is going to help maintain your knowledge base, the knowledge base needs to be able to tell the AI when something is wrong.

## Install

```bash
go install github.com/sergio-bershadsky/secondbrain-db@latest
```

Or download from [GitHub Releases](https://github.com/sergio-bershadsky/secondbrain-db/releases).

## Quickstart

```bash
# Initialize a project
sbdb init --template notes

# Create a document
echo '{"id":"hello","created":"2026-04-08","status":"active","content":"# Hello World\n\nMy first note."}' | sbdb create -s notes --input -

# Query
sbdb query -s notes --filter status=active --format json

# Get by ID
sbdb get -s notes --id hello --format json

# Check consistency
sbdb doctor check -s notes

# Update
sbdb update -s notes --id hello --field status=archived

# Delete
sbdb delete -s notes --id hello --yes
```

## Schema format

Schemas live in `schemas/*.yaml`:

```yaml
version: 1
entity: notes
docs_dir: docs/notes
filename: "{id}.md"
records_dir: data/notes
id_field: id
integrity: strict

fields:
  id:      { type: string, required: true }
  created: { type: date, required: true }
  status:  { type: enum, values: [active, archived], default: active }
  tags:    { type: list, items: { type: string } }

virtuals:
  title:
    returns: string
    source: |
      def compute(content, fields):
          for line in content.splitlines():
              if line.startswith("# "):
                  return line.removeprefix("# ").strip()
          return fields["id"]
```

## Tutorial: building a custom schema from scratch

`sbdb` ships with three built-in templates (`notes`, `blog`, `adr`), but the real power is defining your own schemas for any entity type you need. This tutorial walks through creating a **recipe book** knowledge base end-to-end.

### Step 1: Initialize a project

Start with a fresh directory. `sbdb init` creates the folder structure and a starter config.

```bash
mkdir my-recipes && cd my-recipes
sbdb init --template notes
```

This creates:

```
my-recipes/
├── .sbdb.toml          # project config
├── schemas/
│   └── notes.yaml      # default schema (we'll replace this)
├── docs/               # markdown files live here
└── data/               # records + integrity manifests live here
```

### Step 2: Design your schema

Think about what fields your entity needs. For recipes:

- **Scalar fields** (searchable via `records.yaml`): `slug`, `created`, `cuisine`, `difficulty`, `prep_time`
- **Complex fields** (frontmatter only): `ingredients` (list of objects), `tags`
- **Virtual fields** (computed from content): `title`, `step_count`

Create `schemas/recipes.yaml`:

```yaml
version: 1
entity: recipes
docs_dir: docs/recipes
filename: "{slug}.md"
records_dir: data/recipes
partition: none
id_field: slug
integrity: strict

fields:
  slug:       { type: string, required: true }
  created:    { type: date, required: true }
  cuisine:    { type: string, required: true }
  difficulty: { type: enum, values: [easy, medium, hard], default: easy }
  prep_time:  { type: int }
  tags:       { type: list, items: { type: string } }
  ingredients:
    type: list
    items:
      type: object
      fields:
        name:     { type: string, required: true }
        amount:   { type: string, required: true }
        unit:     { type: string }
```

Each field declaration follows this format:

```yaml
field_name: { type: <type>, required: <bool>, default: <value> }
```

Supported types: `string`, `int`, `float`, `bool`, `date`, `datetime`, `enum`, `list`, `object`.

### Step 3: Add virtual fields

Virtual fields are Starlark functions that extract or compute values from the markdown body. Add them to your schema:

```yaml
# append to schemas/recipes.yaml

virtuals:
  title:
    returns: string
    source: |
      def compute(content, fields):
          for line in content.splitlines():
              if line.startswith("# "):
                  return line.removeprefix("# ").strip()
          return fields["slug"]

  step_count:
    returns: int
    source: |
      def compute(content, fields):
          count = 0
          for line in content.splitlines():
              stripped = line.strip()
              if len(stripped) > 2 and stripped[0].isdigit() and stripped[1] == ".":
                  count += 1
          return count
```

Rules for Starlark virtuals:
- The function must be named `compute` and accept `(content, fields)`
- `content` is the markdown body as a string
- `fields` is a dict of all field values
- `re.findall(pattern, text)` is available for regex
- No file I/O, no imports, no network — fully sandboxed

### Step 4: Update your config

Edit `.sbdb.toml` to point at your new schema:

```toml
schema_dir = "./schemas"
base_path = "."
default_schema = "recipes"

[output]
format = "auto"

[integrity]
key_source = "env"
```

With `default_schema = "recipes"`, you no longer need `-s recipes` on every command.

### Step 5: Verify the schema loads

```bash
# List available schemas
sbdb schema list

# Inspect your schema
sbdb schema show --format json

# Generate JSON Schema (useful for AI agents)
sbdb json-schema
```

`schema show` outputs field classifications (scalar vs complex), virtual field metadata, and all config. This is the self-describing API that agents use to discover your data model.

### Step 6: Create your first document

Option A — from a JSON payload via stdin:

```bash
cat << 'EOF' | sbdb create --input -
{
  "slug": "pad-thai",
  "created": "2026-04-08",
  "cuisine": "Thai",
  "difficulty": "medium",
  "prep_time": 30,
  "tags": ["noodles", "stir-fry", "weeknight"],
  "ingredients": [
    {"name": "rice noodles", "amount": "200", "unit": "g"},
    {"name": "shrimp", "amount": "300", "unit": "g"},
    {"name": "fish sauce", "amount": "3", "unit": "tbsp"}
  ],
  "content": "# Pad Thai\n\nClassic Thai stir-fried noodles.\n\n## Steps\n\n1. Soak noodles in warm water for 20 minutes\n2. Heat oil in a wok over high heat\n3. Cook shrimp until pink, about 2 minutes\n4. Add noodles and sauce, toss for 3 minutes\n5. Serve with lime wedges and crushed peanuts"
}
EOF
```

Option B — from CLI flags:

```bash
sbdb create \
  --field slug=pasta-aglio \
  --field created=2026-04-08 \
  --field cuisine=Italian \
  --field difficulty=easy \
  --field prep_time=15 \
  --field 'tags=["pasta","garlic","quick"]' \
  --content-file pasta-aglio.md
```

After creating, `sbdb` writes two files:

```
docs/recipes/pad-thai.md     ← markdown with YAML frontmatter + body
data/recipes/records.yaml    ← flat scalar projection (fast queries)
data/recipes/.integrity.yaml ← SHA-256 hashes for tamper detection
```

### Step 7: Query your data

Queries hit `records.yaml` only — no file I/O per record, fast even for thousands of documents.

```bash
# All recipes
sbdb list

# Filter by cuisine
sbdb query --filter cuisine=Thai

# Filter with lookups
sbdb query --filter difficulty=easy --filter prep_time__lte=20

# Order by prep time, take top 5
sbdb query --order prep_time --limit 5

# Count recipes by difficulty
sbdb query --filter difficulty=hard --count

# Check if a recipe exists
sbdb query --filter slug=pad-thai --exists

# Full-text search across markdown bodies
sbdb search "fish sauce"

# Get a single record with full content
sbdb get --id pad-thai
```

Available lookup suffixes: `__gte`, `__lte`, `__gt`, `__lt`, `__in`, `__contains`, `__icontains`, `__startswith`.

### Step 8: Update and delete

```bash
# Update a field
sbdb update --id pad-thai --field difficulty=hard

# Append to a list
sbdb update --id pad-thai --field 'tags+=spicy'

# Remove from a list
sbdb update --id pad-thai --field 'tags-=weeknight'

# Replace the markdown body
sbdb update --id pad-thai --content-file updated-pad-thai.md

# Soft delete (sets status field to "archived" if schema has one)
sbdb delete --id pad-thai --soft --yes

# Hard delete (removes .md + record + manifest entry)
sbdb delete --id pad-thai --yes
```

### Step 9: Set up integrity verification

The integrity system ensures every file was last written by `sbdb`. Any hand-edit (or AI bypass) is detected.

```bash
# Generate an HMAC signing key (one-time setup)
sbdb doctor init-key

# Check for issues
sbdb doctor check

# Simulate a tamper: edit a file by hand
echo "TAMPERED" >> docs/recipes/pad-thai.md

# Doctor catches it
sbdb doctor check
# → exit code 6, reports "content changed" for pad-thai

# If the edit was intentional, re-sign it
sbdb doctor sign --force --id pad-thai

# If it was accidental, revert it
git checkout docs/recipes/pad-thai.md
```

Exit codes from `doctor check`:
- `0` — clean
- `4` — drift (frontmatter vs record mismatch)
- `6` — tamper (file hash doesn't match manifest)
- `7` — both drift and tamper

### Step 10: Use multiple schemas in one project

A single project can have many schemas. Just add more YAML files:

```
schemas/
├── recipes.yaml
├── ingredients.yaml
└── meal-plans.yaml
```

Switch between them with `-s`:

```bash
sbdb list -s recipes
sbdb list -s ingredients
sbdb list -s meal-plans
```

Each schema has its own `docs_dir`, `records_dir`, and integrity manifest — they don't interfere with each other.

### Schema design reference

**Field type routing (automatic):**

| Field type | Stored in | Queryable without file I/O |
|---|---|---|
| `string`, `int`, `float`, `bool`, `date`, `enum` | frontmatter + records.yaml | Yes |
| `list`, `object` | frontmatter only | No (requires `--load-content`) |
| `virtual` (scalar return) | both (materialized on save) | Yes |
| `virtual` (complex return) | frontmatter only | No |

**Partitioning:** use `partition: monthly` for time-series data. Records split into `data/<entity>/2026-04.yaml`, `2026-05.yaml`, etc. Requires `date_field` to specify which date field drives the partition.

**Virtual field patterns:**

```yaml
# Extract title from first heading
title:
  returns: string
  source: |
    def compute(content, fields):
        for line in content.splitlines():
            if line.startswith("# "):
                return line.removeprefix("# ").strip()
        return fields["slug"]

# Count words
word_count:
  returns: int
  source: |
    def compute(content, fields):
        return len(content.split())

# Extract ticket references
ticket_refs:
  returns: list[string]
  source: |
    def compute(content, fields):
        return re.findall("[A-Z]+-[0-9]+", content)

# Parse a structured marker from the body
status:
  returns: string
  source: |
    def compute(content, fields):
        for line in content.splitlines():
            if "**Status:**" in line:
                return line.split("**Status:**")[1].strip().lower()
        return "draft"
```

**All schema options:**

```yaml
version: 1                    # schema format version (always 1)
entity: <name>                # entity name, used in directory paths
docs_dir: <path>              # where .md files live (relative to project root)
filename: "{field}.md"        # filename template with {field} placeholders
records_dir: <path>           # where records.yaml lives
partition: none               # "none" or "monthly"
date_field: <field>           # required when partition is "monthly"
id_field: <field>             # which field is the primary key (default: "id")
integrity: strict             # "strict", "warn", or "off"
```

## How it works

- **Scalar fields** (string, int, date, enum...) are stored in both frontmatter and `records.yaml`
- **Complex fields** (list, object) are stored in frontmatter only
- **Virtual fields** are computed from content via sandboxed Starlark, materialized on save
- **Queries** read only `records.yaml` (fast, no file I/O per record)
- **Integrity manifest** tracks SHA-256 of content, frontmatter, and record for every doc
- **Doctor** detects drift (frontmatter vs record) and tamper (hash mismatch)

## Compatibility

sbdb layers on top of existing markdown tools — it doesn't replace them. Your wiki/docs site keeps working exactly as before; sbdb adds typed schemas, integrity verification, a knowledge graph, and semantic search.

### How sbdb works alongside your tools

| Tool | Compatibility | How it integrates |
|------|---------------|-------------------|
| **Obsidian** | Full | Same YAML frontmatter format. sbdb reads/writes frontmatter that Obsidian understands. `[[wikilinks]]` can be extracted as graph edges via virtual fields. Obsidian vault = sbdb knowledge base. |
| **VitePress** | Full | sbdb manages the `docs/` directory that VitePress serves. VitePress data loaders can read `records.yaml` for dynamic tables. Crawl mode indexes all pages including index files with Vue components. |
| **Docusaurus** | Full | Same frontmatter convention. `.mdx` files indexed via crawl mode. Sidebars are independent of sbdb. |
| **Jekyll** | Full | Jekyll's YAML frontmatter IS sbdb's frontmatter — identical format. `_posts/` maps directly to a schema with monthly partitions. |
| **MkDocs** | Full | Standard markdown + optional YAML frontmatter. sbdb adds structure without changing what MkDocs reads. |
| **Hugo** | Partial | Hugo supports YAML frontmatter (default is TOML). Use `---` delimiters for YAML mode and sbdb works seamlessly. |
| **Notion** (exported) | Full | Export as markdown, then `sbdb index build --crawl` to index everything. No schema needed for crawl mode. |
| **Plain markdown** | Full | Any directory of `.md` files works with `sbdb index build --crawl`. No frontmatter required. |

### Integration patterns

**Pattern 1: Schema-managed entities** (structured data)

Best for: ADRs, meeting notes, incident reports — anything with a repeatable structure.

```bash
sbdb init --template notes    # creates schemas/ + .sbdb.toml
sbdb create --input -         # create via CLI, writes .md + records.yaml
sbdb query --filter ...       # fast structured queries
```

sbdb writes both the `.md` file and `records.yaml`. Your wiki tool renders the `.md` files as pages.

**Pattern 2: Crawl mode** (unstructured content)

Best for: existing wikis, guides, architecture docs — content that doesn't follow a schema.

```bash
sbdb index build --crawl      # walks docs/, indexes everything
sbdb search "topic" --semantic # semantic search across all pages
sbdb graph export --export-format json  # knowledge graph from markdown links
```

No schema needed. sbdb extracts titles from `# headings`, entities from directory names, and edges from `[markdown](links.md)`.

**Pattern 3: Hybrid** (both together)

Use schemas for structured entities (notes, ADRs) and crawl mode for everything else. Both coexist in the same SQLite knowledge graph.

```bash
# Schema-backed
sbdb create -s notes --input -
sbdb query -s notes --filter status=active

# Crawl the rest
sbdb index build --crawl --docs-dir docs/

# Search across everything
sbdb search "deployment" --semantic
```

### What sbdb adds to your existing workflow

```
Your existing tool         sbdb adds
─────────────────         ─────────────
.md files              →  typed schemas + validation
YAML frontmatter       →  scalar/complex field routing + records.yaml index
manual editing         →  integrity signing (SHA-256 + HMAC tamper detection)
file browsing          →  QuerySet with filters, ordering, pagination
Ctrl+F                 →  semantic search (embeddings + cosine similarity)
mental model           →  knowledge graph (auto-extracted from links + refs)
```

## AI agent integration

Every command outputs structured JSON when piped or with `--format json`. Exit codes are stable (0=ok, 2=not found, 3=validation, 4=drift, 6=tamper). Designed as a CLI API for Claude Code and other AI agents.

A Claude Code plugin is available via the [bershadsky-claude-tools marketplace](https://github.com/sergio-bershadsky/ai). Install with: `/plugin marketplace add sergio-bershadsky/ai` then `/plugin install secondbrain-db`.

## License

MIT
