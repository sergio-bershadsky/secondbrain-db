package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// RecordsPathForPartition returns the YAML file path for a record based on partition mode.
// For "none": returns recordsDir/records.yaml
// For "monthly": returns recordsDir/YYYY-MM.yaml based on the date field value.
func RecordsPathForPartition(recordsDir, partition, dateField string, record map[string]any) (string, error) {
	if partition == "" || partition == "none" {
		return filepath.Join(recordsDir, "records.yaml"), nil
	}

	if partition == "monthly" {
		dateVal, ok := record[dateField]
		if !ok {
			return "", fmt.Errorf("record missing date field %q for monthly partition", dateField)
		}
		t, err := parseDate(dateVal)
		if err != nil {
			return "", fmt.Errorf("parsing date field %q: %w", dateField, err)
		}
		return filepath.Join(recordsDir, t.Format("2006-01")+".yaml"), nil
	}

	return "", fmt.Errorf("unknown partition mode: %q", partition)
}

// LoadAllPartitions reads all YAML record files from a directory and merges them.
// For "none" partition, reads just records.yaml.
// For "monthly", reads all YYYY-MM.yaml files and merges them.
func LoadAllPartitions(recordsDir, partition string) ([]map[string]any, error) {
	if partition == "" || partition == "none" {
		return LoadRecords(filepath.Join(recordsDir, "records.yaml"))
	}

	if partition == "monthly" {
		return loadMonthlyPartitions(recordsDir)
	}

	return nil, fmt.Errorf("unknown partition mode: %q", partition)
}

func loadMonthlyPartitions(dir string) ([]map[string]any, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []map[string]any{}, nil
		}
		return nil, fmt.Errorf("reading partitions directory %s: %w", dir, err)
	}

	// Collect YYYY-MM.yaml files
	var files []string
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() && strings.HasSuffix(name, ".yaml") && len(name) == 12 {
			// Validate YYYY-MM format
			prefix := strings.TrimSuffix(name, ".yaml")
			if _, err := time.Parse("2006-01", prefix); err == nil {
				files = append(files, name)
			}
		}
	}

	sort.Strings(files)

	var all []map[string]any
	for _, f := range files {
		records, err := LoadRecords(filepath.Join(dir, f))
		if err != nil {
			return nil, fmt.Errorf("loading partition %s: %w", f, err)
		}
		all = append(all, records...)
	}

	return all, nil
}

func parseDate(v any) (time.Time, error) {
	switch val := v.(type) {
	case string:
		// Try common formats
		for _, layout := range []string{
			"2006-01-02",
			"2006-01-02T15:04:05Z07:00",
			time.RFC3339,
		} {
			if t, err := time.Parse(layout, val); err == nil {
				return t, nil
			}
		}
		return time.Time{}, fmt.Errorf("cannot parse date string %q", val)
	case time.Time:
		return val, nil
	default:
		return time.Time{}, fmt.Errorf("unexpected date type %T", v)
	}
}
