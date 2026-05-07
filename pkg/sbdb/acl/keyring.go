package acl

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Recipient is one entry in the local keyring.
type Recipient struct {
	Nickname      string `yaml:"nickname"`
	Token         Token  `yaml:"token"`
	Fingerprint   string `yaml:"fingerprint"`
	Name          string `yaml:"name"`
	Email         string `yaml:"email"`
	PubkeyFile    string `yaml:"pubkey_file"`
	ImportedAt    string `yaml:"imported_at,omitempty"`
	PubkeyArmored []byte `yaml:"-"`
}

// Keyring holds the loaded recipients.yaml entries with pubkeys read in.
type Keyring struct {
	Version    int         `yaml:"version"`
	Recipients []Recipient `yaml:"recipients"`
	root       string
}

// LoadKeyring loads the per-clone keyring from repoRoot. If the file is
// missing the returned Keyring is empty (still usable for ByNickname etc).
func LoadKeyring(repoRoot string) (*Keyring, error) {
	b, err := os.ReadFile(LocalRecipientsFile(repoRoot))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &Keyring{Version: 1, root: repoRoot}, nil
		}
		return nil, fmt.Errorf("sbdb/acl: read keyring: %w", err)
	}
	var kr Keyring
	if err := yaml.Unmarshal(b, &kr); err != nil {
		return nil, fmt.Errorf("sbdb/acl: parse keyring: %w", err)
	}
	if kr.Version == 0 {
		kr.Version = 1
	}
	kr.root = repoRoot
	for i := range kr.Recipients {
		path := filepath.Join(LocalKeysDir(repoRoot), kr.Recipients[i].PubkeyFile)
		armored, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("sbdb/acl: read pubkey %s: %w", path, err)
		}
		kr.Recipients[i].PubkeyArmored = armored
	}
	return &kr, nil
}

// Root returns the repo root the keyring was loaded from.
func (k *Keyring) Root() string { return k.root }

// SetRoot configures the directory the keyring saves to.
func (k *Keyring) SetRoot(root string) { k.root = root }

// ByNickname returns the recipient with this nickname, if any.
func (k *Keyring) ByNickname(nick string) (Recipient, bool) {
	for _, r := range k.Recipients {
		if r.Nickname == nick {
			return r, true
		}
	}
	return Recipient{}, false
}

// ByToken returns the recipient with this token, if any.
func (k *Keyring) ByToken(t Token) (Recipient, bool) {
	for _, r := range k.Recipients {
		if r.Token == t {
			return r, true
		}
	}
	return Recipient{}, false
}

// ResolveTokens maps nicknames to tokens, returning an error listing any
// unresolved names.
func (k *Keyring) ResolveTokens(nicks []string) ([]Token, error) {
	out := make([]Token, 0, len(nicks))
	var missing []string
	for _, n := range nicks {
		r, ok := k.ByNickname(n)
		if !ok {
			missing = append(missing, n)
			continue
		}
		out = append(out, r.Token)
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("sbdb/acl: unknown nickname(s): %v", missing)
	}
	return out, nil
}

// Save writes the keyring back to disk.
func (k *Keyring) Save() error {
	if err := os.MkdirAll(LocalKeysDir(k.root), 0o755); err != nil {
		return err
	}
	out := struct {
		Version    int         `yaml:"version"`
		Recipients []Recipient `yaml:"recipients"`
	}{Version: k.Version, Recipients: k.Recipients}
	if out.Version == 0 {
		out.Version = 1
	}
	b, err := yaml.Marshal(out)
	if err != nil {
		return err
	}
	return os.WriteFile(LocalRecipientsFile(k.root), b, 0o644)
}
