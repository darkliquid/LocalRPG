# Content Registry Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#75 PKG-4](https://github.com/darkliquid/LocalRPG/issues/75)
**Epic:** [#24 Content packages and registries](https://github.com/darkliquid/LocalRPG/issues/24)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §6 (PKG-4)
**Depends on:** [#72 PKG-1](https://github.com/darkliquid/LocalRPG/issues/72), [#73 PKG-2](https://github.com/darkliquid/LocalRPG/issues/73), [#74 PKG-3](https://github.com/darkliquid/LocalRPG/issues/74)
**Scope:** new `pkg/registry`, `pkg/config`, `cmd/localrpg`, `pkg/gui`, `frontend`

---

## 1. Problem

With packages (PKG-1), import/export (PKG-2), and versioning (PKG-3), content can move between two
machines by hand. There is still no way to **discover** or **fetch** content: no index, no subscribe,
no update. The deep-dive's "world registries" and "systems repositories" are unreachable.

## 2. Goals

- A `registries` configuration listing index URLs.
- A **static index** format: a JSON catalogue of packages with enough to choose and fetch one.
- A client that **searches**, **installs**, and **updates**, reusing PKG-2's install and PKG-1's
  verification.
- Works offline: the index is cached and a previously fetched package is installable without a
  network.
- A **git-based** registry works without new server software, so a user can publish by pushing a
  repository.

## 3. Non-goals

- Hosting or moderation. The client reads an index; who publishes it is out of scope.
- Signatures (PKG-5); checksums (PKG-1) are the baseline.
- A full package manager's dependency solver; PKG-3's constraints are checked, not solved.

## 4. Design

### 4.1 The index

A registry is a JSON document at a URL (or a path in a git repository):

```json
{
  "name": "LocalRPG Community",
  "homepage": "https://example.org/localrpg",
  "packages": [
    {
      "type": "world",
      "id": "ashen_reach",
      "name": "Ashen Reach",
      "version": "1.2.0",
      "description": "A dying frontier.",
      "author": "someone",
      "license": "CC-BY-4.0",
      "download": "https://example.org/packages/ashen_reach-1.2.0.lrpgpack",
      "sha256": "…",
      "requires": [{ "type": "system", "id": "narrative_2d6", "version": ">=1.0.0" }]
    }
  ]
}
```

`download` is an absolute URL or a path relative to the index. `sha256` is the package digest from
PKG-1.

### 4.2 The client

`pkg/registry` (a leaf importing `pkg/content`, `pkg/core`, and the standard library):

```go
// Registry is one configured index.
type Registry struct {
	Name    string
	URL     string
	Index   Index
	Fetched time.Time
}

// Client fetches, caches, and installs from configured registries.
type Client struct { /* … */ }

func NewClient(cfg config.RegistriesConfig, cacheDir string, logger trace.Logger) *Client

// Search returns matching packages across every registry.
func (c *Client) Search(ctx context.Context, query string) ([]PackageRef, error)

// Install downloads, verifies, and installs a package.
func (c *Client) Install(ctx context.Context, ref PackageRef, onConflict string) (content.Manifest, error)

// Update returns packages with a newer version than what is installed.
func (c *Client) Update(ctx context.Context) ([]PackageRef, error)
```

- **Fetch**: GET the index, cache it under the app cache with its ETag/Last-Modified, and revalidate.
- **Search**: substring/prefix match on id, name, description, author across the cached indexes.
- **Install**: resolve the download URL, fetch to a temp file, verify the SHA-256 against the index,
  then hand the file to PKG-2's `ImportContent` (staging, validate, conflict).
- **Update**: for each installed content package, find a higher version in any index; report it (and
  install on request). Version comparison is PKG-3's semver.

### 4.3 Git registries

A registry URL may be a git repository. The client clones (or fetches) it shallowly into the cache
and reads `index.json` at the root (or a configured path). This lets a publisher use GitHub Pages, a
raw file, or a git repo without running a server. A `git+https://…` scheme selects it; a plain
`https://…` fetches the JSON directly.

### 4.4 Offline

- The index is cached; `search` works from the cache when the network is unavailable.
- A package whose `.lrpgpack` has been fetched before is cached and installable offline.
- The app's offline intent (LF-4's verify) does not apply here; a registry is a network feature by
  nature, and the client says so.

### 4.5 Surfaces

- **CLI**: `localrpg registry add|list|remove <url>`, `localrpg registry search <query>`,
  `localrpg registry install <id>[@version]`, `localrpg registry update`.
- **GUI**: a **Registry** view in the launcher that lists packages from the configured indexes with
  search, an install action (reusing PKG-2's confirmation, including the `mechanics.js` warning), and
  an update indicator.

### 4.6 Trust

Install goes through PKG-2, so the manifest summary and the script warning are unchanged. The index's
`sha256` is verified before install; a mismatch is refused. PKG-5 adds signatures on top.

## 5. Behaviour

| Action | Result |
| --- | --- |
| search "ash" | matching packages from every index |
| install a world | downloaded, checksum-verified, installed via PKG-2 |
| install with an id clash | PKG-2's conflict choice |
| install a package with a bad checksum | refused |
| update | packages with a newer version than installed |
| offline search | served from the cached index |
| a git registry | cloned and read |
| an unreachable registry | reported; other registries still work |

## 6. Testing

- `pkg/registry`: parsing an index; search across two indexes; install verifies the checksum and
  refuses a mismatch; update finds a newer version; a fetch failure for one registry does not fail
  the others; the index cache is used offline.
- `cmd/localrpg`: the registry verbs.
- `pkg/gui`: the registry list and install reach the client.
- A regression guard: a config with no registries shows an empty list and no errors.

## 7. Rollout

Additive: a new package, a config field, CLI verbs, and a launcher view. No registries are configured
by default.

## 8. Risks

- **Untrusted code.** A registry package can contain `mechanics.js`. PKG-2's confirmation is the
  control; the index's checksum is integrity, not trust. PKG-5 is the trust layer.
- **Network in a local-first app.** A registry is explicitly a network feature, off by default; the
  UI says when it is fetching.
- **Index staleness.** The cache is revalidated; a stale index may offer a version that no longer
  downloads, which is reported at install, not search.
