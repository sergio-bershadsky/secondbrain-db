package events

import (
	"os"
	"path/filepath"
	"sort"
	"time"

	"gopkg.in/yaml.v3"
)

// RegistryFileName is the canonical projection path under the project root.
const RegistryFileName = "internal/events/registry.yaml"

// Registry is a queryable view of the closed event-type catalog. Since
// authors cannot register new types (see builtin.go), the registry is
// purely a derived projection of BuiltinTypes — it contains no state
// that isn't already in code. The on-disk YAML exists only as a
// machine-readable convenience for tooling.
type Registry struct {
	Version     int                       `yaml:"version"`
	GeneratedAt time.Time                 `yaml:"generated_at"`
	Buckets     map[string]*RegistryEntry `yaml:"buckets"`
}

// RegistryEntry holds the registered state of one bucket.
type RegistryEntry struct {
	Owner string   `yaml:"owner"` // always "builtin"
	Types []string `yaml:"types"`
}

// NewBuiltinRegistry returns a registry seeded with the built-in catalog.
// Since the catalog is closed, this is the only way to construct a registry.
func NewBuiltinRegistry() *Registry {
	r := &Registry{
		Version:     1,
		GeneratedAt: time.Now().UTC(),
		Buckets:     make(map[string]*RegistryEntry),
	}
	for _, t := range BuiltinTypes {
		bucket := Bucket(t)
		verb := Verb(t)
		entry, ok := r.Buckets[bucket]
		if !ok {
			entry = &RegistryEntry{Owner: "builtin", Types: []string{}}
			r.Buckets[bucket] = entry
		}
		entry.Types = appendUniq(entry.Types, verb)
	}
	for _, e := range r.Buckets {
		sort.Strings(e.Types)
	}
	return r
}

// IsKnownType reports whether typeName is in the closed catalog.
func (r *Registry) IsKnownType(typeName string) bool {
	if !ValidTypeName(typeName) {
		return false
	}
	bucket := Bucket(typeName)
	verb := Verb(typeName)
	entry, ok := r.Buckets[bucket]
	if !ok {
		return false
	}
	for _, t := range entry.Types {
		if t == verb {
			return true
		}
	}
	return false
}

// Save writes the registry to path atomically (temp file + rename).
func (r *Registry) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	r.GeneratedAt = r.GeneratedAt.UTC()
	data, err := yaml.Marshal(r)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// LoadRegistry reads a registry from disk. Callers that just need to query
// type membership should prefer NewBuiltinRegistry — the on-disk file is
// only useful for external tooling that wants a stable YAML view.
func LoadRegistry(path string) (*Registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r Registry
	if err := yaml.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	if r.Buckets == nil {
		r.Buckets = make(map[string]*RegistryEntry)
	}
	return &r, nil
}

func appendUniq(slice []string, v string) []string {
	for _, s := range slice {
		if s == v {
			return slice
		}
	}
	return append(slice, v)
}
