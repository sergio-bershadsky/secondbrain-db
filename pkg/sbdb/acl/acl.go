package acl

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ACLFile is the on-disk YAML for docs/.sbdb/acl/<doc>.yaml.
type ACLFile struct {
	Version int     `yaml:"version"`
	Readers []Token `yaml:"readers"`
}

// ReadACL loads an ACL file from path. Missing file returns ErrACLNotFound.
func ReadACL(path string) (ACLFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ACLFile{}, ErrACLNotFound
		}
		return ACLFile{}, fmt.Errorf("sbdb/acl: read %s: %w", path, err)
	}
	var a ACLFile
	if err := yaml.Unmarshal(b, &a); err != nil {
		return ACLFile{}, fmt.Errorf("sbdb/acl: parse %s: %w", path, err)
	}
	if a.Version == 0 {
		a.Version = 1
	}
	for _, r := range a.Readers {
		if _, err := ParseToken(string(r)); err != nil {
			return ACLFile{}, fmt.Errorf("sbdb/acl: %s: %w", path, err)
		}
	}
	return a, nil
}

// WriteACL serialises an ACL file to path, creating parent dirs.
func WriteACL(path string, a ACLFile) error {
	if a.Version == 0 {
		a.Version = 1
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := yaml.Marshal(a)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
