# Codex Character Portraits & Notes/Memories Tabs Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the Codex regenerate a character's portrait on demand, and separate Notes and Memories into explicit sidebar tabs.

**Architecture:** Add a synchronous `PortraitWorker.Regenerate`, expose it as `Service.RegenerateCharacterPortrait` behind `POST /api/game/{id}/character/{id}/portrait`, and add a Codex regenerate button with cache-busting. Replace the stacked sidebar list + memories block with a Notes/Memories tab control.

**Tech Stack:** Go 1.27 (stdlib tests, no testify), React 19 + TypeScript, Tailwind v4.

**Spec:** `docs/superpowers/specs/2026-09-26-codex-character-portraits-and-memory-tabs-design.md`

## Global Constraints

- Use `interface{}`, not `any`. Wrap errors with `fmt.Errorf("...: %w", err)`. `go vet` must stay clean.
- Reuse `harness.GenerationFailure` for provider/config failures so the client sees the structured body (the provider-error-surfacing work landed).
- Use `engine.BuildPortraitPrompt` and `media.ArtExtension`; do not duplicate them.
- TypeScript `strict`, `noUnusedLocals`, `noUnusedParameters`.
- Test gates: `mise run test:backend`, `mise run lint`, `mise run test:frontend`.
- Do not commit unless the user asks.

---

## File Map

- Modify: `pkg/engine/portrait_worker.go`, `pkg/engine/portrait_worker_test.go`
- Modify: `pkg/gui/types.go`, `pkg/gui/service.go`, `pkg/gui/server.go`
- Create/Modify: `pkg/gui/portrait_regenerate_test.go`
- Modify: `frontend/src/types.ts`, `frontend/src/api/client.ts`, `frontend/src/components/CodexDrawer.tsx`

---

### Task 1: Synchronous `PortraitWorker.Regenerate`

**Files:**
- Modify: `pkg/engine/portrait_worker.go`, `pkg/engine/portrait_worker_test.go`

**Interfaces:**
- Produces: `(*PortraitWorker).Regenerate(ctx context.Context, gameID string, ent *entity.Entity, artStyle string) (string, error)` returning the game-relative path.

- [x] **Step 1: Write the failing test**

Append to `pkg/engine/portrait_worker_test.go`:

```go
func TestPortraitWorker_RegenerateOverwritesExisting(t *testing.T) {
	tmpDir := t.TempDir()
	resolver := core.NewPathResolver(tmpDir)
	gameID := "regen-game"
	gameDir := resolver.GameDir(gameID)
	entitiesDir := filepath.Join(gameDir, "entities")
	if err := os.MkdirAll(entitiesDir, 0755); err != nil {
		t.Fatal(err)
	}
	portraitsDir := filepath.Join(gameDir, "assets", "portraits")
	if err := os.MkdirAll(portraitsDir, 0755); err != nil {
		t.Fatal(err)
	}

	char := &entity.Entity{
		ID: "elena", Name: "Elena", Type: "character",
		Gender: "female", Age: "28", Appearance: "Silver hair",
		Portrait: filepath.Join("assets", "portraits", "elena.jpg"),
	}
	data, err := char.SerializeMarkdown()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(entitiesDir, "elena.md"), data, 0644); err != nil {
		t.Fatal(err)
	}
	// A stale clip under a different extension.
	if err := os.WriteFile(filepath.Join(portraitsDir, "elena.jpg"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}

	pngBytes := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00}
	gen := &mockPortraitGenerator{returnBytes: pngBytes}
	worker := NewPortraitWorker(resolver, nil, gen)

	relPath, err := worker.Regenerate(context.Background(), gameID, char, "oil painting")
	if err != nil {
		t.Fatalf("Regenerate failed: %v", err)
	}
	if relPath != filepath.Join("assets", "portraits", "elena.png") {
		t.Fatalf("relPath = %q, want elena.png", relPath)
	}
	if _, err := os.Stat(filepath.Join(gameDir, relPath)); err != nil {
		t.Fatalf("expected the new portrait on disk: %v", err)
	}
	if _, err := os.Stat(filepath.Join(portraitsDir, "elena.jpg")); !os.IsNotExist(err) {
		t.Fatalf("expected the stale .jpg to be removed, stat err = %v", err)
	}
}
```

