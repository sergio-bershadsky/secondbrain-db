// Package acl implements per-document access control for sbdb knowledge
// bases. Documents marked as ACL'd are stored on disk as OpenPGP
// multi-recipient envelopes; readers see plaintext after a smudge filter
// run, non-readers see the envelope intact. Recipient identities are
// blinded behind opaque tokens; mappings from tokens to people live only
// in a per-clone gitignored local keyring.
//
// The entry points are:
//
//	Encrypt(cleartext, recipients, opts)  -> envelope bytes
//	Decrypt(envelope, identity, opts)     -> cleartext bytes
//	FilterClean(in, out, path, ctx)       -> envelope (or pass-through)
//	FilterSmudge(in, out, path, ctx)      -> cleartext (or pass-through)
//
// On-disk layout is documented in docs/superpowers/specs/2026-05-07-acl-pgp-design.md.
package acl
