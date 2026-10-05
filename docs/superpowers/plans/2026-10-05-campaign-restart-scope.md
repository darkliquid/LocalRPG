# Campaign Restart Scope Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make restarting a campaign reset only the turn history and the world's mutable cast, so the campaign's artwork, narrator voice, settings, spend ledger and character survive; and let a campaign borrow its world's banner and icon when it has none of its own.

**Architecture:** `RestartGame` stops deleting the campaign directory. A new engine operation `ResetCampaign` classifies each note under `entities/` against the world's template set, restores the templates, deletes notes created during play, clears the protagonist's runtime fields, then clears the derived index and re-syncs the surviving Markdown. Artwork and settings are never touched. A read-time fallback in `GetGameAsset` (and the two DTO builders) serves the world's banner and icon when the campaign has none.

**Tech Stack:** Go standard library, `modernc.org/sqlite` (no CGO), React 19 + TypeScript + Tailwind v4.

**Spec:** `docs/superpowers/specs/2026-10-05-campaign-restart-scope-design.md`

**Depends on:** nothing.

## Global Constraints

- Go standard library only for tests (`testing`, `t.TempDir()`); no testify. Use `interface{}`, not `any`. `go vet ./...` must stay clean.
- Errors wrapped with `fmt.Errorf("...: %w", err)`. Reuse `entity.Slugify`, `entity.WikilinkTarget`, `pathutil.SanitizeID`, `pathutil.ResolveSafeChild`; do not write local slug or link parsing.
- Never call `storage.NewStore` for a game; go through `storage.OpenGameStore`.
- The campaign directory is never deleted by a restart. `assets/`, `game.yaml` and `usage_records` survive a restart untouched. Batch synthesis jobs do not: they speak narration the reset discards, so an in-flight job is cancelled at the provider, best-effort, and every job row is then removed.
- Frontend: `tsconfig.json` sets `strict`, `noUnusedLocals`, `noUnusedParameters`, so `npm run build` fails on an unused import.
- Commits are Conventional Commits with a scope; subject under 72 characters.

---

### File Map

- **`pkg/storage/store.go`** — add `ResetDerivedState`.
- **`pkg/storage/store_test.go`** — reset clears derived tables and keeps the ledger; `DeleteTTSJobs` removes one campaign's jobs.
- **`pkg/storage/ttsjobs.go`** — add `DeleteTTSJobs`.
- **`pkg/engine/restart.go`** (new) — `ResetCampaign`, entity classification, protagonist runtime reset, batch-job removal.
- **`pkg/engine/restart_test.go`** (new) — restore, delete, keep, and manifest/asset/ledger preservation.
- **`pkg/gui/service.go`** — rewrite `RestartGame`; cancel in-flight batch jobs; `gameAssetSource`/`gameAssetURL`; `DeleteGameAsset`; `campaignArtPrompt`; world fallback in `GetGameAsset`, `ListGames` and `GetGameState`.
- **`pkg/gui/types.go`** — `BannerSource`/`IconSource` on the game summary.
- **`pkg/gui/server.go`** — `DELETE` on the banner/icon route.
- **`pkg/gui/restart_player_test.go`** — extend for artwork, narrator voice, usage and batch-job handling.
- **`pkg/gui/assets_endpoint_test.go`** — world fallback, versioned URLs, and revert coverage.
- **`pkg/gui/asset_campaign_prompt_test.go`** (new) — campaign-specific prompt coverage.
- **`frontend/src/types.ts`** — `banner_source`/`icon_source`.
- **`frontend/src/api/client.ts`** — `deleteGameAsset`.
- **`frontend/src/components/LauncherHub.tsx`** — `handleUseWorldArtwork`.
- **`frontend/src/components/launcher/CampaignSettingsModal.tsx`** — restart copy; "Use world artwork" control.

---

### Task 1: `Store.ResetDerivedState`

**Files:**
- Modify: `pkg/storage/store.go`
- Test: `pkg/storage/store_test.go`

**Interfaces:**
- Produces: `func (s *Store) ResetDerivedState() error`

- [ ] **Step 1: Write the failing test**

Append to `pkg/storage/store_test.go`:

