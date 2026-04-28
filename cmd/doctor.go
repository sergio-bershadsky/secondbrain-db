package cmd

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sergio-bershadsky/secondbrain-db/internal/document"
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
	doctorAll          bool
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
	doctorCheckCmd.Flags().BoolVar(&doctorAll, "all", false, "audit all docs, not just uncommitted changes")

	rootCmd.AddCommand(doctorCmd)
}

func runDoctorCheck(cmd *cobra.Command, args []string) error {
	if os.Getenv("SBDB_USE_SIDECAR") == "1" {
		return runDoctorCheckV2(cmd, args)
	}
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

	result := map[string]any{
		"drift_count":  len(driftIssues),
		"tamper_count": len(tamperIssues),
		"drift":        driftIssues,
		"tamper":       tamperIssues,
	}

	if err := output.PrintData(format, result); err != nil {
		return err
	}

	hasDrift := len(driftIssues) > 0
	hasTamper := len(tamperIssues) > 0

	if hasDrift && hasTamper {
		os.Exit(7)
	} else if hasTamper {
		os.Exit(6)
	} else if hasDrift {
		os.Exit(4)
	}

	return nil
}

func runDoctorCheckV2(cmd *cobra.Command, _ []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}
	s, err := loadSchema(cfg)
	if err != nil {
		return err
	}
	docsDir := filepath.Join(cfg.BasePath, s.DocsDir)

	paths, err := scopedDocPaths(cfg.BasePath, docsDir, doctorAll)
	if err != nil {
		return err
	}

	key, _ := integrity.LoadKey()
	var drifts []map[string]any
	for _, mdPath := range paths {
		if report := checkOneDoc(s, cfg.BasePath, mdPath, key); report != nil {
			drifts = append(drifts, report)
		}
	}

	format := outputFormat(cfg)
	_ = output.PrintData(format, map[string]any{
		"action": "doctor.check",
		"scope":  scopeLabel(doctorAll),
		"drifts": drifts,
	})
	if len(drifts) > 0 {
		os.Exit(1)
	}
	return nil
}

func scopedDocPaths(basePath, docsDir string, all bool) ([]string, error) {
	if all {
		return collectMDsFromWalker(docsDir)
	}
	scope, err := integrity.NewGitScope(basePath)
	if err != nil {
		return nil, err
	}
	if !scope.IsRepo {
		fmt.Fprintln(os.Stderr, "not a git repo; falling back to --all")
		return collectMDsFromWalker(docsDir)
	}
	var out []string
	for _, p := range scope.PairScopedPaths() {
		if !strings.HasSuffix(p, ".md") {
			continue
		}
		if !strings.HasPrefix(p, docsDir+string(filepath.Separator)) && p != docsDir {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

func collectMDsFromWalker(docsDir string) ([]string, error) {
	docs, err := storage.WalkDocsToSlice(docsDir)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(docs))
	for _, d := range docs {
		out = append(out, d.Path)
	}
	return out, nil
}

func scopeLabel(all bool) string {
	if all {
		return "all"
	}
	return "uncommitted"
}

func checkOneDoc(s *schemapkg.Schema, basePath, mdPath string, key []byte) map[string]any {
	mdExists := fileExists(mdPath)
	sc, err := integrity.LoadSidecar(mdPath)
	switch {
	case !mdExists && err == nil:
		return map[string]any{"file": relPath(basePath, mdPath), "drift": "missing-md"}
	case mdExists && os.IsNotExist(err):
		return map[string]any{"file": relPath(basePath, mdPath), "drift": "missing-sidecar"}
	case err != nil && !os.IsNotExist(err):
		return map[string]any{"file": relPath(basePath, mdPath), "drift": "sidecar-parse-error", "error": err.Error()}
	case err != nil:
		// covered above; safety
		return nil
	}

	fm, body, perr := storage.ParseMarkdown(mdPath)
	if perr != nil {
		return map[string]any{"file": relPath(basePath, mdPath), "drift": "md-parse-error", "error": perr.Error()}
	}
	rec := schemapkg.BuildRecordData(s, fm, nil)
	if rel, e := filepath.Rel(basePath, mdPath); e == nil {
		rec["file"] = rel
	}
	d, _ := sc.Verify(mdPath, fm, body, rec, key)
	if !d.Any() {
		return nil
	}
	causes := []string{}
	if d.ContentDrift {
		causes = append(causes, "content_sha mismatch")
	}
	if d.FrontmatterDrift {
		causes = append(causes, "frontmatter_sha mismatch")
	}
	if d.RecordDrift {
		causes = append(causes, "record_sha mismatch")
	}
	if d.BadSig {
		causes = append(causes, "bad_sig")
	}
	return map[string]any{
		"file":   relPath(basePath, mdPath),
		"drift":  "tamper",
		"causes": causes,
	}
}

func relPath(base, path string) string {
	if rel, err := filepath.Rel(base, path); err == nil {
		return rel
	}
	return path
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
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

	return output.PrintData(format, map[string]any{
		"action": "fix",
		"fixed":  fixed,
	})
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
