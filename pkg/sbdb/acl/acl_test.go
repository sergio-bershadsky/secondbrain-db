package acl

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestACLRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	a := filepath.Join(tmp, "acl.yaml")

	tok1, _ := NewToken()
	tok2, _ := NewToken()
	want := ACLFile{Version: 1, Readers: []Token{tok1, tok2}}

	require.NoError(t, WriteACL(a, want))
	got, err := ReadACL(a)
	require.NoError(t, err)
	require.Equal(t, want.Version, got.Version)
	require.ElementsMatch(t, want.Readers, got.Readers)
}

func TestReadACLMissingReturnsNotFound(t *testing.T) {
	_, err := ReadACL(filepath.Join(t.TempDir(), "missing.yaml"))
	require.ErrorIs(t, err, ErrACLNotFound)
}
