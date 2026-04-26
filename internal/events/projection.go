package events

import (
	"fmt"
	"path/filepath"
	"time"
)

// RebuildRegistry replays the entire event log under root and returns a
// fresh registry projection. Used by `sbdb doctor check` to verify that
// the on-disk registry.yaml is byte-equal to a deterministic rebuild.
//
// Replay order:
//  1. Seed with built-ins.
//  2. Walk archives in lex order (oldest → newest).
//  3. Walk live daily files in lex order (oldest → newest).
//  4. Apply meta.event_type_* events in order encountered.
func RebuildRegistry(root string) (*Registry, error) {
	r := NewBuiltinRegistry()

	// Archives first.
	archDir := filepath.Join(root, ArchiveDir)
	archives, err := listArchives(archDir)
	if err != nil {
		return nil, fmt.Errorf("listing archives: %w", err)
	}
	for _, archPath := range archives {
		events, err := ReadGzipFile(archPath)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", archPath, err)
		}
		for _, e := range events {
			if err := applyMetaEvent(r, e); err != nil {
				return nil, fmt.Errorf("apply %s in %s: %w", e.Type, archPath, err)
			}
		}
	}

	// Then live files.
	err = IterateLive(root, func(filename string, e *Event) error {
		return applyMetaEvent(r, e)
	})
	if err != nil {
		return nil, err
	}

	r.GeneratedAt = time.Now().UTC()
	return r, nil
}

// applyMetaEvent updates the registry for relevant meta.* events. Non-meta
// events are ignored.
func applyMetaEvent(r *Registry, e *Event) error {
	switch e.Type {
	case "meta.event_type_registered":
		owner, _ := e.Data["owner"].(string)
		if owner == "" {
			owner = "builtin"
		}
		registeredAt := e.TS
		// Skip if already registered (built-in seed may overlap).
		if r.IsKnownType(e.ID) {
			return nil
		}
		return r.RegisterType(e.ID, owner, registeredAt)
	case "meta.event_type_deprecated":
		if !r.IsKnownType(e.ID) {
			// Already deprecated or never existed; idempotent skip.
			return nil
		}
		return r.MarkDeprecated(e.ID)
	case "meta.event_type_evolved":
		var addedOpt []string
		if v, ok := e.Data["added_optional"].([]interface{}); ok {
			for _, x := range v {
				if s, ok := x.(string); ok {
					addedOpt = append(addedOpt, s)
				}
			}
		}
		addedEnums := map[string][]string{}
		if m, ok := e.Data["added_enum_values"].(map[string]interface{}); ok {
			for field, vals := range m {
				if arr, ok := vals.([]interface{}); ok {
					for _, x := range arr {
						if s, ok := x.(string); ok {
							addedEnums[field] = append(addedEnums[field], s)
						}
					}
				}
			}
		}
		if !r.IsKnownType(e.ID) {
			// Cannot evolve unregistered type; spec says emit only after register.
			return nil
		}
		return r.EvolveType(e.ID, addedOpt, addedEnums)
	}
	return nil
}

func listArchives(dir string) ([]string, error) {
	entries, err := readDirOrEmpty(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, name := range entries {
		if len(name) >= 11 && name[len(name)-len(".jsonl.gz"):] == ".jsonl.gz" {
			out = append(out, filepath.Join(dir, name))
		}
	}
	// Lex sort (YYYY-MM.jsonl.gz sorts chronologically).
	sortStrings(out)
	return out, nil
}

func readDirOrEmpty(dir string) ([]string, error) {
	entries, err := readDirNames(dir)
	if err != nil {
		if isNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return entries, nil
}
