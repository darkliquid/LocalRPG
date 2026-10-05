# Content Provenance Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#76 PKG-5](https://github.com/darkliquid/LocalRPG/issues/76)
**Epic:** [#24 Content packages and registries](https://github.com/darkliquid/LocalRPG/issues/24)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §6 (PKG-5)
**Depends on:** [#72 PKG-1](https://github.com/darkliquid/LocalRPG/issues/72)
**Scope:** `pkg/content`, `pkg/config`, `pkg/registry`, `cmd/localrpg`, `frontend`

---

## 1. Problem

PKG-1 verifies a package's **integrity**: every file's checksum is checked on unpack, so a corrupted
package is caught. It says nothing about **provenance**: a package can be built by anyone and
checksum-verified, because the checksum comes from the package itself. A registry index adds a
download checksum (PKG-4), which ties the download to the index, but not to a publisher.

So a user cannot tell whether the world they just installed was written by the author the index
claims. For content that runs JavaScript, that gap matters.

## 2. Goals

- A package can carry a **signature** over its digest.
- The app can **verify** a signature and report a trust state.
- A user can **trust** a publisher's key, and the trust is remembered.
- Signing and verifying are available from the CLI.
- An unsigned package still installs; it is labelled honestly.

## 3. Non-goals

- A certificate authority or a web of trust. Trust is explicit, per key.
- Encrypting content. Signatures are integrity and attribution, not secrecy.
- Signing the registry index itself (a possible follow-up).

## 4. Design

### 4.1 The signature

A signature is an Ed25519 signature over the package's **digest**, which PKG-1 already defines (the
SHA-256 of the packed bytes is not stable across gzip options, so the signed value is the
**content digest**: the hash of the manifest's sorted `Files` entries, which is stable).

A signed package carries one extra tar member, `package.sig`:

```yaml
# package.sig
algorithm: ed25519
key: "a1b2c3…"        # the SHA-256 fingerprint of the public key
signature: "base64…"  # the Ed25519 signature over the content digest
publisher: "someone"  # optional, for display only
```

`package.sig` is excluded from `Files` and from the digest, so adding it does not change what is
signed.

### 4.2 The content digest

`pkg/content` gains:

```go
// ContentDigest returns the stable digest of a manifest's files: the SHA-256 of
// the sorted "path\x00sha256\x00size" lines.
func (m Manifest) ContentDigest() string
```

This is the value a publisher signs and a verifier checks, independent of tar and gzip details.

### 4.3 Signing and verification

```go
// Sign returns the package.sig contents for a manifest and a private key.
func Sign(m Manifest, key ed25519.PrivateKey, publisher string) ([]byte, error)

// Verify checks a package.sig against a manifest and returns the trust state.
func Verify(m Manifest, sig []byte, trusted map[string]string) (Trust, error)

// Trust is the outcome of a provenance check.
type Trust struct {
	State       string // "verified" | "unknown_key" | "unsigned" | "invalid"
	Publisher   string
	Fingerprint string
}
```

- `verified`: the signature is valid and the fingerprint is in `trusted`.
- `unknown_key`: the signature is valid but the fingerprint is not trusted (installs with a warning).
- `unsigned`: no `package.sig` (installs with a note).
- `invalid`: the signature does not verify against the digest (refused).

### 4.4 Trusted publishers

`Config` gains:

```go
	// Publishers maps a key fingerprint (SHA-256, hex) to a display name.
	Publishers map[string]string `yaml:"publishers,omitempty"`
```

The CLI manages it: `localrpg publisher add <pubkey-file> [--name x]`, `list`, `remove`. The GUI's
import confirmation shows the state and offers "trust this publisher" on an `unknown_key`.

### 4.5 The registry

A PKG-4 index entry may carry a `publisher` fingerprint, so the registry view can show the state
before install. The index's `sha256` (download integrity) and the package's signature (provenance)
are complementary and both checked.

### 4.6 Where it shows

- The import confirmation: a trust chip (`verified by someone`, `unknown key`, `unsigned`), with the
  publisher name when known.
- The registry list: the same chip per package.
- The CLI: a line in the import summary and a non-zero exit on `invalid`.

## 5. Behaviour

| Package | State | Install |
| --- | --- | --- |
| signed, key trusted | verified | proceeds |
| signed, key unknown | unknown_key | proceeds with a warning |
| unsigned | unsigned | proceeds with a note |
| signed, bad signature | invalid | refused |
| signed, digest mismatch | invalid | refused |

## 6. Testing

- `pkg/content`: `ContentDigest` is stable across packing; `Sign`/`Verify` round-trip; a tampered
  manifest fails verification; an unknown key reports `unknown_key`; a missing signature reports
  `unsigned`.
- `pkg/gui`/`pkg/registry`: the trust state reaches the import confirmation and the registry list.
- `cmd/localrpg`: `content sign`, `content verify`, and `publisher add|list|remove`.
- A regression guard: an unsigned package installs exactly as before.

## 7. Rollout

Additive. Unsigned packages behave as they do today. Signing is opt-in for publishers, trusting is
opt-in for users.

## 8. Risks

- **Signature theatre.** A verified signature means "this key signed this content", not "this content
  is safe". The UI must say so; the trust chip names the publisher, not a promise of quality.
- **Key management.** Losing a signing key means publishing under a new fingerprint; users re-trust.
  Documented, not solved.
- **Digest definition.** If the digest ever changes, old signatures break. The definition is pinned
  by a test and treated as a format constant.
