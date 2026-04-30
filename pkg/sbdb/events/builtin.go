package events

// Verbs are derived from git diff status, so the catalog of verbs is closed:
const (
	VerbCreated = "created"
	VerbUpdated = "updated"
	VerbDeleted = "deleted"
)

// VerbForStatus maps a `git log --raw` status letter to a verb. Returns
// the empty string for statuses we don't project (e.g. `T` type-change,
// `U` unmerged). `R` renames are intentionally not handled — the projection
// runs with `--no-renames`, so a rename appears as a `D` followed by an `A`
// with the same blob sha; consumers can collapse the pair if they care.
func VerbForStatus(status byte) string {
	switch status {
	case 'A', 'C': // added, copied
		return VerbCreated
	case 'M':
		return VerbUpdated
	case 'D':
		return VerbDeleted
	default:
		return ""
	}
}