- [x] **Step 2: Run to verify it fails**

Run: `go test ./pkg/engine/ -run TestPortraitWorker_RegenerateOverwritesExisting -v`
Expected: FAIL, `worker.Regenerate undefined`.

- [x] **Step 3: Refactor `Enqueue` into a shared writer and add `Regenerate`**

In `pkg/engine/portrait_worker.go`, replace the `Enqueue` goroutine body with a call to a new `writePortrait`, and add `Regenerate`:

```go
// Regenerate generates a fresh portrait for ent regardless of any existing
// Portrait value and returns the game-relative path written.
func (w *PortraitWorker) Regenerate(ctx context.Context, gameID string, ent *entity.Entity, artStyle string) (string, error) {
	if w.generator == nil {
		return "", fmt.Errorf("portrait generator is not configured")
	}
	if ent == nil || ent.ID == "" || ent.Type != "character" {
		return "", fmt.Errorf("portrait regeneration requires a character entity")
	}
	return w.writePortrait(ctx, gameID, ent, artStyle)
}
```

In `Enqueue`, replace the goroutine's body (everything from `prompt := ...` to the end) with:

```go
		_, _ = w.writePortrait(context.Background(), gameID, &entCopy, artStyle)
```

Add the writer and its helpers:

```go
// portraitExtensions are the file names a generated portrait may carry, used to
// remove a stale clip when the format changes.
var portraitExtensions = []string{".png", ".jpg", ".jpeg", ".webp", ".svg"}

func (w *PortraitWorker) writePortrait(ctx context.Context, gameID string, ent *entity.Entity, artStyle string) (string, error) {
	prompt := BuildPortraitPrompt(ent.Name, ent.Gender, ent.Age, ent.Appearance, artStyle)
	imgBytes, err := w.generator.GenerateImage(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("generate portrait: %w", err)
	}
	if len(imgBytes) == 0 {
		return "", fmt.Errorf("portrait generator returned no image")
	}

	ext := media.ArtExtension(imgBytes)
	if ext == "" {
		ext = ".png"
	}
	relPath := filepath.Join("assets", "portraits", ent.ID+ext)
	fullPath := filepath.Join(w.resolver.GameDir(gameID), relPath)

	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return "", fmt.Errorf("create portrait dir: %w", err)
	}
	if err := os.WriteFile(fullPath, imgBytes, 0644); err != nil {
		return "", fmt.Errorf("write portrait: %w", err)
	}
	w.removeStalePortraits(gameID, ent.ID, ext)

	if err := w.updateNote(gameID, ent, relPath); err != nil {
		return "", err
	}
	return relPath, nil
}

func (w *PortraitWorker) removeStalePortraits(gameID, id, keepExt string) {
	dir := filepath.Join(w.resolver.GameDir(gameID), "assets", "portraits")
	for _, ext := range portraitExtensions {
		if ext == keepExt {
			continue
		}
		_ = os.Remove(filepath.Join(dir, id+ext))
	}
}

// updateNote writes the portrait path into the entity's note frontmatter,
// preferring the on-disk note so a hand edit is not clobbered.
func (w *PortraitWorker) updateNote(gameID string, ent *entity.Entity, relPath string) error {
	notePath := filepath.Join(w.resolver.GameDir(gameID), "entities", ent.ID+".md")
	if existingData, err := os.ReadFile(notePath); err == nil {
		if existingEnt, err := entity.ParseMarkdownEntity(existingData); err == nil {
			existingEnt.Portrait = relPath
			if data, err := existingEnt.SerializeMarkdown(); err == nil {
				if err := os.WriteFile(notePath, data, 0644); err != nil {
					return fmt.Errorf("write note: %w", err)
				}
				if w.store != nil {
					_ = storage.NewSyncer(w.store).SyncFile(notePath)
				}
				return nil
			}
		}
	}

	updated := *ent
	updated.Portrait = relPath
	data, err := updated.SerializeMarkdown()
	if err != nil {
		return fmt.Errorf("serialize note: %w", err)
	}
	if err := os.WriteFile(notePath, data, 0644); err != nil {
		return fmt.Errorf("write note: %w", err)
	}
	if w.store != nil {
		_ = storage.NewSyncer(w.store).SyncFile(notePath)
	}
	return nil
}
```

