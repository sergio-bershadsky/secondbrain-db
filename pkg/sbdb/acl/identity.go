package acl

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml/v2"
)

// Identity is the per-clone "who am I, where is my private key" config.
type Identity struct {
	Nickname       string `toml:"nickname"`
	Fingerprint    string `toml:"fingerprint"`
	PrivateKeyPath string `toml:"private_key_path"`
	UseGPGAgent    bool   `toml:"use_gpg_agent"`
}

// LoadIdentity reads .sbdb/local-identity.toml.
func LoadIdentity(repoRoot string) (Identity, error) {
	b, err := os.ReadFile(LocalIdentityFile(repoRoot))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Identity{}, ErrIdentityMissing
		}
		return Identity{}, fmt.Errorf("sbdb/acl: read identity: %w", err)
	}
	var id Identity
	if err := toml.Unmarshal(b, &id); err != nil {
		return Identity{}, fmt.Errorf("sbdb/acl: parse identity: %w", err)
	}
	return id, nil
}

// SaveIdentity writes the per-clone identity TOML.
func SaveIdentity(repoRoot string, id Identity) error {
	if err := os.MkdirAll(filepath.Dir(LocalIdentityFile(repoRoot)), 0o755); err != nil {
		return err
	}
	b, err := toml.Marshal(id)
	if err != nil {
		return err
	}
	return os.WriteFile(LocalIdentityFile(repoRoot), b, 0o600)
}
