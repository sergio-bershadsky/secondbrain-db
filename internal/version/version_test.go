package version

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeBuildInfo returns a debug.ReadBuildInfo replacement that yields the
// given BuildInfo (or ok=false when bi is nil).
func fakeBuildInfo(bi *debug.BuildInfo) func() (*debug.BuildInfo, bool) {
	return func() (*debug.BuildInfo, bool) {
		if bi == nil {
			return nil, false
		}
		return bi, true
	}
}

func TestCompute_LdflagsInjected_Wins(t *testing.T) {
	// When goreleaser injects a version string, it must win regardless of
	// what runtime/debug.BuildInfo says — including a stale module version
	// from an older proxy entry.
	got := compute("v1.2.3", fakeBuildInfo(&debug.BuildInfo{
		Main: debug.Module{Version: "v0.0.0-stale"},
	}))
	require.Equal(t, "v1.2.3", got)
}

func TestCompute_GoInstall_ReadsModuleVersion(t *testing.T) {
	// `go install pkg@v1.3.0` populates Main.Version; ldflags is empty.
	got := compute("", fakeBuildInfo(&debug.BuildInfo{
		Main: debug.Module{Version: "v1.3.0"},
	}))
	require.Equal(t, "v1.3.0", got)
}

func TestCompute_LocalBuild_FallsBackToShortSHA(t *testing.T) {
	// `go build .` from a tagged tree gives Main.Version = "(devel)" plus
	// a vcs.revision setting. We surface the short SHA.
	got := compute("", fakeBuildInfo(&debug.BuildInfo{
		Main: debug.Module{Version: "(devel)"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abc1234deadbeef"},
		},
	}))
	require.Equal(t, "devel-abc1234", got)
}

func TestCompute_NoBuildInfo_FallsBackToDev(t *testing.T) {
	got := compute("", fakeBuildInfo(nil))
	require.Equal(t, "dev", got)
}

func TestCompute_NoVCSInfo_FallsBackToDev(t *testing.T) {
	// `go build .` outside a VCS tree leaves Main.Version = "(devel)" and
	// no vcs.revision setting.
	got := compute("", fakeBuildInfo(&debug.BuildInfo{
		Main: debug.Module{Version: "(devel)"},
	}))
	require.Equal(t, "dev", got)
}

func TestCompute_EmptyMainVersion_TreatedAsDev(t *testing.T) {
	got := compute("", fakeBuildInfo(&debug.BuildInfo{
		Main: debug.Module{Version: ""},
	}))
	require.Equal(t, "dev", got)
}

func TestShortRev(t *testing.T) {
	require.Equal(t, "abc1234", shortRev("abc1234deadbeef"))
	require.Equal(t, "abc", shortRev("abc"))
	require.Equal(t, "", shortRev(""))
	require.Equal(t, "abc1234", shortRev("  abc1234deadbeef\n"))
}

func TestVCSRevision(t *testing.T) {
	bi := &debug.BuildInfo{Settings: []debug.BuildSetting{
		{Key: "GOOS", Value: "linux"},
		{Key: "vcs.revision", Value: "deadbeef"},
		{Key: "vcs.modified", Value: "false"},
	}}
	require.Equal(t, "deadbeef", vcsRevision(bi))

	require.Equal(t, "", vcsRevision(&debug.BuildInfo{}))
}
