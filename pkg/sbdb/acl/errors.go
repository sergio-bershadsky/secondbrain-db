package acl

import "errors"

var (
	// ErrNotEnvelope is returned when input does not have the SBDB-ACL-ENVELOPE header.
	ErrNotEnvelope = errors.New("sbdb/acl: input is not an envelope")
	// ErrUnknownRecipient is returned when an ACL file references a token absent from the local keyring.
	ErrUnknownRecipient = errors.New("sbdb/acl: unknown recipient token")
	// ErrNoMatchingKey is returned when no local private key can unwrap any PKESK packet.
	ErrNoMatchingKey = errors.New("sbdb/acl: no local private key matches any recipient")
	// ErrInnerSHAMismatch is returned when the decrypted payload's inner-sha256 does not match the cleartext bytes.
	ErrInnerSHAMismatch = errors.New("sbdb/acl: inner-sha256 mismatch (decrypted content tampered)")
	// ErrBundleSignature is returned when a bundle's self-signature does not verify against its embedded pubkey.
	ErrBundleSignature = errors.New("sbdb/acl: bundle self-signature invalid")
	// ErrIdentityMissing is returned when no local identity is configured.
	ErrIdentityMissing = errors.New("sbdb/acl: no local identity configured (run 'sbdb keys self-init')")
	// ErrACLNotFound is returned when no ACL file exists for a given doc.
	ErrACLNotFound = errors.New("sbdb/acl: ACL file not found")
)
