package sync

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SidecarPath returns the integrations.yaml sidecar path for a .md file.
// Replaces the .md extension (case-insensitive) with .integrations.yaml.
func SidecarPath(mdPath string) string {
	ext := filepath.Ext(mdPath)
	stem := strings.TrimSuffix(mdPath, ext)
	return stem + ".integrations.yaml"
}

// ReadSidecar loads the sidecar for the given .md doc path. A missing file
// is not an error — it returns an empty Sidecar (the "never published"
// signal). Callers compute drift state from the absence of sections.
func ReadSidecar(mdPath string) (Sidecar, error) {
	path := SidecarPath(mdPath)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Sidecar{}, nil
		}
		return nil, fmt.Errorf("read sidecar %s: %w", path, err)
	}
	var sc Sidecar
	if err := yaml.Unmarshal(data, &sc); err != nil {
		return nil, fmt.Errorf("parse sidecar %s: %w", path, err)
	}
	if sc == nil {
		sc = Sidecar{}
	}
	return sc, nil
}
