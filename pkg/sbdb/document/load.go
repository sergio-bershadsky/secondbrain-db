package document

import (
	"fmt"
	"os"

	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/integrity"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/schema"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/storage"
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

// VerifyIntegrity checks this document against its per-doc sidecar.
// Returns nil if the document passes, or an IntegrityError if tampered.
func (d *Document) VerifyIntegrity() error {
	if d.Schema.Integrity == "off" {
		return nil
	}

	mdPath := d.FilePath()
	sc, err := integrity.LoadSidecar(mdPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no sidecar = not yet tracked
		}
		return fmt.Errorf("loading sidecar for verification: %w", err)
	}

	// Ensure we have content loaded for hashing
	if err := d.EnsureLoaded(); err != nil {
		return err
	}

	// Compute current hashes
	fmData := schema.BuildFrontmatterData(d.Schema, d.Data, d.virtuals)
	recordData := schema.BuildRecordData(d.Schema, d.Data, d.virtuals)
	recordData["file"] = d.RelativeFilePath()

	key, err := integrity.LoadKey()
	if err != nil {
		return fmt.Errorf("loading integrity key for HMAC verification: %w", err)
	}

	drift, _ := sc.Verify(mdPath, fmData, d.Content, recordData, key)
	if !drift.Any() {
		return nil
	}

	id := d.ID()

	if d.Schema.Integrity == "warn" {
		var mismatched []string
		if drift.ContentDrift {
			mismatched = append(mismatched, "content")
		}
		if drift.FrontmatterDrift {
			mismatched = append(mismatched, "frontmatter")
		}
		if drift.RecordDrift {
			mismatched = append(mismatched, "record")
		}
		if drift.BadSig {
			mismatched = append(mismatched, "hmac")
		}
		Logger.Warn("integrity mismatch", "id", id, "file", d.FilePath(), "changed", mismatched)
		return nil
	}

	var mismatched []string
	if drift.ContentDrift {
		mismatched = append(mismatched, "content")
	}
	if drift.FrontmatterDrift {
		mismatched = append(mismatched, "frontmatter")
	}
	if drift.RecordDrift {
		mismatched = append(mismatched, "record")
	}
	if drift.BadSig {
		mismatched = append(mismatched, "hmac")
	}

	return &IntegrityError{
		ID:         id,
		File:       d.FilePath(),
		Mismatched: mismatched,
	}
}

// removeFile is a testable wrapper for os.Remove.
var removeFile = os.Remove

// isNotExist is a testable wrapper for os.IsNotExist.
var isNotExist = os.IsNotExist
