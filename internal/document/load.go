package document

import (
	"fmt"
	"os"

	"github.com/sergio-bershadsky/secondbrain-db/internal/integrity"
	"github.com/sergio-bershadsky/secondbrain-db/internal/schema"
	"github.com/sergio-bershadsky/secondbrain-db/internal/storage"
)

// LoadFromFile reads a document from a markdown file.
// Parses frontmatter into Data and body into Content.
// Does NOT touch records.yaml (read-only load).
func LoadFromFile(s *schema.Schema, basePath, path string) (*Document, error) {
	fm, body, err := storage.ParseMarkdown(path)
	if err != nil {
		return nil, fmt.Errorf("loading document from %s: %w", path, err)
	}

	doc := New(s, basePath)
	doc.Data = fm
	doc.Content = body
	doc.loaded = true

	return doc, nil
}

// LoadFromRecord creates a lazy Document from a record map.
// The Content and complex fields are NOT loaded until accessed.
func LoadFromRecord(s *schema.Schema, basePath string, record map[string]any) *Document {
	doc := New(s, basePath)
	// Copy record fields into Data
	for k, v := range record {
		if k != "file" { // skip auto-derived file field
			doc.Data[k] = v
		}
	}
	doc.loaded = false
	return doc
}

// EnsureLoaded reads the markdown file if content hasn't been loaded yet.
func (d *Document) EnsureLoaded() error {
	if d.loaded {
		return nil
	}

	path := d.FilePath()
	fm, body, err := storage.ParseMarkdown(path)
	if err != nil {
		return fmt.Errorf("loading document content: %w", err)
	}

	// Merge frontmatter data (complex fields not in record)
	for k, v := range fm {
		if _, exists := d.Data[k]; !exists {
			d.Data[k] = v
		}
	}
	d.Content = body
	d.loaded = true

	return nil
}

// VerifyIntegrity checks this document against the integrity manifest.
// Returns nil if the document passes, or an IntegrityError if tampered.
func (d *Document) VerifyIntegrity() error {
	if d.Schema.Integrity == "off" {
		return nil
	}

	recordsDir := d.RecordsDir()
	if !integrity.ManifestExists(recordsDir) {
		return nil
	}

	manifest, err := integrity.LoadManifest(recordsDir)
	if err != nil {
		return fmt.Errorf("loading manifest for verification: %w", err)
	}

	id := d.ID()
	entry, ok := manifest.Entries[id]
	if !ok {
		return nil // no entry = not yet tracked
	}

	// Ensure we have content loaded for hashing
	if err := d.EnsureLoaded(); err != nil {
		return err
	}

	// Compute current hashes
	fmData := schema.BuildFrontmatterData(d.Schema, d.Data, d.virtuals)
	recordData := schema.BuildRecordData(d.Schema, d.Data, d.virtuals)
	recordData["file"] = d.RelativeFilePath()

	contentSHA := integrity.HashContent(d.Content)
	fmSHA := integrity.HashFrontmatter(fmData)
	recSHA := integrity.HashRecord(recordData)

	check := integrity.Verify(entry, contentSHA, fmSHA, recSHA)
	if check == nil {
		// Also verify HMAC if manifest says HMAC is enabled
		if manifest.HMAC && entry.Sig != "" {
			key, err := integrity.LoadKey()
			if err != nil {
				return fmt.Errorf("loading integrity key for HMAC verification: %w", err)
			}
			if key != nil && !integrity.VerifySignature(entry, key) {
				if d.Schema.Integrity == "strict" {
					return &IntegrityError{
						ID:         id,
						File:       d.FilePath(),
						Mismatched: []string{"hmac"},
					}
				}
			}
		}
		return nil
	}

	if d.Schema.Integrity == "warn" {
		fmt.Fprintf(os.Stderr, "warning: integrity mismatch for %q (%s): %v changed\n",
			id, d.FilePath(), check.Mismatched)
		return nil
	}

	return &IntegrityError{
		ID:         id,
		File:       d.FilePath(),
		Mismatched: check.Mismatched,
	}
}

// removeFile is a testable wrapper for os.Remove.
var removeFile = os.Remove

// isNotExist is a testable wrapper for os.IsNotExist.
var isNotExist = os.IsNotExist
