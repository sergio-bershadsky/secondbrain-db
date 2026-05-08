package schema

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// ParseLegacy parses a sbdb-dialect (pre-JSON-Schema) YAML document into
// the internal Schema struct.
func ParseLegacy(data []byte) (*Schema, error) {
	var s Schema
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing schema YAML: %w", err)
	}
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
