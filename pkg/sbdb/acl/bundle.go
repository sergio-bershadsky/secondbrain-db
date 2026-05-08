package acl

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"github.com/ProtonMail/go-crypto/openpgp/armor"
	openpgp "github.com/ProtonMail/go-crypto/openpgp/v2"
	"gopkg.in/yaml.v3"
)

// Bundle is the on-disk YAML form of a shareable identity bundle.
type Bundle struct {
	Version       int    `yaml:"version"`
	Nickname      string `yaml:"nickname"`
	Token         Token  `yaml:"token"`
	Fingerprint   string `yaml:"fingerprint"`
	Name          string `yaml:"name"`
	Email         string `yaml:"email"`
	Pubkey        string `yaml:"pubkey"`
	SelfSignature string `yaml:"self_signature"`
}

// BundleExportOpts controls bundle export.
type BundleExportOpts struct {
	Nickname string
	Token    Token
	Entity   *openpgp.Entity
	Random   io.Reader
}

// canonical produces the bytes that the self-signature covers.
func (b Bundle) canonical() []byte {
	return []byte(fmt.Sprintf(
		"version=%d\nnickname=%s\ntoken=%s\nfingerprint=%s\nname=%s\nemail=%s\npubkey_sha256=%s\n",
		b.Version, b.Nickname, b.Token, b.Fingerprint, b.Name, b.Email, sha256Hex(b.Pubkey),
	))
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// ExportBundle produces a YAML bundle signed by the entity's primary key.
func ExportBundle(opts BundleExportOpts) ([]byte, error) {
	var pubBuf bytes.Buffer
	armorW, err := armor.Encode(&pubBuf, openpgp.PublicKeyType, nil)
	if err != nil {
		return nil, fmt.Errorf("sbdb/acl: armor pub: %w", err)
	}
	if err := opts.Entity.Serialize(armorW); err != nil {
		return nil, fmt.Errorf("sbdb/acl: serialize pub: %w", err)
	}
	if err := armorW.Close(); err != nil {
		return nil, err
	}

	bundle := Bundle{
		Version:     1,
		Nickname:    opts.Nickname,
		Token:       opts.Token,
		Fingerprint: hex.EncodeToString(opts.Entity.PrimaryKey.Fingerprint),
		Pubkey:      pubBuf.String(),
	}
	for _, id := range opts.Entity.Identities {
		bundle.Name = id.UserId.Name
		bundle.Email = id.UserId.Email
		break
	}

	var sigBuf bytes.Buffer
	armorSig, err := armor.Encode(&sigBuf, "PGP SIGNATURE", nil)
	if err != nil {
		return nil, fmt.Errorf("sbdb/acl: armor sig: %w", err)
	}
	if err := openpgp.DetachSign(armorSig, []*openpgp.Entity{opts.Entity}, bytes.NewReader(bundle.canonical()), nil); err != nil {
		return nil, fmt.Errorf("sbdb/acl: sign: %w", err)
	}
	if err := armorSig.Close(); err != nil {
		return nil, err
	}
	bundle.SelfSignature = sigBuf.String()

	out, err := yaml.Marshal(bundle)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ImportBundle parses a YAML bundle and verifies its self-signature.
func ImportBundle(b []byte) (Bundle, error) {
	var bundle Bundle
	if err := yaml.Unmarshal(b, &bundle); err != nil {
		return Bundle{}, fmt.Errorf("sbdb/acl: parse bundle: %w", err)
	}
	if _, err := ParseToken(string(bundle.Token)); err != nil {
		return Bundle{}, err
	}
	keyring, err := openpgp.ReadArmoredKeyRing(strings.NewReader(bundle.Pubkey))
	if err != nil {
		return Bundle{}, fmt.Errorf("sbdb/acl: read pubkey: %w", err)
	}
	sigBlock, err := armor.Decode(strings.NewReader(bundle.SelfSignature))
	if err != nil {
		return Bundle{}, fmt.Errorf("sbdb/acl: decode sig: %w", err)
	}
	if _, _, err := openpgp.VerifyDetachedSignature(keyring, bytes.NewReader(bundle.canonical()), sigBlock.Body, nil); err != nil {
		return Bundle{}, ErrBundleSignature
	}
	return bundle, nil
}
