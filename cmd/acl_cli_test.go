package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// runRoot executes rootCmd with args, capturing combined out+err.
func runRoot(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetIn(strings.NewReader(""))
	rootCmd.SetArgs(args)
	err := rootCmd.Execute()
	return out.String(), err
}

func TestKeysSelfInitCreatesIdentity(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)
	_, err := runRoot(t, "keys", "self-init", "--name", "Alice", "--email", "a@x.com", "--nickname", "alice")
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(tmp, ".sbdb", "local-identity.toml"))
	require.NoError(t, err)
}

func TestKeysExportImportRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)
	_, err := runRoot(t, "keys", "self-init", "--name", "Alice", "--email", "a@x.com", "--nickname", "alice")
	require.NoError(t, err)

	bundlePath := filepath.Join(tmp, "alice.bundle.yaml")
	_, err = runRoot(t, "keys", "export", "alice", "-o", bundlePath)
	require.NoError(t, err)

	other := t.TempDir()
	t.Chdir(other)
	// Need our own identity in this clone too so LoadKeyring works for import path.
	_, err = runRoot(t, "keys", "self-init", "--name", "Bob", "--email", "b@x.com", "--nickname", "bob")
	require.NoError(t, err)
	_, err = runRoot(t, "keys", "import", bundlePath)
	require.NoError(t, err)

	out, err := runRoot(t, "keys", "list")
	require.NoError(t, err)
	require.Contains(t, out, "alice")
	require.Contains(t, out, "bob")
}

func TestACLSetAndGet(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)
	_, err := runRoot(t, "keys", "self-init", "--name", "Alice", "--email", "a@x", "--nickname", "alice")
	require.NoError(t, err)

	docPath := filepath.Join(tmp, "docs", "secret.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(docPath), 0o755))
	require.NoError(t, os.WriteFile(docPath, []byte("# secret\n"), 0o644))

	_, err = runRoot(t, "acl", "set", docPath, "--readers", "alice")
	require.NoError(t, err)

	out, err := runRoot(t, "acl", "get", docPath)
	require.NoError(t, err)
	require.Contains(t, out, "alice")
}

func TestUnlockWritesGitConfigAndAttributes(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, ".git", "config"), []byte(""), 0o644))

	_, err := runRoot(t, "unlock")
	require.NoError(t, err)

	cfg, _ := os.ReadFile(filepath.Join(tmp, ".git", "config"))
	require.Contains(t, string(cfg), "filter \"sbdb-acl\"")

	attrs, _ := os.ReadFile(filepath.Join(tmp, ".gitattributes"))
	require.Contains(t, string(attrs), "filter=sbdb-acl")

	gi, _ := os.ReadFile(filepath.Join(tmp, ".gitignore"))
	require.Contains(t, string(gi), ".sbdb/local-keys")
}
