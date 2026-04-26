package events

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"gopkg.in/yaml.v3"
)

// RegistryFileName is the canonical projection path under the project root.
const RegistryFileName = "internal/events/registry.yaml"

// Registry is the projection of all meta.event_type_* events into a
// queryable form. It is regenerable from the event log; the on-disk file
// is byte-equal to a fresh rebuild (verified by doctor).
type Registry struct {
	Version          int                       `yaml:"version"`
	GeneratedAt      time.Time                 `yaml:"generated_at"`
	GeneratedFromSeq map[int]map[int]int       `yaml:"generated_from_seq,omitempty"`
	Buckets          map[string]*RegistryEntry `yaml:"buckets"`
}

// RegistryEntry holds the registered state of one bucket.
type RegistryEntry struct {
	Owner          string              `yaml:"owner"` // "builtin" or schema path
	RegisteredAt   *time.Time          `yaml:"registered_at,omitempty"`
	Types          []string            `yaml:"types"`
	Deprecated     []string            `yaml:"deprecated,omitempty"`
	Enums          map[string][]string `yaml:"enums,omitempty"`
	SchemaVersions map[string]int      `yaml:"schema_versions,omitempty"`
}

// NewBuiltinRegistry returns a registry seeded with the built-in catalog.
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
	// Sort verbs within each bucket for stable output.
	for _, e := range r.Buckets {
		sort.Strings(e.Types)
	}
	return r
}

// IsKnownType reports whether typeName is registered (and not deprecated).
// Deprecated types are still valid for emission per spec §6.6.
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
	for _, t := range entry.Deprecated {
		if t == verb {
			return true // deprecated still valid
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

// LoadRegistry reads a registry from disk.
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

// RegisterType records a new type registration in the registry. Returns
// an error if the bucket is reserved by a different owner, or if the type
// is already registered.
func (r *Registry) RegisterType(typeName, owner string, registeredAt time.Time) error {
	if !ValidTypeName(typeName) {
		return fmt.Errorf("%w: %q", ErrInvalidType, typeName)
	}
	bucket := Bucket(typeName)
	verb := Verb(typeName)

	// Built-in types may only be claimed by builtin owner.
	builtin := IsBuiltinType(typeName)
	if builtin && owner != "builtin" {
		return fmt.Errorf("type %q is built-in; cannot be registered by %q", typeName, owner)
	}

	// Author types must use x.* prefix.
	if !builtin && !IsAuthorType(typeName) && owner != "builtin" {
		return fmt.Errorf("non-builtin type %q must use x.* prefix", typeName)
	}

	// Author types cannot claim reserved buckets.
	if IsAuthorType(typeName) {
		// strip leading "x." for reserved-bucket check (the rest is the actual claim)
		// e.g. x.recipe → check that "recipe" is not reserved
		stripped := bucket[2:]
		if IsReservedBucket(stripped) {
			return fmt.Errorf("bucket %q is reserved", stripped)
		}
	}

	entry, ok := r.Buckets[bucket]
	if !ok {
		entry = &RegistryEntry{
			Owner:          owner,
			SchemaVersions: map[string]int{},
		}
		if !registeredAt.IsZero() && owner != "builtin" {
			t := registeredAt.UTC()
			entry.RegisteredAt = &t
		}
		r.Buckets[bucket] = entry
	} else if entry.Owner != owner {
		return fmt.Errorf("bucket %q already owned by %q (cannot be re-claimed by %q)",
			bucket, entry.Owner, owner)
	}

	for _, t := range entry.Types {
		if t == verb {
			return fmt.Errorf("type %q already registered", typeName)
		}
	}
	entry.Types = appendUniq(entry.Types, verb)
	sort.Strings(entry.Types)
	if entry.SchemaVersions == nil {
		entry.SchemaVersions = map[string]int{}
	}
	if _, ok := entry.SchemaVersions[verb]; !ok {
		entry.SchemaVersions[verb] = 1
	}
	return nil
}

// MarkDeprecated moves a type from active to deprecated. The type stays
// valid for emission and consumption per spec §6.6.
func (r *Registry) MarkDeprecated(typeName string) error {
	bucket := Bucket(typeName)
	verb := Verb(typeName)
	entry, ok := r.Buckets[bucket]
	if !ok {
		return fmt.Errorf("bucket %q not registered", bucket)
	}
	idx := -1
	for i, t := range entry.Types {
		if t == verb {
			idx = i
			break
		}
	}
	if idx < 0 {
		// already deprecated or never existed
		for _, t := range entry.Deprecated {
			if t == verb {
				return nil // idempotent
			}
		}
		return fmt.Errorf("type %q not registered in bucket %q", verb, bucket)
	}
	entry.Types = append(entry.Types[:idx], entry.Types[idx+1:]...)
	entry.Deprecated = appendUniq(entry.Deprecated, verb)
	sort.Strings(entry.Deprecated)
	return nil
}

// EvolveType bumps schema_version for a type when an additive change lands.
func (r *Registry) EvolveType(typeName string, addedOptional []string, addedEnumValues map[string][]string) error {
	bucket := Bucket(typeName)
	verb := Verb(typeName)
	entry, ok := r.Buckets[bucket]
	if !ok {
		return fmt.Errorf("bucket %q not registered", bucket)
	}
	if entry.SchemaVersions == nil {
		entry.SchemaVersions = map[string]int{}
	}
	entry.SchemaVersions[verb]++
	if entry.Enums == nil && len(addedEnumValues) > 0 {
		entry.Enums = map[string][]string{}
	}
	for field, vals := range addedEnumValues {
		key := verb + "." + field
		entry.Enums[key] = appendUniqMany(entry.Enums[key], vals)
	}
	_ = addedOptional // tracked in events; not stored verbatim in projection
	return nil
}

func appendUniq(slice []string, v string) []string {
	for _, s := range slice {
		if s == v {
			return slice
		}
	}
	return append(slice, v)
}

func appendUniqMany(slice []string, vals []string) []string {
	for _, v := range vals {
		slice = appendUniq(slice, v)
	}
	return slice
}
