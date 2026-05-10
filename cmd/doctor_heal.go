package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sergio-bershadsky/secondbrain-db/internal/cli/output"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/config"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/integrity"
	schemapkg "github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/schema"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/storage"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/virtuals"
)

// heal is sugar over fix + sign, with two safety properties: virtuals are
// always recomputed before signing (the order is load-bearing — otherwise
// sidecars freeze stale derived state), and tamper requires --i-meant-it
// because tamper detection is the integrity guard's whole point.
//
// Out of the existing primitives, fix and sign each handle half the work.
// heal exists so callers reach for one command instead of two and so
// programmatic callers (the post-fix Stop hook) have a single contract.

var (
	healMeantIt bool
	healIDs     []string
	healSince   string
	healAll     bool
)

var doctorHealCmd = &cobra.Command{
	Use:   "heal",
	Short: "Reconcile sidecars after manual edits (fix + sign in one step)",
	Long: `Heal composes 'doctor fix --recompute' and 'doctor sign --force':
checks each target document, recomputes virtual fields, and writes a fresh
sidecar in lockstep with the on-disk markdown.

Tamper requires --i-meant-it. This exists because tamper detection is a
safety feature; the flag is your acknowledgement that the edit was
intentional. Without it, tampered files are reported and the command
exits 6.`,
	RunE: runDoctorHeal,
}

func init() {
	doctorHealCmd.Flags().BoolVar(&healMeantIt, "i-meant-it", false, "acknowledge intentional edits to tampered files (required to re-sign)")
	doctorHealCmd.Flags().StringSliceVar(&healIDs, "id", nil, "heal a specific document; repeatable")
	doctorHealCmd.Flags().StringVar(&healSince, "since", "", "heal docs that differ from <git-ref> (e.g. HEAD, main)")
	doctorHealCmd.Flags().BoolVar(&healAll, "all", false, "heal every doc in every schema's docs_dir")
	doctorCmd.AddCommand(doctorHealCmd)
}

func runDoctorHeal(_ *cobra.Command, _ []string) error {
	scopeFlags := 0
	if healAll {
		scopeFlags++
	}
	if len(healIDs) > 0 {
		scopeFlags++
	}
	if healSince != "" {
		scopeFlags++
	}
	if scopeFlags > 1 {
		err := fmt.Errorf("--all, --id, and --since are mutually exclusive")
		fmt.Fprintln(os.Stderr, err)
		return err
	}

	cfg, err := resolveConfig()
	if err != nil {
		return err
	}
	schemas, err := loadAllSchemas(cfg)
	if err != nil {
		return err
	}
	if len(schemas) == 0 {
		return fmt.Errorf("no schemas found in %s", cfg.SchemaDir)
	}
	key, _ := integrity.LoadKey()

	paths, err := resolveHealPaths(cfg, schemas)
	if err != nil {
		return err
	}

	results := make([]map[string]any, 0, len(paths))
	counts := map[string]int{}
	tamperUnacked := 0

	for _, p := range paths {
		s := matchSchema(cfg.BasePath, schemas, p)
		rel := relPath(cfg.BasePath, p)
		if s == nil {
			results = append(results, map[string]any{"file": rel, "outcome": "skipped_no_schema"})
			counts["skipped_no_schema"]++
			continue
		}
		report := checkOneDoc(s, cfg.BasePath, p, key)
		if report == nil {
			results = append(results, map[string]any{"file": rel, "outcome": "no_change"})
			counts["no_change"]++
			continue
		}
		driftKind, _ := report["drift"].(string)
		switch driftKind {
		case "tamper":
			if !healMeantIt {
				results = append(results, map[string]any{
					"file":    rel,
					"outcome": "tamper_unacked",
					"causes":  report["causes"],
				})
				counts["tamper_unacked"]++
				tamperUnacked++
				continue
			}
			if flagDryRun {
				results = append(results, map[string]any{"file": rel, "outcome": "would_re_sign"})
				counts["would_re_sign"]++
				continue
			}
			if err := healOneDoc(s, cfg.BasePath, p, key); err != nil {
				return fmt.Errorf("re-signing %s: %w", p, err)
			}
			results = append(results, map[string]any{"file": rel, "outcome": "re_signed"})
			counts["re_signed"]++
		case "missing-sidecar":
			if flagDryRun {
				results = append(results, map[string]any{"file": rel, "outcome": "would_create_sidecar"})
				counts["would_create_sidecar"]++
				continue
			}
			if err := healOneDoc(s, cfg.BasePath, p, key); err != nil {
				return fmt.Errorf("creating sidecar for %s: %w", p, err)
			}
			results = append(results, map[string]any{"file": rel, "outcome": "drift_fixed"})
			counts["drift_fixed"]++
		case "missing-md":
			results = append(results, map[string]any{"file": rel, "outcome": "missing_md_skipped"})
			counts["missing_md_skipped"]++
		default:
			results = append(results, map[string]any{
				"file":    rel,
				"outcome": "error",
				"kind":    driftKind,
				"details": report["error"],
			})
			counts["error"]++
		}
	}

	payload := map[string]any{
		"action":     "doctor.heal",
		"scope":      healScopeLabel(),
		"results":    results,
		"counts":     counts,
		"dry_run":    flagDryRun,
		"i_meant_it": healMeantIt,
	}

	if tamperUnacked > 0 {
		_ = output.PrintData(outputFormat(cfg), payload)
		fmt.Fprintf(os.Stderr,
			"Tamper detected on %d file(s). If your edits were intentional, re-run with --i-meant-it to re-sign. Otherwise revert via git.\n",
			tamperUnacked)
		os.Exit(6)
	}
	return output.PrintData(outputFormat(cfg), payload)
}

