package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestShaContent_MatchesGitHashObject verifies that shaContent produces the
// same digest `git hash-object` would for the same bytes. This is the property
// that makes event `sha` fields directly usable as git object identifiers
// (e.g. `git cat-file blob <sha>`).
func TestShaContent_MatchesGitHashObject(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	cases := []struct {
		name    string
		content string
	}{
		{"empty", ""},
		{"ascii", "hello world\n"},
		{"multiline markdown", "# Title\n\nSome body text.\n"},
		{"unicode", "café — résumé naïve\n"},
		{"no trailing newline", "no newline"},
		{"binary-ish", "\x00\x01\x02\xff\xfe"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "blob.txt")
			require.NoError(t, os.WriteFile(path, []byte(c.content), 0o644))

			out, err := exec.Command("git", "hash-object", path).Output()
			require.NoError(t, err)
			gitHash := strings.TrimSpace(string(out))

			ours := shaContent([]byte(c.content))
			require.Equal(t, gitHash, ours, "must match git hash-object")

			// shaFile path equivalent.
			require.Equal(t, gitHash, shaFile(path))
		})
	}
}

// TestShaFile_MissingPath returns empty string rather than panicking, matching
// the best-effort contract documented in event_emit.go.
func TestShaFile_MissingPath(t *testing.T) {
	require.Equal(t, "", shaFile("/nonexistent/path/that/does/not/exist"))
}
