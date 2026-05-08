package acl

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const (
	envelopeBegin = "-----BEGIN SBDB-ACL-ENVELOPE-----"
	envelopeEnd   = "-----END SBDB-ACL-ENVELOPE-----"
)

// Envelope is the outermost wire structure for an ACL'd document on disk.
// The plaintext header carries only Version and RecipientCount; the PGP
// armored body holds N PKESK packets (key-id stripped) and the SEIPD
// payload that contains the inner sbdb-acl-payload-v1 framing.
type Envelope struct {
	Version        int
	RecipientCount int
	PGPArmored     string
}

// IsEnvelopePrefix reports whether b begins with the envelope sentinel.
// Used by the git filter to short-circuit non-ACL content quickly.
func IsEnvelopePrefix(b []byte) bool {
	return bytes.HasPrefix(b, []byte(envelopeBegin))
}

// WriteTo serialises the envelope to w. Implements io.WriterTo.
func (e Envelope) WriteTo(w io.Writer) (int64, error) {
	var buf strings.Builder
	buf.WriteString(envelopeBegin)
	buf.WriteByte('\n')
	fmt.Fprintf(&buf, "Version: %d\n", e.Version)
	fmt.Fprintf(&buf, "Recipients: %d\n", e.RecipientCount)
	buf.WriteString(envelopeEnd)
	buf.WriteByte('\n')
	buf.WriteString(strings.TrimRight(e.PGPArmored, "\n"))
	buf.WriteByte('\n')
	n, err := io.WriteString(w, buf.String())
	return int64(n), err
}

// ParseEnvelope reads an envelope from r.
func ParseEnvelope(r io.Reader) (Envelope, error) {
	br := bufio.NewReader(r)
	first, err := br.ReadString('\n')
	if err != nil {
		return Envelope{}, fmt.Errorf("sbdb/acl: read envelope header: %w", err)
	}
	if strings.TrimRight(first, "\r\n") != envelopeBegin {
		return Envelope{}, ErrNotEnvelope
	}
	var env Envelope
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return Envelope{}, fmt.Errorf("sbdb/acl: read envelope: %w", err)
		}
		trim := strings.TrimRight(line, "\r\n")
		if trim == envelopeEnd {
			break
		}
		k, v, ok := strings.Cut(trim, ":")
		if !ok {
			return Envelope{}, fmt.Errorf("sbdb/acl: malformed envelope header line %q", trim)
		}
		v = strings.TrimSpace(v)
		switch strings.TrimSpace(k) {
		case "Version":
			n, err := strconv.Atoi(v)
			if err != nil {
				return Envelope{}, fmt.Errorf("sbdb/acl: version: %w", err)
			}
			env.Version = n
		case "Recipients":
			n, err := strconv.Atoi(v)
			if err != nil {
				return Envelope{}, fmt.Errorf("sbdb/acl: recipients: %w", err)
			}
			env.RecipientCount = n
		}
	}
	rest, err := io.ReadAll(br)
	if err != nil {
		return Envelope{}, fmt.Errorf("sbdb/acl: read envelope body: %w", err)
	}
	env.PGPArmored = string(rest)
	return env, nil
}
