# Content Import and Export Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#73 PKG-2](https://github.com/darkliquid/LocalRPG/issues/73)
**Epic:** [#24 Content packages and registries](https://github.com/darkliquid/LocalRPG/issues/24)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §6 (PKG-2)
**Depends on:** [#72 PKG-1](https://github.com/darkliquid/LocalRPG/issues/72)
**Scope:** `pkg/gui`, `cmd/localrpg`, `frontend`

---

## 1. Problem

PKG-1 defines a `.lrpgpack` and a safe pack/unpack. Nothing produces or consumes one: a user cannot
export a world to send it, nor import one they were given. Content still moves by copying folders by
hand, and the CLI has no content verbs.

## 2. Goals

- Export a system or world as a `.lrpgpack` from the CLI and the studio.
- Import a package from the CLI and the studio, with checksum verification and conflict handling.
- Never overwrite silently: an id clash is refused or renamed by explicit choice.
- Be explicit that an imported package can run JavaScript.

## 3. Non-goals

- Version resolution and lockfiles (PKG-3).
- Registries (PKG-4) and signatures (PKG-5).
- The pack format itself (PKG-1).

## 4. Design

### 4.1 Export

```
POST /api/content/export   { "type": "world"|"system", "id": "<id>" }
```

Streams the `.lrpgpack` with `Content-Type: application/gzip` and
`Content-Disposition: attachment; filename="<id>-<version>.lrpgpack"`. The handler resolves the
content directory through `core.PathResolver` and calls `content.Pack`.

CLI: `localrpg content export <world|system> <id> [--out <file>]`. Without `--out`, the archive
goes to stdout so it can be piped.

### 4.2 Import

```
POST /api/content/import   multipart/form-data: file=<package>
                           query/body: on_conflict=refuse|rename|overwrite
```

The handler streams the upload to a temporary file, calls `content.Unpack` into a **staging**
directory (never the live content directory), then validates and installs:

1. **Validate**: the staged tree has the expected manifest (`world.yaml`/`system.yaml`), the id
   matches the package id, and the content parses (a `core.LoadWorldManifest`/`LoadSystemManifest`).
2. **Conflict**: if `systems/<id>` or `worlds/<id>` exists:
   - `refuse` (default): a `409` with the existing id;
   - `rename`: install under `<id>-<n>` and rewrite the manifest id;
   - `overwrite`: replace the existing directory, after a backup to a temp path for the duration of
     the swap.
3. **Install**: rename the staged directory into place atomically.

CLI: `localrpg content import <file> [--on-conflict refuse|rename|overwrite]`.

### 4.3 Trust

Importing a world or system can install a `mechanics.js` that the engine will execute. The import
response includes the package's manifest (id, name, version, author, license, file count, whether it
contains a `mechanics.js`), and the studio shows a confirmation naming those facts before installing.
The CLI prints the same summary and requires `--yes` unless `--on-conflict` is given.

The engine's existing JS sandbox is the runtime control; this is the informed-consent control.

### 4.4 The studio

- **Export**: an action on a system or world in the studio (and on a campaign's world) that downloads
  the package.
- **Import**: an action in the launcher and both studios that opens a file picker, uploads, shows the
  manifest summary, and offers the conflict choice when needed.

### 4.5 Rebuildability

After an install, the content directory is authoritative and the cache is rebuilt on next open
(`Timeline.EnsureIndexed`), so an imported world's entities are indexed like any other. No new
rebuild path is needed.

## 5. Behaviour

| Action | Result |
| --- | --- |
| export a world | a `.lrpgpack` download |
| import a new world | installed; indexed on next open |
| import an existing id, refuse | `409`, nothing written |
| import an existing id, rename | installed as `<id>-<n>` |
| import an existing id, overwrite | the old directory is replaced |
| import a tampered package | `400`, nothing written |
| import a package with a `mechanics.js` | a confirmation names it before install |

## 6. Testing

- `pkg/gui`: export streams a valid package; import installs a new world; refuse/rename/overwrite
  behave per the table; a tampered package is `400`; the response manifest reports `has_script`.
- `cmd/localrpg`: export writes a file or stdout; import installs and refuses without `--yes`.
- `frontend`: the export action downloads; the import action shows the manifest and the conflict
  choice.
- A regression guard: exporting then importing a world reproduces it byte-for-byte (modulo the id).

## 7. Rollout

New endpoints, CLI verbs, and studio actions. Additive; no existing content is touched until an
import runs.

## 8. Risks

- **Overwrite data loss.** `overwrite` replaces a directory. It backs up for the duration of the swap
  and is never the default; the UI requires an explicit choice.
- **Zip-bomb-style archives.** PKG-1 bounds size and member count; the upload also has a request-size
  limit.
- **Trust messaging.** A user who clicks through the confirmation runs code. The summary is explicit
  and names `mechanics.js` when present; that is the best a local-first app can do.
