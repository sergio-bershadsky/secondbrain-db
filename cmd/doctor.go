package cmd

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sergio-bershadsky/secondbrain-db/internal/cli/output"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/integrity"
	schemapkg "github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/schema"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/storage"
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

var doctorMigrateDryRun bool

var doctorMigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate v1 (data/) layout to v2 (per-md sidecars)",
	RunE:  runDoctorMigrate,
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
	doctorMigrateCmd.Flags().BoolVar(&doctorMigrateDryRun, "dry-run", false, "report planned changes without writing")
	doctorCmd.AddCommand(doctorMigrateCmd)

	doctorFixCmd.Flags().BoolVar(&doctorFixRecompute, "recompute", false, "recompute virtual fields")
	doctorSignCmd.Flags().BoolVar(&doctorSignForce, "force", false, "overwrite existing entries")
	doctorSignCmd.Flags().StringVar(&doctorSignID, "id", "", "sign a specific document (default: all)")
	doctorCheckCmd.Flags().BoolVar(&doctorAll, "all", false, "audit all docs, not just uncommitted changes")
	doctorFixCmd.Flags().BoolVar(&doctorAll, "all", false, "audit all docs, not just uncommitted changes")
	doctorSignCmd.Flags().BoolVar(&doctorAll, "all", false, "audit all docs, not just uncommitted changes")

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

func runDoctorFix(cmd *cobra.Command, _ []string) error {
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

	fixed := 0
	for _, mdPath := range paths {
		if err := writeSidecarFromMD(s, cfg.BasePath, mdPath, key, false); err != nil {
			return err
		}
		fixed++
	}
	return output.PrintData(outputFormat(cfg), map[string]any{
		"action": "doctor.fix",
		"scope":  scopeLabel(doctorAll),
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
	key, err := integrity.LoadKey()
	if err != nil {
		return err
	}
	if key == nil {
		keyErr := fmt.Errorf("sign requires an HMAC key; run sbdb doctor init-key")
		fmt.Fprintln(os.Stderr, keyErr)
		return keyErr
	}

	docsDir := filepath.Join(cfg.BasePath, s.DocsDir)
	paths, err := scopedDocPaths(cfg.BasePath, docsDir, doctorAll)
	if err != nil {
		return err
	}

	signed := 0
	for _, mdPath := range paths {
		if err := writeSidecarFromMD(s, cfg.BasePath, mdPath, key, true); err != nil {
			return err
		}
		signed++
	}
	return output.PrintData(outputFormat(cfg), map[string]any{
		"action": "doctor.sign",
		"scope":  scopeLabel(doctorAll),
		"signed": signed,
	})
}

// writeSidecarFromMD parses mdPath, recomputes hashes, and writes a fresh
// sidecar. If requireKey is true, key must be non-nil and the sidecar is
// HMAC-signed; otherwise signing is best-effort.
func writeSidecarFromMD(s *schemapkg.Schema, basePath, mdPath string, key []byte, requireKey bool) error {
	fm, body, err := storage.ParseMarkdown(mdPath)
	if err != nil {
		return fmt.Errorf("parsing %s: %w", mdPath, err)
	}
	rec := schemapkg.BuildRecordData(s, fm, nil)
	if rel, e := filepath.Rel(basePath, mdPath); e == nil {
		rec["file"] = rel
	}
	sc := &integrity.Sidecar{
		Version:        1,
		Algo:           "sha256",
		File:           filepath.Base(mdPath),
		ContentSHA:     integrity.HashContent(body),
		FrontmatterSHA: integrity.HashFrontmatter(fm),
		RecordSHA:      integrity.HashRecord(rec),
	}
	if key != nil {
		sc.HMAC = true
		sc.Sig = sc.SignWith(key)
	} else if requireKey {
		return fmt.Errorf("sign requires an HMAC key")
	}
	return sc.Save(mdPath)
}

func runDoctorMigrate(cmd *cobra.Command, _ []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}

	dataDir := filepath.Join(cfg.BasePath, "data")
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		return output.PrintData(outputFormat(cfg), map[string]any{
			"action": "doctor.migrate",
			"result": "already-v2",
		})
	}

	schemas, err := loadAllSchemas(cfg)
	if err != nil {
		return err
	}

	migrated := 0
	for _, s := range schemas {
		if s.RecordsDir == "" {
			continue
		}
		recordsDir := filepath.Join(cfg.BasePath, s.RecordsDir)
		if _, err := os.Stat(recordsDir); os.IsNotExist(err) {
			continue
		}

		records, err := storage.LoadAllPartitions(recordsDir, s.Partition)
		if err != nil {
			return fmt.Errorf("loading legacy records for %s: %w", s.Entity, err)
		}
		manifest, mErr := integrity.LoadManifest(recordsDir)
		if mErr != nil {
			return fmt.Errorf("loading legacy manifest for %s: %w", s.Entity, mErr)
		}

		for _, rec := range records {
			id := fmt.Sprintf("%v", rec[s.IDField])
			file, _ := rec["file"].(string)
			if file == "" {
				continue
			}
			mdPath := filepath.Join(cfg.BasePath, file)
			entry := manifest.Entries[id]
			sc := buildSidecarFromV1(rec, entry, mdPath)
			if doctorMigrateDryRun {
				continue
			}
			if err := sc.Save(mdPath); err != nil {
				return fmt.Errorf("writing sidecar for %s: %w", id, err)
			}
			migrated++
		}
	}

	if !doctorMigrateDryRun {
		if err := os.RemoveAll(dataDir); err != nil {
			return fmt.Errorf("removing legacy data/: %w", err)
		}
	}

	return output.PrintData(outputFormat(cfg), map[string]any{
		"action":   "doctor.migrate",
		"migrated": migrated,
		"dry_run":  doctorMigrateDryRun,
	})
}

// buildSidecarFromV1 converts a legacy records.yaml + manifest entry to a
// v2 per-doc Sidecar. If the manifest has no entry for this id, the sidecar
// is rebuilt by recomputing hashes from the on-disk markdown.
func buildSidecarFromV1(rec map[string]any, entry *integrity.Entry, mdPath string) *integrity.Sidecar {
	sc := &integrity.Sidecar{
		Version: 1, Algo: "sha256",
		File: filepath.Base(mdPath),
	}
	if entry != nil {
		sc.ContentSHA = entry.ContentSHA
		sc.FrontmatterSHA = entry.FrontmatterSHA
		sc.RecordSHA = entry.RecordSHA
		sc.Sig = entry.Sig
		sc.HMAC = entry.Sig != ""
		sc.UpdatedAt = entry.UpdatedAt
		sc.Writer = entry.Writer
		return sc
	}
	// Rebuild from disk if no manifest entry.
	fm, body, err := storage.ParseMarkdown(mdPath)
	if err == nil {
		// Normalise the legacy `file` field to OS-native separators so the
		// migrated sidecar's record_sha matches what `doctor check` recomputes
		// later (which derives `file` via filepath.Rel — backslashes on
		// Windows). Without this, post-migrate verification fails on Windows.
		if f, ok := rec["file"].(string); ok && f != "" {
			rec["file"] = filepath.FromSlash(f)
		}
		sc.ContentSHA = integrity.HashContent(body)
		sc.FrontmatterSHA = integrity.HashFrontmatter(fm)
		sc.RecordSHA = integrity.HashRecord(rec)
	}
	return sc
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
