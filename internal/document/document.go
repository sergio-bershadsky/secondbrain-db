package document

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/sergio-bershadsky/secondbrain-db/internal/schema"
)

// PostSaveHook is called after a document is successfully saved to disk.
// Used by the KG layer to index documents without creating an import cycle.
type PostSaveHook func(doc *Document) error

// Document represents a single knowledge base document with its data, content, and schema.
type Document struct {
	Schema   *schema.Schema
	Data     map[string]any // all field values (scalar + complex)
	Content  string         // markdown body
	BasePath string         // root of the knowledge base on disk

	// Hooks
	OnSave   PostSaveHook          // called after successful save (optional)
	OnDelete func(id string) error // called after successful delete (optional)

	// Computed
	virtuals map[string]any // cached virtual field values
	loaded   bool           // whether content was loaded from disk
}

// New creates a new Document with the given schema and base path.
func New(s *schema.Schema, basePath string) *Document {
	return &Document{
		Schema:   s,
		Data:     make(map[string]any),
		BasePath: basePath,
		virtuals: make(map[string]any),
	}
}

// ID returns the document's id field value.
func (d *Document) ID() string {
	v, ok := d.Data[d.Schema.IDField]
	if !ok {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

// FilePath returns the full path to this document's markdown file.
func (d *Document) FilePath() string {
	filename := d.resolveFilename()
	return filepath.Join(d.BasePath, d.Schema.DocsDir, filename)
}

// RelativeFilePath returns the docs-relative path for the records `file` field.
func (d *Document) RelativeFilePath() string {
	filename := d.resolveFilename()
	return filepath.Join(d.Schema.DocsDir, filename)
}

// Get returns a field value, checking data then virtuals.
func (d *Document) Get(field string) (any, bool) {
	if v, ok := d.Data[field]; ok {
		return v, true
	}
	if v, ok := d.virtuals[field]; ok {
		return v, true
	}
	return nil, false
}

// Set sets a data field value.
func (d *Document) Set(field string, value any) {
	d.Data[field] = value
}

// SetVirtuals stores computed virtual field values.
func (d *Document) SetVirtuals(v map[string]any) {
	d.virtuals = v
}

// Virtuals returns the cached virtual field values.
func (d *Document) Virtuals() map[string]any {
	return d.virtuals
}

// AllData returns a merged view of data + virtuals.
func (d *Document) AllData() map[string]any {
	merged := make(map[string]any, len(d.Data)+len(d.virtuals))
	for k, v := range d.Data {
		merged[k] = v
	}
	for k, v := range d.virtuals {
		merged[k] = v
	}
	return merged
}

func (d *Document) resolveFilename() string {
	filename := d.Schema.Filename
	for key, val := range d.Data {
		placeholder := "{" + key + "}"
		filename = strings.ReplaceAll(filename, placeholder, fmt.Sprintf("%v", val))
	}
	clean := filepath.Clean(filename)
	if strings.Contains(clean, "..") {
		return "untitled.md"
	}
	return clean
}