```go
func TestResetDerivedStateClearsTimelineAndKeepsDurableRecords(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	if err := store.SaveTurn(TurnRecord{Number: 1, Mode: "Do", Input: "look"}); err != nil {
		t.Fatalf("SaveTurn: %v", err)
	}
	ent := &entity.Entity{ID: "sean", Name: "Sean", Type: "character"}
	if err := store.SaveEntity(ent); err != nil {
		t.Fatalf("SaveEntity: %v", err)
	}
	if _, err := store.SaveMemory(&entity.Memory{Turn: 1, Kind: "event", Text: "met a sailor", Source: "gm"}); err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}
	if err := store.SaveUsage(UsageRecord{GameID: "campaign-01", Role: "gm", Provider: "echo", Requests: 1}); err != nil {
		t.Fatalf("SaveUsage: %v", err)
	}
	if err := store.UpsertTTSJob(TTSJob{ID: "job-1", GameID: "campaign-01", Provider: "native-os", Status: "done"}); err != nil {
		t.Fatalf("UpsertTTSJob: %v", err)
	}

	if err := store.ResetDerivedState(); err != nil {
		t.Fatalf("ResetDerivedState: %v", err)
	}

	turns, err := store.CountTurns()
	if err != nil {
		t.Fatalf("CountTurns: %v", err)
	}
	if turns != 0 {
		t.Errorf("CountTurns = %d, want 0", turns)
	}
	if summaries, err := store.ListEntities(); err != nil || len(summaries) != 0 {
		t.Errorf("ListEntities = %d (err %v), want 0", len(summaries), err)
	}
	if hits, err := store.SearchMemories("sailor", "", "", 0, 10); err != nil || len(hits) != 0 {
		t.Errorf("SearchMemories = %d (err %v), want 0", len(hits), err)
	}
	if rows, err := store.UsageByGame("campaign-01"); err != nil || len(rows) != 1 {
		t.Errorf("UsageByGame = %d (err %v), want the ledger preserved", len(rows), err)
	}
	if jobs, err := store.ListTTSJobs("campaign-01"); err != nil || len(jobs) != 1 {
		t.Errorf("ListTTSJobs = %d (err %v), want job tracking preserved", len(jobs), err)
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test -run TestResetDerivedStateClearsTimelineAndKeepsDurableRecords ./pkg/storage/`
Expected: build failure, `store.ResetDerivedState undefined`.

- [ ] **Step 3: Implement**

Add to `pkg/storage/store.go`:

```go
// ResetDerivedState discards everything the index derives from history.jsonl and
// the Markdown notes: the timeline, the memory store, the working set, the entity
// graph and every embedding. Durable records -- the usage ledger and TTS job
// tracking -- are deliberately left alone, because they are not derivable and a
// campaign reset must not lose them. The caller re-syncs the notes afterwards.
func (s *Store) ResetDerivedState() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	tables := []string{
		"turn_entities",
		"turn_contexts",
		"turns",
		"memory_entities",
		"memory_tags",
		"memories_fts",
		"memories",
		"working_set",
		"embeddings",
		"edges",
		"entities",
	}
	for _, table := range tables {
		if _, err := tx.Exec("DELETE FROM " + table); err != nil {
			return fmt.Errorf("clear %s: %w", table, err)
		}
	}
	return tx.Commit()
}
```

- [ ] **Step 4: Run it and watch it pass**

Run: `go test -run TestResetDerivedStateClearsTimelineAndKeepsDurableRecords ./pkg/storage/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/storage/store.go pkg/storage/store_test.go
git commit -m "feat(storage): reset derived index state while keeping the spend ledger"
```

---

### Task 2: `engine.ResetCampaign`

**Files:**
- Create: `pkg/engine/restart.go`
- Test: `pkg/engine/restart_test.go`

**Interfaces:**
- Consumes: `storage.Store.ResetDerivedState`, `storage.Store.DeleteTTSJobs`, `storage.Syncer.Sync`, `HistoryLogger.RewindToTurn`, `ResolvePlayerID`, `OpeningSceneEntityID`.
- Produces: `func ResetCampaign(paths *core.PathResolver, store *storage.Store, manifest *core.GameManifest) error`

- [ ] **Step 1: Write the failing test**

Create `pkg/engine/restart_test.go` with a helper that builds a system, a world with one template, and a campaign; then asserts the four outcomes.

```go
package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/storage"
)
```

Add `TestResetCampaignRestoresWorldCastAndKeepsConfiguration` that:

1. builds paths with `core.NewPathResolver(t.TempDir())`, writes a system and a world whose `entities/harbourmaster.md` has `id: harbourmaster, name: Harbourmaster, type: character, state: {mood: gruff}`, and an `assets/banner.png` and `assets/icon.png` in the campaign after `InitGame`.
2. writes `game.yaml` settings `narrator_voice: narrator-01`.
3. after `InitGame`, modifies `entities/harbourmaster.md` (new body and `history: [1]`), adds `entities/gull-crier.md` (play-created), and writes a portrait for the protagonist.
4. calls `ResetCampaign`.
5. asserts: `harbourmaster.md` equals the world template bytes; `gull-crier.md` is gone; the protagonist note still has its authored fields, an empty `history` and no `state`; `game.yaml` still carries `narrator_voice`; `assets/banner.png` and `assets/icon.png` are unchanged; the index has zero turns and only the world cast plus the protagonist and opening scene.

