package document

import (
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/sergio-bershadsky/secondbrain-db/internal/integrity"
	"github.com/sergio-bershadsky/secondbrain-db/internal/schema"
	"github.com/sergio-bershadsky/secondbrain-db/internal/storage"
	"github.com/sergio-bershadsky/secondbrain-db/internal/virtuals"
)

// Logger is the slog handler used for non-fatal warnings (post-hook
// failures, deprecation notices). Default: slog.Default().
var Logger = slog.Default()

// Save writes the document to disk.
// Always writes <id>.yaml sidecar next to <id>.md.
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

	if err := d.writeSidecar(mdPath, fmData, recordData); err != nil {
		return fmt.Errorf("writing sidecar: %w", err)
	}

	if d.OnSave != nil {
		if err := d.OnSave(d); err != nil {
			Logger.Warn("post-save hook failed", "id", d.ID(), "error", err)
		}
	}
	return nil
}

// Delete removes the document's markdown file and its sidecar.
func (d *Document) Delete() error {
	id := d.ID()
	mdPath := d.FilePath()
	if err := removeIfExists(mdPath); err != nil {
		return fmt.Errorf("deleting markdown file: %w", err)
	}

	if err := removeIfExists(integrity.SidecarPath(mdPath)); err != nil {
		return fmt.Errorf("deleting sidecar: %w", err)
	}

	if d.OnDelete != nil {
		if err := d.OnDelete(id); err != nil {
			Logger.Warn("post-delete hook failed", "id", id, "error", err)
		}
	}
	return nil
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

func removeIfExists(path string) error {
	err := removeFile(path)
	if err != nil && !isNotExist(err) {
		return err
	}
	return nil
}
