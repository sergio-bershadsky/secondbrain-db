# Developer Guide

This guide walks you through setting up and using `secondbrain-db` (sbdb) to manage a markdown knowledge base with typed schemas, computed fields, integrity signing, and a knowledge graph.

## What sbdb does

sbdb turns a directory of markdown files into a queryable, integrity-verified database:

- **Schemas** define your data model (YAML files)
- **Records** are fast-queryable projections of your docs (YAML)
- **Frontmatter** stores all fields alongside the markdown body
- **Virtual fields** are computed from content via sandboxed Starlark
- **Integrity manifest** detects unauthorized edits (SHA-256 + HMAC)
- **Knowledge graph** maps relationships between documents (bbolt)
- **Semantic search** finds documents by meaning (embeddings + cosine similarity)

Everything is plain files on disk — no server, no database daemon, single static binary.

## Installation

```bash
# From source (requires Go 1.22+)
go install github.com/sergio-bershadsky/secondbrain-db@latest

# Verify
sbdb version
```

## Project setup

### Initialize a new project

```bash
mkdir my-kb && cd my-kb
sbdb init --template notes
```

This creates:

```
my-kb/
├── .sbdb.toml          # config: default schema, output format, integrity settings
├── schemas/
│   └── notes.yaml      # your first schema
├── docs/               # markdown files will live here
└── data/               # records + integrity manifests
```

### Project config (.sbdb.toml)

```toml
schema_dir = "./schemas"
base_path = "."
default_schema = "notes"

[output]
format = "auto"          # "auto" = table on TTY, json when piped

[integrity]
key_source = "env"       # where to find HMAC key: env, file, keyring

[knowledge_graph]
enabled = true
db_path = "data/.sbdb.db"

[knowledge_graph.embeddings]
provider = "openai"
model = "text-embedding-3-small"
dimension = 1536

[knowledge_graph.graph]
auto_index = true        # update graph on every save
extract_links = true     # auto-extract markdown links as edges
```

## Defining schemas

A schema is a YAML file in `schemas/` that describes an entity type.

### Example: meeting notes

```yaml
# schemas/meetings.yaml
version: 1
entity: meetings
docs_dir: docs/meetings
filename: "{date}-{slug}.md"
records_dir: data/meetings
partition: monthly
date_field: date
id_field: slug
integrity: strict

fields:
  slug:       { type: string, required: true }
  date:       { type: date, required: true }
  facilitator: { type: string }
  status:     { type: enum, values: [scheduled, completed, cancelled], default: scheduled }
  attendees:  { type: list, items: { type: string } }
  follows_up: { type: ref, entity: meetings }     # ref to another meeting
  action_items:
    type: list
    items:
      type: object
      fields:
        owner: { type: string, required: true }
        task:  { type: string, required: true }

virtuals:
  title:
    returns: string
    source: |
      def compute(content, fields):
          for line in content.splitlines():
              if line.startswith("# "):
                  return line.removeprefix("# ").strip()
          return fields["slug"]
  action_count:
    returns: int
    source: |
      def compute(content, fields):
          count = 0
          for line in content.splitlines():
              if line.startswith("- [ ]") or line.startswith("- [x]"):
                  count += 1
          return count
```

### Field types

| Type | Stored in | Fast-queryable |
|------|-----------|----------------|
| `string`, `int`, `float`, `bool`, `date`, `enum` | frontmatter + records.yaml | Yes |
| `ref` | frontmatter + records.yaml | Yes (also creates graph edges) |
| `list`, `object` | frontmatter only | No |
| `virtual` (scalar return) | both | Yes |
| `virtual` (complex return) | frontmatter only | No |

### Virtual fields

Starlark functions that compute values from the markdown body. They're sandboxed (no I/O, no imports, deterministic) and materialized into frontmatter/records on every save.

Available builtins:
- All Python string methods (`splitlines()`, `startswith()`, `strip()`, etc.)
- `re.findall(pattern, text)` for regex
- `len()`, `max()`, `min()`, `int()`, `str()`, `list()`

## CRUD operations

### Create

```bash
# From JSON
echo '{"slug":"standup","date":"2026-04-08","status":"scheduled","content":"# Daily Standup\n\nAgenda here."}' \
  | sbdb create -s meetings --input -

# From flags
sbdb create -s meetings \
  --field slug=retro \
  --field date=2026-04-10 \
  --field status=scheduled \
  --content-file retro.md
```

### Read

```bash
# Get one document
sbdb get --id standup

# List all (fast — reads records.yaml only)
sbdb list --order -date --limit 10

# Query with filters
sbdb query --filter status=scheduled --filter date__gte=2026-04-01

# Full-text search
sbdb search "deployment strategy"
```

### Update

```bash
# Set a field
sbdb update --id standup --field status=completed

# Append to a list
sbdb update --id standup --field 'attendees+=["Alice","Bob"]'

# Replace the body
sbdb update --id standup --content-file updated-standup.md
```

### Delete

```bash
# Hard delete (removes file + record + manifest entry)
sbdb delete --id standup --yes

# Soft delete (sets status=archived)
sbdb delete --id standup --soft --yes
```

## Integrity system

sbdb signs every document with SHA-256 hashes so you can detect unauthorized edits.

### Setup

```bash
# Generate an HMAC signing key (one-time)
sbdb doctor init-key
# → writes key to ~/.config/secondbrain-db/integrity.key
```

### How it works

