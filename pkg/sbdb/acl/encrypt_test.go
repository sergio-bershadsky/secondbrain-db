package acl

import (
	"bytes"
	"crypto/rand"
	"testing"

	openpgp "github.com/ProtonMail/go-crypto/openpgp/v2"
	"github.com/stretchr/testify/require"
)

func newTestEntity(t *testing.T, name string) *openpgp.Entity {
	t.Helper()
	e, err := openpgp.NewEntity(name, "", name+"@test", nil)
	require.NoError(t, err)
	return e
}

func TestEncryptProducesEnvelope(t *testing.T) {
	alice := newTestEntity(t, "alice")
	bob := newTestEntity(t, "bob")

	plaintext := []byte("# secret\n\nhello\n")
	tokA, _ := NewToken()
	tokB, _ := NewToken()

	out, err := Encrypt(plaintext, EncryptOpts{
		Recipients: []EncRecipient{
			{Token: tokA, Entity: alice},
			{Token: tokB, Entity: bob},
		},
		Random: rand.Reader,
	})
	require.NoError(t, err)
	env, err := ParseEnvelope(bytes.NewReader(out))
	require.NoError(t, err)
	require.Equal(t, 1, env.Version)
	require.Equal(t, 2, env.RecipientCount)
	require.Contains(t, env.PGPArmored, "BEGIN PGP MESSAGE")
}

func TestEncryptRequiresRecipients(t *testing.T) {
	_, err := Encrypt([]byte("x"), EncryptOpts{Random: rand.Reader})
	require.Error(t, err)
}
