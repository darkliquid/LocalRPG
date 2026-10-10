# Content Registry Sources Design

**Date:** 2026-10-10
**Status:** Proposed
**Issue:** [#133](https://github.com/darkliquid/LocalRPG/issues/133)
**Epic:** [#125 Content registry discoverability](https://github.com/darkliquid/LocalRPG/issues/125)
**Depends on:** [Content Registry Design](2026-10-05-content-registry-design.md) (PKG-4)
**Scope:** `frontend`, `pkg/gui`, `pkg/registry`, `cmd/localrpg`, `pkg/gui/docs`

---

## 1. Problem

The Content Registry view is a dead end when nothing is configured, and it does not
say where its packages come from.

1. **Where items come from is invisible.** `RegistryModal`
   (`frontend/src/components/RegistryModal.tsx`) lists packages and never states that
   they were fetched from configured index URLs. Each card shows `registry_name`, but
   nothing explains that a registry is a user-configured URL, or that there are none
   configured by default.
2. **There is no way to set a source in the UI.** The only way to add a registry is
   `localrpg registry add <url>` (`cmd/localrpg/registry.go`), which appends to
   `config.Registries.URLs` (`pkg/config/types.go:451`). The GUI reads that config
   through `Service.registryClient()` (`pkg/gui/service.go:6176`) but never edits it.
3. **The empty search is nonsense.** `registry.Client.Search` treats an empty query as
   match-all (`pkg/registry/client.go:145`), so an empty box browses the catalogue.
   That is correct, but the UI renders
   `No packages found matching ""` (`RegistryModal.tsx:167`) whenever the catalogue is
   empty. The message both quotes an empty query and misdescribes the state: the real
   states are "no registries configured", "registries configured but empty", and "no
   match for the query".
4. **Search runs on every keystroke** with no debounce (`RegistryModal.tsx:56-60`),
   refetching the indexes on each character typed.

## 2. Goals

- The Registry view says where packages come from and lists the configured sources.
- A user can add and remove registry sources from the GUI, persisted to
  `registries.urls`, with the same validation and persistence the CLI uses.
- The empty states are distinct and truthful: no sources, empty catalogue, no match.
- An empty query browses everything and is labelled as such, not as a match on `""`.
- A source that fails to fetch is reported, so "nothing shows" has a reason.
- Search is debounced.

## 3. Non-goals

- Hosting, moderation, signatures (PKG-5), or a dependency solver; unchanged.
- Editing package contents. The source editor configures URLs only.
- Changing the index format or the `search`/`install`/`update` client behaviour beyond
  the empty-query labelling.

## 4. Design

### 4.1 Source management on the client

`pkg/registry` gains a URL normaliser, so the CLI and the GUI agree on what a valid
source is (DRY, and the register of accepted forms lives in one place):

```go
// NormalizeSource validates a registry URL and returns its canonical string.
// Accepted forms: https://…, http://…, git+https://…, git+ssh://…, git+file://…
// (the git+ scheme selects the clone path in fetchOrCachedIndex).
func NormalizeSource(raw string) (string, error)
```

`cmd/localrpg/registry.go`'s `runRegistryAdd` switches to `NormalizeSource` so the two
front ends share one acceptance rule.

### 4.2 Service methods and routes

Four thin methods on `gui.Service`, mirroring the CLI's load/mutate/`Save` shape and
resolving the config the same way `registryClient` does (`s.configMgr.Get()` /
`Save`):

```go
// RegistrySources lists configured registries with their fetch state.
func (s *Service) RegistrySources(ctx context.Context) ([]RegistrySourceDTO, error)
// AddRegistrySource normalises, de-duplicates, persists, and returns the source.
func (s *Service) AddRegistrySource(ctx context.Context, url string) (RegistrySourceDTO, error)
// RemoveRegistrySource persists the config without the URL; absent URL is a no-op error.
func (s *Service) RemoveRegistrySource(ctx context.Context, url string) error
```

`RegistrySources` reuses `registry.Client.Registries` (`pkg/registry/client.go:89`),
which already fetches every configured index and tolerates individual failures, so it
can report a resolved `name` and `package_count`, or the `error` for one that would not
load:

```go
type RegistrySourceDTO struct {
    URL          string `json:"url"`
    Name         string `json:"name,omitempty"`
    PackageCount int    `json:"package_count"`
    Error        string `json:"error,omitempty"`
}
```

`AddRegistrySource` returns `409` on a duplicate and `400` on an invalid URL (the
`NormalizeSource` error). `RemoveRegistrySource` returns `404` when the URL is not
configured, so a stale UI cannot silently "remove nothing".

Routes, added to `pkg/gui/routes.go` beside the existing registry routes:

```go
{"/api/registry/sources", "handleRegistrySourcesRoute", (*Server).handleRegistrySourcesRoute}
```

The handler dispatches `GET` (list), `POST` (`{ "url": … }`, add), and `DELETE`
(`?url=…`, remove).

### 4.3 The Registry view

`RegistryModal.tsx` gains a **Sources** panel above the search, collapsed to a summary
line by default (`3 sources`) and expandable. Expanded it shows one row per source:
resolved name or raw URL, package count, a per-source error when the fetch failed, and
a Remove button, plus a URL input and an Add button. Add and Remove call the new client
methods and then re-run `checkUpdates` and the current search.

The client (`frontend/src/api/client.ts`) gains:

```ts
static async listRegistrySources(): Promise<RegistrySourceDTO[]>
static async addRegistrySource(url: string): Promise<RegistrySourceDTO>
static async removeRegistrySource(url: string): Promise<void>
```

Types are added to `frontend/src/types.ts`.

A one-line explainer sits under the title: *"Packages are fetched from the registries
you add. None are configured by default."*

### 4.4 Empty and query states

The render decision becomes explicit, keyed on the number of sources and the query:

| Condition | Copy |
| --- | --- |
| sources == 0 | "No registries configured. Add a registry URL to browse community packages." + the Add control |
| sources > 0, packages == 0, query == "" | "The configured registries contain no packages." |
| sources > 0, packages == 0, query != "" | `No packages match "<query>".` |
| sources > 0, packages > 0, query == "" | results, labelled **All packages** |

The empty query is never interpolated into a "matching" sentence again. The results
heading reads "All packages" for an empty query and `Results for "<query>"` otherwise.

A source with an `error` is shown in the Sources panel; when a search returns nothing
and one or more sources errored, the empty state adds "Some registries could not be
reached." so the reason is visible.

### 4.5 Debounce

The fetch effect keys on a debounced query: a 250 ms `setTimeout` that the effect
clears on the next keystroke, so typing does not refetch per character. The
`useEffect` currently keyed on `[isOpen, searchQuery]` becomes
`[isOpen, debouncedQuery]`.

### 4.6 Docs

`pkg/gui/docs/22-editing-content.md`'s registry section gains the GUI path (the
Sources panel) beside the CLI verbs; `13-configuration-reference.md` already documents
`registries.urls` and needs no change beyond a note that the Studio writes it.

## 5. Behaviour

| Action | Result |
| --- | --- |
| Open the registry with no sources | "No registries configured" and a URL field; no `matching ""` |
| Add a valid URL | persisted to `registries.urls`; its packages join the list |
| Add the same URL twice | `409`, shown as "already configured" |
| Add `ftp://…` | `400`, shown as an invalid registry URL |
| Remove a source | persisted; its packages leave the list |
| Empty query | all packages, labelled "All packages" |
| Query with no match | `No packages match "<query>".` |
| A source that will not load | listed with its error; other sources still search |

## 6. Testing

- `pkg/registry`: `NormalizeSource` accepts and rejects the documented forms.
- `pkg/gui`: `RegistrySources` reports a resolved name and a per-source error; add
  persists and rejects a duplicate; remove persists and reports an absent URL; the new
  routes dispatch by method (`registry_test.go` already boots a config manager and a
  registry test server to extend).
- `cmd/localrpg`: `registry add` still works through the shared normaliser.
- `frontend`: vitest for the four empty states and the debounced fetch
  (`RegistryModal.test.tsx`), and for the Sources panel calling add/remove.
- The existing regression guard (a config with no registries shows an empty list and
  no errors) stays green.

## 7. Rollout

Additive: three service methods, three routes, one client helper, one panel. No config
schema change (the field already exists) and no default registries. The CLI behaviour
is unchanged except that both surfaces now reject the same malformed URLs.

## 8. Risks

- **Writing config from the GUI must respect the local-override rule.** `Save` already
  writes the local override when one exists (`pkg/config/manager.go`), which is the same
  path `SaveSettings` uses; the source methods must call `Save`, not write YAML directly.
- **A slow source blocks the list.** `Client.Registries` has a 30 s HTTP timeout and
  tolerates one failing source, but adding a source should surface "fetching" so a slow
  index does not look like a hang. The Sources panel shows a spinner per row while
  `RegistrySources` is in flight.
- **Fetching on every open.** `RegistrySources` re-fetches indexes (subject to the ETag
  cache); the panel is collapsed by default so the cost is paid only when expanded or
  when search runs.
