package cmd

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/sergio-bershadsky/secondbrain-db/internal/config"
	"github.com/sergio-bershadsky/secondbrain-db/internal/document"
	"github.com/sergio-bershadsky/secondbrain-db/internal/events"
	"github.com/sergio-bershadsky/secondbrain-db/internal/integrity"
	"github.com/sergio-bershadsky/secondbrain-db/internal/output"
	"github.com/sergio-bershadsky/secondbrain-db/internal/query"
	schemapkg "github.com/sergio-bershadsky/secondbrain-db/internal/schema"
	"github.com/sergio-bershadsky/secondbrain-db/internal/storage"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Consistency checker and repair tool",
}

var doctorCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Check for drift and integrity issues",
	Long:  `Scans all records, loads frontmatter, and compares. Reports drift (frontmatter vs record) and tamper (hash mismatch) issues.`,
	RunE:  runDoctorCheck,
}

var doctorFixCmd = &cobra.Command{
	Use:   "fix",
	Short: "Fix drift issues (does NOT re-sign tampered files)",
	RunE:  runDoctorFix,
}

var doctorSignCmd = &cobra.Command{
	Use:   "sign",
	Short: "Re-sign manifest entries from current on-disk state",
	Long:  `Use after intentional out-of-band edits. Requires --force to overwrite existing entries.`,
	RunE:  runDoctorSign,
}

var doctorStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Summary of KB consistency state",
	RunE:  runDoctorStatus,
}

var doctorInitKeyCmd = &cobra.Command{
	Use:   "init-key",
	Short: "Generate a new HMAC integrity key",
	RunE:  runDoctorInitKey,
}

var (
	doctorFixRecompute bool
	doctorSignForce    bool
	doctorSignID       string
)

func init() {
	doctorCmd.AddCommand(doctorCheckCmd)
	doctorCmd.AddCommand(doctorFixCmd)
	doctorCmd.AddCommand(doctorSignCmd)
	doctorCmd.AddCommand(doctorStatusCmd)
	doctorCmd.AddCommand(doctorInitKeyCmd)

	doctorFixCmd.Flags().BoolVar(&doctorFixRecompute, "recompute", false, "recompute virtual fields")
	doctorSignCmd.Flags().BoolVar(&doctorSignForce, "force", false, "overwrite existing entries")
	doctorSignCmd.Flags().StringVar(&doctorSignID, "id", "", "sign a specific document (default: all)")

	rootCmd.AddCommand(doctorCmd)
}

func runDoctorCheck(cmd *cobra.Command, _ []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}

	s, err := loadSchema(cfg)
	if err != nil {
		return err
	}

	format := outputFormat(cfg)
	qs := query.NewQuerySet(s, cfg.BasePath)

	docs, err := qs.All()
	if err != nil {
		return err
	}

	manifest, err := integrity.LoadManifest(filepath.Join(cfg.BasePath, s.RecordsDir))
	if err != nil {
		return err
	}

	rt, err := loadRuntime(s)
	if err != nil {
		return err
	}

	var driftIssues []map[string]any
	var tamperIssues []map[string]any

	for _, doc := range docs {
		if err := doc.EnsureLoaded(); err != nil {
			continue
		}

		// Re-evaluate virtuals so hashes match what save() produced
		if rt != nil && len(s.Virtuals) > 0 {
			vResults, vErr := rt.EvaluateAll(doc.Content, doc.Data)
			if vErr == nil {
				doc.SetVirtuals(vResults)
			}
		}

		id := doc.ID()

		// Check drift: compare frontmatter vs record for scalar fields
		drifts := checkDrift(s, doc)
		driftIssues = append(driftIssues, drifts...)

		// Check integrity
		entry, ok := manifest.Entries[id]
		if !ok {
			continue
		}

		fmData := schemapkg.BuildFrontmatterData(s, doc.Data, doc.Virtuals())
		recordData := schemapkg.BuildRecordData(s, doc.Data, doc.Virtuals())
		recordData["file"] = doc.RelativeFilePath()

		tc := integrity.Verify(entry,
			integrity.HashContent(doc.Content),
			integrity.HashFrontmatter(fmData),
			integrity.HashRecord(recordData),
		)
		if tc != nil {
			tamperIssues = append(tamperIssues, map[string]any{
				"kind":       "tamper",
				"id":         id,
				"file":       doc.RelativeFilePath(),
				"mismatched": tc.Mismatched,
			})
		}
	}

	// Spec §7.5: event-window invariant check.
	windowIssues, _ := checkEventWindow(cfg)

	result := map[string]any{
		"drift_count":         len(driftIssues),
		"tamper_count":        len(tamperIssues),
		"drift":               driftIssues,
		"tamper":              tamperIssues,
		"event_window_issues": windowIssues,
	}

	if err := output.PrintData(format, result); err != nil {
		return err
	}

	hasDrift := len(driftIssues) > 0
	hasTamper := len(tamperIssues) > 0
	hasWindow := len(windowIssues) > 0

	// Window violations report as drift (exit 4) — they're recoverable
	// via `sbdb doctor fix` (which performs the archival).
	if hasWindow {
		hasDrift = true
	}

	if hasDrift && hasTamper {
		os.Exit(7)
	} else if hasTamper {
		os.Exit(6)
	} else if hasDrift {
		os.Exit(4)
	}

	return nil
}