- [ ] **Step 2: Run it and watch it fail**

Run: `go test -run TestResetCampaignRestoresWorldCastAndKeepsConfiguration ./pkg/engine/`
Expected: build failure, `ResetCampaign undefined`.

- [ ] **Step 3: Implement**

Create `pkg/engine/restart.go`:

```go
package engine

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/pathutil"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// ResetCampaign returns a campaign to its opening state without touching its
// configuration. The world's template cast is restored, notes created during
// play are removed, the protagonist keeps its authored sheet but loses its
// runtime state, and the timeline is cleared. The manifest, the assets directory
// and the usage ledger are left alone, so a restart costs the player their story
// and nothing else.
//
// Batch synthesis jobs are removed too: they exist to speak narration the reset
// has just discarded. A caller that can reach the provider should cancel an
// active job before calling this; the row is deleted either way.
func ResetCampaign(paths *core.PathResolver, store *storage.Store, manifest *core.GameManifest) error {
	if paths == nil || store == nil || manifest == nil {
		return fmt.Errorf("reset campaign: missing paths, store, or manifest")
	}

	gameDir := paths.GameDir(manifest.ID)
	entitiesDir := filepath.Join(gameDir, "entities")

	templates, err := worldEntityTemplates(paths, manifest.WorldID)
	if err != nil {
		return err
	}

	playerID := manifest.Player
	if id, err := ResolvePlayerID(store, manifest); err == nil && id != "" {
		playerID = id
	}

	if err := resetEntityNotes(entitiesDir, templates, playerID); err != nil {
		return err
	}

	if err := store.ResetDerivedState(); err != nil {
		return fmt.Errorf("reset derived state: %w", err)
	}
	if _, err := store.DeleteTTSJobs(manifest.ID); err != nil {
		return fmt.Errorf("delete batch jobs: %w", err)
	}

	history := NewHistoryLogger(filepath.Join(gameDir, "history.jsonl"))
	if err := history.RewindToTurn(0); err != nil {
		return fmt.Errorf("clear history: %w", err)
	}

	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		return fmt.Errorf("reindex entities: %w", err)
	}
	return nil
}
```

Then `worldEntityTemplates`, `resetEntityNotes`, `noteID`, `restoreTemplate` and `clearRuntimeFields`. `worldEntityTemplates` mirrors `InitGame`'s import (top-level `worlds/<id>/entities/*.md`, keyed by frontmatter id with filename fallback). `resetEntityNotes` walks `entities/` recursively, classifies by id, and:

- protagonist or opening scene -> parse, clear `History` and `State`, rewrite;
- template -> write template bytes to `entities/<id>.md`, remove any other copy;
- otherwise -> remove.

- [ ] **Step 4: Run it and watch it pass**

Run: `go test -run TestResetCampaign ./pkg/engine/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/restart.go pkg/engine/restart_test.go
git commit -m "feat(engine): reset a campaign to its world cast without losing configuration"
```

---

### Task 3: `Service.RestartGame` uses the reset in place

**Files:**
- Modify: `pkg/gui/service.go`
- Test: `pkg/gui/restart_player_test.go`, `pkg/gui/turn_test.go`

**Interfaces:**
- Consumes: `engine.ResetCampaign`.
- Produces: unchanged `func (s *Service) RestartGame(ctx context.Context, gameID string) (*GameSummaryDTO, error)`.

- [ ] **Step 1: Write the failing test**

Extend `TestRestartGamePreservesPlayerMetadata` in `pkg/gui/restart_player_test.go` to write `assets/banner.png`, `assets/icon.png`, a `narrator_voice` setting, one usage row and one in-flight batch job before restarting, and assert the first four survive the restart byte-for-byte while the job row is gone.

- [ ] **Step 2: Run it and watch it fail**

Run: `go test -run TestRestartGamePreservesPlayerMetadata ./pkg/gui/`
Expected: FAIL, the banner/icon/narrator voice are gone.

- [ ] **Step 3: Implement**

