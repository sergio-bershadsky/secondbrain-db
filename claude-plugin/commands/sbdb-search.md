---
name: sbdb-search
description: Full-text search over markdown documents
argument-hint: "<phrase> [--semantic] [--k 10]"
allowed-tools:
  - Bash
---

# sbdb-search

Search for content across all markdown documents.

## Usage

Full-text grep search:
```bash
sbdb search "${1}" -s ${2:-notes} --format json
```

Semantic search (requires configured embedding API):
```bash
sbdb search "${1}" -s ${2:-notes} --semantic --k 10 --format json
```
