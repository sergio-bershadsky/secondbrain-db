package schema

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Dialect identifies the on-disk schema format.
type Dialect int

const (
	DialectUnknown Dialect = iota
	DialectLegacy
	DialectNew
)

func (d Dialect) String() string {
	switch d {
	case DialectLegacy:
		return "legacy"
	case DialectNew:
		return "json-schema"
	default:
		return "unknown"
	}
}

// DetectDialect inspects the top-level keys of a YAML/JSON schema document
// and reports which dialect it is.
func DetectDialect(data []byte) (Dialect, error) {
	var top map[string]any
	if err := yaml.Unmarshal(data, &top); err != nil {
		return DialectUnknown, fmt.Errorf("schema: parse top-level: %w", err)
	}

	hasNewSignal := false
	hasLegacySignal := false

	for k := range top {
		switch {
		case k == "$schema" || k == "$id" || k == "properties":
			hasNewSignal = true
		case strings.HasPrefix(k, "x-"):
			hasNewSignal = true
		case k == "entity" || k == "fields":
			hasLegacySignal = true
		}
	}

	switch {
	case hasNewSignal && hasLegacySignal:
		return DialectUnknown, fmt.Errorf("schema: ambiguous dialect (both legacy 'entity'/'fields' and new '$schema'/'x-*' keys present)")
	case hasNewSignal:
		return DialectNew, nil
	case hasLegacySignal:
		return DialectLegacy, nil
	default:
		return DialectUnknown, fmt.Errorf("schema: cannot detect dialect (no recognisable top-level keys)")
	}
}
