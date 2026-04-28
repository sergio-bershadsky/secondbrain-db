package document

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/sergio-bershadsky/secondbrain-db/internal/integrity"
	"github.com/sergio-bershadsky/secondbrain-db/internal/schema"
	"github.com/sergio-bershadsky/secondbrain-db/internal/storage"
	"github.com/sergio-bershadsky/secondbrain-db/internal/virtuals"
)

// Save writes the document to disk.
// When SBDB_USE_SIDECAR=1, it writes <id>.yaml next to <id>.md and skips
// records.yaml + .integrity.yaml. Otherwise it follows the legacy aggregate path.
func (d *Document) Save(rt *virtuals.Runtime) error {
	if rt != nil && len(d.Schema.Virtuals) > 0 {
		vResults, err := rt.EvaluateAll(d.Content, d.Data)
		if err != nil {
			return fmt.Errorf("evaluating virtuals: %w", err)
		}
		d.SetVirtuals(vResults)
	}

	fmData := schema.BuildFrontmatterData(d.Schema, d.Data, d.virtuals)
	recordData := schema.BuildRecordData(d.Schema, d.Data, d.virtuals)
	recordData["file"] = d.RelativeFilePath()

	mdPath := d.FilePath()
	if err := storage.WriteMarkdown(mdPath, fmData, d.Content); err != nil {
		return fmt.Errorf("writing markdown: %w", err)
	}

	if useSidecar() {
		if err := d.writeSidecar(mdPath, fmData, recordData); err != nil {
			return fmt.Errorf("writing sidecar: %w", err)
		}
	} else {
		if err := d.writeLegacyRecordsAndManifest(fmData, recordData); err != nil {
			return err
		}
	}

	if d.OnSave != nil {
		if err := d.OnSave(d); err != nil {
			fmt.Fprintf(os.Stderr, "warning: post-save hook failed for %s: %v\n", d.ID(), err)
		}
	}
	return nil
}

// Delete removes the document's markdown file and its integrity record.
// In sidecar mode it removes the sidecar file; in legacy mode it mutates
// records.yaml and the aggregate manifest.
func (d *Document) Delete() error {
	id := d.ID()
	mdPath := d.FilePath()
	if err := removeIfExists(mdPath); err != nil {
		return fmt.Errorf("deleting markdown file: %w", err)
	}

	if useSidecar() {
		if err := removeIfExists(integrity.SidecarPath(mdPath)); err != nil {
			return fmt.Errorf("deleting sidecar: %w", err)
		}
	} else {
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
		manifest, err := integrity.LoadManifest(d.RecordsDir())
		if err != nil {
			return fmt.Errorf("loading manifest: %w", err)
		}
		manifest.RemoveEntry(id)
		if err := manifest.Save(d.RecordsDir()); err != nil {
			return fmt.Errorf("saving manifest: %w", err)
		}
	}

	if d.OnDelete != nil {
		if err := d.OnDelete(id); err != nil {
			fmt.Fprintf(os.Stderr, "warning: post-delete hook failed for %s: %v\n", id, err)
		}
	}
	return nil
}

// useSidecar reports whether the per-document sidecar mode is enabled.
func useSidecar() bool {
	return os.Getenv("SBDB_USE_SIDECAR") == "1"
}

// writeSidecar computes integrity hashes and saves a .yaml sidecar next to mdPath.
func (d *Document) writeSidecar(mdPath string, fmData, recordData map[string]any) error {
	sc := &integrity.Sidecar{
		Version:        1,
		Algo:           "sha256",
		File:           filepath.Base(mdPath),
		ContentSHA:     integrity.HashContent(d.Content),
		FrontmatterSHA: integrity.HashFrontmatter(fmData),
		RecordSHA:      integrity.HashRecord(recordData),
	}
	key, err := integrity.LoadKey()
	if err != nil {
		return fmt.Errorf("loading integrity key: %w", err)
	}
	if key != nil {
		sc.HMAC = true
		sc.Sig = sc.SignWith(key)
	}
	return sc.Save(mdPath)
}

// writeLegacyRecordsAndManifest upserts records.yaml and updates the aggregate manifest.
func (d *Document) writeLegacyRecordsAndManifest(fmData, recordData map[string]any) error {
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
	if err := d.updateManifest(fmData, recordData); err != nil {
		return fmt.Errorf("updating manifest: %w", err)
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
