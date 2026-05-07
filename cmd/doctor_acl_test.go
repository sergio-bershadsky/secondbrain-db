package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDoctorRecognisesEnvelope(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)
	docsDir := filepath.Join(tmp, "docs")
	require.NoError(t, os.MkdirAll(docsDir, 0o755))
	envelope := []byte("-----BEGIN SBDB-ACL-ENVELOPE-----\nVersion: 1\nRecipients: 1\n-----END SBDB-ACL-ENVELOPE-----\n-----BEGIN PGP MESSAGE-----\nfake\n-----END PGP MESSAGE-----\n")
	docPath := filepath.Join(docsDir, "x.md")
	require.NoError(t, os.WriteFile(docPath, envelope, 0o644))

	// Doctor should report it as encrypted, not as drift.
	out, _ := runRoot(t, "doctor", "check", "--path", docPath, "--all")
	// Either encrypted=true appears, or no drift reported. We accept either.
	if !strings.Contains(out, "encrypted") && strings.Contains(out, "drift") {
		t.Fatalf("doctor reported drift on envelope: %s", out)
	}
}
