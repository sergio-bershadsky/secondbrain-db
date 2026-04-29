package query

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sergio-bershadsky/secondbrain-db/internal/document"
	"github.com/sergio-bershadsky/secondbrain-db/internal/schema"
	"github.com/sergio-bershadsky/secondbrain-db/internal/storage"
)

// QuerySet provides a chainable query interface over records.
type QuerySet struct {
	schema     *schema.Schema
	basePath   string
	filters    []Lookup
	excludes   []Lookup
	ordering   []string
	maxResults int
	skipCount  int
}

// NewQuerySet creates a new QuerySet for the given schema and base path.
func NewQuerySet(s *schema.Schema, basePath string) *QuerySet {
	return &QuerySet{
		schema:   s,
		basePath: basePath,
	}
}

// Filter adds filter conditions. Returns a new QuerySet (immutable chaining).
func (qs *QuerySet) Filter(conditions map[string]any) *QuerySet {
	next := qs.clone()
	for key, val := range conditions {
		next.filters = append(next.filters, ParseLookup(key, val))
	}
	return next
}

// Exclude adds exclusion conditions. Returns a new QuerySet.
func (qs *QuerySet) Exclude(conditions map[string]any) *QuerySet {
	next := qs.clone()
	for key, val := range conditions {
		next.excludes = append(next.excludes, ParseLookup(key, val))
	}
	return next
}

// OrderBy sets ordering. Prefix with "-" for descending. Returns a new QuerySet.
func (qs *QuerySet) OrderBy(fields ...string) *QuerySet {
	next := qs.clone()
	next.ordering = fields
	return next
}

// Limit sets the max number of results. Returns a new QuerySet.
func (qs *QuerySet) Limit(n int) *QuerySet {
	next := qs.clone()
	next.maxResults = n
	return next
}

// Offset sets the number of results to skip. Returns a new QuerySet.
func (qs *QuerySet) Offset(n int) *QuerySet {
	next := qs.clone()
	next.skipCount = n
	return next
}

// All executes the query and returns all matching documents (lazy-loaded from records).
func (qs *QuerySet) All() ([]*document.Document, error) {
	records, err := qs.loadRecords()
	if err != nil {
		return nil, err
	}

	filtered := qs.applyFilters(records)
	ordered := qs.applyOrdering(filtered)
	paged := qs.applyPaging(ordered)

	var docs []*document.Document
	for _, rec := range paged {
		doc := document.LoadFromRecord(qs.schema, qs.basePath, rec)
		docs = append(docs, doc)
	}

	return docs, nil
}

// Get returns exactly one document matching the conditions.
// Returns NotFoundError or MultipleFoundError as appropriate.
func (qs *QuerySet) Get(conditions map[string]any) (*document.Document, error) {
	results, err := qs.Filter(conditions).All()
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		id := ""
		if v, ok := conditions[qs.schema.IDField]; ok {
			id = fmt.Sprintf("%v", v)
		}
		return nil, &document.NotFoundError{ID: id, Entity: qs.schema.Entity}
	}
	if len(results) > 1 {
		return nil, &document.MultipleFoundError{Entity: qs.schema.Entity, Count: len(results)}
	}

	return results[0], nil
}

// First returns the first matching document, or nil if none match.
func (qs *QuerySet) First() (*document.Document, error) {
	results, err := qs.Limit(1).All()
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return results[0], nil
}

// Count returns the number of matching records.
func (qs *QuerySet) Count() (int, error) {
	records, err := qs.loadRecords()
	if err != nil {
		return 0, err
	}
	return len(qs.applyFilters(records)), nil
}

// Exists returns true if any records match.
func (qs *QuerySet) Exists() (bool, error) {
	count, err := qs.Count()
	return count > 0, err
}

// Records returns the raw record maps after filtering (no Document wrapping).
func (qs *QuerySet) Records() ([]map[string]any, error) {
	records, err := qs.loadRecords()
	if err != nil {
		return nil, err
	}
	filtered := qs.applyFilters(records)
	ordered := qs.applyOrdering(filtered)
	return qs.applyPaging(ordered), nil
}

func (qs *QuerySet) loadRecords() ([]map[string]any, error) {
	return qs.loadRecordsViaWalker()
}

func (qs *QuerySet) loadRecordsViaWalker() ([]map[string]any, error) {
	docsDir := filepath.Join(qs.basePath, qs.schema.DocsDir)
	docs, err := storage.WalkDocsToSlice(docsDir)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(docs))
	for _, d := range docs {
		rec := schema.BuildRecordData(qs.schema, d.Frontmatter, nil)
		if rel, e := filepath.Rel(qs.basePath, d.Path); e == nil {
			rec["file"] = rel
		}
		out = append(out, rec)
	}
	return out, nil
}

func (qs *QuerySet) applyFilters(records []map[string]any) []map[string]any {
	var result []map[string]any
	for _, rec := range records {
		if qs.matchesAll(rec) {
			result = append(result, rec)
		}
	}
	return result
}

func (qs *QuerySet) matchesAll(rec map[string]any) bool {
	for _, f := range qs.filters {
		if !f.Match(rec) {
			return false
		}
	}
	for _, e := range qs.excludes {
		if e.Match(rec) {
			return false
		}
	}
	return true
}

func (qs *QuerySet) applyOrdering(records []map[string]any) []map[string]any {
	if len(qs.ordering) == 0 {
		return records
	}

	result := make([]map[string]any, len(records))
	copy(result, records)

	sort.SliceStable(result, func(i, j int) bool {
		for _, field := range qs.ordering {
			desc := false
			if strings.HasPrefix(field, "-") {
				desc = true
				field = field[1:]
			}

			cmp := compareValues(result[i][field], result[j][field])
			if cmp == 0 {
				continue
			}
			if desc {
				return cmp > 0
			}
			return cmp < 0
		}
		return false
	})

	return result
}

func (qs *QuerySet) applyPaging(records []map[string]any) []map[string]any {
	start := qs.skipCount
	if start > len(records) {
		return nil
	}
	records = records[start:]

	if qs.maxResults > 0 && qs.maxResults < len(records) {
		records = records[:qs.maxResults]
	}

	return records
}

func (qs *QuerySet) clone() *QuerySet {
	return &QuerySet{
		schema:     qs.schema,
		basePath:   qs.basePath,
		filters:    append([]Lookup{}, qs.filters...),
		excludes:   append([]Lookup{}, qs.excludes...),
		ordering:   append([]string{}, qs.ordering...),
		maxResults: qs.maxResults,
		skipCount:  qs.skipCount,
	}
}
