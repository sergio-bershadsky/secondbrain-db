package acl

import (
	"crypto/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBundleRoundTripVerify(t *testing.T) {
	alice := newTestEntity(t, "alice")
	tok, _ := NewToken()

	bundle, err := ExportBundle(BundleExportOpts{
		Nickname: "alice", Token: tok, Entity: alice, Random: rand.Reader,
	})
	require.NoError(t, err)

	imported, err := ImportBundle(bundle)
	require.NoError(t, err)
	require.Equal(t, "alice", imported.Nickname)
	require.Equal(t, tok, imported.Token)
}

func TestBundleSignatureMustVerify(t *testing.T) {
	alice := newTestEntity(t, "alice")
	tok, _ := NewToken()
	bundle, err := ExportBundle(BundleExportOpts{Nickname: "alice", Token: tok, Entity: alice, Random: rand.Reader})
	require.NoError(t, err)

	// Tamper with the nickname after signing.
	tampered := []byte(strings.Replace(string(bundle), "nickname: alice", "nickname: malice", 1))
	require.NotEqual(t, string(bundle), string(tampered))
	_, err = ImportBundle(tampered)
	require.ErrorIs(t, err, ErrBundleSignature)
}