- [x] **Step 4: Run the test and the package suite**

Run: `go test -count=1 ./pkg/engine/ -run TestPortraitWorker -v`
Expected: PASS.

---

### Task 2: Service method and route

**Files:**
- Modify: `pkg/gui/types.go`, `pkg/gui/service.go`, `pkg/gui/server.go`
- Create: `pkg/gui/portrait_regenerate_test.go`

**Interfaces:**
- Produces: `CharacterPortraitDTO{PortraitURL, GeneratedAt}`, `Service.RegenerateCharacterPortrait(ctx, gameID, characterID) (CharacterPortraitDTO, error)`, `POST /api/game/{id}/character/{id}/portrait`.

- [x] **Step 1: DTO**

Add to `pkg/gui/types.go`:

```go
// CharacterPortraitDTO reports a freshly written portrait so the Codex can bust
// its image cache without reloading the note.
type CharacterPortraitDTO struct {
	PortraitURL string `json:"portrait_url"`
	GeneratedAt string `json:"generated_at"`
}
```

- [x] **Step 2: Service method**

Add to `pkg/gui/service.go` (near `GetCharacterPortrait`):

```go
// RegenerateCharacterPortrait generates a fresh portrait for a character and
// overwrites any existing one, returning a cache-busted URL.
func (s *Service) RegenerateCharacterPortrait(ctx context.Context, gameID, characterID string) (CharacterPortraitDTO, error) {
	s.ensureIndexed(gameID)
	store, err := s.store(gameID)
	if err != nil {
		return CharacterPortraitDTO{}, err
	}

	ent, err := store.GetEntity(characterID)
	if err != nil || ent == nil {
		notePath := filepath.Join(s.resolver.GameDir(gameID), "entities", characterID+".md")
		if data, readErr := os.ReadFile(notePath); readErr == nil {
			if parsed, parseErr := entity.ParseMarkdownEntity(data); parseErr == nil {
				ent = parsed
			}
		}
	}
	if ent == nil {
		return CharacterPortraitDTO{}, fmt.Errorf("character %q not found", characterID)
	}
	if ent.Type != "character" {
		return CharacterPortraitDTO{}, &harness.GenerationFailure{
			Code:    harness.FailureInvalidRequest,
			Message: fmt.Sprintf("%q is not a character", characterID),
		}
	}

	cfg := s.configMgr.Get()
	if cfg.Media.Image.Type == "" || cfg.Media.Image.Type == "disabled" {
		return CharacterPortraitDTO{}, &harness.GenerationFailure{
			Code:    harness.FailureProviderUnavailable,
			Message: "image generation is disabled or unconfigured",
		}
	}
	client, err := imageClientFactory(cfg.Media.Image, cfg.Providers.Gemini.APIKey)
	if err != nil {
		return CharacterPortraitDTO{}, &harness.GenerationFailure{
			Code:    harness.FailureProviderUnavailable,
			Message: fmt.Sprintf("image provider: %v", err),
			Cause:   err,
		}
	}

	provider := cfg.Media.Image.BuiltinName
	if provider == "" {
		provider = cfg.Media.Image.Type
	}
	started := time.Now()
	ctx, span := startImageSpan(ctx, s.logger, "portrait")
	defer span.End()

	worker := engine.NewPortraitWorker(s.resolver, store, client)
	relPath, err := worker.Regenerate(ctx, gameID, ent, s.worldArtStyle(gameID))
	if err != nil {
		failure := &harness.GenerationFailure{
			Code:    harness.ClassifyProviderError(err),
			Message: fmt.Sprintf("generate portrait: %v", err),
			Cause:   err,
		}
		s.recordImage(ctx, span, "portrait", provider, 0, started, failure)
		return CharacterPortraitDTO{}, failure
	}
	s.recordImage(ctx, span, "portrait", provider, 0, started, nil)

	return CharacterPortraitDTO{
		PortraitURL: fmt.Sprintf("/api/game/%s/character/%s/portrait?t=%d", gameID, characterID, time.Now().UnixNano()),
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}
```

