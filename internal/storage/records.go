package storage

import (
	"fmt"
	"os"
	"path/filepath"

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

// SaveRecords writes a list of record maps to a YAML file atomically.
func SaveRecords(path string, records []map[string]any) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	data, err := yaml.Marshal(records)
	if err != nil {
		return fmt.Errorf("marshaling records: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".sbdb-records-*.yaml.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming temp file: %w", err)
	}

	return nil
}

// UpsertRecord inserts or updates a record in the list, matched by idField.
// Returns the updated list.
func UpsertRecord(records []map[string]any, record map[string]any, idField string) []map[string]any {
	id, ok := record[idField]
	if !ok {
		return append(records, record)
	}

	for i, existing := range records {
		if existing[idField] == id {
			records[i] = record
			return records
		}
	}

	return append(records, record)
}

// RemoveRecord removes a record from the list by its id field value.
// Returns the updated list and whether a record was removed.
func RemoveRecord(records []map[string]any, idField string, idValue any) ([]map[string]any, bool) {
	for i, existing := range records {
		if fmt.Sprintf("%v", existing[idField]) == fmt.Sprintf("%v", idValue) {
			return append(records[:i], records[i+1:]...), true
		}
	}
	return records, false
}
