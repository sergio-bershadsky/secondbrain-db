package schema

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Load reads and parses a schema from a YAML file.
func Load(path string) (*Schema, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading schema file %s: %w", path, err)
	}
	return Parse(data)
}

// Parse parses a schema from YAML bytes.
func Parse(data []byte) (*Schema, error) {
	var s Schema
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing schema YAML: %w", err)
	}

	// Set field names from map keys
	for name, f := range s.Fields {
		f.Name = name
	}
	for name, v := range s.Virtuals {
		v.Name = name
	}

	if err := s.Validate(); err != nil {
		return nil, err
	}

	return &s, nil
}

// LoadFromDir finds and loads a schema by entity name from a schemas directory.
func LoadFromDir(schemasDir, name string) (*Schema, error) {
	path := filepath.Join(schemasDir, name+".yaml")
	return Load(path)
}

// ListSchemas returns the names of all available schemas in a directory.
func ListSchemas(schemasDir string) ([]string, error) {
	entries, err := os.ReadDir(schemasDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading schemas directory: %w", err)
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".yaml" {
			name := e.Name()[:len(e.Name())-5] // strip .yaml
			names = append(names, name)
		}
	}
	return names, nil
}
