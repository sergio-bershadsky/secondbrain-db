package integrity

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/version"
)

// Clock can be overridden by callers (the pkg/sbdb facade) to make
// sidecar timestamps deterministic in tests. Default is time.Now.
var Clock = time.Now

// Sidecar is the per-doc integrity manifest stored next to the .md file.
// File path: replaces the .md extension with .yaml (e.g. hello.md → hello.yaml).
type Sidecar struct {
	Version        int    `yaml:"version"`
	Algo           string `yaml:"algo"`
	HMAC           bool   `yaml:"hmac"`
	File           string `yaml:"file"`
	ContentSHA     string `yaml:"content_sha"`
	FrontmatterSHA string `yaml:"frontmatter_sha"`
	RecordSHA      string `yaml:"record_sha"`
	Sig            string `yaml:"sig,omitempty"`
	UpdatedAt      string `yaml:"updated_at,omitempty"`
	Writer         string `yaml:"writer,omitempty"`
}

// SidecarPath returns the sidecar path for a given .md file:
// "<dir>/<basename>.yaml". For non-.md inputs, the extension is replaced anyway.
func SidecarPath(mdPath string) string {
	ext := filepath.Ext(mdPath)
	if ext == "" {
		return mdPath + ".yaml"
	}
	return strings.TrimSuffix(mdPath, ext) + ".yaml"
}

// LoadSidecar reads the sidecar for the given .md path. Returns os.IsNotExist
// errors unwrapped so callers can detect "no sidecar" with errors.Is/IsNotExist.
func LoadSidecar(mdPath string) (*Sidecar, error) {
	path := SidecarPath(mdPath)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sc Sidecar
	if err := yaml.Unmarshal(data, &sc); err != nil {
		return nil, fmt.Errorf("parsing sidecar %s: %w", path, err)
	}
	return &sc, nil
}

// Save writes the sidecar atomically (temp + rename).
func (s *Sidecar) Save(mdPath string) error {
	if s.UpdatedAt == "" {
		s.UpdatedAt = Clock().UTC().Format(time.RFC3339)
	}
	if s.Writer == "" {
		s.Writer = "secondbrain-db/" + version.Version
	}
	if s.Algo == "" {
		s.Algo = "sha256"
	}
	if s.Version == 0 {
		s.Version = 1
	}

	data, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Errorf("marshaling sidecar: %w", err)
	}

	path := SidecarPath(mdPath)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating sidecar directory: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".sbdb-sidecar-*.yaml.tmp")
	if err != nil {
		return fmt.Errorf("creating sidecar temp: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("writing sidecar: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming sidecar: %w", err)
	}
	return nil
}

// SignWith returns the HMAC-SHA-256 sig over the three SHAs concatenated.
// Caller assigns result to s.Sig.
func (s *Sidecar) SignWith(key []byte) string {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(s.ContentSHA))
	h.Write([]byte("\n"))
	h.Write([]byte(s.FrontmatterSHA))
	h.Write([]byte("\n"))
	h.Write([]byte(s.RecordSHA))
	return hex.EncodeToString(h.Sum(nil))
}

// VerifyWith checks the stored Sig against the three SHAs and the given key.
func (s *Sidecar) VerifyWith(key []byte) bool {
	if s.Sig == "" {
		return false
	}
	expected := s.SignWith(key)
	return hmac.Equal([]byte(expected), []byte(s.Sig))
}

// Drift describes mismatches between a sidecar and the on-disk doc.
type Drift struct {
	MissingSidecar   bool
	MissingMD        bool
	ContentDrift     bool
	FrontmatterDrift bool
	RecordDrift      bool
	BadSig           bool
}

// Any returns true if any drift bit is set.
func (d Drift) Any() bool {
	return d.MissingSidecar || d.MissingMD || d.ContentDrift ||
		d.FrontmatterDrift || d.RecordDrift || d.BadSig
}

// Verify recomputes hashes from the supplied frontmatter/body/record and
// compares against the sidecar. If key is non-nil and sidecar has HMAC,
// also verifies the signature.
func (s *Sidecar) Verify(mdPath string, fm map[string]any, body string, record map[string]any, key []byte) (Drift, error) {
	var d Drift
	if HashContent(body) != s.ContentSHA {
		d.ContentDrift = true
	}
	if HashFrontmatter(fm) != s.FrontmatterSHA {
		d.FrontmatterDrift = true
	}
	if HashRecord(record) != s.RecordSHA {
		d.RecordDrift = true
	}
	if s.HMAC && key != nil && !s.VerifyWith(key) {
		d.BadSig = true
	}
	return d, nil
}