On every `save()`, sbdb:
1. Computes SHA-256 of the content, frontmatter, and record
2. Signs with HMAC (if key exists)
3. Stores in `data/<entity>/.integrity.yaml`

On every `get`/`query`, sbdb verifies the hashes match.

### Detecting issues

```bash
sbdb doctor check
```

Exit codes:
- `0` — clean
- `4` — **drift**: frontmatter and records.yaml are out of sync
- `6` — **tamper**: a file was modified outside sbdb
- `7` — both

### Fixing issues

```bash
# Fix drift (safe — re-syncs frontmatter ↔ records)
sbdb doctor fix --recompute

# After intentional hand-edits — re-sign the files
sbdb doctor sign --force

# After accidental edits — revert from git
git checkout docs/meetings/standup.md
```

**Rule**: `doctor fix` never re-signs tampered files. A clean `doctor check` means every file was last written by the ORM.

## Knowledge graph

sbdb builds a knowledge graph from three sources:

1. **Markdown links**: `[see also](../notes/deploy.md)` → directed edge
2. **ref fields**: `follows_up: { type: ref, entity: meetings }` → typed edge
3. **Virtual edges**: virtuals with `edge: true` → computed edges

### Building the graph

```bash
# Schema mode (indexes docs from records.yaml)
sbdb index build

# Crawl mode (walks ALL .md files, even unstructured ones)
sbdb index build --crawl

# Force re-index everything
sbdb index build --force
```

### Querying the graph

```bash
# What links TO a document?
sbdb graph incoming --id standup

# What does a document link TO?
sbdb graph outgoing --id standup

# Documents within 2 hops
sbdb graph neighbors --id standup --depth 2

# Export for visualization
sbdb graph export --export-format json    # for D3.js, Cytoscape, React Flow
sbdb graph export --export-format mermaid # for markdown viewers
sbdb graph export --export-format dot     # for Graphviz
```

## Semantic search

Search by meaning instead of keywords. Requires an embedding API.

### Setup

```bash
# Set your API key (OpenAI, Voyage, or any compatible endpoint)
export SBDB_EMBED_API_KEY="sk-..."

# Build the semantic index
sbdb index build
```

### Searching

```bash
# Find documents by meaning
sbdb search "how to handle database migrations" --semantic --k 5

# Combine with graph expansion
sbdb search "deployment" --semantic --expand --depth 1
```

### Custom embedding providers

Configure in `.sbdb.toml`:

```toml
[knowledge_graph.embeddings]
provider = "openai"
base_url = "https://api.openai.com"     # or Ollama: http://localhost:11434
model = "text-embedding-3-small"         # or nomic-embed-text for Ollama
dimension = 1536
```

Works with any OpenAI-compatible `/v1/embeddings` endpoint: OpenAI, Voyage AI, Mistral, Ollama, LiteLLM, vLLM.

## AI agent integration

sbdb is designed as a CLI API for AI agents (Claude Code, GPT, etc.).

### JSON output

Every command outputs structured JSON when piped or with `--format json`:

```bash
sbdb query --filter status=active --format json
```

```json
{
  "version": 1,
  "data": [
    {"slug": "standup", "date": "2026-04-08", "status": "active", "title": "Daily Standup"}
  ]
}
```

### Schema introspection

Agents can discover the data model without reading source code:

```bash
# List available schemas
sbdb schema list --format json

# Get schema details (fields, types, virtuals)
sbdb schema show --format json

# Get JSON Schema for payload validation
sbdb schema json-schema
```

### Claude Code plugin

Install the plugin from the marketplace or directly:

```bash
# The plugin provides:
# - Auto-review hook: checks integrity after every file edit
# - /sbdb-review: manual health check command
# - /sbdb-query, /sbdb-search, /sbdb-doctor, /sbdb-init: slash commands
# - kb-curator agent: comprehensive KB audit
```

The hook automatically detects when Claude edits a KB file and tells Claude how to fix any resulting drift.

## Multiple schemas

A project can have any number of schemas:

```
schemas/
├── notes.yaml
├── meetings.yaml
├── adrs.yaml
└── incidents.yaml
```

Switch between them with `-s`:

```bash
sbdb list -s notes
sbdb list -s meetings
sbdb query -s adrs --filter status=accepted
```

Each schema has its own `docs_dir`, `records_dir`, and integrity manifest — they don't interfere.

## Monthly partitions

For time-series data, use `partition: monthly`:

```yaml
partition: monthly
date_field: date
```

Records split into `data/<entity>/2026-04.yaml`, `2026-05.yaml`, etc. Queries transparently merge all partitions.

## File layout

After working with sbdb for a while, your project looks like:

```
my-kb/
├── .sbdb.toml                      # config
├── schemas/
│   ├── notes.yaml                  # schema definitions
│   └── meetings.yaml
├── docs/
│   ├── notes/
│   │   ├── deploy-guide.md         # ← markdown + frontmatter
│   │   └── arch-overview.md
│   └── meetings/
│       ├── 2026-04-08-standup.md
│       └── 2026-04-10-retro.md
├── data/
│   ├── notes/
│   │   ├── records.yaml            # ← scalar projections (fast queries)
│   │   └── .integrity.yaml         # ← SHA-256 hashes
│   ├── meetings/
│   │   ├── 2026-04.yaml            # ← monthly partition
│   │   └── .integrity.yaml
│   ├── .sbdb.db                    # ← SQLite: semantic search vectors
│   └── .sbdb-graph.db              # ← bbolt: knowledge graph
└── .gitignore
```
