package events

// BuiltinTypes is the canonical list of every event type sbdb itself emits.
// Author types live under x.* and are added to the registry dynamically.
//
// Order is preserved for stable output in `sbdb event types`.
var BuiltinTypes = []string{
	// Document lifecycle
	"note.created",
	"note.updated",
	"note.deleted",
	"task.created",
	"task.updated",
	"task.deleted",
	"task.status_changed",
	"task.completed",
	"adr.created",
	"adr.proposed",
	"adr.accepted",
	"adr.superseded",
	"adr.rejected",
	"discussion.created",
	"discussion.updated",
	"discussion.action_added",
	"discussion.action_resolved",

	// Knowledge graph
	"graph.node_added",
	"graph.node_removed",
	"graph.edge_added",
	"graph.edge_removed",
	"graph.reindexed",

	// Index / embeddings
	"kb.indexed",
	"kb.chunk_added",
	"kb.chunk_removed",
	"kb.embedding_updated",
	"kb.model_changed",

	// Records
	"records.upserted",
	"records.removed",
	"records.partition_rotated",

	// Integrity
	"integrity.signed",
	"integrity.recomputed",
	"integrity.drift_detected",
	"integrity.tamper_detected",

	// Review / freshness
	"review.stamped",
	"freshness.stale_flagged",

	// Meta
	"meta.archived",
	"meta.event_type_registered",
	"meta.event_type_evolved",
	"meta.event_type_deprecated",
	"meta.config_changed",

	// Search (opt-in)
	"search.queried",
}

// ReservedBuckets lists every bucket name owned by built-ins. Author schemas
// MUST NOT claim these. The `sbdb` prefix is also reserved by §3.3 but is
// matched as a prefix, not a literal bucket.
var ReservedBuckets = []string{
	"note", "task", "adr", "discussion",
	"graph", "kb", "records",
	"integrity", "review", "freshness",
	"meta", "search",
}

// IsReservedBucket reports whether the given bucket may be claimed by an
// author schema. Returns true if the bucket is built-in or starts with the
// reserved `sbdb.` prefix.
func IsReservedBucket(bucket string) bool {
	for _, r := range ReservedBuckets {
		if bucket == r {
			return true
		}
	}
	if len(bucket) >= 5 && bucket[:5] == "sbdb." {
		return true
	}
	if bucket == "sbdb" {
		return true
	}
	return false
}

// IsBuiltinType reports whether the given type is in the built-in catalog.
func IsBuiltinType(typeName string) bool {
	for _, t := range BuiltinTypes {
		if t == typeName {
			return true
		}
	}
	return false
}
