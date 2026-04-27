package events

// BuiltinTypes is the canonical, closed catalog of every event type sbdb emits.
//
// The catalog is intentionally closed — sbdb does not support author-defined
// event types. Events are pure pointers (`{ts, type, id, sha}`) into git's
// content-addressed store; there is no per-type `data` payload that would
// require schema declaration. New verbs ship by editing this slice and adding
// the corresponding emit site, not by author registration.
//
// Order is preserved for stable output in `sbdb event types`.
var BuiltinTypes = []string{
	// Document lifecycle. The canonical CRUD triplet, emitted from
	// cmd/{create,update,delete}.go via emitDocEvent. Workers diff
	// content at sha against prev to derive any per-bucket semantics
	// they care about (status changes, action items, ADR transitions, etc.).
	"note.created",
	"note.updated",
	"note.deleted",
	"task.created",
	"task.updated",
	"task.deleted",
	"adr.created",
	"adr.updated",
	"adr.deleted",
	"discussion.created",
	"discussion.updated",
	"discussion.deleted",

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
	"meta.config_changed",

	// Search (opt-in)
	"search.queried",
}

// ReservedBuckets lists every bucket name owned by built-ins.
// The `sbdb` prefix is also reserved by §3.3 but is matched as a prefix,
// not a literal bucket.
var ReservedBuckets = []string{
	"note", "task", "adr", "discussion",
	"graph", "kb", "records",
	"integrity", "review", "freshness",
	"meta", "search",
}

// IsReservedBucket reports whether the given bucket is built-in. Kept for
// validation of incoming type names from `sbdb event append`.
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
// Since the catalog is closed (no author-defined types), this is equivalent
// to "is the type valid".
func IsBuiltinType(typeName string) bool {
	for _, t := range BuiltinTypes {
		if t == typeName {
			return true
		}
	}
	return false
}
