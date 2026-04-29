package storage

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadRecords reads a YAML file containing a list of record maps.
// Returns an empty slice if the file doesn't exist.
func LoadRecords(path string) ([]map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []map[string]any{}, nil
		}
		return nil, fmt.Errorf("reading records file %s: %w", path, err)
	}

	if len(data) == 0 {
		return []map[string]any{}, nil
	}

	var records []map[string]any
	if err := yaml.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("parsing records YAML %s: %w", path, err)
	}

	if records == nil {
		records = []map[string]any{}
	}

	return records, nil
}
