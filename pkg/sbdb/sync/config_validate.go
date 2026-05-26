package sync

import (
	"fmt"
	"strings"
)

// PayloadSource is a known symbol that may appear in PayloadField.From.
// Validation rejects any From: not in this set so typos fail loudly.
var knownPayloadSources = map[string]bool{
	"rendered_markdown": true,
	"body":              true,
}

func isKnownPayloadSource(from string) bool {
	if knownPayloadSources[from] {
		return true
	}
	// frontmatter.<anything> is allowed; the actual path is checked at
	// payload-resolution time against the live doc.
	return strings.HasPrefix(from, "frontmatter.")
}

// ValidateConfig checks one IntegrationConfig against the set of known
// entity names from schemas. Returns a non-nil error describing all
// problems found (does NOT stop at the first).
func ValidateConfig(c IntegrationConfig, knownEntities map[string]bool) error {
	var problems []string
	if c.Integration == "" {
		problems = append(problems, "missing 'integration' field")
	}
	for entity, binding := range c.AppliesTo {
		if !knownEntities[entity] {
			problems = append(problems,
				fmt.Sprintf("applies_to.%s: unknown entity (no schema with x-entity=%q)", entity, entity))
		}
		if binding.TargetRef == "" {
			problems = append(problems,
				fmt.Sprintf("applies_to.%s: 'target_ref' is required", entity))
		}
		if len(binding.Payload) == 0 {
			problems = append(problems,
				fmt.Sprintf("applies_to.%s: 'payload' must declare at least one field", entity))
		}
		for fieldName, pf := range binding.Payload {
			hasFrom := pf.From != ""
			hasConst := pf.Const != nil
			if !hasFrom && !hasConst {
				problems = append(problems, fmt.Sprintf(
					"applies_to.%s.payload.%s: must have 'from' or 'const'", entity, fieldName))
				continue
			}
			if hasFrom && hasConst {
				problems = append(problems, fmt.Sprintf(
					"applies_to.%s.payload.%s: 'from' and 'const' are mutually exclusive", entity, fieldName))
				continue
			}
			if hasFrom && !isKnownPayloadSource(pf.From) {
				problems = append(problems, fmt.Sprintf(
					"applies_to.%s.payload.%s: unknown payload source %q (allowed: 'rendered_markdown', 'body', 'frontmatter.*')",
					entity, fieldName, pf.From))
			}
		}
	}
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("integration config %s invalid:\n  - %s",
		c.SourcePath, strings.Join(problems, "\n  - "))
}