Note `relPath` is intentionally not used further; either use `_ = relPath` or drop the assignment with `if _, err := worker.Regenerate(...)`. Use `if _, err := ...`.

- [x] **Step 3: Route**

In `pkg/gui/server.go`, replace the character portrait GET block so it handles both methods:

```go
		if len(parts) >= 4 && parts[3] == "portrait" {
			if r.Method == http.MethodPost {
				dto, err := s.service.RegenerateCharacterPortrait(r.Context(), gameID, characterID)
				if err != nil {
					if writeGenerationFailure(w, err) {
						return
					}
					writeGameError(w, err)
					return
				}
				writeJSON(w, dto)
				return
			}
			if r.Method == http.MethodGet {
				data, contentType, err := s.service.GetCharacterPortrait(r.Context(), gameID, characterID)
				if err != nil {
					writeGameError(w, err)
					return
				}
				w.Header().Set("Content-Type", contentType)
				w.Header().Set("Cache-Control", "no-cache")
				_, _ = w.Write(data)
				return
			}
		}
		http.NotFound(w, r)
		return
```

- [x] **Step 4: Test**

Create `pkg/gui/portrait_regenerate_test.go`:

```go
package gui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
)

type stubPortraitImageClient struct{ data []byte }

func (s stubPortraitImageClient) GenerateImage(context.Context, string) ([]byte, error) {
	return s.data, nil
}

func TestRegenerateCharacterPortraitEndpoint(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(tmpDir)
	setupFreeformSystem(t, svc)
	game, err := svc.CreateGame(context.Background(), CreateGameRequestDTO{
		Name: "Portrait Regen", SystemID: "freeform", WorldID: "harbour-realm",
		PlayerName: "Hero Vance",
		Player:     PlayerCharacterDTO{Appearance: "A tall adventurer.", Age: "30"},
	})
	if err != nil {
		t.Fatalf("CreateGame failed: %v", err)
	}

	cfg, _ := svc.GetSettings(context.Background())
	cfg.Config.Media.Image = config.ImageConfig{Type: "builtin", BuiltinName: "echo"}
	if _, err := svc.SaveSettings(context.Background(), cfg.Config); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	prev := imageClientFactory
	imageClientFactory = func(config.ImageConfig, string) (media.ImageClient, error) {
		return stubPortraitImageClient{data: []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00}}, nil
	}
	t.Cleanup(func() { imageClientFactory = prev })

	server := NewServer(svc, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/game/"+game.ID+"/character/hero-vance/portrait", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var dto CharacterPortraitDTO
	if err := json.Unmarshal(w.Body.Bytes(), &dto); err != nil {
		t.Fatalf("decode DTO: %v", err)
	}
	if dto.PortraitURL == "" {
		t.Fatalf("expected a portrait URL, got %+v", dto)
	}
	if _, err := os.Stat(filepath.Join(svc.GetResolver().GameDir(game.ID), "assets", "portraits", "hero-vance.png")); err != nil {
		t.Fatalf("expected the regenerated portrait on disk: %v", err)
	}
}
```

- [x] **Step 5: Run tests and vet**

Run: `go test -count=1 ./pkg/gui/ ./pkg/engine/ && go vet ./pkg/gui/ ./pkg/engine/`
Expected: PASS.

---

### Task 3: Codex regenerate button

**Files:**
- Modify: `frontend/src/types.ts`, `frontend/src/api/client.ts`, `frontend/src/components/CodexDrawer.tsx`

**Interfaces:**
- Produces: `CharacterPortraitDTO { portrait_url: string; generated_at: string }`, `APIClient.regenerateCharacterPortrait(gameID, characterID): Promise<CharacterPortraitDTO>`.

- [x] **Step 1: Types**

Add to `frontend/src/types.ts`:

```ts
export interface CharacterPortraitDTO {
  portrait_url: string;
  generated_at: string;
}
```

- [x] **Step 2: API client**

Add to `frontend/src/api/client.ts` (near `generateCharacter`):

