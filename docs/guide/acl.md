# Per-document ACL with PGP

> **Status:** v1. Read-ACL only; encryption naturally limits writes. Browser/UI integration ships in a follow-up.

`sbdb` can mark any document as readable by a chosen subset of collaborators. Encrypted documents live on disk as **envelopes** — a small plaintext header plus an OpenPGP message with one wrapped session key per recipient. The envelope's plaintext header carries only a version and a recipient count; recipient identities are blinded behind opaque random tokens. The mapping from tokens to people lives only on your machine, in a gitignored directory you maintain.

The data flow:

- **Envelope** — what is committed to git.
- **Cleartext** — what your editor sees, after the sbdb git filter decrypts on `git checkout`.
- **Token-to-person mapping** — your local address book, never committed.

## 1. Bootstrap

```bash
sbdb keys self-init --name "Alice Smith" --email alice@example.com --nickname alice
```

This creates:

- A fresh OpenPGP keypair for you.
- `.sbdb/local-keys/private.asc` — your private key (mode 0600).
- `.sbdb/local-keys/pubkeys/<fingerprint>.asc` — your pubkey.
- `.sbdb/local-keys/recipients.yaml` — your address book (currently containing only you).
- `.sbdb/local-identity.toml` — pointer to your private key + nickname.
- A token (`tok_…`) printed on stdout.

```bash
sbdb keys whoami
# nickname=alice fingerprint=… key=.sbdb/local-keys/private.asc
```

## 2. Register the git filter

Once per clone:

```bash
sbdb unlock
```

This appends a `[filter "sbdb-acl"]` block to `.git/config`, adds `docs/**/*.md filter=sbdb-acl diff=sbdb-acl` to `.gitattributes`, and ensures `.sbdb/local-keys/` is in `.gitignore`. After this, `git checkout` transparently decrypts files you have keys for and `git add` transparently encrypts files with an ACL.

## 3. Share your bundle

```bash
sbdb keys export alice -o alice.bundle.yaml
```

The bundle contains your nickname, token, fingerprint, public key, and a self-signature over all of the above. Send it to teammates over a channel you trust:

- **Signal / iMessage** — strong delivery confidentiality, ad-hoc.
- **Internal email** — fine for non-paranoid threat models.
- **A private gist or company file share** — fine if access to that share is itself controlled.

## 4. Import a peer's bundle

```bash
sbdb keys import bob.bundle.yaml
# imported: bob (tok_5e02…)
```

`sbdb` verifies the bundle's self-signature against the embedded pubkey before trusting any field. A bundle with a broken signature is rejected.

```bash
sbdb keys list
# alice  tok_a8f3…  Alice Smith <alice@example.com>
# bob    tok_5e02…  Bob Jones   <bob@example.com>
```

## 5. ACL your first doc

```bash
sbdb acl set docs/meetings/q2-strategy.md --readers alice,bob
git add .
git commit -m "feat(meetings): q2 strategy notes (acl: alice, bob)"
```

The clean filter encrypts on commit. `git show HEAD:docs/meetings/q2-strategy.md` will print the envelope, not the cleartext.

```bash
sbdb acl get docs/meetings/q2-strategy.md
#   alice       tok_a8f3…  D4F28B91…
#   bob         tok_5e02…  7AE31C04…
```

## 6. What teammates see

When bob runs `git pull && git checkout HEAD -- .`:

- His clone smudges `docs/meetings/q2-strategy.md` to cleartext using his private key.
- His editor sees plain markdown.

When a third party clones who has neither alice's nor bob's key:

- Their clone sees the envelope verbatim on disk: `-----BEGIN SBDB-ACL-ENVELOPE-----` followed by the PGP message.
- They can see *that* a document is encrypted to two recipients but not to whom or with what content.

## 7. Lock and unlock

`sbdb lock` re-encrypts the working tree to scrub cleartext from disk — useful before lending the laptop or leaving the room. `sbdb unlock` re-runs the smudge filter to materialise cleartext for files you can read.

```bash
sbdb lock     # cleartext scrubbed; envelopes remain on disk
sbdb unlock   # working tree decrypted again
```

## 8. Browser setup

> Browser/UI integration ships in a follow-up plan. This section will be filled in when that lands.

The intended model: openpgp.js reads ciphertext envelopes baked into the static site, decrypts them in JS memory using a passphrase-unlocked private key held in IndexedDB. The key never leaves the browser tab.

## 9. Recovery

| Situation | Action |
|---|---|
| Lost your private key | You permanently lose access to anything encrypted to you. Generate a new identity (`sbdb keys self-init`) and ask collaborators to re-ACL with your new token. |
| Compromised key | Rotate via a fresh `keys self-init` in a new clone, share the new bundle, ask everyone to re-ACL relevant docs. (Automated `sbdb keys rotate` is a follow-up.) |
| Forgot to `sbdb unlock` after cloning | Files appear as envelope ciphertext in your editor. Run `sbdb unlock` then `git checkout HEAD -- .`. |

## 10. Threat model

| Adversary | What they learn |
|---|---|
| Public clone of repo | Encrypted blobs; per-doc recipient counts; per-doc token sets; cross-doc correlation of tokens. **No identities.** |
| Insider with one private key | Their own readable docs and the in-body reader names. |
| Insider who steals another user's `.sbdb/local-keys/` | The token → identity mapping for that user. |
| Insider who steals a private key | Everything that key could decrypt. (Standard PGP threat.) |
| Network observer at bundle-share time | Whatever the chosen out-of-band channel reveals. |
| `git log` reader | Author name/email of the committer who touched a file. |

## 11. Limitations

- **File paths and filenames are not blinded.** `docs/meetings/q2-strategy.md` itself reveals the topic.
- **Token correlation across docs.** Same token set across many docs implies same group of readers, even without naming them.
- **Historical access.** A user removed from an ACL still has historical cleartext in their git history and any local caches.
- **Write-ACL is implicit.** Encryption naturally prevents non-readers from producing valid ciphertext, but they can still touch a file in a way that breaks the envelope (the clean filter detects and refuses).
- **Commit metadata.** `git log` still shows the committer's name and email.

## 12. Internals

If you want to understand what's on disk:

- **Committed encrypted file** at `docs/<path>.md` — an envelope: a plaintext SBDB-ACL-ENVELOPE header (version, recipient count) followed by an ASCII-armored OpenPGP message with PKESK packets that have been emitted with key-id zero (`throw_keyids=true` semantics in openpgp/v2).
- **Committed ACL file** at `docs/.sbdb/acl/<path>.yaml` — version + a list of opaque tokens.
- **Local keyring** at `.sbdb/local-keys/recipients.yaml` (gitignored) — token to (nickname, fingerprint, name, email, pubkey path).
- **Local pubkeys** at `.sbdb/local-keys/pubkeys/<fingerprint>.asc` (gitignored).
- **Local identity** at `.sbdb/local-identity.toml` (gitignored) — your nickname, fingerprint, private-key path.
- **Bundles** are YAML files with the public bits of an identity plus a detached self-signature over a canonical form of `(version, nickname, token, fingerprint, name, email, pubkey_sha256)`.

The full design is in `docs/superpowers/specs/2026-05-07-acl-pgp-design.md`.
