package untracked

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/sergio-bershadsky/secondbrain-db/internal/schema"
	"github.com/sergio-bershadsky/secondbrain-db/internal/storage"
)

// FileClass describes how sbdb classifies a file.
type FileClass int

const (
	ClassSchemaManaged FileClass = iota // in a schema's records.yaml
	ClassUntracked                      // in data/.untracked.yaml
	ClassUnregistered                   // exists on disk but not tracked by either
)

// ClassifyFile determines whether a file is schema-managed, untracked, or unregistered.
func ClassifyFile(relPath string, schemas []*schema.Schema, basePath string, registry *Registry) FileClass {
	// Check untracked registry first (fastest)
	if registry.Has(relPath) {
		return ClassUntracked
	}

	// Check each schema's records
	for _, s := range schemas {
		if !strings.HasPrefix(relPath, s.DocsDir+"/") {
			continue
		}

		// Load records for this schema and check if file is there
		recordsDir := filepath.Join(basePath, s.RecordsDir)
		records, err := storage.LoadAllPartitions(recordsDir, s.Partition)
		if err != nil {
			continue
		}

		for _, rec := range records {
			recFile, ok := rec["file"]
			if ok && recFile == relPath {
				return ClassSchemaManaged
			}
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

		class := ClassifyFile(relPath, schemas, basePath, registry)
		if class == ClassUnregistered {
			unregistered = append(unregistered, relPath)
		}

		return nil
	})

	return unregistered, err
}