```ts
  // regenerateCharacterPortrait asks the backend for a fresh portrait and
  // returns its cache-busted URL.
  static async regenerateCharacterPortrait(gameID: string, characterID: string): Promise<CharacterPortraitDTO> {
    const res = await fetch(
      `/api/game/${encodeURIComponent(gameID)}/character/${encodeURIComponent(characterID)}/portrait`,
      { method: 'POST' }
    );
    if (!res.ok) return throwGenerationError(res);
    return res.json();
  }
```

Import `CharacterPortraitDTO` in the client's type import list.

- [x] **Step 3: CodexDrawer state and button**

Add imports: `RotateCw`, `AlertCircle` to the `lucide-react` import; `GenerationError` to the `api/client` import; `formatGenerationError` from `../lib/generationError`; `GenerationFailure` to the `types` import; `CharacterPortraitDTO` not needed if using the string URL.

State additions:

```tsx
  const [portraitVersion, setPortraitVersion] = useState(0);
  const [isRegeneratingPortrait, setIsRegeneratingPortrait] = useState(false);
  const [portraitError, setPortraitError] = useState<GenerationFailure | null>(null);
```

Reset on entity change (extend the existing entity effect):

```tsx
  useEffect(() => {
    if (entity) {
      setMarkdown(entity.markdown);
    } else {
      setIsSidebarOpen(true);
    }
    setPortraitVersion(0);
    setPortraitError(null);
  }, [entity]);
```

Handler:

```tsx
  const handleRegeneratePortrait = () => {
    if (!gameID || !entity || isRegeneratingPortrait) return;
    setIsRegeneratingPortrait(true);
    setPortraitError(null);
    APIClient.regenerateCharacterPortrait(gameID, entity.id)
      .then((dto) => {
        const version = dto.generated_at ? Date.parse(dto.generated_at) : Date.now();
        setPortraitVersion(Number.isFinite(version) ? version : Date.now());
      })
      .catch((err: unknown) => {
        setPortraitError(
          err instanceof GenerationError ? err.failure : { code: 'provider_error', message: err instanceof Error ? err.message : String(err) }
        );
      })
      .finally(() => setIsRegeneratingPortrait(false));
  };
```

Portrait URL helper (before `return`):

```tsx
  const portraitURL =
    gameID && entity
      ? `/api/game/${encodeURIComponent(gameID)}/character/${encodeURIComponent(entity.id)}/portrait${
          portraitVersion ? `?v=${portraitVersion}` : ''
        }`
      : '';
```

Replace the portrait block (currently `:295-311`) with a portrait plus a regenerate button:

```tsx
                {entity.type === 'character' && gameID && (
                  <div className="flex items-center gap-1.5 shrink-0">
                    <div
                      onClick={() => setLightbox({ src: portraitURL, alt: entity.name })}
                      className="w-14 h-14 rounded-xl overflow-hidden shrink-0 border-2 border-purple-500/30 shadow-lg bg-black/40 cursor-zoom-in transition-transform hover:scale-105"
                      title={`View portrait of ${entity.name}`}
                    >
                      <img src={portraitURL} alt={entity.name} className="w-full h-full object-cover" loading="lazy" />
                    </div>
                    <button
                      type="button"
                      onClick={handleRegeneratePortrait}
                      disabled={isRegeneratingPortrait}
                      className="p-1.5 rounded-lg bg-stone-900/80 border border-purple-500/30 text-purple-300 hover:text-purple-100 hover:border-purple-400 transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
                      title="Regenerate portrait"
                      aria-label="Regenerate portrait"
                    >
                      {isRegeneratingPortrait ? (
                        <Loader2 className="w-4 h-4 animate-spin" />
                      ) : (
                        <RotateCw className="w-4 h-4" />
                      )}
                    </button>
                  </div>
                )}
```

Show the error under the header, beside the save-error block:

```tsx
            {portraitError && (
              <div className="text-xs rounded-lg border border-red-500/40 bg-red-950/40 text-red-200 px-3 py-2 flex items-start gap-2">
                <AlertCircle className="w-3.5 h-3.5 text-red-400 shrink-0 mt-0.5" />
                <span>{formatGenerationError(portraitError)}</span>
              </div>
            )}
```

