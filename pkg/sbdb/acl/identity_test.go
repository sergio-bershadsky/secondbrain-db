package acl

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIdentityRoundTrip(t *testing.T) {
	tmp := t.TempDir()

	want := Identity{Nickname: "alice", Fingerprint: "ABC123", PrivateKeyPath: "/tmp/key.asc", UseGPGAgent: false}
	require.NoError(t, SaveIdentity(tmp, want))
	got, err := LoadIdentity(tmp)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestLoadIdentityMissingReturnsErr(t *testing.T) {
	_, err := LoadIdentity(t.TempDir())
	require.ErrorIs(t, err, ErrIdentityMissing)
}
