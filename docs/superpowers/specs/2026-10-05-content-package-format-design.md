# Content Package Format Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#72 PKG-1](https://github.com/darkliquid/LocalRPG/issues/72)
**Epic:** [#24 Content packages and registries](https://github.com/darkliquid/LocalRPG/issues/24)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §6 (PKG-1)
**Scope:** new `pkg/content`, `pkg/core`, `pkg/pathutil`

---

## 1. Problem

A system or a world is a directory. To share one, a user copies a folder; to receive one, they paste
it in. There is no archive, no manifest, no checksum, and no version:

- `systems/<id>/` and `worlds/<id>/` are plain trees (`pkg/core/types.go:76-149`);
- `SystemManifest.Version` is free text with no semantics (`pkg/core/types.go:14`);
- archive handling exists only for model downloads (`pkg/models/manager.go:419-457`);
- there is no import or export of content anywhere (`pkg/export` is story replay only).

So content cannot be published, versioned, verified, or installed, and everything downstream
(registries, generation output, sharing) has nothing to stand on.

## 2. Goals

- A single-file **content package** for a system or a world.
- A **manifest** carrying identity, version, type, dependencies, license, author, and a checksum per
  file.
- Deterministic packing (the same tree yields the same bytes) so a package can be content-addressed.
- Safe unpacking: no path traversal, no absolute paths, bounded size.
- A library both the CLI and the GUI use, with no GUI dependency.

## 3. Non-goals

- Installing into a live environment, conflicts, and updates (PKG-2).
- Version resolution and lockfiles (PKG-3).
- Registries (PKG-4) and signatures (PKG-5).

## 4. Design

### 4.1 The format

A `.lrpgpack` is a **gzip-compressed tar** with a fixed member order. The first member is
`package.yaml`; the rest are the content tree, sorted by path.

```
package.yaml
world.yaml            (or system.yaml)
prompts/lore.md
entities/*.md
assets/*              (banner, icon, portraits — if present)
```

Tar is chosen over zip for deterministic ordering, streaming, and standard-library support without a
new dependency.

### 4.2 The manifest

`pkg/content` (a new leaf importing only `pkg/core` and the standard library):

```go
// Manifest describes a content package.
type Manifest struct {
	ID           string       `yaml:"id"`
	Name         string       `yaml:"name"`
	Version      string       `yaml:"version"`
	Type         string       `yaml:"type"` // "system" | "world"
	Description  string       `yaml:"description,omitempty"`
	Author       string       `yaml:"author,omitempty"`
	License      string       `yaml:"license,omitempty"`
	MinApp       string       `yaml:"min_app_version,omitempty"`
	Dependencies []Dependency `yaml:"dependencies,omitempty"`
	Files        []FileEntry  `yaml:"files"`
}

// Dependency is a required system or world and its version constraint.
type Dependency struct {
	Type    string `yaml:"type"` // "system" | "world"
	ID      string `yaml:"id"`
	Version string `yaml:"version,omitempty"` // semver constraint, PKG-3
}

// FileEntry records a file's path and checksum.
type FileEntry struct {
	Path   string `yaml:"path"`
	SHA256 string `yaml:"sha256"`
	Size   int64  `yaml:"size"`
}
```

`Files` is sorted by path and covers every member except `package.yaml` itself.

### 4.3 Packing

```go
// Pack writes dir as a .lrpgpack to w. typ is "system" or "world".
func Pack(dir, typ string, meta ManifestMeta, w io.Writer) (Manifest, error)

// Unpack reads a package into a destination directory, verifying every checksum.
func Unpack(r io.Reader, destDir string) (Manifest, error)
```

`Pack`:

- reads the content tree, skipping dot-directories, `cache/`, and any `.legacy` file (the same rules
  the storage sync uses, `pkg/storage/sync.go:30-79`);
- computes a SHA-256 per file;
- builds the manifest, filling `ID`/`Name`/`Version`/`Type` from the content's own manifest
  (`world.yaml`/`system.yaml`) when the caller does not override them;
- writes `package.yaml` first, then the members in sorted order, with a fixed mtime (zero) so the
  bytes are reproducible.

`Unpack`:

- reads and validates `package.yaml`;
- for each member, resolves the path under `destDir` with `pathutil.ResolveSafeChild`
  (`pkg/pathutil/pathutil.go:71-87`), rejecting `..`, absolute paths, and symlinks;
- verifies the SHA-256 and size against the manifest before writing;
- bounds the total size and the member count so a malicious package cannot exhaust the disk;
- writes to a temporary directory and renames into place, so a failed unpack leaves nothing partial.

### 4.4 Determinism

The same content tree must yield byte-identical packages, so a package can be content-addressed (a
registry can dedupe, and PKG-5 can sign a stable digest). The rules: sorted members, zero mtimes,
no owner/group names, and a fixed tar format (GNU or USTAR) with the gzip header's mtime zeroed.

### 4.5 The library boundary

`pkg/content` is a leaf: it imports `pkg/core` (for `SystemManifest`/`WorldManifest`) and
`pkg/pathutil` (for safe paths), and nothing from `pkg/gui` or `pkg/engine`. The CLI and the GUI both
use it, and the GUI's import/export endpoints (PKG-2) are thin wrappers.

## 5. Behaviour

| Action | Result |
| --- | --- |
| pack a world | a `.lrpgpack` with `package.yaml` first and sorted members |
| pack the same tree twice | byte-identical output |
| unpack a valid package | the tree in `destDir`, every checksum verified |
| unpack a tampered member | an error; nothing written |
| unpack a `../` path | an error; rejected before writing |
| unpack a package over the size bound | an error |
| unpack into a directory that exists | refused unless the caller allows an overwrite (PKG-2) |

## 6. Testing

- `pkg/content`: packing a fixture tree yields a manifest with a checksum per file; packing twice is
  byte-identical; unpacking round-trips; a tampered member fails; a `../` member fails; an oversized
  package fails; a symlink member fails.
- `pkg/content`: the manifest's `ID`/`Name`/`Version`/`Type` are taken from the content manifest when
  not overridden.
- A property test: `Unpack(Pack(tree))` reproduces the tree for arbitrary small trees.
- A golden test: a fixed tree packs to a fixed SHA-256, so a format change is caught.

## 7. Rollout

A new package and format. No existing behaviour changes; nothing reads a package until PKG-2.

## 8. Risks

- **Security is the headline.** An imported package is untrusted input that can contain
  `mechanics.js` (executed by the engine) and Markdown (parsed). This spec makes the *archive* safe
  (paths, size, checksums); PKG-2 makes the *install* explicit about running code.
- **Format churn.** A golden digest pins the format; changing it is a deliberate, tested act.
- **Cross-platform paths.** Tar paths use `/`; the unpacker converts to the OS separator via
  `filepath.FromSlash` and re-validates, so a Windows path cannot escape.
