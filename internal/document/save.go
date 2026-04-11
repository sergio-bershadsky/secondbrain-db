package document

import (
	"fmt"
	"os"

	"github.com/sergio-bershadsky/secondbrain-db/internal/integrity"
	"github.com/sergio-bershadsky/secondbrain-db/internal/schema"
	"github.com/sergio-bershadsky/secondbrain-db/internal/storage"
	"github.com/sergio-bershadsky/secondbrain-db/internal/virtuals"
)

// Save writes the document to disk: markdown file + records.yaml + integrity manifest.
// This is the core save lifecycle:
// 1. Evaluate virtual fields
// 2. Build frontmatter (all fields + all virtuals)
// 3. Build record (scalar fields + scalar virtuals + file path)
// 4. Write .md atomically
// 5. Upsert record in YAML file
// 6. Update integrity manifest
func (d *Document) Save(rt *virtuals.Runtime) error {
	// 1. Evaluate virtuals
	if rt != nil && len(d.Schema.Virtuals) > 0 {
		vResults, err := rt.EvaluateAll(d.Content, d.Data)
		if err != nil {
			return fmt.Errorf("evaluating virtuals: %w", err)
		}
		d.SetVirtuals(vResults)
	}

	// 2. Build frontmatter data
	fmData := schema.BuildFrontmatterData(d.Schema, d.Data, d.virtuals)

	// 3. Build record data
	recordData := schema.BuildRecordData(d.Schema, d.Data, d.virtuals)
	recordData["file"] = d.RelativeFilePath()

	// 4. Write markdown file
	mdPath := d.FilePath()
	if err := storage.WriteMarkdown(mdPath, fmData, d.Content); err != nil {
		return fmt.Errorf("writing markdown: %w", err)
	}

	// 5. Upsert record
	recordsPath, err := storage.RecordsPathForPartition(
		d.RecordsDir(), d.Schema.Partition, d.Schema.DateField, d.Data,
	)
	if err != nil {
		return fmt.Errorf("resolving records path: %w", err)
	}

	records, err := storage.LoadRecords(recordsPath)
	if err != nil {
		return fmt.Errorf("loading records: %w", err)
	}

	records = storage.UpsertRecord(records, recordData, d.Schema.IDField)
	if err := storage.SaveRecords(recordsPath, records); err != nil {
		return fmt.Errorf("saving records: %w", err)
	}

	// 6. Update integrity manifest
	if err := d.updateManifest(fmData, recordData); err != nil {
		return fmt.Errorf("updating manifest: %w", err)
	}

	// 7. Post-save hook (KG indexing, etc.)
	if d.OnSave != nil {
		if err := d.OnSave(d); err != nil {
			// Log but don't fail the save — KG is secondary to the file write
			fmt.Fprintf(os.Stderr, "warning: post-save hook failed for %s: %v\n", d.ID(), err)
		}
	}

	return nil
}

// Delete removes the document's markdown file, record entry, and manifest entry.
func (d *Document) Delete() error {
	id := d.ID()

	// Remove markdown file
	mdPath := d.FilePath()
	if err := removeIfExists(mdPath); err != nil {
		return fmt.Errorf("deleting markdown file: %w", err)
	}

	// Remove from records
	recordsPath, err := storage.RecordsPathForPartition(
		d.RecordsDir(), d.Schema.Partition, d.Schema.DateField, d.Data,
	)
	if err != nil {
		return fmt.Errorf("resolving records path: %w", err)
	}

	records, err := storage.LoadRecords(recordsPath)
	if err != nil {
		return fmt.Errorf("loading records: %w", err)
	}

	records, _ = storage.RemoveRecord(records, d.Schema.IDField, id)
	if err := storage.SaveRecords(recordsPath, records); err != nil {
		return fmt.Errorf("saving records after delete: %w", err)
	}

	// Remove from manifest
	manifest, err := integrity.LoadManifest(d.RecordsDir())
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}
	manifest.RemoveEntry(id)
	if err := manifest.Save(d.RecordsDir()); err != nil {
		return fmt.Errorf("saving manifest: %w", err)
	}

	// Post-delete hook (KG cleanup)
	if d.OnDelete != nil {
		if err := d.OnDelete(id); err != nil {
			fmt.Fprintf(os.Stderr, "warning: post-delete hook failed for %s: %v\n", id, err)
		}
	}

	return nil
}

func (d *Document) updateManifest(fmData, recordData map[string]any) error {
	manifest, err := integrity.LoadManifest(d.RecordsDir())
	if err != nil {
		return err
	}

	entry := &integrity.Entry{
		File:           d.RelativeFilePath(),
		ContentSHA:     integrity.HashContent(d.Content),
		FrontmatterSHA: integrity.HashFrontmatter(fmData),
		RecordSHA:      integrity.HashRecord(recordData),
	}

	// Sign with HMAC if key is available
	key, err := integrity.LoadKey()
	if err != nil {
		return fmt.Errorf("loading integrity key: %w", err)
	}
	if key != nil {
		entry.Sig = integrity.SignEntry(entry, key)
		manifest.HMAC = true
	}

	manifest.SetEntry(d.ID(), entry)
	return manifest.Save(d.RecordsDir())
}

func removeIfExists(path string) error {
	err := removeFile(path)
	if err != nil && !isNotExist(err) {
		return err
	}
	return nil
}
