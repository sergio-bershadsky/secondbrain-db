// Package version exposes the build's version string.
//
// The string is resolved in this order, from most specific to least:
//
//  1. The ldflags-injected `injected` variable. Goreleaser sets this in the
//     official release archives; when set, it always wins.
//  2. runtime/debug.BuildInfo.Main.Version — populated when the binary was
//     produced via `go install pkg@vX.Y.Z`. This is the path most users
//     take and it carries the exact module version.
//  3. A composed `devel-<short-sha>` for local `go build .` invocations
//     where VCS info is embedded but no module version exists.
//  4. The literal "dev" as a final fallback.
package version

import (
	"runtime/debug"
	"strings"
)

// injected is the build-time version, set via:
//
//	-ldflags="-X github.com/sergio-bershadsky/secondbrain-db/internal/version.injected=v1.2.3"
//
// Goreleaser sets this for the official release archives. Any non-empty
// value short-circuits the resolution chain.
var injected = ""

// Version is the resolved version string for this build, computed once at
// package init.
var Version = compute(injected, debug.ReadBuildInfo)

// compute is the testable inner: caller passes the ldflags value plus a
// readBuildInfo function so unit tests can drive each branch.
func compute(ldflagsInjected string, readBuildInfo func() (*debug.BuildInfo, bool)) string {
	if ldflagsInjected != "" {
		return ldflagsInjected
	}
	bi, ok := readBuildInfo()
	if !ok || bi == nil {
		return "dev"
	}
	if v := bi.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	if rev := vcsRevision(bi); rev != "" {
		return "devel-" + shortRev(rev)
	}
	return "dev"
}

// vcsRevision pulls the git commit hash from BuildInfo.Settings (set by Go
// 1.18+ when building inside a VCS-tracked tree). Returns empty if absent.
func vcsRevision(bi *debug.BuildInfo) string {
	for _, s := range bi.Settings {
		if s.Key == "vcs.revision" {
			return s.Value
		}
	}
	return ""
}

// shortRev returns the first 7 characters of a git revision string —
// the conventional short-sha length.
func shortRev(rev string) string {
	const short = 7
	rev = strings.TrimSpace(rev)
	if len(rev) > short {
		return rev[:short]
	}
	return rev
}
