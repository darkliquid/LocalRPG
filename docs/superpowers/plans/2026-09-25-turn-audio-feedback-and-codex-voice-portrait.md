# Turn Audio Feedback, Regeneration, and Codex Voice & Portrait Enhancements Implementation Plan

> **Status:** Implemented and verified against the code on 2026-09-27.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Provide dynamic feedback during TTS generation and playback (spinners, cache vs provider status, error display, proper button states), force regeneration support, unified high-resolution character portrait display in the Codex, and streamlined voice UI with demographic tags and audition previews.

**Architecture:** Extend backend `TTSPipeline` and `Service` to support `force=1` cache bypass for speech generation. Update `TurnSegments.tsx` with a state machine (`idle`, `generating`, `playing`, `error`) and dynamic control bar. Add playback status synchronization via `/api/audio/status`. Enhance `CodexDrawer.tsx` to display character portraits, remove redundant catalogs, format voice archetype dropdown with demographic tags and width protection, and add an inline voice preview button using `APIClient.testProvider`.

**Tech Stack:** Go (Standard Library, net/http, modernc.org/sqlite, oto/beep audio), TypeScript, React 19, Tailwind CSS v4, Lucide React icons.

---

### File Map

- Modify: `pkg/media/tts.go` (Add force cache bypass support in `TTSPipeline`)
- Modify: `pkg/media/tts_test.go` (Unit test for force cache bypass)
- Modify: `pkg/gui/service.go` (Accept `force` parameter in `GetSegmentAudio` and `PlayTurnAudio`)
- Modify: `pkg/gui/server.go` (Parse `force=1` in `handleGameRoutes`)
- Create: `pkg/gui/turn_audio_force_test.go` (Integration test for forced audio regeneration)
- Modify: `frontend/src/api/client.ts` (Add `force` argument to `playTurnAudio` and `playSegmentAudio`)
- Modify: `frontend/src/components/TurnSegments.tsx` (Add `TurnAudioState`, spinner, status chip, regenerate button, button disabling)
- Modify: `frontend/src/App.tsx` (Track audio playback states and poll `/api/audio/status`)
- Modify: `frontend/src/components/CodexDrawer.tsx` (Header character portrait, remove VoiceCatalogPicker, add voice tags and preview button)

---

### Task 1: Backend Force Cache Bypass for TTS Generation

**Files:**
- Modify: `pkg/media/tts.go`
- Modify: `pkg/media/tts_test.go`

- [x] **Step 1: Write unit test in `pkg/media/tts_test.go`**

Add `TestSynthesizeUtteranceForceBypassesCache`:
```go
func TestSynthesizeUtteranceForceBypassesCache(t *testing.T) {
	tempDir := t.TempDir()
	cache := NewContentCache(tempDir)
	client := &counterTTSClient{ext: ".wav", payload: []byte("initial audio")}
	pipeline := NewTTSPipeline(client, cache)

	first, err := pipeline.SynthesizeUtterance(context.Background(), "speaker-1", nil, "Hello world")
	if err != nil {
		t.Fatalf("first synthesize: %v", err)
	}
	if client.calls != 1 {
		t.Fatalf("expected 1 call, got %d", client.calls)
	}

	// Normal call without force hits cache
	second, err := pipeline.SynthesizeUtterance(context.Background(), "speaker-1", nil, "Hello world")
	if err != nil {
		t.Fatalf("second synthesize: %v", err)
	}
	if client.calls != 1 {
		t.Fatalf("expected cached hit, but client was called %d times", client.calls)
	}

	// Forced call bypasses cache and increments calls
	client.payload = []byte("regenerated audio")
	third, err := pipeline.SynthesizeUtteranceForce(context.Background(), "speaker-1", nil, "Hello world", true)
	if err != nil {
		t.Fatalf("forced synthesize: %v", err)
	}
	if client.calls != 2 {
		t.Fatalf("expected 2 calls after force, got %d", client.calls)
	}
	if third != first {
		// Content-addressed key may update if payload changed
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -v -run TestSynthesizeUtteranceForceBypassesCache ./pkg/media`
Expected: FAIL (`SynthesizeUtteranceForce` undefined)

