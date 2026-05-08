# Design: PGP-based ACL with blinded recipients

- **Issue:** [#44](https://github.com/sergio-bershadsky/secondbrain-db/issues/44)
- **Date:** 2026-05-07
- **Status:** Draft for review
- **Type:** `feat`

## Summary

Add per-document ACLs to sbdb using OpenPGP multi-recipient encryption. Encrypted documents live in the working tree as ciphertext envelopes; a git smudge/clean filter decrypts them transparently for users whose private key matches a recipient slot, so the experience after `git pull` is "just open the markdown file." The committed repo state blinds recipient identities behind opaque tokens. Each user maintains a per-clone gitignored keyring mapping `token → name/email/pubkey`. A static-site browser flow uses openpgp.js with a passphrase-unlocked private key held only in JS memory.

## Goals

1. Mark any document as readable by a chosen subset of collaborators with one CLI call.
2. After `git pull`, readers see plaintext markdown; non-readers see a structurally-intact ciphertext envelope.
3. Committed repo content does not reveal who can read which document.
4. Static-site UIs (VitePress) can decrypt ACL'd pages client-side with a key that never leaves the browser tab.
5. Existing sbdb tooling (doctor, kg, query, semantic) keeps working — gracefully degrading on docs the local user cannot decrypt.
6. A definitive user guide exists alongside the implementation.

## Non-goals (v1)

- Write-ACL enforcement beyond what encryption naturally provides (a non-reader cannot produce valid ciphertext for a doc they cannot decrypt; that is sufficient for v1).
- Automated key revocation cascades — operators rotate keys with `sbdb keys rotate` and re-run the clean filter.
- WebAuthn / passkey-gated browser keys — passphrase + IndexedDB is the v1 model.
- Content-addressed file paths to blind filenames.
- Per-frontmatter-field selective encryption — whole-file is the chosen model.

## Architecture overview

```
working tree                       git index / repo
─────────────────                  ──────────────────
plain markdown   (reader)     <──  envelope (ciphertext)
envelope         (non-reader) ──>  envelope (ciphertext)
```

A new package `pkg/sbdb/acl` plus an `sbdb _filter` subcommand registered as a git filter driver. The filter is wired up by `sbdb unlock`, exactly like `git-crypt unlock`.

Three new on-disk artifacts:

| Path | Committed? | Purpose |
|---|---|---|
| `docs/.sbdb/acl/<doc-path>.yaml` | yes | Per-doc list of opaque recipient tokens. |
| `.sbdb/local-keys/recipients.yaml` | **no** (gitignored) | Per-clone mapping from tokens to identities + pubkeys. |
| `.sbdb/local-keys/pubkeys/<fingerprint>.asc` | **no** (gitignored) | ASCII-armored pubkeys, referenced from `recipients.yaml`. |
| `.sbdb/local-identity.toml` | **no** (gitignored) | This clone's own identity + private-key location. |

There is **no** committed `KEYS.yaml`. Identity bootstrapping is out-of-band.

## Envelope format

Encrypted files have a recognisable two-part structure: a minimal plaintext header followed by an OpenPGP ASCII-armored message.

```
-----BEGIN SBDB-ACL-ENVELOPE-----
Version: 1
Recipients: 2
-----END SBDB-ACL-ENVELOPE-----
-----BEGIN PGP MESSAGE-----
<OpenPGP message:
  - N PKESK packets, key-id stripped (throw_keyids=true)
  - SEIPD payload, contents below>
-----END PGP MESSAGE-----
```

The encrypted payload is itself a small framed structure:

```
sbdb-acl-payload-v1
acl-readers: [alice, bob]    # human-readable; visible only post-decrypt
inner-sha256: <hex>          # SHA256 of the original markdown bytes
---
<original markdown file: frontmatter + body, byte-identical>
```

The plaintext envelope header carries no doc-id, schema, fingerprint, or name. Doc identity is implicit from the file path; sidecars and ACL files key by path. The recipient count is unavoidable (one PKESK packet per recipient is observable in any OpenPGP message).

PGP "hidden recipient" mode (`throw_keyids=true`) emits PKESK packets with key-id zero. Decryption walks every available private key against every PKESK and tries to unwrap. This is a standard openpgp.js / gopenpgp feature.

## Blinded recipient tokens

Each recipient is identified in committed files only by an opaque random token (32 bytes, hex- or base64-encoded with a `tok_` prefix). Tokens are generated once at recipient creation time and shared as part of the bundle. They carry no information; they are pure random and only meaningful when joined against a local keyring entry.

Token re-use across documents is the residual leak: an outsider can see "the same set of tokens reads these 14 docs" and infer group structure, even without identifying members. This is documented and accepted.

## ACL file

```yaml
# docs/.sbdb/acl/meetings/q2-strategy.yaml
version: 1
readers:
  - tok_a8f3e92c1d4bce17ff03b920abc81f4d
  - tok_5e021ac98770bb39c4ee5d10872afe83
```

Presence of an ACL file at `docs/.sbdb/acl/<path>.yaml` is the signal to the git clean filter that the file at `<path>` must be encrypted before being written to the index. Absence means pass-through.

`sbdb acl set <doc> --readers alice,bob` resolves nicknames against the local keyring and writes this file.

## Local keyring

```
.sbdb/local-keys/
  recipients.yaml
  pubkeys/
    D4F28B919C3A....asc
    7AE31C04....asc
```

```yaml
# .sbdb/local-keys/recipients.yaml
version: 1
recipients:
  - nickname: alice
    token: tok_a8f3e92c1d4bce17ff03b920abc81f4d
    fingerprint: D4F28B919C3A...
    name: Alice Smith
    email: alice@example.com
    pubkey_file: pubkeys/D4F28B919C3A....asc
    imported_at: 2026-05-07
  - nickname: bob
    token: tok_5e021ac98770bb39c4ee5d10872afe83
    fingerprint: 7AE31C04...
    name: Bob Jones
    email: bob@example.com
    pubkey_file: pubkeys/7AE31C04....asc
    imported_at: 2026-05-07
```

Local identity:

```toml
# .sbdb/local-identity.toml
nickname = "bob"
fingerprint = "7AE31C04..."
private_key_path = "/home/bob/.gnupg/secring.gpg"   # or ascii-armored file
use_gpg_agent = true                                 # if true, defer signing/decryption to gpg-agent
```

Both directories are added to `.gitignore` by `sbdb unlock` if not already present.

## Bundles (out-of-band sharing)

Bootstrap is out-of-band: a new collaborator runs `sbdb keys self-init`, exports a bundle, and shares it via Signal / email / internal chat / a private gist / an internal HTTPS endpoint. Existing collaborators run `sbdb keys import <bundle>`.

```yaml
# alice.sbdb-key-bundle (YAML)
version: 1
nickname: alice
token: tok_a8f3e92c1d4bce17ff03b920abc81f4d
fingerprint: D4F28B919C3A...
name: Alice Smith
email: alice@example.com
pubkey: |
  -----BEGIN PGP PUBLIC KEY BLOCK-----
  ...
  -----END PGP PUBLIC KEY BLOCK-----
self_signature: |
  -----BEGIN PGP SIGNATURE-----
  ...
  -----END PGP SIGNATURE-----
```

`self_signature` is a detached PGP signature by the bundle's pubkey over the canonical YAML serialization of `(version, nickname, token, fingerprint, name, email, pubkey)`. Importing verifies the signature, ensuring the (token, fingerprint) binding originates from the keyholder.

The token is generated once at `sbdb keys self-init` time and is stable across all clones that import the bundle, so ACL files are interpretable team-wide.

## Git filter behavior

`sbdb unlock` writes to local `.git/config`:

```ini
[filter "sbdb-acl"]
    clean   = sbdb _filter clean %f
    smudge  = sbdb _filter smudge %f
    process = sbdb _filter process     # optional, long-running protocol for perf
    required = true
[diff "sbdb-acl"]
    textconv = sbdb _filter textconv %f
```

And appends to `.gitattributes` (committed):

```
docs/**/*.md filter=sbdb-acl diff=sbdb-acl
```

The `required = true` flag makes git fail the operation if the filter exits non-zero, so a misconfigured environment cannot silently commit cleartext.

### Clean (working tree → index, on `git add` / `git commit`)

1. Read the path argument (`%f`).
2. Look up `docs/.sbdb/acl/<path>.yaml`.
3. If absent: copy stdin → stdout unchanged.
4. If present:
   a. If stdin already starts with `-----BEGIN SBDB-ACL-ENVELOPE-----` and is structurally valid, pass through unchanged (this covers the non-reader-committing-without-modifying case).
   b. Otherwise resolve each token in the ACL via `recipients.yaml`. If any token is unknown, fail with `unknown recipient tok_xxx in docs/.sbdb/acl/<path>.yaml — ask a teammate for the bundle, then 'sbdb keys import'`.
   c. Compute `inner-sha256` over stdin bytes.
   d. Build the encrypted payload (acl-readers list, inner-sha256, original bytes).
   e. Encrypt with `throw_keyids=true` to all recipient pubkeys.
   f. Wrap with the plaintext envelope header.
   g. Write to stdout.

### Smudge (index → working tree, on `git checkout`)

1. Read path argument.
2. If stdin does not start with `-----BEGIN SBDB-ACL-ENVELOPE-----`: copy stdin → stdout unchanged.
3. Otherwise:
   a. Parse the envelope; extract the OpenPGP message.
   b. Try every available local private key against every PKESK; on success, decrypt the SEIPD payload.
   c. If decrypt succeeds, validate `inner-sha256` against decrypted-body bytes; on mismatch fail loudly. On success, write the original markdown bytes to stdout.
   d. If no key matches: write the envelope to stdout unchanged. The non-reader sees the envelope on disk and knows the file is encrypted (and to how many recipients) but not its content.

### Textconv (for `git diff`)

Same as smudge but reads the file path directly. Readers see decrypted diffs; non-readers see a `<encrypted document, N recipients>` placeholder.

### `sbdb lock` and `sbdb unlock`

- `sbdb unlock` (one-time per clone): writes filter/diff/attributes config; ensures `.gitignore` excludes the local-keys dir; runs smudge over the working tree to decrypt anything the user can read.
- `sbdb lock`: re-runs clean over the working tree to scrub cleartext from disk (useful before lending the laptop or when leaving the room). Symmetric, idempotent.

### Performance

For large checkouts, spawning `sbdb` per file is slow. We implement git's [long-running filter process protocol](https://git-scm.com/docs/gitattributes#_long_running_filter_process), so a single `sbdb _filter process` handles every file in a checkout. Trade-off: more code; pay-off: linear cost in the number of ACL'd files only.

## Doctor, kg, query, semantic

- **Doctor** (no keys): validates envelope structure, plaintext header well-formedness, sidecar SHA over the on-disk-form (the envelope), and ACL file readability. With `--with-keys`: additionally decrypts and verifies `inner-sha256` against the cleartext.
- **Sidecar SHA** is over the on-disk form. For ACL'd docs that's the envelope. PGP encryption is non-deterministic so each re-encryption produces a fresh envelope and a fresh sidecar SHA — expected; git tracks per-commit. To avoid history churn, the clean filter compares `inner-sha256` of the new cleartext against the existing envelope's `inner-sha256` (decrypting if possible) and skips re-encryption when neither cleartext nor ACL has changed. When the user is a non-reader, this comparison is impossible; in that case the filter passes through unchanged (option 4(a) above).
- **Query / kg / semantic** treat docs the local user cannot decrypt as opaque. They appear in result sets as `{path: ..., encrypted: true, recipients: <count>}` placeholders so listings stay coherent. Indexes that *do* cover ACL'd docs (e.g. semantic embeddings of cleartext) are written to a **per-user cache** at `.sbdb/local-cache/`, gitignored. Committing such indexes would re-leak content.

## CLI surface

```
# identity
sbdb keys self-init [--name --email] [--key <existing-key>]
sbdb keys export <nickname> [-o <file>]
sbdb keys import <bundle-file>
sbdb keys list
sbdb keys whoami
sbdb keys rotate <nickname> --new-pubkey <file>   # re-encrypts every doc this nickname is reader of

# ACL management
sbdb acl set <doc> --readers a,b,c
sbdb acl get <doc>
sbdb acl add <doc> --reader d
sbdb acl rm  <doc> --reader b
sbdb acl ls

# clone-level transparency
sbdb unlock                # one-time: register filters + identity
sbdb lock                  # scrub cleartext from working tree
sbdb _filter clean|smudge|textconv|process <path>   # internal git driver entrypoints
```

`sbdb acl set` is the one-shot from the requirements: it writes the ACL file, runs the clean filter over the doc, refreshes sidecars, and is ready to commit.

## Browser / UI flow

The browser path uses [openpgp.js](https://github.com/openpgpjs/openpgpjs) for OpenPGP operations and [@noble/hashes](https://github.com/paulmillr/noble-hashes) (or argon2-browser) for Argon2id.

**One-time setup**

1. User imports an ASCII-armored private key (their own) plus, optionally, their bundle and any peer bundles.
2. UI prompts for a session passphrase (independent of the key's own passphrase).
3. Browser derives a wrapping key with Argon2id (passphrase + per-user random salt, salt persisted in IndexedDB).
4. Wraps the private key under the wrapping key and stores the ciphertext + salt + KDF params in IndexedDB.
5. Stores peer recipients (from imported bundles) verbatim in IndexedDB so decrypted pages can be labelled with friendly names.

**Per-session unlock**

1. On page load the UI prompts for the passphrase.
2. Argon2id → wrapping key → decrypts the private key into JS memory as an `openpgp.PrivateKey` object.
3. The key is held by an `AuthSession` module in a closure scope; references are dropped on `pagehide` / explicit "lock". No exfiltration to localStorage, no `console.log`, no network egress.

**Rendering an ACL'd page**

- VitePress build emits ciphertext envelopes as static assets (e.g. `<page>.enc.txt`) and a manifest mapping page paths to envelope assets.
- The page component fetches the envelope, calls `openpgp.decrypt({ message, decryptionKeys: session.privateKey })`, parses the inner payload, validates `inner-sha256`, and renders the markdown via VitePress's renderer.
- On decrypt failure: render a placeholder showing recipient count and a "you don't have access" message.

**Authentication model**

Possession of a private key matching a recipient PKESK *is* the read capability. There is no server, no session token, no auth endpoint. If signed write paths show up later (e.g. signed comments on a discussion), the same key signs them with `openpgp.sign`.

## User-facing definitive guide

A separate deliverable: `docs/guide/acl.md`. The guide is part of acceptance for this work, not an afterthought.

Outline:

1. **Concepts** — encryption boundary, blinded tokens, local keyring, what's in git vs. on disk vs. in your head.
2. **Bootstrap** — `sbdb keys self-init`, what gets created, where it lives.
3. **Sharing your bundle** — out-of-band channels, threat model of each.
4. **Importing peers' bundles** — verifying the self-signature, when to trust.
5. **ACL'ing your first doc** — copy-pasteable: `sbdb acl set ... && git add ... && git commit`.
6. **What teammates see** — both readers and non-readers.
7. **Locking and unlocking** — `sbdb lock` / `sbdb unlock` workflow and when to use them.
8. **Browser setup** — importing the key, passphrase, what's stored where.
9. **Recovering** — lost passphrase, lost key, rotating a compromised key (`sbdb keys rotate`).
10. **Threat model** — what's blinded, what's not, residual leaks (token correlation, file paths, commit metadata).
11. **Limitations** — write-ACL not enforced beyond encryption, history retention, etc.

## Threat model summary

| Adversary | What they learn |
|---|---|
| Public clone of repo | Encrypted blobs; per-doc recipient counts; per-doc token sets; cross-doc correlation of tokens. **No identities.** |
| Insider with a single private key | Their own readable docs and the in-body `acl-readers` lists for those docs. |
| Insider who steals another user's `.sbdb/local-keys/` | The full token → identity map for that user, but no plaintext beyond what that user could already see. |
| Insider who steals a private key | Everything that key could decrypt. (Standard PGP threat.) |
| Network observer at bundle-share time | Whatever the chosen out-of-band channel reveals. |
| `git log` reader | Author name/email of the committer who touched a file (separate from ACL). |

## Risks and mitigations

- **Filter performance on big checkouts.** Mitigated by implementing the long-running `process` filter protocol from day one.
- **Non-deterministic ciphertext churn in git history.** Mitigated by inner-SHA short-circuit: skip re-encryption when neither cleartext nor ACL changed.
- **Accidental cleartext commit if filter mis-registered.** Mitigated by `filter.sbdb-acl.required = true` and a CI lint that runs `sbdb doctor --with-keys=false` to verify every doc with an ACL file is, in fact, an envelope in the index.
- **Lost passphrase locking out browser.** Mitigated by allowing re-import of the private key with a fresh passphrase; documented in the guide's recovery section.
- **Tampering with an ACL file (e.g. removing yourself).** Mitigated by ACL files participating in the existing sidecar HMAC; doctor flags ACL drift.
- **Forgotten unlock on a fresh clone leading to garbled markdown.** Mitigated by a `SessionStart`-style hook (or simply `sbdb status` showing a clear warning) that detects unconfigured filters and prints next steps.

## Open questions

None blocking. Questions deferred to implementation PRs:

- Exact wire format for the long-running filter `process` protocol (mirror git-crypt's, or define our own?).
- Whether to support symmetric password-based recipients (group passphrases) in addition to public-key recipients — likely yes, as a minor extension to the bundle/keyring model.
- Whether `sbdb keys rotate` re-encrypts in a single transaction or streams per-doc commits (the latter has nicer history).

## Acceptance criteria

- Spec landed and reviewed.
- Implementation plan landed.
- All CLI commands implemented with unit + integration tests.
- Git filter round-trips verified on macOS and Linux for both readers and non-readers, including the long-running `process` protocol path.
- Doctor, kg, query, semantic gracefully handle ACL'd docs, with placeholder records for unreadable ones.
- Browser demo decrypts a sample encrypted page given an imported private key and survives a passphrase round-trip.
- Definitive user guide at `docs/guide/acl.md` covers bootstrap → ACL → share → browser → recovery, with copy-pasteable commands.
- New e2e test that simulates a two-user setup (alice + bob), exchanges bundles, ACLs a doc, commits, and verifies that bob's clone smudges to plaintext while a third clone without keys sees only the envelope.
