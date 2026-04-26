**secondbrain-db v1.1 — Audit Trail Release**

Every change to your knowledge base now leaves a permanent, cryptographically verifiable trail. The repo itself becomes the change feed.

**What's new**

• **Append-only event log** — every CRUD, every doctor run, every schema evolution writes a JSONL line to `.sbdb/events/<date>.jsonl`. Lock-free POSIX `O_APPEND`. Multi-process safe. 4 KiB cap per line.
• **40+ built-in event types** across notes, tasks, ADRs, discussions, knowledge graph, KB index, records, integrity, and meta.
• **Author extensions via `x.*` namespace** — declare your own entity events in schema YAML. No code changes.
• **Doctor-enforced 2-month window** — current + previous month live; older months auto-archive to gzipped JSONL on `sbdb doctor fix`.
• **Configurable archive targets** — keep archives in git, stream to S3, or both. Pointer YAMLs always remain in the repo for offline audit.
• **Plugin v0.2 ships a second guard hook** — `guard-events.py` blocks any direct AI edit to `.sbdb/events/**`. Audit-trail integrity by construction.

**Why it matters**

Workers tail your repo and stream events to SNS / SQS / Kafka / webhooks — no markdown re-parsing. Renames are derivable from `(type, id, sha)` matching. Every mutation is `(year, month, seq)`-addressable. The registry projection is regenerable byte-for-byte from the log.

Concurrency proven by 16 subprocesses × 5,000 events with zero corruption.

**Install / upgrade**

```
go install github.com/sergio-bershadsky/secondbrain-db@latest
```

Plugin: `/plugin update secondbrain-db` (now v0.2.0)

GitHub: https://github.com/sergio-bershadsky/secondbrain-db
