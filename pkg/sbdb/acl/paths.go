package acl

import (
	"path/filepath"
	"strings"
)

// LocalKeysDir returns the per-clone gitignored keyring directory.
func LocalKeysDir(repoRoot string) string {
	return filepath.Join(repoRoot, ".sbdb", "local-keys")
}

// LocalRecipientsFile returns the path to the recipients mapping YAML.
func LocalRecipientsFile(repoRoot string) string {
	return filepath.Join(LocalKeysDir(repoRoot), "recipients.yaml")
}

// LocalPubkeysDir returns the directory holding ASCII-armored peer pubkeys.
func LocalPubkeysDir(repoRoot string) string {
	return filepath.Join(LocalKeysDir(repoRoot), "pubkeys")
}

// LocalIdentityFile returns the per-clone identity TOML path.
func LocalIdentityFile(repoRoot string) string {
	return filepath.Join(repoRoot, ".sbdb", "local-identity.toml")
}

// ACLDir returns the committed ACL directory.
func ACLDir(repoRoot string) string {
	return filepath.Join(repoRoot, "docs", ".sbdb", "acl")
}

// ACLFileFor returns the canonical ACL file path for a given doc path.
// docPath may be absolute or relative to repoRoot; the returned path
// strips the leading "docs/" segment and replaces the .md extension
// with .yaml under ACLDir.
func ACLFileFor(repoRoot, docPath string) string {
	rel := docPath
	// Always try to resolve docPath relative to repoRoot first. On Windows
	// a path like "\repo\docs\x\y.md" is not flagged absolute by
	// filepath.IsAbs (Windows requires a drive letter), but it still
	// shares the prefix with repoRoot. filepath.Rel handles both cases
	// uniformly; if it errors we fall back to docPath as-is (which is
	// the right behavior for paths already relative to docs/).
	if r, err := filepath.Rel(repoRoot, docPath); err == nil && !strings.HasPrefix(r, "..") {
		rel = r
	}
	rel = strings.TrimPrefix(filepath.ToSlash(rel), "docs/")
	rel = strings.TrimSuffix(rel, ".md") + ".yaml"
	return filepath.Join(ACLDir(repoRoot), filepath.FromSlash(rel))
}
