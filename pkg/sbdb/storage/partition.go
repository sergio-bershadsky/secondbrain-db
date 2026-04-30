package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

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
