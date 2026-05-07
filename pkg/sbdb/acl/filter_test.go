package acl

import (
	"bytes"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp/armor"
	openpgp "github.com/ProtonMail/go-crypto/openpgp/v2"
	"github.com/stretchr/testify/require"
)

func pubArmor(t *testing.T, e *openpgp.Entity) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := armor.Encode(&buf, "PGP PUBLIC KEY BLOCK", nil)
	require.NoError(t, err)
	require.NoError(t, e.Serialize(w))
	require.NoError(t, w.Close())
	return buf.Bytes()
}

func TestFilterPassThroughWhenNoACL(t *testing.T) {
	tmp := t.TempDir()
	docPath := filepath.Join(tmp, "docs", "x.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(docPath), 0o755))
	cleartext := []byte("# hello\n")
	var out bytes.Buffer
	require.NoError(t, FilterClean(bytes.NewReader(cleartext), &out, docPath, FilterContext{RepoRoot: tmp}))
	require.Equal(t, cleartext, out.Bytes())
}

func TestFilterCleanThenSmudgeRoundTrip(t *testing.T) {
	tmp := t.TempDir()

	alice := newTestEntity(t, "alice")
	tokA, _ := NewToken()

	require.NoError(t, os.MkdirAll(LocalPubkeysDir(tmp), 0o755))
	pubBytes := pubArmor(t, alice)
	require.NoError(t, os.WriteFile(filepath.Join(LocalPubkeysDir(tmp), "alice.asc"), pubBytes, 0o644))

	kr := &Keyring{Version: 1, Recipients: []Recipient{{
		Nickname: "alice", Token: tokA, Fingerprint: "ABC",
		Name: "Alice", Email: "a@x", PubkeyFile: "pubkeys/alice.asc",
		PubkeyArmored: pubBytes,
	}}}
	kr.SetRoot(tmp)
	require.NoError(t, kr.Save())

	docRel := filepath.Join("docs", "secret.md")
	docPath := filepath.Join(tmp, docRel)
	aclPath := ACLFileFor(tmp, docRel)
	require.NoError(t, WriteACL(aclPath, ACLFile{Version: 1, Readers: []Token{tokA}}))

	cleartext := []byte("# top secret\nbody\n")
	var ciphertext bytes.Buffer
	ctx := FilterContext{RepoRoot: tmp, Random: rand.Reader}
	require.NoError(t, FilterClean(bytes.NewReader(cleartext), &ciphertext, docPath, ctx))
	require.True(t, IsEnvelopePrefix(ciphertext.Bytes()))

	var recovered bytes.Buffer
	smudgeCtx := FilterContext{RepoRoot: tmp, PrivateKeys: openpgp.EntityList{alice}}
	require.NoError(t, FilterSmudge(bytes.NewReader(ciphertext.Bytes()), &recovered, docPath, smudgeCtx))
	require.Equal(t, cleartext, recovered.Bytes())
}

func TestFilterSmudgeNonReaderPassesThrough(t *testing.T) {
	tmp := t.TempDir()
	alice := newTestEntity(t, "alice")
	stranger := newTestEntity(t, "stranger")
	tokA, _ := NewToken()
	cipher, err := Encrypt([]byte("x"), EncryptOpts{
		Recipients: []EncRecipient{{Token: tokA, Entity: alice}},
		Random:     rand.Reader,
	})
	require.NoError(t, err)
	var out bytes.Buffer
	docPath := filepath.Join(tmp, "docs", "x.md")
	ctx := FilterContext{RepoRoot: tmp, PrivateKeys: openpgp.EntityList{stranger}}
	require.NoError(t, FilterSmudge(bytes.NewReader(cipher), &out, docPath, ctx))
	require.True(t, IsEnvelopePrefix(out.Bytes()))
}

func TestFilterCleanPassesThroughExistingEnvelope(t *testing.T) {
	tmp := t.TempDir()
	docPath := filepath.Join(tmp, "docs", "x.md")
	envelope := []byte("-----BEGIN SBDB-ACL-ENVELOPE-----\nVersion: 1\nRecipients: 1\n-----END SBDB-ACL-ENVELOPE-----\n-----BEGIN PGP MESSAGE-----\nABC\n-----END PGP MESSAGE-----\n")
	var out bytes.Buffer
	require.NoError(t, FilterClean(bytes.NewReader(envelope), &out, docPath, FilterContext{RepoRoot: tmp}))
	require.Equal(t, envelope, out.Bytes())
}
