package acl

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

// Token is a 32-byte random identifier for a recipient, encoded as
// "tok_" followed by 64 hex chars. It carries no information; only a
// local keyring entry can join it to a person.
type Token string

const (
	tokenPrefix = "tok_"
	tokenHexLen = 64
)

// NewToken generates a fresh random token.
func NewToken() (Token, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("sbdb/acl: token rand: %w", err)
	}
	return Token(tokenPrefix + hex.EncodeToString(b[:])), nil
}

// ParseToken validates the textual form of a token.
func ParseToken(s string) (Token, error) {
	if !strings.HasPrefix(s, tokenPrefix) {
		return "", fmt.Errorf("sbdb/acl: token must start with %q", tokenPrefix)
	}
	hexPart := strings.TrimPrefix(s, tokenPrefix)
	if len(hexPart) != tokenHexLen {
		return "", fmt.Errorf("sbdb/acl: token hex must be %d chars, got %d", tokenHexLen, len(hexPart))
	}
	if _, err := hex.DecodeString(hexPart); err != nil {
		return "", fmt.Errorf("sbdb/acl: token hex invalid: %w", err)
	}
	return Token(s), nil
}
