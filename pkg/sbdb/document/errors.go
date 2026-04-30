package document

import (
	"fmt"
	"strings"
)

// NotFoundError indicates a document was not found.
type NotFoundError struct {
	ID     string
	Entity string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s with id %q not found", e.Entity, e.ID)
}

// MultipleFoundError indicates multiple documents matched when one was expected.
type MultipleFoundError struct {
	Entity string
	Count  int
}

func (e *MultipleFoundError) Error() string {
	return fmt.Sprintf("expected 1 %s, found %d", e.Entity, e.Count)
}

// IntegrityError indicates a document failed integrity verification.
type IntegrityError struct {
	ID         string
	File       string
	Mismatched []string // "content", "frontmatter", "record"
}

func (e *IntegrityError) Error() string {
	return fmt.Sprintf("integrity check failed for %q (%s): %s changed",
		e.ID, e.File, strings.Join(e.Mismatched, ", "))
}

// DriftError indicates frontmatter and record are out of sync.
type DriftError struct {
	ID    string
	Field string
	FM    any // frontmatter value
	Rec   any // record value
}

func (e *DriftError) Error() string {
	return fmt.Sprintf("drift in %q: field %q — frontmatter=%v, record=%v",
		e.ID, e.Field, e.FM, e.Rec)
}
