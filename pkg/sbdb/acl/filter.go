package acl

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	openpgp "github.com/ProtonMail/go-crypto/openpgp/v2"
)

// FilterContext is the runtime context passed to Filter* functions.
type FilterContext struct {
	RepoRoot    string
	Random      io.Reader
	PrivateKeys openpgp.EntityList
	Keyring     *Keyring
}

// FilterClean reads cleartext from in and writes either cleartext (no ACL)
// or an envelope (ACL present) to out. If in already begins with the
// envelope sentinel it is passed through unchanged (covers the
// non-reader-committing-without-modification case).
func FilterClean(in io.Reader, out io.Writer, path string, ctx FilterContext) error {
	br := bufio.NewReader(in)
	peek, _ := br.Peek(len(envelopeBegin))
	if bytes.HasPrefix(peek, []byte(envelopeBegin)) {
		_, err := io.Copy(out, br)
		return err
	}
	aclPath := ACLFileFor(ctx.RepoRoot, path)
	acl, err := ReadACL(aclPath)
	if errors.Is(err, ErrACLNotFound) {
		_, err := io.Copy(out, br)
		return err
	}
	if err != nil {
		return err
	}

	kr := ctx.Keyring
	if kr == nil {
		kr, err = LoadKeyring(ctx.RepoRoot)
		if err != nil {
			return err
		}
	}

	recs := make([]EncRecipient, 0, len(acl.Readers))
	for _, tok := range acl.Readers {
		r, ok := kr.ByToken(tok)
		if !ok {
			return fmt.Errorf("%w: %s in %s", ErrUnknownRecipient, tok, aclPath)
		}
		ents, err := openpgp.ReadArmoredKeyRing(strings.NewReader(string(r.PubkeyArmored)))
		if err != nil {
			return fmt.Errorf("sbdb/acl: parse pubkey for %s: %w", r.Nickname, err)
		}
		if len(ents) == 0 {
			return fmt.Errorf("sbdb/acl: empty pubkey for %s", r.Nickname)
		}
		recs = append(recs, EncRecipient{Token: tok, Entity: ents[0]})
	}

	plaintext, err := io.ReadAll(br)
	if err != nil {
		return err
	}
	cipher, err := Encrypt(plaintext, EncryptOpts{Recipients: recs, Random: ctx.Random})
	if err != nil {
		return err
	}
	_, err = out.Write(cipher)
	return err
}

// FilterSmudge reads from in and writes cleartext to out if the user has
// a matching key, or the envelope unchanged otherwise.
func FilterSmudge(in io.Reader, out io.Writer, path string, ctx FilterContext) error {
	br := bufio.NewReader(in)
	peek, _ := br.Peek(len(envelopeBegin))
	if !bytes.HasPrefix(peek, []byte(envelopeBegin)) {
		_, err := io.Copy(out, br)
		return err
	}
	envBytes, err := io.ReadAll(br)
	if err != nil {
		return err
	}
	cleartext, err := Decrypt(envBytes, DecryptOpts{Keyring: ctx.PrivateKeys})
	if errors.Is(err, ErrNoMatchingKey) {
		_, werr := out.Write(envBytes)
		return werr
	}
	if err != nil {
		return err
	}
	_, err = out.Write(cleartext)
	return err
}

// FilterTextconv reads a file at path from disk and writes a human-friendly
// representation: cleartext if the user can decrypt, a placeholder otherwise.
func FilterTextconv(path string, out io.Writer, ctx FilterContext) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !IsEnvelopePrefix(b) {
		_, err := out.Write(b)
		return err
	}
	cleartext, err := Decrypt(b, DecryptOpts{Keyring: ctx.PrivateKeys})
	if errors.Is(err, ErrNoMatchingKey) {
		env, _ := ParseEnvelope(bytes.NewReader(b))
		fmt.Fprintf(out, "<sbdb-acl: encrypted, %d recipients>\n", env.RecipientCount)
		return nil
	}
	if err != nil {
		return err
	}
	_, err = out.Write(cleartext)
	return err
}
