# Content Package Format Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A deterministic `.lrpgpack` archive with a checksummed manifest, packed and unpacked safely by a new `pkg/content` leaf.

**Architecture:** gzip-compressed tar with `package.yaml` first and sorted members; `Pack`/`Unpack` verify checksums and reject unsafe paths; a golden digest pins the format.

**Tech Stack:** Go standard library (`archive/tar`, `compress/gzip`, `crypto/sha256`); `gopkg.in/yaml.v3`.

**Spec:** `docs/superpowers/specs/2026-10-05-content-package-format-design.md`

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `any`, not `interface{}`, in Go.
- Deterministic output: sorted members, zero mtimes, zeroed gzip mtime.
- Unpack rejects `..`, absolute paths, and symlinks, and bounds size and member count.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The manifest

**Files:**
- Create: `pkg/content/manifest.go`
- Test: `pkg/content/manifest_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `Manifest`, `Dependency`, `FileEntry`, `ManifestMeta`, `func ParseManifest([]byte) (Manifest, error)`, `func (m Manifest) Marshal() ([]byte, error)`.

- [ ] **Step 1: Write the failing test**

```go
package content

import "testing"

func TestManifestRoundTrip(t *testing.T) {
	m := Manifest{ID: "ashen_reach", Name: "Ashen Reach", Version: "1.0.0", Type: "world",
		Files: []FileEntry{{Path: "world.yaml", SHA256: "ab", Size: 3}}}
	b, err := m.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseManifest(b)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != m.ID || len(got.Files) != 1 {
		t.Fatalf("manifest = %+v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/content/ -run TestManifestRoundTrip -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add the types from the spec §4.2 and the marshal/parse helpers (`yaml.Marshal`/`yaml.Unmarshal`).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/content/ -run TestManifestRoundTrip -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/content/manifest.go pkg/content/manifest_test.go
git commit -m "feat(content): add the package manifest"
```

---

### Task 2: `Pack`

**Files:**
- Create: `pkg/content/pack.go`
- Test: `pkg/content/pack_test.go`

**Interfaces:**
- Consumes: `Manifest` (Task 1), `core.LoadWorldManifest`/`LoadSystemManifest`.
- Produces: `func Pack(dir, typ string, meta ManifestMeta, w io.Writer) (Manifest, error)`.

- [ ] **Step 1: Write the failing test**

```go
func TestPackProducesManifestAndMembers(t *testing.T) {
	dir := writeFixtureWorld(t) // a world.yaml and one entity
	var buf bytes.Buffer
	m, err := Pack(dir, "world", ManifestMeta{}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if m.ID == "" || len(m.Files) == 0 {
		t.Fatalf("manifest = %+v", m)
	}
	if !bytes.Contains(buf.Bytes(), []byte("package.yaml")) {
		t.Fatal("package.yaml should be the first member")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/content/ -run TestPack -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Walk `dir` (skipping dot-dirs, `cache/`, `.legacy`), compute a SHA-256 and size per file, fill the
manifest from the content's own manifest when meta is empty, and write `package.yaml` then the
members in sorted order with zero mtimes and a zeroed gzip mtime.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/content/ -run TestPack -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/content/pack.go pkg/content/pack_test.go
git commit -m "feat(content): pack a content tree"
```

---

### Task 3: `Unpack`

**Files:**
- Create: `pkg/content/unpack.go`
- Test: `pkg/content/unpack_test.go`

**Interfaces:**
- Consumes: `Manifest` (Task 1), `pathutil.ResolveSafeChild`.
- Produces: `func Unpack(r io.Reader, destDir string) (Manifest, error)`.

- [ ] **Step 1: Write the failing tests**

```go
func TestUnpackRoundTrip(t *testing.T) {
	dir := writeFixtureWorld(t)
	var buf bytes.Buffer
	if _, err := Pack(dir, "world", ManifestMeta{}, &buf); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	if _, err := Unpack(&buf, dest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "world.yaml")); err != nil {
		t.Fatal("world.yaml missing after unpack")
	}
}

func TestUnpackRejectsTraversalAndTamper(t *testing.T) {
	// A member with "../evil" and a member whose checksum does not match both fail
	// and leave the destination empty.
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/content/ -run TestUnpack -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Read `package.yaml`, then each member: resolve the path under `destDir` with
`pathutil.ResolveSafeChild`, reject symlinks and absolute paths, verify SHA-256 and size, bound the
total size and member count, and write into a temporary directory that is renamed into place on
success.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/content/ -run TestUnpack -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/content/unpack.go pkg/content/unpack_test.go
git commit -m "feat(content): unpack a package safely"
```

---

### Task 4: Determinism and the golden digest

**Files:**
- Test: `pkg/content/determinism_test.go`

**Interfaces:**
- Consumes: `Pack`, `Unpack`.
- Produces: no production symbols.

- [ ] **Step 1: Write the tests**

```go
func TestPackIsDeterministic(t *testing.T) {
	dir := writeFixtureWorld(t)
	a := packToBytes(t, dir)
	b := packToBytes(t, dir)
	if !bytes.Equal(a, b) {
		t.Fatal("packing twice produced different bytes")
	}
}

func TestPackGoldenDigest(t *testing.T) {
	dir := writeFixtureWorld(t)
	sum := sha256.Sum256(packToBytes(t, dir))
	if got := hex.EncodeToString(sum[:]); got != goldenWorldDigest {
		t.Fatalf("digest = %s, want %s", got, goldenWorldDigest)
	}
}
```

- [ ] **Step 2: Run them**

Run: `go test ./pkg/content/ -run 'TestPackIsDeterministic|TestPackGoldenDigest' -v`
Expected: PASS once `goldenWorldDigest` is filled from the first run.

- [ ] **Step 3: Record the golden value**

Run the golden test once, copy the printed digest into the constant, and re-run.

- [ ] **Step 4: Property test**

Add `TestUnpackReproducesTree` that packs and unpacks a set of generated small trees and asserts the
file sets and contents match.

- [ ] **Step 5: Commit**

```bash
git add pkg/content/determinism_test.go
git commit -m "test(content): pin the package format determinism"
```

---

### Task 5: Verification

- [ ] **Step 1: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 2: Confirm the acceptance criteria**

- A `.lrpgpack` packs and unpacks.
- Packing twice is byte-identical.
- A tampered, oversized, or traversal member fails safely.
- The manifest carries a checksum per file and identity from the content.

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "chore: finalise the content package format"
```
