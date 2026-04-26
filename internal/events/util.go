package events

import (
	"os"
	"sort"
)

func readDirNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

func isNotExist(err error) bool {
	return os.IsNotExist(err)
}

func sortStrings(s []string) { sort.Strings(s) }