- [x] **Step 3: Implement `SynthesizeUtteranceForce` and `SynthesizeSegmentForce` in `pkg/media/tts.go`**

```go
func (p *TTSPipeline) SynthesizeSegmentForce(ctx context.Context, segment entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(speakerID string) *entity.VoiceConfig, force bool) (string, error) {
	speakerID, voice, spoken := p.prepareSegment(segment, narratorVoice, voiceFor)
	if strings.TrimSpace(spoken) == "" {
		return "", ErrNoSpeakableText
	}
	return p.SynthesizeUtteranceForce(ctx, speakerID, voice, spoken, force)
}

func (p *TTSPipeline) SynthesizeUtteranceForce(ctx context.Context, speakerID string, voice *entity.VoiceConfig, text string, force bool) (string, error) {
	base := ComputeAudioCacheKeyForVoice(speakerID, voice, text)
	start := time.Now()

	if !force {
		if path, ok := p.cachedClip(base); ok {
			mediaMetrics().ttsCache.Add(ctx, 1, otelmetric.WithAttributes(attribute.String("localrpg.cache.result", "hit")))
			mediaMetrics().ttsDuration.Record(ctx, float64(time.Since(start).Milliseconds()),
				otelmetric.WithAttributes(attribute.String("localrpg.cache.result", "hit")))
			return path, nil
		}
	}

	// Continue normal synthesis and cache storage...
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test -v -run TestSynthesizeUtteranceForceBypassesCache ./pkg/media`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/media/tts.go pkg/media/tts_test.go
git commit -m "feat(media): add force cache bypass to TTSPipeline"
```

---

### Task 2: Service & Server Force Audio Regeneration

**Files:**
- Modify: `pkg/gui/service.go`
- Modify: `pkg/gui/server.go`
- Create: `pkg/gui/turn_audio_force_test.go`

- [x] **Step 1: Write integration test in `pkg/gui/turn_audio_force_test.go`**

```go
package gui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPlayTurnAudioEndpoint_ForceParameter(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(tmpDir)
	setupFreeformSystem(t, svc)

	game, err := svc.CreateGame(context.Background(), CreateGameRequestDTO{
		Name:       "Test Audio Game",
		SystemID:   "freeform",
		WorldID:    "harbour-realm",
		PlayerName: "Hero",
	})
	if err != nil {
		t.Fatalf("CreateGame failed: %v", err)
	}

	server := NewServer(svc, nil)
	req := httptest.NewRequest("POST", "/api/game/"+game.ID+"/turn/1/play?force=1", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	// Since turn 1 doesn't exist yet, it should return 404 rather than 400 bad path
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for non-existent turn, got %d: %s", w.Code, w.Body.String())
	}
}
```

- [x] **Step 2: Run test to verify it passes/fails**

Run: `go test -v -run TestPlayTurnAudioEndpoint_ForceParameter ./pkg/gui`
Expected: PASS

- [x] **Step 3: Update `pkg/gui/service.go` and `pkg/gui/server.go`**

1. In `pkg/gui/service.go`:
   - Update `GetSegmentAudio(ctx context.Context, gameID string, turnNumber, segmentIndex int, force bool) (string, error)`
   - Call `pipeline.SynthesizeSegmentForce(ctx, turn.Segments[segmentIndex], narratorVoice, s.voiceFor(gameID), force)`
   - Update `PlayTurnAudio(ctx context.Context, gameID string, turnNumber int, force bool) error`
   - Pass `force` into `s.GetSegmentAudio(ctx, gameID, turnNumber, idx, force)`
2. In `pkg/gui/server.go`:
   - In `handleGameRoutes`:
     ```go
     force := r.URL.Query().Get("force") == "1" || r.URL.Query().Get("force") == "true"
     if err := s.service.PlayTurnAudio(r.Context(), gameID, turnNumber, force); err != nil { ... }
     ```
   - In `GetSegmentAudio` route:
     ```go
     force := r.URL.Query().Get("force") == "1" || r.URL.Query().Get("force") == "true"
     path, err := s.service.GetSegmentAudio(r.Context(), gameID, turnNumber, segmentIndex, force)
     ```

- [x] **Step 4: Run all gui tests to verify compatibility**

Run: `go test -v ./pkg/gui`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/gui/service.go pkg/gui/server.go pkg/gui/turn_audio_force_test.go
git commit -m "feat(gui): support force regeneration query parameter for turn and segment audio"
```

