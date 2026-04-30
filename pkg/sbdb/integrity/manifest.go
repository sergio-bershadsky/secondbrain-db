package integrity

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
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

// ManifestPath returns the path to the integrity manifest for an entity.
func ManifestPath(recordsDir string) string {
	return filepath.Join(recordsDir, ".integrity.yaml")
}

// ManifestExists checks if a manifest file exists.
func ManifestExists(recordsDir string) bool {
	_, err := os.Stat(ManifestPath(recordsDir))
	return err == nil
}
