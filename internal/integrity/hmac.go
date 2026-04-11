package integrity

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// SignEntry computes the HMAC signature for a manifest entry.
func SignEntry(entry *Entry, key []byte) string {
	payload := entry.ContentSHA + "||" + entry.FrontmatterSHA + "||" + entry.RecordSHA
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifySignature checks that an entry's HMAC signature is valid.
func VerifySignature(entry *Entry, key []byte) bool {
	expected := SignEntry(entry, key)
	return hmac.Equal([]byte(expected), []byte(entry.Sig))
}

// GenerateKey creates a new 32-byte random key.
func GenerateKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generating key: %w", err)
	}
	return key, nil
}

// SaveKeyFile writes a key to a file with 0600 permissions.
func SaveKeyFile(path string, key []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating key directory: %w", err)
	}
	encoded := hex.EncodeToString(key) + "\n"
	return os.WriteFile(path, []byte(encoded), 0o600)
}

// LoadKey loads an integrity key from multiple sources in order:
// 1. SBDB_INTEGRITY_KEY env var (hex-encoded)
// 2. ~/.config/secondbrain-db/integrity.key file
// Returns nil key (no error) if no key is configured.
func LoadKey() ([]byte, error) {
	// 1. Environment variable
	if envKey := os.Getenv("SBDB_INTEGRITY_KEY"); envKey != "" {
		key, err := hex.DecodeString(envKey)
		if err != nil {
			return nil, fmt.Errorf("SBDB_INTEGRITY_KEY is not valid hex: %w", err)
		}
		return key, nil
	}

	// 2. Config file
	home, err := os.UserHomeDir()
	if err == nil {
		keyPath := filepath.Join(home, ".config", "secondbrain-db", "integrity.key")
		data, err := os.ReadFile(keyPath)
		if err == nil {
			// Trim whitespace/newline
			hexStr := string(data)
			hexStr = trimWhitespace(hexStr)
			key, err := hex.DecodeString(hexStr)
			if err != nil {
				return nil, fmt.Errorf("integrity.key is not valid hex: %w", err)
			}
			return key, nil
		}
	}

	return nil, nil
}

// DefaultKeyPath returns the default path for the integrity key file.
func DefaultKeyPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	return filepath.Join(home, ".config", "secondbrain-db", "integrity.key"), nil
}

func trimWhitespace(s string) string {
	result := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' && s[i] != '\t' && s[i] != '\n' && s[i] != '\r' {
			result = append(result, s[i])
		}
	}
	return string(result)
}
