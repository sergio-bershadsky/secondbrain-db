package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/sync"
)

// enforceRequiredTargets checks every integration that declares
// applies_to.<entity>.required=true and verifies the frontmatter resolves
// the back-ref path to a non-empty value. Returns a non-nil error listing
// every missing target.
func enforceRequiredTargets(root, entity string, frontmatter map[string]interface{}) error {
	cfgs, err := sync.LoadConfigs(filepath.Join(root, sync.ConfigsDir))
	if err != nil {
		return err
	}
	var missing []string
	for _, c := range cfgs {
		binding, applies := c.AppliesTo[entity]
		if !applies || !binding.Required {
			continue
		}
		if v, ok := sync.ResolveBackref(frontmatter, binding.TargetRef); !ok || v == "" {
			missing = append(missing,
				fmt.Sprintf("integration %q: required target_ref %q is missing",
					c.Integration, binding.TargetRef))
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("sync target validation failed:\n  - %s",
		strings.Join(missing, "\n  - "))
}

// validateAllIntegrationConfigs loads every config in
// `<root>/.sbdb/integrations/` and validates each against `knownEntities`.
// Returns the first error found. Used by `doctor check`.
func validateAllIntegrationConfigs(root string, knownEntities map[string]bool) error {
	cfgs, err := sync.LoadConfigs(filepath.Join(root, sync.ConfigsDir))
	if err != nil {
		return err
	}
	for _, c := range cfgs {
		if err := sync.ValidateConfig(c, knownEntities); err != nil {
			return err
		}
	}
	return nil
}
