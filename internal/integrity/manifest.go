package integrity

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/sergio-bershadsky/secondbrain-db/internal/version"
)

// Entry represents a single document's integrity record.
type Entry struct {
	File           string `yaml:"file"`
	ContentSHA     string `yaml:"content_sha"`
	FrontmatterSHA string `yaml:"frontmatter_sha"`
	RecordSHA      string `yaml:"record_sha"`
	Sig            string `yaml:"sig,omitempty"` // HMAC signature (optional)
	UpdatedAt      string `yaml:"updated_at"`
	Writer         string `yaml:"writer"`
}

// Manifest holds all integrity entries for an entity.
type Manifest struct {
	Version int               `yaml:"version"`
	Algo    string            `yaml:"algo"`
	HMAC    bool              `yaml:"hmac"`
	Entries map[string]*Entry `yaml:"entries"`
}

// LoadManifest reads the integrity manifest for an entity.
// Returns an empty manifest if the file doesn't exist.
func LoadManifest(recordsDir string) (*Manifest, error) {
	path := ManifestPath(recordsDir)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Manifest{
				Version: 1,
				Algo:    "sha256",
				Entries: make(map[string]*Entry),
			}, nil
		}
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}
	if m.Entries == nil {
		m.Entries = make(map[string]*Entry)
	}
	return &m, nil
}

// Save writes the manifest to disk atomically.
func (m *Manifest) Save(recordsDir string) error {
	path := ManifestPath(recordsDir)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating manifest directory: %w", err)
	}

	data, err := yaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".sbdb-manifest-*.yaml.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("writing manifest: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming manifest: %w", err)
	}

	return nil
}

// SetEntry creates or updates an entry in the manifest.
func (m *Manifest) SetEntry(id string, entry *Entry) {
	entry.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	entry.Writer = "secondbrain-db/" + version.Version
	m.Entries[id] = entry
}

// RemoveEntry deletes an entry from the manifest.
func (m *Manifest) RemoveEntry(id string) {
	delete(m.Entries, id)
}

// ManifestPath returns the path to the integrity manifest for an entity.
func ManifestPath(recordsDir string) string {
	return filepath.Join(recordsDir, ".integrity.yaml")
}

// ManifestExists checks if a manifest file exists.
func ManifestExists(recordsDir string) bool {
	_, err := os.Stat(ManifestPath(recordsDir))
	return err == nil
}