// resolveHealPaths returns the absolute md paths that the current flag
// combination targets. Mutual exclusion has already been validated.
func resolveHealPaths(cfg *config.Config, schemas []*schemapkg.Schema) ([]string, error) {
	if healAll {
		var out []string
		for _, s := range schemas {
			docsDir := filepath.Join(cfg.BasePath, s.DocsDir)
			ps, err := collectMDsFromWalker(docsDir)
			if err != nil {
				return nil, err
			}
			out = append(out, ps...)
		}
		return out, nil
	}
	if len(healIDs) > 0 {
		return resolveHealPathsByID(cfg, schemas, healIDs)
	}
	if healSince != "" {
		return gitChangedPaths(cfg.BasePath, healSince)
	}
	// Default: uncommitted changes across every schema's docs dir.
	var out []string
	for _, s := range schemas {
		docsDir := filepath.Join(cfg.BasePath, s.DocsDir)
		ps, err := scopedDocPaths(cfg.BasePath, docsDir, false)
		if err != nil {
			return nil, err
		}
		out = append(out, ps...)
	}
	return out, nil
}

// resolveHealPathsByID maps each id to a path under whichever schema actually
// hosts a file with that id. Errors if an id resolves to no existing file.
func resolveHealPathsByID(cfg *config.Config, schemas []*schemapkg.Schema, ids []string) ([]string, error) {
	var out []string
	for _, id := range ids {
		var found string
		for _, s := range schemas {
			candidate := filepath.Join(cfg.BasePath, s.DocsDir, id+".md")
			if fileExists(candidate) {
				found = candidate
				break
			}
		}
		if found == "" {
			err := fmt.Errorf("id %q: no .md file under any schema's docs_dir", id)
			fmt.Fprintln(os.Stderr, err)
			return nil, err
		}
		out = append(out, found)
	}
	return out, nil
}

// gitChangedPaths returns absolute paths of .md files that differ between
// the working tree and ref. Filtering against schema docs_dirs happens later
// (matchSchema) — here we just collect candidates.
func gitChangedPaths(basePath, ref string) ([]string, error) {
	cmd := exec.Command("git", "-C", basePath, "diff", "--name-only", ref)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff %s: %w", ref, err)
	}
	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" || !strings.HasSuffix(line, ".md") {
			continue
		}
		abs := filepath.Join(basePath, line)
		paths = append(paths, abs)
	}
	return paths, nil
}

// matchSchema returns the schema whose docs_dir contains absPath, or nil
// if absPath is outside every schema. Used to filter unrelated paths from
// git diff output without erroring.
func matchSchema(basePath string, schemas []*schemapkg.Schema, absPath string) *schemapkg.Schema {
	for _, s := range schemas {
		docsDir := filepath.Join(basePath, s.DocsDir)
		sep := string(filepath.Separator)
		if absPath == docsDir || strings.HasPrefix(absPath, docsDir+sep) {
			return s
		}
	}
	return nil
}

// healOneDoc parses the on-disk markdown, recomputes virtuals via the
// schema's runtime, then writes a fresh sidecar. If a key is available the
// sidecar is HMAC-signed; otherwise the unsigned sidecar still has fresh
// content/frontmatter/record hashes.
//
// This is heal's load-bearing helper: it differs from writeSidecarFromMD by
// recomputing virtuals before BuildRecordData. Without that step, sidecars
// freeze whatever record_sha was present at last write — which silently
// hides virtual drift.
func healOneDoc(s *schemapkg.Schema, basePath, mdPath string, key []byte) error {
	fm, body, err := storage.ParseMarkdown(mdPath)
	if err != nil {
		return fmt.Errorf("parsing %s: %w", mdPath, err)
	}
	rt, err := loadRuntime(s)
	if err != nil {
		return fmt.Errorf("compiling virtuals for %s: %w", s.Entity, err)
	}
	virtualData, err := rt.EvaluateAll(body, fm)
	if err != nil {
		return fmt.Errorf("evaluating virtuals for %s: %w", mdPath, err)
	}
	rec := schemapkg.BuildRecordData(s, fm, virtualData)
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
	}
	return sc.Save(mdPath)
}

func healScopeLabel() string {
	switch {
	case healAll:
		return "all"
	case len(healIDs) > 0:
		return "id"
	case healSince != "":
		return "since:" + healSince
	default:
		return "uncommitted"
	}
}

// Compile-time check that we use virtuals — pulls the package into the
// binary even when no schema declares any virtual field.
var _ = virtuals.NewRuntime
