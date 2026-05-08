package acl

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewTokenFormat(t *testing.T) {
	tok, err := NewToken()
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(string(tok), "tok_"))
	require.Len(t, string(tok), 4+64)
}

func TestNewTokenIsRandom(t *testing.T) {
	a, _ := NewToken()
	b, _ := NewToken()
	require.NotEqual(t, a, b)
}

func TestParseTokenRoundTrip(t *testing.T) {
	tok, _ := NewToken()
	parsed, err := ParseToken(string(tok))
	require.NoError(t, err)
	require.Equal(t, tok, parsed)
}

func TestParseTokenRejectsBadInput(t *testing.T) {
	for _, bad := range []string{"", "abc", "tok_", "tok_zzz", "tok_" + strings.Repeat("g", 64)} {
		_, err := ParseToken(bad)
		require.Error(t, err, "input: %q", bad)
	}
}
