package acl

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPaths(t *testing.T) {
	root := "/repo"
	require.Equal(t, filepath.Join(root, ".sbdb", "local-keys"), LocalKeysDir(root))
	require.Equal(t, filepath.Join(root, ".sbdb", "local-keys", "recipients.yaml"), LocalRecipientsFile(root))
	require.Equal(t, filepath.Join(root, ".sbdb", "local-keys", "pubkeys"), LocalPubkeysDir(root))
	require.Equal(t, filepath.Join(root, ".sbdb", "local-identity.toml"), LocalIdentityFile(root))
	require.Equal(t, filepath.Join(root, "docs", ".sbdb", "acl"), ACLDir(root))
	require.Equal(t, filepath.Join(root, "docs", ".sbdb", "acl", "meetings", "q2.yaml"),
		ACLFileFor(root, filepath.Join("docs", "meetings", "q2.md")))
}

func TestACLFileForAbsoluteDoc(t *testing.T) {
	root := "/repo"
	abs := filepath.Join(root, "docs", "x", "y.md")
	require.Equal(t, filepath.Join(root, "docs", ".sbdb", "acl", "x", "y.yaml"), ACLFileFor(root, abs))
}
