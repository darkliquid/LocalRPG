# Content Versioning Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#74 PKG-3](https://github.com/darkliquid/LocalRPG/issues/74)
**Epic:** [#24 Content packages and registries](https://github.com/darkliquid/LocalRPG/issues/24)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §6 (PKG-3)
**Depends on:** [#72 PKG-1](https://github.com/darkliquid/LocalRPG/issues/72)
**Scope:** `pkg/core`, new `pkg/content` additions, `pkg/engine`, `pkg/gui`

---

## 1. Problem

Versions are free text. `SystemManifest.Version` and `WorldManifest.Version` are plain strings
(`pkg/core/types.go:12-52`) with no semantics: `"1.0"`, `"v1"`, and `"latest"` are all accepted, and
nothing compares them. A world cannot say which system it needs, and a campaign cannot record which
versions it played against.

So an imported world can break when its system changes, and a campaign cannot be reproduced. PKG-1
gives a package a version field; nothing enforces or resolves it.

## 2. Goals

- Content versions are **semver**, enforced at load and at pack.
- A world declares the **system it requires**, with a version constraint.
- A campaign records a **lockfile** of the resolved content versions and checksums it was played
  against, so a save is reproducible.
- Opening a campaign whose content has changed is **surfaced**, not silently accepted.

## 3. Non-goals

- Registries (PKG-4) and signatures (PKG-5).
- Automatic updates; the lockfile records, it does not fetch.
- Migrating content between versions.

## 4. Design

### 4.1 Semver enforcement

`golang.org/x/mod/semver` is already in the module graph (indirect), so it is the parser of choice.

- `core.SystemManifest.Version` and `WorldManifest.Version` must be valid semver (`semver.IsValid`).
  A load that finds an invalid version reports it; a missing version is treated as `0.0.0` for
  comparison but flagged as "unversioned".
- `content.Pack` refuses to pack content with an invalid version (PKG-1's manifest already carries it).

`config.Version` (the app config) is unrelated and untouched.

### 4.2 Declared dependencies

`WorldManifest` gains:

```go
	// Requires names the systems this world needs, with semver constraints.
	Requires []ContentRequirement `yaml:"requires,omitempty"`
```

```go
type ContentRequirement struct {
	Type    string `yaml:"type"`    // "system"
	ID      string `yaml:"id"`
	Version string `yaml:"version,omitempty"` // a semver constraint, e.g. ">=1.2.0 <2.0.0"
}
```

A world's `DefaultSystem` (already present, `pkg/core/types.go:47`) is the system it is played with;
`Requires` is the constraint on it. If `Requires` is empty, `DefaultSystem` is required with any
version.

### 4.3 The campaign lockfile

A campaign writes `games/<id>/content.lock.yaml` when it is created (and on any content change it
accepts):

```go
// ContentLock records the content versions a campaign was played against.
type ContentLock struct {
	App      string          `yaml:"app"`
	Entries  []LockEntry     `yaml:"entries"`
}

type LockEntry struct {
	Type    string `yaml:"type"` // "system" | "world"
	ID      string `yaml:"id"`
	Version string `yaml:"version"`
	SHA256  string `yaml:"sha256"` // the package digest, when known, else the directory digest
}
```

`SHA256` is the content directory's deterministic digest (a hash of its file checksums, the same
computation PKG-1's manifest uses), so a change to a system's `mechanics.js` changes the lock.

The lock is written by `engine.InitGame` at creation and by `localrpg play`/the GUI on open when the
content differs and the user accepts the change.

### 4.4 Resolution and mismatch

On campaign open:

1. Read the lock.
2. Compute the current content's version and digest.
3. If they match the lock, proceed.
4. If the world's `Requires` is no longer satisfied by the current system, **refuse** to play with a
   clear message naming the constraint, the required system, and the current version.
5. If a digest changed but the constraint still holds, **surface** the change (a warning in the GUI,
   a prompt in the CLI) and offer to update the lock.

A refusal is a hard stop (the content is incompatible); a change is a warning (the content is
compatible but different). Both are explicit.

### 4.5 The lock is not authoritative

The lock records what was played; the content directories remain authoritative. Deleting the lock
re-locks on next open. The lock never fetches or installs; that is PKG-4.

## 5. Behaviour

| Situation | Result |
| --- | --- |
| a system with `version: 1.2.0` | loads |
| a system with `version: latest` | a load error naming the invalid semver |
| a world requiring `>=1.0.0 <2.0.0` and a 1.4 system | plays |
| the same world and a 2.0 system | refused, naming the constraint |
| a compatible but changed system | a warning and a lock-update offer |
| a campaign with no lock | locks on open |
| a campaign whose lock matches | plays silently |

## 6. Testing

- `pkg/core`: a manifest with an invalid version reports it; a valid one loads; `Requires` parses.
- `pkg/content`: `Pack` refuses an invalid version; a directory digest is deterministic.
- `pkg/engine`: `InitGame` writes a lock; opening with a matching lock is silent; a violated
  constraint refuses; a changed digest warns.
- `pkg/gui`: the mismatch warning reaches the client.
- A regression guard: a campaign created before this change (no lock) opens and locks.

## 7. Rollout

Additive: `Requires` is optional, and a campaign without a lock gains one on open. A world with no
`Requires` behaves as today (its default system, any version). Invalid versions are now load errors,
which is a behaviour change; the migration is to fix the version string.

## 8. Risks

- **A refusal that blocks a playable campaign.** A world with a strict `Requires` and a bumped system
  refuses. That is the point, but the message must say exactly what to do (update the world, or
  relax the constraint).
- **Digest churn.** Editing a system's prose changes its digest and warns on every campaign. Consider
  hashing only the behavioural files (`system.yaml`, `mechanics.js`) for the lock, not the rules
  prose, so a documentation edit does not warn. The spec chooses behavioural files only.
- **Constraint syntax.** Using semver's own constraint grammar (`golang.org/x/mod/semver` supports
  `>=`, `<`, and ranges via `semver.Compare`) keeps it standard; a full constraint parser is
  deferred.
