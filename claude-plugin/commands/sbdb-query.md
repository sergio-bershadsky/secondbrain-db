---
name: sbdb-query
description: Query knowledge base records with filters and ordering
argument-hint: "<schema> [--filter key=value] [--order -field] [--limit N]"
allowed-tools:
  - Bash
---

# sbdb-query

Query records from the knowledge base using filters and ordering.

## Usage

```bash
sbdb query -s ${1:-notes} --filter ${2:-status=active} --format json
```

## Common patterns

Count active records:
```bash
sbdb query -s notes --filter status=active --count --format json
```

Latest 5 records:
```bash
sbdb query -s notes --order -created --limit 5 --format json
```

Check if a record exists:
```bash
sbdb query -s notes --filter id=my-note --exists --format json
```
