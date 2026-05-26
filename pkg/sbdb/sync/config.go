package sync

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ConfigsDir is the directory inside the project root where integration
// configs live. Each *.yaml file is one integration.
const ConfigsDir = ".sbdb/integrations"

// LoadConfigs reads every *.yaml file in dir and returns a map keyed by
// each config's `integration` field. A missing directory returns an empty
// map without error — integration configs are optional.
func LoadConfigs(dir string) (map[string]IntegrationConfig, error) {
	out := map[string]IntegrationConfig{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return out, nil
		}
		return nil, fmt.Errorf("read integrations dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := filepath.Ext(e.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		var cfg IntegrationConfig
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		if cfg.Integration == "" {
			return nil, fmt.Errorf("%s: missing required field 'integration'", path)
		}
		if existing, dup := out[cfg.Integration]; dup {
			return nil, fmt.Errorf("duplicate integration %q in %s and %s",
				cfg.Integration, existing.SourcePath, path)
		}
		cfg.SourcePath = path
		out[cfg.Integration] = cfg
	}
	return out, nil
}