// checkEventWindow verifies that no live daily file lies outside the live
// window (current month + previous month). Returns one issue per offending
// (year, month) group.
func checkEventWindow(cfg *config.Config) ([]map[string]any, error) {
	if !cfg.Events.Enabled {
		return nil, nil
	}
	dir := filepath.Join(cfg.BasePath, events.EventsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	type key struct{ y, m int }
	expired := map[key]bool{}
	now := time.Now().UTC()
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		y, m, _, _, ok := events.ParseDailyName(ent.Name())
		if !ok {
			continue
		}
		if !events.IsLiveMonth(y, m, now) {
			expired[key{y, m}] = true
		}
	}
	var out []map[string]any
	for k := range expired {
		out = append(out, map[string]any{
			"kind":  "event_window",
			"month": fmt.Sprintf("%04d-%02d", k.y, k.m),
			"hint":  "run `sbdb doctor fix` to archive expired month(s)",
		})
	}
	return out, nil
}

func checkDrift(s *schemapkg.Schema, doc *document.Document) []map[string]any {
	recordsDir := filepath.Join(doc.BasePath, s.RecordsDir)
	records, err := storage.LoadAllPartitions(recordsDir, s.Partition)
	if err != nil {
		return nil
	}

	// Find the record matching this doc
	id := doc.ID()
	var rec map[string]any
	for _, r := range records {
		if fmt.Sprintf("%v", r[s.IDField]) == id {
			rec = r
			break
		}
	}
	if rec == nil {
		return nil
	}

	var issues []map[string]any
	for name, f := range s.Fields {
		if !f.Type.IsScalar() {
			continue
		}
		fmVal := doc.Data[name]
		recVal := rec[name]
		if fmt.Sprintf("%v", fmVal) != fmt.Sprintf("%v", recVal) {
			issues = append(issues, map[string]any{
				"kind":        "drift",
				"id":          id,
				"field":       name,
				"frontmatter": fmVal,
				"record":      recVal,
			})
		}
	}
	return issues
}

func runDoctorFix(cmd *cobra.Command, _ []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}

	s, err := loadSchema(cfg)
	if err != nil {
		return err
	}

	format := outputFormat(cfg)
	qs := query.NewQuerySet(s, cfg.BasePath)

	docs, err := qs.All()
	if err != nil {
		return err
	}

	rt, err := loadRuntime(s)
	if err != nil {
		return err
	}

	fixed := 0
	for _, doc := range docs {
		if err := doc.EnsureLoaded(); err != nil {
			continue
		}
		if err := doc.Save(rt); err != nil {
			return fmt.Errorf("fixing %s: %w", doc.ID(), err)
		}
		fixed++
	}

	// Spec §7.7: doctor fix archives any expired months.
	archived, err := archiveExpiredMonths(cmd.Context(), cfg)
	if err != nil {
		return fmt.Errorf("archiving expired months: %w", err)
	}

	return output.PrintData(format, map[string]any{
		"action":   "fix",
		"fixed":    fixed,
		"archived": archived,
	})
}

