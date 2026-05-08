package schema

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/storage"
)

// CompatFailure is one document that fails the compat check.
type CompatFailure struct {
	Path  string
	Error string
}

// CompatReport collects all failures.
type CompatReport struct {
	Failures []CompatFailure
}

// CheckExisting validates every existing doc under s.DocsDir against s.
func CheckExisting(s *Schema, repoRoot string) (*CompatReport, error) {
	v, err := NewValidator(s)
	if err != nil {
		return nil, err
	}
	docsDir := filepath.Join(repoRoot, s.DocsDir)
	report := &CompatReport{}
	walkErr := filepath.WalkDir(docsDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(p, ".md") {
			return nil
		}
		fm, _, perr := storage.ParseMarkdown(p)
		if perr != nil {
			report.Failures = append(report.Failures, CompatFailure{Path: p, Error: perr.Error()})
			return nil
		}
		if verr := v.ValidateMap(fm); verr != nil {
			report.Failures = append(report.Failures, CompatFailure{Path: p, Error: verr.Error()})
		}
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("walk: %w", walkErr)
	}
	return report, nil
}
