package untracked

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/integrity"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/schema"
)

// FileClass describes how sbdb classifies a file.
type FileClass int

const (
	ClassSchemaManaged FileClass = iota // has a sidecar under a schema's docs_dir
	ClassUntracked                      // in data/.untracked.yaml
	ClassUnregistered                   // exists on disk but not tracked by either
)

// ClassifyFile determines whether a file is schema-managed, untracked, or unregistered.
func ClassifyFile(relPath string, schemas []*schema.Schema, basePath string, registry *Registry) FileClass {
	// Check untracked registry first (fastest)
	if registry.Has(relPath) {
		return ClassUntracked
	}

	// A file is schema-managed if it falls under a schema's docs_dir AND
	// has a sidecar file next to it.
	for _, s := range schemas {
		if !strings.HasPrefix(relPath, s.DocsDir+"/") {
			continue
		}
		fullPath := filepath.Join(basePath, relPath)
		if _, err := os.Stat(integrity.SidecarPath(fullPath)); err == nil {
			return ClassSchemaManaged
		}
	}

	return ClassUnregistered
}

// DiscoverUnregistered walks a docs directory and returns files that are
// neither schema-managed nor in the untracked registry.
func DiscoverUnregistered(docsRoot, basePath string, schemas []*schema.Schema, registry *Registry) ([]string, error) {
	var unregistered []string

	excludeDirs := map[string]bool{
		".vitepress":   true,
		"node_modules": true,
		".git":         true,
		"public":       true,
		"dist":         true,
	}

	err := filepath.Walk(docsRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			if excludeDirs[filepath.Base(path)] {
				return filepath.SkipDir
			}
			return nil
		}

		if filepath.Ext(path) != ".md" {
			return nil
		}

		relPath, err := filepath.Rel(basePath, path)
		if err != nil {
			return nil
		}
		relPath = filepath.ToSlash(relPath) // normalize for cross-platform

		class := ClassifyFile(relPath, schemas, basePath, registry)
		if class == ClassUnregistered {
			unregistered = append(unregistered, relPath)
		}

		return nil
	})

	return unregistered, err
}
