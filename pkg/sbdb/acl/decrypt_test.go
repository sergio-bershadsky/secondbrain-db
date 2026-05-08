package acl

import (
	"crypto/rand"
	"testing"

	openpgp "github.com/ProtonMail/go-crypto/openpgp/v2"
	"github.com/stretchr/testify/require"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	alice := newTestEntity(t, "alice")
	bob := newTestEntity(t, "bob")

	plaintext := []byte("# secret\n\nhello\n")

	tokA, _ := NewToken()
	tokB, _ := NewToken()
	cipher, err := Encrypt(plaintext, EncryptOpts{
		Recipients: []EncRecipient{{Token: tokA, Entity: alice}, {Token: tokB, Entity: bob}},
		Random:     rand.Reader,
	})
	require.NoError(t, err)

	got, err := Decrypt(cipher, DecryptOpts{Keyring: openpgp.EntityList{bob}})
	require.NoError(t, err)
	require.Equal(t, plaintext, got)
}

func TestDecryptNoMatchingKey(t *testing.T) {
	alice := newTestEntity(t, "alice")
	stranger := newTestEntity(t, "stranger")

	tokA, _ := NewToken()
	cipher, err := Encrypt([]byte("x"), EncryptOpts{
		Recipients: []EncRecipient{{Token: tokA, Entity: alice}},
		Random:     rand.Reader,
	})
	require.NoError(t, err)

	_, err = Decrypt(cipher, DecryptOpts{Keyring: openpgp.EntityList{stranger}})
	require.ErrorIs(t, err, ErrNoMatchingKey)
}