- [x] **Step 4: Typecheck**

Run: `mise run test:frontend`
Expected: exit 0.

---

### Task 4: Notes and Memories tabs

**Files:**
- Modify: `frontend/src/components/CodexDrawer.tsx`

- [x] **Step 1: Add the tab state**

```tsx
  const [sidebarTab, setSidebarTab] = useState<'notes' | 'memories'>('notes');
```

- [x] **Step 2: Replace the sidebar header title with the tab bar**

Replace the header inner span (`Notes ({entities?.length ?? 0})`) with:

```tsx
          <div className="flex flex-wrap gap-1">
            <button
              type="button"
              onClick={() => setSidebarTab('notes')}
              className={`text-[10px] font-sans font-bold uppercase tracking-wider px-2 py-1 rounded border transition-colors cursor-pointer ${
                sidebarTab === 'notes'
                  ? 'bg-purple-600/30 border-purple-500/50 text-purple-200'
                  : 'bg-black/40 border-white/10 text-stone-400 hover:text-stone-200'
              }`}
            >
              Notes ({entities?.length ?? 0})
            </button>
            <button
              type="button"
              onClick={() => setSidebarTab('memories')}
              className={`text-[10px] font-sans font-bold uppercase tracking-wider px-2 py-1 rounded border transition-colors cursor-pointer ${
                sidebarTab === 'memories'
                  ? 'bg-purple-600/30 border-purple-500/50 text-purple-200'
                  : 'bg-black/40 border-white/10 text-stone-400 hover:text-stone-200'
              }`}
            >
              Memories ({memories.length})
            </button>
          </div>
```

- [x] **Step 3: Gate the search/filters/list and the memories block by tab**

Wrap the search (`<div className="relative">`), the type-filter pills, and the entity list in `{sidebarTab === 'notes' && (<> ... </>)}`, and replace the old `{entity && memories.length > 0 && (...)}` block with a memories panel:

```tsx
          {sidebarTab === 'memories' && (
            <div className="flex-1 min-h-[160px] overflow-y-auto space-y-1 pr-1">
              {!entity ? (
                <p className="text-stone-500 text-xs italic p-2">Select a note to see its memories.</p>
              ) : memories.length === 0 ? (
                <p className="text-stone-500 text-xs italic p-2">No memories yet.</p>
              ) : (
                memories.map((memory, index) => (
                  <div
                    key={`${memory.turn}-${index}`}
                    className="text-[11px] text-stone-300 bg-black/30 border border-white/5 rounded px-2 py-1"
                  >
                    <span className="font-mono text-stone-500 mr-1">t{memory.turn}</span>
                    {memory.text}
                  </div>
                ))
              )}
            </div>
          )}
```

The notes list keeps `flex-1 min-h-[160px] overflow-y-auto space-y-1 pr-1`, so the two branches are mutually exclusive and one fills the sidebar.

- [x] **Step 4: Typecheck and build**

Run: `mise run test:frontend && mise run build:frontend`
Expected: exit 0.

---

### Task 5: Verification

- [x] **Step 1: Backend tests**

Run: `mise run test:backend`
Expected: PASS.

- [x] **Step 2: Vet**

Run: `mise run lint`
Expected: clean.

- [x] **Step 3: Frontend**

Run: `mise run test:frontend && mise run build:frontend`
Expected: exit 0.

- [x] **Step 4: Manual**

Open a character in the Codex: Regenerate Portrait replaces the header image without a reload and shows a provider message on failure; the Memories tab shows a prompt with no selection and the entity's timeline when selected; the Notes tab is unchanged.

---

## Self-Review

**Spec coverage:** synchronous regenerate (Task 1), endpoint + DTO + telemetry + failure shape (Task 2), Codex button with cache-bust and error (Task 3), Notes/Memories tabs (Task 4).

**Placeholder scan:** none.

**Type consistency:** `regenerateCharacterPortrait` and `CharacterPortraitDTO.portrait_url/generated_at` match Tasks 2/3; `PortraitWorker.Regenerate` signature is used verbatim in Task 2; `sidebarTab` values match the two buttons.