// archiveExpiredMonths runs the events Archiver against the configured
// target (currently git only; s3 is wired in archive_s3.go follow-up).
func archiveExpiredMonths(ctx context.Context, cfg *config.Config) ([]string, error) {
	if !cfg.Events.Enabled {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	target := events.NewGitTarget(cfg.BasePath)
	arc := events.NewArchiver(cfg.BasePath, target)
	if cfg.Events.Archive.GzipLevel > 0 {
		arc.GzipLevel = cfg.Events.Archive.GzipLevel
	}
	if cfg.Events.Archive.SettleDays > 0 {
		arc.SettleDays = cfg.Events.Archive.SettleDays
	}
	if cfg.Events.Archive.MaxArchiveBytes > 0 {
		arc.MaxBytes = cfg.Events.Archive.MaxArchiveBytes
	}

	sealed, err := arc.ArchiveExpired(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]string, 0, len(sealed))
	for _, p := range sealed {
		out = append(out, p.Month)
	}
	return out, nil
}

func runDoctorSign(cmd *cobra.Command, _ []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}

	s, err := loadSchema(cfg)
	if err != nil {
		return err
	}

	format := outputFormat(cfg)

	if !doctorSignForce {
		output.PrintError(format, "CONFIRMATION_REQUIRED",
			"use --force to re-sign entries from current on-disk state", nil)
		os.Exit(1)
	}

	recordsDir := filepath.Join(cfg.BasePath, s.RecordsDir)
	manifest, err := integrity.LoadManifest(recordsDir)
	if err != nil {
		return err
	}

	key, err := integrity.LoadKey()
	if err != nil {
		return err
	}

	qs := query.NewQuerySet(s, cfg.BasePath)
	docs, err := qs.All()
	if err != nil {
		return err
	}

	rt, rtErr := loadRuntime(s)

	signed := 0
	for _, doc := range docs {
		id := doc.ID()
		if doctorSignID != "" && id != doctorSignID {
			continue
		}

		if err := doc.EnsureLoaded(); err != nil {
			continue
		}

		// Re-evaluate virtuals from current content so hashes are correct
		if rtErr == nil && rt != nil && len(s.Virtuals) > 0 {
			vResults, vErr := rt.EvaluateAll(doc.Content, doc.Data)
			if vErr == nil {
				doc.SetVirtuals(vResults)
			}
		}

		fmData := schemapkg.BuildFrontmatterData(s, doc.Data, doc.Virtuals())
		recordData := schemapkg.BuildRecordData(s, doc.Data, doc.Virtuals())
		recordData["file"] = doc.RelativeFilePath()

		entry := &integrity.Entry{
			File:           doc.RelativeFilePath(),
			ContentSHA:     integrity.HashContent(doc.Content),
			FrontmatterSHA: integrity.HashFrontmatter(fmData),
			RecordSHA:      integrity.HashRecord(recordData),
		}

		if key != nil {
			entry.Sig = integrity.SignEntry(entry, key)
			manifest.HMAC = true
		}

		manifest.SetEntry(id, entry)
		signed++
	}

	if err := manifest.Save(recordsDir); err != nil {
		return err
	}

	// Spec §4.5: emit integrity.signed when one or more entries are re-signed.
	if signed > 0 {
		emitIntegrityEvent(cfg, "signed", recordsDir, signed)
	}

	return output.PrintData(format, map[string]any{
		"action": "sign",
		"signed": signed,
	})
}

func runDoctorStatus(cmd *cobra.Command, _ []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}

	s, err := loadSchema(cfg)
	if err != nil {
		return err
	}

	format := outputFormat(cfg)
	recordsDir := filepath.Join(cfg.BasePath, s.RecordsDir)

	manifest, err := integrity.LoadManifest(recordsDir)
	if err != nil {
		return err
	}

	return output.PrintData(format, map[string]any{
		"entity":    s.Entity,
		"entries":   len(manifest.Entries),
		"hmac":      manifest.HMAC,
		"algo":      manifest.Algo,
		"integrity": s.Integrity,
	})
}

func runDoctorInitKey(cmd *cobra.Command, _ []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}

	format := outputFormat(cfg)

	key, err := integrity.GenerateKey()
	if err != nil {
		return err
	}

	keyPath, err := integrity.DefaultKeyPath()
	if err != nil {
		return err
	}

	if err := integrity.SaveKeyFile(keyPath, key); err != nil {
		return err
	}

	return output.PrintData(format, map[string]any{
		"action":   "init-key",
		"key_path": keyPath,
		"key_hex":  hex.EncodeToString(key),
		"warning":  "back up this key — losing it means you cannot verify HMAC signatures",
	})
}
