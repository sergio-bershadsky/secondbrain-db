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

// WriteSidecar serialises sc to YAML and writes it atomically next to mdPath.
// Atomic = write to a temp file in the same directory, then rename.
func WriteSidecar(mdPath string, sc Sidecar) error {
	path := SidecarPath(mdPath)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	data, err := yaml.Marshal(sc)
	if err != nil {
		return fmt.Errorf("marshal sidecar: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".sidecar-*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename to %s: %w", path, err)
	}
	return nil
}

// RecordPush updates the sidecar for (mdPath, integration) on a SUCCESSFUL
// push: overwrites LastPush, clears LastError. LastCheck is left alone.
func RecordPush(mdPath, integration, targetID string, p PushRecord) error {
	return mutateSection(mdPath, integration, func(s *SidecarSection) {
		s.TargetID = targetID
		s.LastPush = &p
		s.LastError = nil
	})
}

// RecordCheck updates the sidecar on every drift check: overwrites LastCheck.
// LastPush and LastError are not touched.
func RecordCheck(mdPath, integration string, c CheckRecord) error {
	return mutateSection(mdPath, integration, func(s *SidecarSection) {
		s.LastCheck = &c
	})
}

// RecordError updates the sidecar on a FAILED attempt: overwrites LastError.
// LastPush is preserved (last good state survives a transient failure).
func RecordError(mdPath, integration string, e ErrorRecord) error {
	return mutateSection(mdPath, integration, func(s *SidecarSection) {
		s.LastError = &e
	})
}

func mutateSection(mdPath, integration string, fn func(*SidecarSection)) error {
	sc, err := ReadSidecar(mdPath)
	if err != nil {
		return err
	}
	section := sc[integration]
	fn(&section)
	sc[integration] = section
	return WriteSidecar(mdPath, sc)
}