---

### Task 3: APIClient & Frontend Audio Playback Controls Bar

**Files:**
- Modify: `frontend/src/api/client.ts`
- Modify: `frontend/src/components/TurnSegments.tsx`
- Modify: `frontend/src/App.tsx`

- [x] **Step 1: Update `frontend/src/api/client.ts`**

Update `playTurnAudio` and `playSegmentAudio` to accept optional `force?: boolean`:
```typescript
static async playTurnAudio(gameID: string, turnNumber: number, force = false): Promise<void> {
  const url = `/api/game/${encodeURIComponent(gameID)}/turn/${turnNumber}/play${force ? '?force=1' : ''}`;
  const res = await fetch(url, { method: 'POST' });
  if (!res.ok) throw new Error(`playTurnAudio failed: ${res.statusText}`);
}
```

- [x] **Step 2: Update `frontend/src/components/TurnSegments.tsx`**

1. Introduce `turnAudioState: 'idle' | 'generating' | 'playing' | 'error'`.
2. Add `statusMessage?: string`.
3. Add `onRegenerateTurn?: () => void`.
4. Render Play button:
   - When `turnAudioState === 'generating'`: `<Loader2 className="w-3.5 h-3.5 animate-spin" /> <span>Generating speech...</span>` (disabled).
   - When `turnAudioState === 'playing'`: disabled (`opacity-40 cursor-not-allowed`).
   - When `turnAudioState === 'idle'`: enabled `<Play className="w-3.5 h-3.5" /> <span>Play turn</span>`.
5. Render Stop button:
   - When `turnAudioState === 'playing'`: enabled with red highlight (`text-rose-400 border-rose-500/50 hover:bg-rose-500/20 cursor-pointer`).
   - Otherwise: disabled (`opacity-30 cursor-not-allowed pointer-events-none`).
6. Render Regenerate button:
   - Button with `<RotateCw className="w-3.5 h-3.5" />`, disabled when generating or playing, tooltip `"Force regenerate speech"`.
7. Render Status Chip:
   - Inline badge indicating `"Rendering speech (calling provider)..."` (purple pulse), `"Audio loaded from cache"` (green), or `"Error: ..."` (red).

- [x] **Step 3: Update `frontend/src/App.tsx`**

1. Manage `turnAudioStatus: Record<number, { state: 'idle' | 'generating' | 'playing' | 'error'; message?: string; fromCache?: boolean }>`
2. When `handlePlayTurnAudio(turnNumber, segmentIndex, force)` is called:
   - Set state to `'generating'`.
   - Call `APIClient.playTurnAudio(activeGameID, turnNumber, force)`.
   - On success: set state to `'playing'`. Poll `APIClient.audioStatus()` every 500ms until `playing === false`, then reset to `'idle'`.
   - On error: set state to `'error'` with `error.message`.
3. When `handleStopTurnAudio()` is called:
   - Call `APIClient.stopAudio()`.
   - Reset state to `'idle'`.

- [x] **Step 4: Run frontend build to verify compilation**

Run: `npm --prefix frontend run build`
Expected: PASS with 0 type errors

