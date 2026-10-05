# Content Provenance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Sign a package's content digest with Ed25519, verify it on import, and report a trust state.

**Architecture:** `pkg/content` gains a stable `ContentDigest`, `Sign`, `Verify`, and a `Trust` value; a `publishers` config holds trusted fingerprints; import and the registry show the state.

**Tech Stack:** Go standard library (`crypto/ed25519`, `crypto/sha256`, `encoding/base64`).

**Spec:** `docs/superpowers/specs/2026-10-05-content-provenance-design.md`
**Depends on:** PKG-1.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `interface{}`, not `any`, in Go.
- The content digest definition is a format constant; pin it with a test.
- An unsigned package installs exactly as before.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The content digest

**Files:**
- Modify: `pkg/content/manifest.go`
- Test: `pkg/content/digest_test.go`

**Interfaces:**
- Consumes: `Manifest.Files`.
- Produces: `func (m Manifest) ContentDigest() string`.

- [ ] **Step 1: Write the failing test**

```go
func TestContentDigestIsStable(t *testing.T) {
	m := Manifest{Files: []FileEntry{
		{Path: "b", SHA256: "2", Size: 2},
		{Path: "a", SHA256: "1", Size: 1},
	}}
	reordered := Manifest{Files: []FileEntry{
		{Path: "a", SHA256: "1", Size: 1},
		{Path: "b", SHA256: "2", Size: 2},
	}}
	if m.ContentDigest() != reordered.ContentDigest() {
		t.Fatal("the digest must not depend on file order")
	}
	if m.ContentDigest() == (Manifest{Files: []FileEntry{{Path: "a", SHA256: "9", Size: 1}}}).ContentDigest() {
		t.Fatal("a changed checksum must change the digest")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/content/ -run TestContentDigestIsStable -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Sort the entries by path, hash `path\x00sha256\x00size\n` lines with SHA-256, and return the hex
string.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/content/ -run TestContentDigestIsStable -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/content/manifest.go pkg/content/digest_test.go
git commit -m "feat(content): add a stable content digest"
```

---

### Task 2: Sign, Verify, and Trust

**Files:**
- Create: `pkg/content/provenance.go`
- Test: `pkg/content/provenance_test.go`

**Interfaces:**
- Consumes: `ContentDigest` (Task 1).
- Produces: `Trust`, `func Sign(m Manifest, key ed25519.PrivateKey, publisher string) ([]byte, error)`, `func Verify(m Manifest, sig []byte, trusted map[string]string) (Trust, error)`.

- [ ] **Step 1: Write the failing tests**

```go
func TestSignVerifyRoundTrip(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	m := Manifest{Files: []FileEntry{{Path: "a", SHA256: "1", Size: 1}}}
	sig, err := Sign(m, priv, "someone")
	if err != nil {
		t.Fatal(err)
	}
	fp := fingerprint(pub)
	tr, err := Verify(m, sig, map[string]string{fp: "someone"})
	if err != nil || tr.State != "verified" || tr.Publisher != "someone" {
		t.Fatalf("trust = %+v err %v", tr, err)
	}
	if tr, _ := Verify(m, sig, nil); tr.State != "unknown_key" {
		t.Fatalf("untrusted key state = %q", tr.State)
	}
}

func TestVerifyRejectsTamperedManifest(t *testing.T) { /* a changed file fails */ }
func TestVerifyUnsigned(t *testing.T) { /* empty sig -> unsigned */ }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/content/ -run TestSignVerify -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Implement the `package.sig` YAML, `Sign` over the content digest, and `Verify` returning the four
states. Add a `fingerprint(pub)` helper (SHA-256 of the public key, hex).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/content/ -run TestVerify -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/content/provenance.go pkg/content/provenance_test.go
git commit -m "feat(content): sign and verify a package"
```

---

### Task 3: Pack and unpack carry the signature

**Files:**
- Modify: `pkg/content/pack.go`, `pkg/content/unpack.go`
- Test: `pkg/content/pack_test.go` (append)

**Interfaces:**
- Consumes: Task 2.
- Produces: `Pack` includes `package.sig` when present; `Unpack` returns the signature alongside the manifest.

- [ ] **Step 1: Write the failing test**

```go
func TestPackIncludesSignature(t *testing.T) {
	// A directory with package.sig packs it as a member excluded from Files.
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/content/ -run TestPackIncludesSignature -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Treat `package.sig` as a reserved member: include it in the archive but exclude it from `Files` and
the digest. `Unpack` returns it (or an empty slice) so the caller can verify.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/content/ -run TestPackIncludesSignature -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/content/pack.go pkg/content/unpack.go pkg/content/pack_test.go
git commit -m "feat(content): carry a signature in the package"
```

---

### Task 4: Publishers and the CLI

**Files:**
- Modify: `pkg/config/types.go`
- Create: `cmd/localrpg/provenance.go`
- Modify: `cmd/localrpg/main.go`
- Test: `cmd/localrpg/provenance_test.go`

**Interfaces:**
- Consumes: `content.Sign`/`Verify`.
- Produces: `Config.Publishers`, `localrpg content sign|verify`, `localrpg publisher add|list|remove`.

- [ ] **Step 1: Write the failing tests**

```go
func TestPublisherAddListRemove(t *testing.T) { /* config publishers map round-trips */ }
func TestContentVerifyCLIReportsState(t *testing.T) { /* prints the trust state, non-zero on invalid */ }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/localrpg/ -run 'TestPublisher|TestContentVerify' -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add `Publishers map[string]string` to `Config`, and the CLI verbs: `sign` reads a private key file and
writes `package.sig`; `verify` prints the state and exits non-zero on `invalid`; `publisher` manages
the config map.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/localrpg/ -run 'TestPublisher|TestContentVerify' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/config cmd/localrpg
git commit -m "feat(cli): sign, verify, and trust publishers"
```

---

### Task 5: Surface the trust state

**Files:**
- Modify: `pkg/gui/service.go` (import), `pkg/gui/types.go`
- Modify: `frontend/src/components/ContentImportDialog.tsx`, `frontend/src/components/LauncherHub.tsx`
- Modify: `pkg/registry/index.go` (an optional publisher fingerprint)
- Test: `pkg/gui/provenance_test.go`

**Interfaces:**
- Consumes: `content.Verify` (Task 2).
- Produces: a `trust` field on the import result and the registry entry, rendered as a chip.

- [ ] **Step 1: Write the failing test**

```go
func TestImportReportsTrustState(t *testing.T) { /* the result carries verified/unknown_key/unsigned */ }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/gui/ -run TestImportReportsTrustState -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Verify during import and return the trust state; add a `publisher` field to the registry entry and
verify a downloaded package before install. Render a trust chip in the import dialog and the registry
list, with a "trust this publisher" action on `unknown_key`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/gui/ -run TestImportReportsTrustState -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui pkg/registry frontend/src
git commit -m "feat: show a package's trust state"
```

---

### Task 6: Verification

- [ ] **Step 1: Regression guard**

Add a test that an unsigned package installs exactly as before.

- [ ] **Step 2: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 3: Confirm the acceptance criteria**

- A signed package verifies against a trusted key.
- An unknown key installs with a warning; an invalid signature is refused.
- Signing and verifying work from the CLI.
- The trust state is shown on import and in the registry.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: guard the unsigned-package path"
```
