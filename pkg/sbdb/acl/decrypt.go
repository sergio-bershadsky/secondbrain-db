package acl

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/ProtonMail/go-crypto/openpgp/armor"
	pgperrors "github.com/ProtonMail/go-crypto/openpgp/errors"
	openpgp "github.com/ProtonMail/go-crypto/openpgp/v2"
)

// DecryptOpts controls decryption.
type DecryptOpts struct {
	Keyring openpgp.EntityList
	// PromptFunc is called by openpgp to unlock encrypted private keys.
	PromptFunc openpgp.PromptFunction
}

// Decrypt parses an envelope and tries every key in opts.Keyring against
// every PKESK packet. On success it returns the original plaintext bytes.
func Decrypt(envelopeBytes []byte, opts DecryptOpts) ([]byte, error) {
	env, err := ParseEnvelope(bytes.NewReader(envelopeBytes))
	if err != nil {
		return nil, err
	}
	block, err := armor.Decode(strings.NewReader(env.PGPArmored))
	if err != nil {
		return nil, fmt.Errorf("sbdb/acl: armor decode: %w", err)
	}
	md, err := openpgp.ReadMessage(block.Body, opts.Keyring, opts.PromptFunc, nil)
	if err != nil {
		if errors.Is(err, pgperrors.ErrKeyIncorrect) || errors.Is(err, pgperrors.ErrUnknownIssuer) ||
			strings.Contains(err.Error(), "no key") || strings.Contains(err.Error(), "key not found") {
			return nil, ErrNoMatchingKey
		}
		return nil, fmt.Errorf("sbdb/acl: read message: %w", err)
	}
	body, err := io.ReadAll(md.UnverifiedBody)
	if err != nil {
		return nil, fmt.Errorf("sbdb/acl: read body: %w", err)
	}
	return parseInner(body)
}

func parseInner(body []byte) ([]byte, error) {
	header, content, ok := bytes.Cut(body, []byte("\n---\n"))
	if !ok {
		return nil, fmt.Errorf("sbdb/acl: malformed inner payload (missing ---)")
	}
	if !bytes.HasPrefix(header, []byte(innerFraming)) {
		return nil, fmt.Errorf("sbdb/acl: unknown inner framing")
	}
	var declared string
	for _, line := range bytes.Split(header, []byte("\n")) {
		k, v, ok := bytes.Cut(line, []byte(":"))
		if !ok {
			continue
		}
		if string(bytes.TrimSpace(k)) == "inner-sha256" {
			declared = string(bytes.TrimSpace(v))
		}
	}
	sum := sha256.Sum256(content)
	got := hex.EncodeToString(sum[:])
	if declared != "" && declared != got {
		return nil, ErrInnerSHAMismatch
	}
	return content, nil
}
