package integrity

import (
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/storage"
)

// HashContent returns the canonical SHA-256 of a markdown body.
func HashContent(body string) string {
	return storage.CanonicalBodyHash(body)
}

// HashFrontmatter returns the canonical SHA-256 of a frontmatter map.
func HashFrontmatter(fm map[string]any) string {
	return storage.CanonicalHash(fm)
}

// HashRecord returns the canonical SHA-256 of a record map.
func HashRecord(record map[string]any) string {
	return storage.CanonicalHash(record)
}

// TamperCheck represents a mismatch between expected and actual hashes.
type TamperCheck struct {
	ID         string
	File       string
	Mismatched []string // "content", "frontmatter", "record"
	Expected   map[string]string
	Actual     map[string]string
}

// Verify checks a document's current state against its manifest entry.
// Returns nil if everything matches.
func Verify(entry *Entry, contentSHA, frontmatterSHA, recordSHA string) *TamperCheck {
	var mismatched []string

	if entry.ContentSHA != contentSHA {
		mismatched = append(mismatched, "content")
	}
	if entry.FrontmatterSHA != frontmatterSHA {
		mismatched = append(mismatched, "frontmatter")
	}
	if entry.RecordSHA != recordSHA {
		mismatched = append(mismatched, "record")
	}

	if len(mismatched) == 0 {
		return nil
	}

	return &TamperCheck{
		File:       entry.File,
		Mismatched: mismatched,
		Expected: map[string]string{
			"content":     entry.ContentSHA,
			"frontmatter": entry.FrontmatterSHA,
			"record":      entry.RecordSHA,
		},
		Actual: map[string]string{
			"content":     contentSHA,
			"frontmatter": frontmatterSHA,
			"record":      recordSHA,
		},
	}
}
