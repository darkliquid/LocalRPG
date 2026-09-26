# Design Spec: Codex Character Portrait Regeneration & Notes/Memories Tabs

**Date:** 2026-09-26
**Status:** Proposed
**Target:** `frontend` (`CodexDrawer.tsx`, `api/client.ts`, `types.ts`, `ui/AIGenerateButton.tsx`), `pkg/gui` (`service.go`, `server.go`, `types.go`), `pkg/engine` (`portrait_worker.go`)

---

## 1. Executive Summary

Two Codex usability gaps:

1. **Character portraits cannot be regenerated.** Portraits are generated once, in the background, only when an entity's `portrait` frontmatter field is empty: `PortraitWorker.Enqueue` returns early if `ent.Portrait != ""` (`pkg/engine/portrait_worker.go:69-71`), and the read endpoint `GET /api/game/{game_id}/character/{character_id}/portrait` (`pkg/gui/server.go:459-469`) has no force/regenerate path. If a portrait comes out wrong, is generated with a poor provider, or the world art style changes, there is no way to ask for a new one. The Codex header (`CodexDrawer.tsx:295-310`) shows the portrait and opens a lightbox, but offers no refresh control.
2. **Memories look like notes.** The Codex sidebar stacks a "Notes (N)" entity list and, below it, a "Memories" block whose entries render as small bordered cards (`CodexDrawer.tsx:225-242`) that read like a second, inline list of notes. Memories are derived, read-only turn facts, not editable notes. Rendering them as a stacked list under the note list invites the wrong mental model.

This spec adds synchronous portrait regeneration (backend + Codex button) and separates the sidebar into explicit **Notes** and **Memories** tabs.

---

## 2. Architecture & Data Flow

```
Codex portrait "Regenerate"
        |
        v
POST /api/game/{id}/character/{characterID}/portrait
        |
        v
Service.RegenerateCharacterPortrait
  - load entity (store, markdown fallback)
  - build prompt via engine.BuildPortraitPrompt(name, gender, age, appearance, artStyle)
  - image provider (recordImage span/telemetry)
  - write games/<id>/assets/portraits/<id><ext>  (remove stale other-ext file)
  - update note frontmatter `portrait`, Syncer.SyncFile
  - return { portrait_url: ".../portrait?t=<unix>", generated_at }
        |
        v
Codex sets img src to returned URL (cache-busted), refreshes note
```

---

## 3. Detailed Component Designs

### 3.1 Backend: synchronous portrait regeneration

**`pkg/engine/portrait_worker.go`**
- Extract the generate-write-sync body of `Enqueue`'s goroutine into an exported synchronous method:
  ```go
  // Regenerate generates a fresh portrait for ent regardless of any existing
  // Portrait value and returns the game-relative path written.
  func (w *PortraitWorker) Regenerate(ctx context.Context, gameID string, ent *entity.Entity, artStyle string) (string, error)
  ```
- `Regenerate` ignores `ent.Portrait`, calls `BuildPortraitPrompt`, writes `assets/portraits/<id><ext>`, removes any stale `assets/portraits/<id>.<other-ext>` file, updates the entity note's `portrait` frontmatter, syncs the file, and returns the relative path. Errors are returned (not swallowed) so the service can surface a `harness.GenerationFailure`.
- `Enqueue` keeps its fire-and-forget behavior and should reuse the same helper, still skipping when `ent.Portrait != ""`.
- Guard: only `ent.Type == "character"` and non-empty `ent.ID`; a nil generator returns a `provider_unavailable` failure.

**`pkg/gui/service.go`**
- New method:
  ```go
  func (s *Service) RegenerateCharacterPortrait(ctx context.Context, gameID, characterID string) (CharacterPortraitDTO, error)
  ```
  - `ensureIndexed`, load entity via store with the markdown fallback already used by `GetCharacterPortrait` (`service.go:1396-1426`).
  - Resolve the world `art_style` the same way `scanAndEnrichCharacters` does when it builds the `PortraitWorker` (`service.go:1443-1494`).
  - Build a `PortraitWorker` with the image client from `imageClientFactory`; if image generation is disabled/unconfigured, return a `harness.GenerationFailure` with code `provider_unavailable` and a message naming the image provider.
  - Wrap provider errors through the existing image telemetry path (`recordImage` / `startImageSpan`, `pkg/gui/generation_telemetry.go`) so the regenerate is traced and counted like every other generation.
  - Return a `CharacterPortraitDTO{PortraitURL, GeneratedAt}`.