Replace the body of `RestartGame` (`pkg/gui/service.go:3832`) with: validate the id, take the game lock, load the manifest, open the store, cancel any in-flight batch job with a new `cancelBatchJobs(ctx, gameID)` helper (best-effort, mirroring `ResumePendingBatches`'s provider guard), call `engine.ResetCampaign`, return the summary with `TurnCount` 0. Remove the delete/recreate, portrait save-and-restore, and settings replay. `s.forgetGame` is no longer needed here, because the index is reset in place.

- [ ] **Step 4: Run it and watch it pass**

Run: `go test -run 'TestRestartGame' ./pkg/gui/`
Expected: PASS, including the untouched `TestRestartGameClearsHistoryAndKeepsTheCampaign`.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/service.go pkg/gui/restart_player_test.go
git commit -m "fix(gui): restart a campaign in place so its artwork and voice survive"
```

---

### Task 4: World banner and icon fallback

**Files:**
- Modify: `pkg/gui/service.go`
- Test: `pkg/gui/assets_endpoint_test.go`

**Interfaces:**
- Produces: `func (s *Service) gameAssetSource(gameDir, worldID, assetKind string) (string, string, bool)` for `GetGameAsset`; `func (s *Service) gameAssetURL(gameDir, worldID, assetKind, gameID string) (string, string)` for `ListGames` and `GetGameState`.

- [ ] **Step 1: Write the failing test**

Add `TestGameAssetFallsBackToTheWorld` to `pkg/gui/assets_endpoint_test.go`: a world with `assets/icon.png` and `assets/banner.png`, a campaign with neither, assert `ListGames` reports both URLs with source `world` and `GET /api/game/<id>/icon` serves the world's bytes with `image/png`; then give the campaign its own icon and assert the campaign's bytes win at a different URL; then `DELETE` it and assert the world's bytes return at the world's URL.

- [ ] **Step 2: Run it and watch it fail**

Run: `go test -run TestGameAssetFallsBackToTheWorld ./pkg/gui/`
Expected: FAIL, `IconURL` is empty and the route 404s.

- [ ] **Step 3: Implement**

In `GetGameAsset`, when the campaign has no such asset and its world does, return the world's file and content type. In `ListGames` and `GetGameState`, resolve `BannerURL`/`IconURL` through `gameAssetURL`, which prefers the campaign's asset, falls back to the world's, and stamps the URL with the served file's size and modification time so a changed asset is a new URL. `ListGames` also reports `BannerSource`/`IconSource`.

- [ ] **Step 4: Run it and watch it pass**

Run: `go test -run 'TestGameAsset|TestAssetEndpoints' ./pkg/gui/`
Expected: PASS; the exact-match assertions in `TestAssetEndpointsAndSummary` become prefix checks because the URL is now versioned.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/service.go pkg/gui/types.go pkg/gui/assets_endpoint_test.go
git commit -m "feat(gui): let a campaign borrow its world's banner and icon"
```

---

### Task 6: Campaign-specific artwork, and reverting to the world's

**Files:**
- Modify: `pkg/gui/service.go`, `pkg/gui/server.go`
- Create: `pkg/gui/asset_campaign_prompt_test.go`
- Modify: `frontend/src/types.ts`, `frontend/src/api/client.ts`, `frontend/src/components/LauncherHub.tsx`, `frontend/src/components/launcher/CampaignSettingsModal.tsx`

**Interfaces:**
- Produces: `func (s *Service) campaignArtPrompt(gameID, kind string) string`; `func (s *Service) DeleteGameAsset(gameID, assetKind string) error`; `DELETE /api/game/{id}/{banner|icon}`; `APIClient.deleteGameAsset`.

- [ ] **Step 1: Write the failing test**

Create `pkg/gui/asset_campaign_prompt_test.go` asserting `campaignArtPrompt` carries the campaign name, start location, opening directive, protagonist name and appearance, and the world's art style, and that it falls back to the campaign name when nothing campaign-specific exists.

- [ ] **Step 2: Run it and watch it fail**

Run: `go test -run TestCampaignArtPrompt ./pkg/gui/`
Expected: build failure, `campaignArtPrompt undefined`.

- [ ] **Step 3: Implement**

Replace `GenerateGameAsset`'s world-only prompt with `campaignArtPrompt`, which joins the campaign's start location, opening directive, and protagonist name and appearance, grounded by the world name and rendered in the world's art style. Add `DeleteGameAsset` and a `DELETE` branch on the banner/icon route, and expose `deleteGameAsset` plus a "Use world artwork" control in the campaign settings modal, shown only when `banner_source`/`icon_source` is `campaign`.

- [ ] **Step 4: Run it and watch it pass**

Run: `go test -run TestCampaignArtPrompt ./pkg/gui/ && (cd frontend && npx tsc --noEmit)`
Expected: PASS and a clean typecheck.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui frontend/src
git commit -m "fix(gui): generate campaign artwork from the campaign, not its world"
```

---

### Task 5: Frontend copy and full verification

**Files:**
- Modify: `frontend/src/components/launcher/CampaignSettingsModal.tsx`

- [ ] **Step 1: Update the restart description**

Change "Resets timeline to Turn 0. Retains character & world." to say it keeps artwork, voice and settings and resets only the story and the world's cast.

- [ ] **Step 2: Verify**

Run: `go vet ./... && go test -count=1 ./... && (cd frontend && npx tsc --noEmit)`
Expected: clean.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/launcher/CampaignSettingsModal.tsx
git commit -m "docs(frontend): describe what a campaign restart keeps"
```
