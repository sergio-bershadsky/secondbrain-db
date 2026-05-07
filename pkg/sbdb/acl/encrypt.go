package acl

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
	openpgp "github.com/ProtonMail/go-crypto/openpgp/v2"
)

// EncRecipient is a recipient for Encrypt.
type EncRecipient struct {
	Token  Token
	Entity *openpgp.Entity
}

// EncryptOpts controls encryption.
type EncryptOpts struct {
	Recipients []EncRecipient
	// Random is the source of randomness; defaults to crypto/rand.Reader if nil.
	Random io.Reader
}

const innerFraming = "sbdb-acl-payload-v1"

// Encrypt produces an envelope ready to write to disk for plaintext.
// All PKESK packets are emitted as hidden recipients (key id stripped) so
// the wire form reveals nothing about who can read.
func Encrypt(plaintext []byte, opts EncryptOpts) ([]byte, error) {
	if len(opts.Recipients) == 0 {
		return nil, fmt.Errorf("sbdb/acl: encrypt: no recipients")
	}
	hidden := make([]*openpgp.Entity, len(opts.Recipients))
	nicks := make([]string, len(opts.Recipients))
	for i, r := range opts.Recipients {
		hidden[i] = r.Entity
		nicks[i] = primaryName(r.Entity)
	}

	sum := sha256.Sum256(plaintext)
	innerSHA := hex.EncodeToString(sum[:])

	var inner bytes.Buffer
	inner.WriteString(innerFraming)
	inner.WriteString("\nacl-readers: [")
	inner.WriteString(strings.Join(nicks, ", "))
	inner.WriteString("]\ninner-sha256: ")
	inner.WriteString(innerSHA)
	inner.WriteString("\n---\n")
	inner.Write(plaintext)

	cfg := &packet.Config{Rand: opts.Random}

	var pgpBuf bytes.Buffer
	armorWriter, err := armor.Encode(&pgpBuf, "PGP MESSAGE", nil)
	if err != nil {
		return nil, fmt.Errorf("sbdb/acl: armor: %w", err)
	}
	encWriter, err := openpgp.Encrypt(armorWriter, nil, hidden, nil, &openpgp.FileHints{IsUTF8: true}, cfg)
	if err != nil {
		return nil, fmt.Errorf("sbdb/acl: encrypt: %w", err)
	}
	if _, err := encWriter.Write(inner.Bytes()); err != nil {
		return nil, fmt.Errorf("sbdb/acl: encrypt write: %w", err)
	}
	if err := encWriter.Close(); err != nil {
		return nil, fmt.Errorf("sbdb/acl: encrypt close: %w", err)
	}
	if err := armorWriter.Close(); err != nil {
		return nil, fmt.Errorf("sbdb/acl: armor close: %w", err)
	}

	env := Envelope{
		Version:        1,
		RecipientCount: len(hidden),
		PGPArmored:     pgpBuf.String(),
	}
	var out bytes.Buffer
	if _, err := env.WriteTo(&out); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func primaryName(e *openpgp.Entity) string {
	for _, id := range e.Identities {
		return id.UserId.Name
	}
	return e.PrimaryKey.KeyIdShortString()
}
