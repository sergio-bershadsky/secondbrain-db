package acl

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSerializeEnvelopeRoundTrip(t *testing.T) {
	armored := "-----BEGIN PGP MESSAGE-----\nVersion: 1\n\nXYZ\n-----END PGP MESSAGE-----\n"
	env := Envelope{Version: 1, RecipientCount: 2, PGPArmored: armored}
	var buf bytes.Buffer
	_, err := env.WriteTo(&buf)
	require.NoError(t, err)

	parsed, err := ParseEnvelope(&buf)
	require.NoError(t, err)
	require.Equal(t, env.Version, parsed.Version)
	require.Equal(t, env.RecipientCount, parsed.RecipientCount)
	require.Equal(t, strings.TrimSpace(env.PGPArmored), strings.TrimSpace(parsed.PGPArmored))
}

func TestParseEnvelopeRejectsNonEnvelope(t *testing.T) {
	_, err := ParseEnvelope(strings.NewReader("# some markdown\n"))
	require.ErrorIs(t, err, ErrNotEnvelope)
}

func TestIsEnvelopePeek(t *testing.T) {
	require.True(t, IsEnvelopePrefix([]byte("-----BEGIN SBDB-ACL-ENVELOPE-----\n")))
	require.False(t, IsEnvelopePrefix([]byte("# hello\n")))
}