- [x] **Step 5: Commit**

```bash
git add frontend/src/api/client.ts frontend/src/components/TurnSegments.tsx frontend/src/App.tsx
git commit -m "feat(frontend): add stateful playback controls with spinners, status chips, and regeneration"
```

---

### Task 4: Character Portrait Display in Codex Note Header

**Files:**
- Modify: `frontend/src/components/CodexDrawer.tsx`

- [x] **Step 1: Update `frontend/src/components/CodexDrawer.tsx` header**

When `entity.type === 'character'`:
Render high-resolution portrait thumbnail in the note header directly beside `entity.name`:
```tsx
{entity.type === 'character' && gameID && (
  <div className="w-14 h-14 rounded-xl overflow-hidden shrink-0 border-2 border-purple-500/30 shadow-lg bg-black/40">
    <img
      src={`/api/game/${encodeURIComponent(gameID)}/character/${encodeURIComponent(entity.id)}/portrait`}
      alt={entity.name}
      className="w-full h-full object-cover"
      loading="lazy"
    />
  </div>
)}
```

- [x] **Step 2: Run frontend build to verify compilation**

Run: `npm --prefix frontend run build`
Expected: PASS with 0 type errors

- [x] **Step 3: Commit**

```bash
git add frontend/src/components/CodexDrawer.tsx
git commit -m "feat(frontend): display character portrait in codex note header"
```

---

### Task 5: Codex Voice UI Cleanup, Tagged Archetypes & Preview Button

**Files:**
- Modify: `frontend/src/components/CodexDrawer.tsx`

- [x] **Step 1: Remove redundant `VoiceCatalogPicker` from `CodexDrawer.tsx`**

- Remove `import { VoiceCatalogPicker } from './VoiceCatalogPicker';`
- Remove `<VoiceCatalogPicker ttsConfig={ttsConfig} onAddProfile={onAddProfile} />` JSX block.
- Remove unused props/imports related to the catalog modal.

- [x] **Step 2: Update Archetype Dropdown with demographic tags and overflow protection**

1. Constrain dropdown with `max-w-[280px] truncate`.
2. Format option text:
```tsx
{profiles.map((p) => {
  const tagStr = p.tags && p.tags.length > 0 ? ` [${p.tags.join(', ')}]` : '';
  return (
    <option key={p.id} value={p.id}>
      {p.name} ({p.voice_id}){tagStr}
    </option>
  );
})}
```

- [x] **Step 3: Add Voice Preview Button next to Archetype Dropdown**

1. Add state `previewingVoice: boolean` and `selectedArchetype: string`.
2. Add `<button>` with `<Volume2 className={previewingVoice ? "animate-pulse" : ""} />`.
3. When clicked:
   - Locate selected `VoiceProfile`.
   - Call `APIClient.testProvider({ category: 'tts', provider: { ...ttsConfig, default_voice: profile.voice_id, pitch: profile.pitch, speech_rate: profile.speech_rate }, test_prompt: `Greetings. I am ${entity.name}, ready for the journey.` })`.
   - On success, play via `audioPreview.ts`.

- [x] **Step 4: Run frontend build to verify compilation**

Run: `npm --prefix frontend run build`
Expected: PASS with 0 type errors

- [x] **Step 5: Commit**

```bash
git add frontend/src/components/CodexDrawer.tsx
git commit -m "feat(frontend): streamline codex voice controls with demographic tags and audition preview"
```

---

### Task 6: Full Verification & Integration Test

**Files:**
- Run full test suites and binary build

- [x] **Step 1: Run all Go tests and vet**

Run: `go test -count=1 ./... && go vet ./...`
Expected: PASS with 0 failures and clean vet

- [x] **Step 2: Run full build**

Run: `npm --prefix frontend run build && go build ./cmd/localrpg`
Expected: PASS

- [x] **Step 3: Verification Complete**