**`pkg/gui/types.go`**
```go
type CharacterPortraitDTO struct {
    PortraitURL string `json:"portrait_url"`
    GeneratedAt string `json:"generated_at"`
}
```

**`pkg/gui/server.go`**
- Register `POST /api/game/{gameID}/character/{characterID}/portrait` -> `RegenerateCharacterPortrait`. On failure use `writeGenerationFailure` (`generation_errors.go:54-61`) so the client receives the full `GenerationFailure` body.
- Keep `GET` read-only. The read handler keeps `Cache-Control: no-cache` and ignores an optional `?t=` cache-bust query parameter.

### 3.2 Frontend: Codex regenerate button

**`frontend/src/api/client.ts`**
```ts
regenerateCharacterPortrait(gameID: string, characterID: string): Promise<CharacterPortraitDTO>
```
Throws `GenerationError` on non-2xx, matching `generateText`/`generateCharacter`.

**`frontend/src/types.ts`**
```ts
export interface CharacterPortraitDTO { portrait_url: string; generated_at: string }
```

**`CodexDrawer.tsx`**
- Add `portraitVersion` / `isRegeneratingPortrait` / `portraitError` state.
- Beside the header portrait (`:295-310`), render a `RotateCw` icon button (`title="Regenerate portrait"`) and, while in flight, a `Loader2` spinner. Disable while regenerating.
- Compute the portrait `src` as `/api/game/{gameID}/character/{entity.id}/portrait?v={portraitVersion}`, where `portraitVersion` starts as the entity's recorded portrait hash (or `0`) and is set to the returned `generated_at` after regeneration. This defeats browser caching without touching the endpoint.
- On success: set the new version, clear error, and re-fetch the selected entity so the updated `portrait` frontmatter is reflected.
- On failure: show an inline error chip beside the button with the provider message from the `GenerationError` (message first, code in the tooltip). This is intentionally the same error surface as generation failures (see the provider-error-surfacing spec).
- Only show the button for `entity.type === 'character'`; keep the existing lightbox click behavior on the image itself (click image = zoom, click button = regenerate).

### 3.3 Sidebar: Notes vs. Memories tabs

Replace the stacked sidebar content (`CodexDrawer.tsx:173-270`) with a two-tab segmented control.

- New state: `const [sidebarTab, setSidebarTab] = useState<'notes' | 'memories'>('notes')`.
- Tab bar directly under the sidebar header, styled like the existing type-filter pills (`:199-223`), showing counts:
  - `Notes ({entities?.length ?? 0})`
  - `Memories ({memories.length})`
- **Notes tab**: the existing search (`:188-197`), dynamic type-filter pills (`:199-223`), and entity list (`:244-270`) move inside this tab unchanged.
- **Memories tab**: the memory timeline moves here. Behavior:
  - No entity selected -> italic prompt "Select a note to see its memories."
  - Entity selected, no memories -> italic "No memories yet."
  - Otherwise the existing `t{turn} {text}` cards (`:231-239`), newest-first, read-only.
  - Keep the `max-h`/`overflow-y-auto` container.
- When the user selects a different entity while on the Memories tab, the memory fetch effect (`:60-76`) already re-runs; no extra wiring.
- The tab choice persists for the drawer session (component state), not across restarts.

This makes the distinction explicit: Notes are authored, editable, graph-backed documents; Memories are derived, read-only, per-entity turn facts.

---

## 4. Non-Goals

- No memory editing, creation, or tags GUI (matches the entity-memories spec's read-only non-goal).
- No change to background portrait generation at campaign creation.
- No per-provider portrait override or prompt editor in the Codex.
- No WebP/avif conversion or image post-processing.

---

## 5. Test Strategy

1. **Go unit tests (`pkg/gui`)**:
   - `POST .../portrait` with a stub image client returns 200 and a `portrait_url`, and overwrites the existing `assets/portraits/<id>.*` file.
   - A provider error returns a `GenerationFailure` JSON body with code `provider_error` and a non-empty message; a disabled image provider returns `provider_unavailable`.
   - Regeneration is allowed when `ent.Portrait != ""` (unlike `Enqueue`).
2. **Go test (`pkg/engine`)**:
   - `PortraitWorker.Regenerate` writes the file, updates frontmatter, and removes a stale file with a different extension.
3. **Frontend build gate**: `npm --prefix frontend run build` passes (strict TS, no unused locals/params).
4. **Manual verification**: regenerate a character portrait from the Codex and confirm the header image updates without a manual reload; confirm the Memories tab shows a prompt with no selection and the correct list for a character; confirm Notes tab behavior is unchanged.
