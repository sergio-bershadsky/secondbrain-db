// Package untracked manages non-schema files that are signed for integrity
// but not part of any schema's records.yaml.
package untracked

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/sergio-bershadsky/secondbrain-db/internal/version"
)

// Clock can be overridden by callers to make timestamps deterministic in tests.
// Default: time.Now.
var Clock = time.Now

// Registry holds all untracked-but-signed file entries.
type Registry struct {
	Version int     `yaml:"version"`
	Entries []Entry `yaml:"entries"`
}

// Entry represents a single untracked file's integrity record.
type Entry struct {
	File           string `yaml:"file"`
	ContentSHA     string `yaml:"content_sha"`
	FrontmatterSHA string `yaml:"frontmatter_sha"`
	Sig            string `yaml:"sig,omitempty"`
	UpdatedAt      string `yaml:"updated_at"`
	Writer         string `yaml:"writer"`
}

const registryFilename = ".untracked.yaml"

// Load reads the untracked registry from data/.untracked.yaml.
// Returns an empty registry if the file doesn't exist.
func Load(basePath string) (*Registry, error) {
	path := RegistryPath(basePath)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Registry{Version: 1}, nil
		}
		return nil, fmt.Errorf("reading untracked registry: %w", err)
	}

	var r Registry
	if err := yaml.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parsing untracked registry: %w", err)
	}
	if r.Version == 0 {
		r.Version = 1
	}
	return &r, nil
}

// Save writes the registry to disk atomically.
func (r *Registry) Save(basePath string) error {
	path := RegistryPath(basePath)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating registry directory: %w", err)
	}

	data, err := yaml.Marshal(r)
	if err != nil {
		return fmt.Errorf("marshaling registry: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".sbdb-untracked-*.yaml.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	tmp.Close()

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}

// Add inserts or updates an entry in the registry.
func (r *Registry) Add(e Entry) {
	e.UpdatedAt = Clock().UTC().Format(time.RFC3339)
	e.Writer = "secondbrain-db/" + version.Version

	for i, existing := range r.Entries {
		if existing.File == e.File {
			r.Entries[i] = e
			return
		}
	}
	r.Entries = append(r.Entries, e)
}

// Remove deletes an entry by file path. Returns true if found.
func (r *Registry) Remove(file string) bool {
	for i, e := range r.Entries {
		if e.File == file {
			r.Entries = append(r.Entries[:i], r.Entries[i+1:]...)
			return true
		}
	}
	return false
}

// Get returns the entry for a file path, or nil if not found.
func (r *Registry) Get(file string) *Entry {
	for i := range r.Entries {
		if r.Entries[i].File == file {
			return &r.Entries[i]
		}
	}
	return nil
}

// Has returns true if the file is tracked.
func (r *Registry) Has(file string) bool {
	return r.Get(file) != nil
}

// Count returns the number of tracked files.
func (r *Registry) Count() int {
	return len(r.Entries)
}

// RegistryPath returns the path to the untracked registry file.
func RegistryPath(basePath string) string {
	return filepath.Join(basePath, "data", registryFilename)
}
