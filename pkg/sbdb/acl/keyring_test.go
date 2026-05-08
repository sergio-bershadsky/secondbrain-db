package acl

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKeyringLoadResolveByNickname(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.MkdirAll(LocalPubkeysDir(tmp), 0o755))
	pubPath := filepath.Join(LocalPubkeysDir(tmp), "ABC123.asc")
	require.NoError(t, os.WriteFile(pubPath, []byte("-----BEGIN PGP PUBLIC KEY BLOCK-----\nfake\n-----END PGP PUBLIC KEY BLOCK-----\n"), 0o644))

	tok, _ := NewToken()
	yamlBody := "version: 1\nrecipients:\n  - nickname: alice\n    token: " + string(tok) +
		"\n    fingerprint: ABC123\n    name: Alice\n    email: a@x\n    pubkey_file: pubkeys/ABC123.asc\n"
	require.NoError(t, os.WriteFile(LocalRecipientsFile(tmp), []byte(yamlBody), 0o644))

	kr, err := LoadKeyring(tmp)
	require.NoError(t, err)

	r, ok := kr.ByNickname("alice")
	require.True(t, ok)
	require.Equal(t, tok, r.Token)
	require.Contains(t, string(r.PubkeyArmored), "BEGIN PGP PUBLIC KEY")

	r, ok = kr.ByToken(tok)
	require.True(t, ok)
	require.Equal(t, "alice", r.Nickname)
}

func TestKeyringEmptyWhenMissing(t *testing.T) {
	kr, err := LoadKeyring(t.TempDir())
	require.NoError(t, err)
	require.Empty(t, kr.Recipients)
}

func TestKeyringResolveTokensError(t *testing.T) {
	kr := &Keyring{Version: 1}
	_, err := kr.ResolveTokens([]string{"missing"})
	require.Error(t, err)
}

func TestKeyringSaveRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	tok, _ := NewToken()
	kr := &Keyring{Version: 1, Recipients: []Recipient{{
		Nickname: "x", Token: tok, Fingerprint: "F", Name: "X", Email: "x@x", PubkeyFile: "pubkeys/F.asc",
	}}}
	kr.SetRoot(tmp)
	// We need a pubkey file present for LoadKeyring later.
	require.NoError(t, os.MkdirAll(LocalPubkeysDir(tmp), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(LocalPubkeysDir(tmp), "F.asc"), []byte("-----BEGIN PGP PUBLIC KEY BLOCK-----\n-----END PGP PUBLIC KEY BLOCK-----\n"), 0o644))
	require.NoError(t, kr.Save())

	got, err := LoadKeyring(tmp)
	require.NoError(t, err)
	require.Len(t, got.Recipients, 1)
	require.Equal(t, "x", got.Recipients[0].Nickname)
}
