# Turn Audio, Persona Attribution, Portrait Pipeline & GUI Polish Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate turn response interactivity lag by decoupling GM turn resolution from background TTS rendering; support cross-turn audio queueing, progressive audio status feedback, and eager sequential playback; strictly gate character voice assignment by gender; generate character portraits in the background with reactive inline chronicle updates and non-zoomable placeholders; and enable text selection and clipboard operations in the Wails desktop GUI.

**Architecture:** 
1. The orchestrator and GUI server release the turn lock and set `turnInFlight = false` immediately when the authoritative `turn` event is announced, while the NDJSON stream remains open in the background to pipe audio progress, speech clips, and portrait events.
2. `streaming_tts.go` emits structured `audio_progress` events across pipeline stages (`waiting` $\rightarrow$ `synthesizing` $\rightarrow$ `encoding` $\rightarrow$ `ready` $\rightarrow$ `failed`), enabling eager playback from sequence #0 and progress pills in the UI.
3. `@persona` records and `AssignVoiceProfile` strictly enforce gender-based voice matching (with pronoun inference).
4. `PortraitWorker` completion triggers a `portrait` event that updates a reactive portrait map in the frontend, swapping all chronicle avatars for that character in-place while keeping placeholders non-clickable.
5. `cmd/localrpg/gui.go` registers Wails' default application menu and window menu options, and CSS selection is restored across documentation, error banners, and prose.

**Tech Stack:** Go 1.27.1, React 19, TypeScript, Tailwind CSS v4, Wails v3 (`github.com/wailsapp/wails/v3`), `beep` audio engine, `modernc.org/sqlite`.

---

## File Structure & Map

### Modified Backend Files
- `cmd/localrpg/gui.go`: Enable Wails v3 application menu, `UseApplicationMenu`, and native context menu.
- `pkg/gui/types.go`: Add `HasCustomPortrait` to `SegmentDTO`, add `AudioProgressDTO`, define `TurnEvent` fields for audio progress and portrait events.
- `pkg/gui/service.go`: Decouple campaign turn lock release from audio synthesis in `TurnSession.Run`; compute `HasCustomPortrait` in `segmentDTOs`; register portrait completion notification callback.
- `pkg/gui/streaming_tts.go`: Add audio pipeline stage tracking (`waiting`, `synthesizing`, `encoding`, `ready`, `failed`); emit `audio_progress` events; make `Close()` flush without blocking turn completion.
- `pkg/media/playback/player.go`: Support multi-queue chaining so a new turn's clips queue behind an active turn's clips without cut-off.
- `pkg/turnstream/parser.go`: Decode full persona fields (`Gender`, `Pronouns`, `Description`, `RoleTags`, `VoiceHint`) in `declarePersona`.
- `pkg/engine/roster.go`: Store persona attributes and voice configurations in `roster`.
- `pkg/engine/timeline.go`: Explicitly set `ent.Gender = persona.Gender` in `stageEntities`.
- `pkg/harness/extractor.go`: Enforce strict gender filtering and pronoun inference in `AssignVoiceProfile`.
- `pkg/engine/portrait_worker.go`: Add `OnPortraitReady` notification hook in `writePortrait`.

### Modified Frontend Files
- `frontend/src/types.ts`: Update `TurnSegment` with `has_custom_portrait?: boolean`; define `AudioProgressEvent` and `PortraitEvent` types.
- `frontend/src/App.tsx`: Drop `turnInFlight` to `false` immediately on `event.type === 'turn'`; handle `audio_progress` and `portrait` events; maintain reactive `characterPortraits` map.
- `frontend/src/components/TurnSegments.tsx`: Support reactive portrait URL resolution; disable cursor-zoom and lightbox click when `has_custom_portrait` is false; render inline audio status pill per segment.
- `frontend/src/components/ActionConsole.tsx`: Render audio progress badge.
- `frontend/src/hooks/useSegmentPlayback.ts`: Support eager playback starting on segment 0 and cross-turn queueing.
- `frontend/src/components/LauncherHub.tsx`, `StoryTheater.tsx`, `SystemsStudio.tsx`, `WorldsStudio.tsx`, `CampaignGallery.tsx`, `WorldGallery.tsx`: Remove blanket `select-none` from root containers.
- `frontend/src/components/MarkdownDocViewer.tsx`, `DocsModal.tsx`, `DebugPanel.tsx`, `MarkdownProse.tsx`: Add `select-text` class to ensure selectable and copyable content.

---

### Task 1: Wails Desktop Text Selection & Clipboard Operations

**Files:**
- Modify: `cmd/localrpg/gui.go:154-196`
- Modify: `frontend/src/components/LauncherHub.tsx:207`
- Modify: `frontend/src/components/StoryTheater.tsx:237`
- Modify: `frontend/src/components/SystemsStudio.tsx:215`
- Modify: `frontend/src/components/WorldsStudio.tsx:488`
- Modify: `frontend/src/components/launcher/CampaignGallery.tsx:215`
- Modify: `frontend/src/components/launcher/WorldGallery.tsx:125`
- Modify: `frontend/src/components/MarkdownDocViewer.tsx:45`
- Modify: `frontend/src/components/DocsModal.tsx:180`
- Modify: `frontend/src/components/DebugPanel.tsx:75`
- Modify: `frontend/src/components/MarkdownProse.tsx:15`
- Modify: `frontend/src/App.tsx:996`

- [ ] **Step 1: Write a Go test checking Wails GUI window options configuration**

Create `cmd/localrpg/gui_menu_test.go`:
```go
package main

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestDefaultApplicationMenuIsConfigured(t *testing.T) {
	menu := application.DefaultApplicationMenu()
	if menu == nil {
		t.Fatal("expected non-nil default application menu")
	}
}
```

- [ ] **Step 2: Run Go test to verify menu availability**

Run: `go test -v ./cmd/localrpg/gui_menu_test.go ./cmd/localrpg/gui.go` (or run backend test suite)  
Expected: PASS

- [ ] **Step 3: Update `cmd/localrpg/gui.go` to set application menu and webview options**

In `cmd/localrpg/gui.go`:
```go
	// 3. Default: Native Wails v3 Desktop Window (Zero-TCP)
	app := application.New(application.Options{
		Name:        "LocalRPG",
		Description: "Local-First LLM Tabletop RPG Client",
		Assets: application.AssetOptions{
			Handler: handler,
		},
	})

	menu := application.DefaultApplicationMenu()
	app.Menu.Set(menu)

	// ... [SetDirectoryPicker callback remains unchanged] ...

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:                      "LocalRPG",
		Width:                      1280,
		Height:                     800,
		MinWidth:                   900,
		MinHeight:                  600,
		URL:                        "/",
		BackgroundType:             application.BackgroundTypeTranslucent,
		UseApplicationMenu:         true,
		DefaultContextMenuDisabled: false,
	})
```

- [ ] **Step 4: Remove blanket `select-none` from frontend containers and add `select-text` to prose/docs/errors**

In `frontend/src/components/LauncherHub.tsx:207`:
Replace:
```tsx
<div className="relative w-full h-full flex overflow-hidden bg-stone-950 text-stone-200 font-sans select-none anim-fade-in">
```
with:
```tsx
<div className="relative w-full h-full flex overflow-hidden bg-stone-950 text-stone-200 font-sans anim-fade-in">
```

In `frontend/src/components/StoryTheater.tsx:237`:
Remove `select-none` from root div class.

In `frontend/src/components/SystemsStudio.tsx:215`:
Remove `select-none` from root div class.

In `frontend/src/components/WorldsStudio.tsx:488`:
Remove `select-none` from root div class.

In `frontend/src/components/launcher/CampaignGallery.tsx:215` and `WorldGallery.tsx:125`:
Remove `select-none` from root modal divs.

In `frontend/src/components/MarkdownDocViewer.tsx`:
Add `select-text` to the markdown container:
```tsx
<div className="prose prose-invert prose-stone max-w-none font-sans text-stone-300 leading-relaxed space-y-4 select-text">
```

In `frontend/src/components/DocsModal.tsx`:
Ensure article content and search results containers include `select-text`.

In `frontend/src/components/DebugPanel.tsx`:
Ensure the log output panel includes `select-text`.

In `frontend/src/components/MarkdownProse.tsx`:
Ensure the prose container includes `select-text`.

In `frontend/src/App.tsx:996`:
Ensure error banner text includes `select-text`:
```tsx
<span className="flex-1 select-text">{turnError}</span>
```

- [ ] **Step 5: Run frontend typecheck**

Run: `mise run test:frontend`  
Expected: PASS with 0 type errors.

- [ ] **Step 6: Commit**

```bash
git add cmd/localrpg/ frontend/
git commit -m "fix(gui): enable desktop text selection, context menu and clipboard shortcuts"
```

---

### Task 2: Strict Character Gender & Voice Profile Assignment

**Files:**
- Modify: `pkg/turnstream/parser.go:240-255`
- Modify: `pkg/engine/roster.go:15-80`
- Modify: `pkg/engine/timeline.go:180-205`
- Modify: `pkg/harness/extractor.go:70-130`
- Test: `pkg/harness/extractor_test.go`
- Test: `pkg/engine/roster_test.go`
- Test: `pkg/turnstream/parser_test.go`

- [ ] **Step 1: Write failing tests for gender-gated voice profile assignment**

In `pkg/harness/extractor_test.go`, add:
```go
func TestAssignVoiceProfile_StrictGenderGating(t *testing.T) {
	profiles := []config.VoiceProfile{
		{ID: "af_female_1", Name: "Female One", VoiceID: "af_female_1", Tags: []string{"american", "female", "young"}},
		{ID: "am_male_1", Name: "Male One", VoiceID: "am_male_1", Tags: []string{"american", "male", "authoritative"}},
	}

	maleChar := &entity.Entity{
		ID:     "sir_garrow",
		Name:   "Sir Garrow",
		Type:   "character",
		Gender: "male",
		Body:   "A young knight with a stern look.",
	}
	AssignVoiceProfile(maleChar, profiles)
	if maleChar.Voice == nil || maleChar.Voice.VoiceID != "am_male_1" {
		t.Fatalf("expected male voice am_male_1 for male character, got %#v", maleChar.Voice)
	}

	femaleChar := &entity.Entity{
		ID:     "lady_elena",
		Name:   "Lady Elena",
		Type:   "character",
		Gender: "female",
		Body:   "An authoritative scholar of magic.",
	}
	AssignVoiceProfile(femaleChar, profiles)
	if femaleChar.Voice == nil || femaleChar.Voice.VoiceID != "af_female_1" {
		t.Fatalf("expected female voice af_female_1 for female character, got %#v", femaleChar.Voice)
	}
}

func TestAssignVoiceProfile_InfersGenderFromPronouns(t *testing.T) {
	profiles := []config.VoiceProfile{
		{ID: "af_female_1", Name: "Female One", VoiceID: "af_female_1", Tags: []string{"female"}},
		{ID: "am_male_1", Name: "Male One", VoiceID: "am_male_1", Tags: []string{"male"}},
	}

	charWithoutExplicitGender := &entity.Entity{
		ID:   "brother_thomas",
		Name: "Brother Thomas",
		Type: "character",
		Body: "He walks silently through the cloisters, his hood pulled low.",
	}
	AssignVoiceProfile(charWithoutExplicitGender, profiles)
	if charWithoutExplicitGender.Voice == nil || charWithoutExplicitGender.Voice.VoiceID != "am_male_1" {
		t.Fatalf("expected inferred male voice am_male_1, got %#v", charWithoutExplicitGender.Voice)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v -run TestAssignVoiceProfile_StrictGenderGating ./pkg/harness/`  
Expected: FAIL (because maleChar matches "young" and picks `af_female_1`).

- [ ] **Step 3: Implement strict gender filtering and pronoun inference in `pkg/harness/extractor.go`**

In `pkg/harness/extractor.go`:
```go
// inferGender attempts to deduce a character's gender from gender fields, state, or prose pronouns.
func inferGender(ent *entity.Entity) string {
	if ent == nil {
		return ""
	}
	if g := strings.ToLower(strings.TrimSpace(ent.Gender)); g != "" {
		return normalizeGender(g)
	}
	if ent.State != nil {
		if raw, ok := ent.State.Get("gender"); ok {
			if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
				return normalizeGender(strings.ToLower(strings.TrimSpace(s)))
			}
		}
	}
	// Heuristic pronoun inference
	text := strings.ToLower(strings.Join([]string{ent.Name, ent.Body, ent.Appearance}, " "))
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !('a' <= r && r <= 'z' || 'A' <= r && r <= 'Z')
	})
	maleScore, femaleScore := 0, 0
	for _, w := range words {
		switch w {
		case "he", "him", "his", "himself", "man", "boy", "sir", "lord", "brother", "father", "king", "prince":
			maleScore++
		case "she", "her", "hers", "herself", "woman", "girl", "lady", "sister", "mother", "queen", "princess":
			femaleScore++
		}
	}
	if maleScore > femaleScore && maleScore > 0 {
		return "male"
	}
	if femaleScore > maleScore && femaleScore > 0 {
		return "female"
	}
	return ""
}

func normalizeGender(raw string) string {
	switch raw {
	case "m", "male", "masculine", "man":
		return "male"
	case "f", "female", "feminine", "woman":
		return "female"
	case "neutral", "nonbinary", "non-binary", "agender":
		return "neutral"
	default:
		return raw
	}
}

// filterProfilesByGender filters profiles strictly by matching gender tag.
func filterProfilesByGender(profiles []config.VoiceProfile, gender string) []config.VoiceProfile {
	if gender == "" {
		return profiles
	}
	var matched []config.VoiceProfile
	for _, p := range profiles {
		hasMale, hasFemale, hasNeutral := false, false, false
		for _, tag := range p.Tags {
			t := strings.ToLower(tag)
			if t == "male" {
				hasMale = true
			} else if t == "female" {
				hasFemale = true
			} else if t == "neutral" || t == "nonbinary" {
				hasNeutral = true
			}
		}
		if gender == "male" && hasMale && !hasFemale {
			matched = append(matched, p)
		} else if gender == "female" && hasFemale && !hasMale {
			matched = append(matched, p)
		} else if (gender == "neutral" || gender == "non-binary") && (hasNeutral || (!hasMale && !hasFemale)) {
			matched = append(matched, p)
		}
	}
	if len(matched) == 0 {
		// Fallback to full list if no profiles exist for this gender
		return profiles
	}
	return matched
}
```

Update `AssignVoiceProfile(ent *entity.Entity, profiles []config.VoiceProfile)` in `pkg/harness/extractor.go`:
```go
func AssignVoiceProfile(ent *entity.Entity, profiles []config.VoiceProfile) {
	if len(profiles) == 0 || ent == nil || !entity.IsCharacterType(ent.Type) || ent.Voice != nil {
		return
	}

	gender := inferGender(ent)
	candidateProfiles := filterProfilesByGender(profiles, gender)

	searchContent := strings.ToLower(strings.Join([]string{
		ent.Name,
		ent.Body,
		ent.Appearance,
		strings.Join(ent.Aliases, " "),
	}, " "))

	// 1. Check direct profile ID match within candidate set
	for _, p := range candidateProfiles {
		if strings.Contains(searchContent, strings.ToLower(p.ID)) {
			ent.Voice = voiceFromProfile(p)
			return
		}
	}

	// 2. Score by tag matches within candidate set
	bestScore := 0
	var bestProfile *config.VoiceProfile
	for i := range candidateProfiles {
		score := 0
		for _, tag := range candidateProfiles[i].Tags {
			if strings.Contains(searchContent, strings.ToLower(tag)) {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			bestProfile = &candidateProfiles[i]
		}
	}

	if bestProfile != nil {
		ent.Voice = voiceFromProfile(*bestProfile)
		return
	}

	// 3. Deterministic hash fallback within candidate set
	h := fnv.New32a()
	h.Write([]byte(ent.ID))
	idx := int(h.Sum32()) % len(candidateProfiles)
	p := candidateProfiles[idx]
	ent.Voice = voiceFromProfile(p)
}
```

- [ ] **Step 4: Update `pkg/engine/timeline.go` to assign `ent.Gender = persona.Gender`**

In `pkg/engine/timeline.go:stageEntities`:
```go
		if persona.Gender != "" {
			ent.Gender = persona.Gender
			ent.State.Set("gender", persona.Gender)
		}
```

- [ ] **Step 5: Update `pkg/turnstream/parser.go` to parse full persona declaration**

In `pkg/turnstream/parser.go:declarePersona`:
```go
func (p *Parser) declarePersona(rec Record) {
	var decl struct {
		Name        string   `json:"name"`
		Type        string   `json:"type"`
		Gender      string   `json:"gender"`
		Pronouns    string   `json:"pronouns"`
		RoleTags    []string `json:"role_tags"`
		VoiceHint   string   `json:"voice_hint"`
		Description string   `json:"description"`
	}
	if err := json.Unmarshal(rec.Payload, &decl); err != nil {
		return
	}
	if id := entity.Slugify(decl.Name); id != "" {
		p.roster.Declare(strings.TrimSpace(decl.Name), id)
		if personaRoster, ok := p.roster.(interface {
			DeclarePersona(id string, decl harness.PersonaDecl)
		}); ok {
			personaRoster.DeclarePersona(id, harness.PersonaDecl{
				Name:        decl.Name,
				Type:        decl.Type,
				Gender:      decl.Gender,
				Pronouns:    decl.Pronouns,
				RoleTags:    decl.RoleTags,
				VoiceHint:   decl.VoiceHint,
				Description: decl.Description,
			})
		}
	}
}
```

- [ ] **Step 6: Update `pkg/engine/roster.go` to store persona and resolve voice matching gender**

In `pkg/engine/roster.go`:
```go
type roster struct {
	store     *storage.Store
	byKey     map[string]string
	personae  map[string]harness.PersonaDecl
	profiles  []config.VoiceProfile
}

func (r *roster) DeclarePersona(id string, decl harness.PersonaDecl) {
	if r == nil || id == "" {
		return
	}
	r.personae[id] = decl
}

func (r *roster) Voice(id string) *entity.VoiceConfig {
	if r == nil {
		return nil
	}
	if r.store != nil {
		if voice := harness.ResolveSpeakerVoice(r.store, id); voice != nil {
			return voice
		}
	}
	// Check declared persona
	if persona, ok := r.personae[id]; ok {
		tempEnt := &entity.Entity{
			ID:          id,
			Name:        persona.Name,
			Type:        "character",
			Gender:      persona.Gender,
			Body:        persona.Description,
			Tags:        persona.RoleTags,
		}
		harness.AssignVoiceProfile(tempEnt, r.profiles)
		return tempEnt.Voice
	}
	return nil
}
```

- [ ] **Step 7: Run backend tests**

Run: `go test -v ./pkg/harness/... ./pkg/turnstream/... ./pkg/engine/...`  
Expected: ALL PASS

- [ ] **Step 8: Commit**

```bash
git add pkg/harness/ pkg/engine/ pkg/turnstream/
git commit -m "feat(harness): enforce strict gender matching for persona voice profile assignment"
```

---

### Task 3: Turn Interactivity Decoupling & Cross-Turn Audio Queue Chaining

**Files:**
- Modify: `pkg/gui/service.go:1545-1670`
- Modify: `pkg/gui/streaming_tts.go:350-377`
- Modify: `pkg/media/playback/player.go:177-225`
- Modify: `frontend/src/App.tsx:313-370`
- Test: `pkg/gui/turn_test.go`
- Test: `pkg/media/playback/player_test.go`

- [ ] **Step 1: Write a test verifying concurrent turns do not return 409 after text completion**

In `pkg/gui/turn_test.go`:
```go
func TestConsecutiveTurnsDoNotConflictWhileAudioIsRendering(t *testing.T) {
	// Setup service with active campaign
	svc, gameID := setupTestServiceWithGame(t)

	session1, err := svc.BeginTurn(gameID)
	if err != nil {
		t.Fatalf("BeginTurn(1) failed: %v", err)
	}

	// Simulate session1 completing turn generation and releasing lock
	session1.Close()

	// Second turn should be permitted immediately
	session2, err := svc.BeginTurn(gameID)
	if err != nil {
		t.Fatalf("BeginTurn(2) failed while audio is in background: %v", err)
	}
	session2.Close()
}
```

- [ ] **Step 2: Update `TurnSession.Run` in `pkg/gui/service.go` to release campaign lock immediately on `turn` event**

In `pkg/gui/service.go:TurnSession.Run`:
```go
	dto := t.service.turnDTO(*turn, t.store, t.cfg, t.gameID)
	if err := announce(TurnEvent{Type: "turn", Turn: &dto}); err != nil {
		return err
	}

	// Release campaign turn lock immediately so the player can submit the next turn
	// without waiting for remaining background TTS audio to synthesize.
	t.Close()
```

- [ ] **Step 3: Modify `streaming_tts.go` so `Close()` flushes without blocking session return**

In `pkg/gui/streaming_tts.go`:
```go
// Close flushes the pending group and stops accepting work. Workers drain in background.
func (s *sentenceStreamer) Close() {
	if s == nil {
		return
	}
	s.Flush()
	s.closeOne.Do(func() { close(s.queue) })
}

// Wait blocks until all queued synthesis jobs have finished.
func (s *sentenceStreamer) Wait() {
	if s == nil {
		return
	}
	s.wg.Wait()
}
```
In `TurnSession.Run`:
Launch `streamer.Wait()` in `t.service.goBackground` along with `finishTurnAudio`, rather than in synchronous `defer streamer.Close()`.

- [ ] **Step 4: Update `Player.PlayQueue` in `pkg/media/playback/player.go` to chain multiple queues without cutting off prior audio**

In `pkg/media/playback/player.go`:
```go
// EnqueueQueue appends a clip stream to the playback pipeline. If audio is already
// playing, the new clips will play immediately after the active queue drains.
func (p *Player) EnqueueQueue(clips <-chan string) error {
	if !p.Available() {
		return ErrUnavailable
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	// If no stream is active, start a new one
	if !p.playing || p.otoPlayer == nil {
		queue := newQueueStreamer(clips)
		return p.playStreamerLocked(queue, []io.Closer{queue}, 0)
	}

	// Otherwise, chain clips into the active queue streamer
	if qs, ok := p.streamer.(*queueStreamer); ok {
		go func() {
			for clip := range clips {
				qs.enqueue(clip)
			}
		}()
		return nil
	}

	queue := newQueueStreamer(clips)
	return p.playStreamerLocked(queue, []io.Closer{queue}, 0)
}
```

- [ ] **Step 5: Update `frontend/src/App.tsx` to set `turnInFlight = false` upon receiving `TurnEvent{Type: "turn"}`**

In `frontend/src/App.tsx:handleActionSubmit`:
```tsx
          } else if (event.type === 'turn' && event.turn) {
            const turn = event.turn;
            setChronicle((prev) => [...prev, turn]);
            streamProcessorRef.current.reset();
            setStreamedSegments([]);
            setPendingAction(null);
            setFundsError(null);
            setRateLimitUntil(null);
            fetchLimits();
            refreshCorpus();
            // Re-enable interaction immediately upon text arrival!
            setTurnInFlight(false);
          }
```
In `finally`:
```tsx
    } finally {
      abortRef.current = null;
      setTurnInFlight(false);
      setPendingAction(null);
      setToolActivity(null);
      // Note: do not stop streamedSpeech here if it is still draining the turn's queued audio
    }
```

- [ ] **Step 6: Run tests to verify**

Run: `go test -v ./pkg/gui/ -run TestTurnSession`  
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add pkg/gui/ pkg/media/playback/ frontend/src/App.tsx
git commit -m "feat(turn): decouple turn interactivity from background audio rendering and chain playback"
```

---

### Task 4: Audio Progress Tracking & Eager Sequential Playback

**Files:**
- Modify: `pkg/gui/types.go:120-170`
- Modify: `pkg/gui/streaming_tts.go:40-230`
- Modify: `pkg/gui/service.go:1590-1615`
- Modify: `frontend/src/types.ts:70-120`
- Modify: `frontend/src/App.tsx:280-360`
- Modify: `frontend/src/hooks/useStreamedSpeech.ts:1-66`
- Modify: `frontend/src/hooks/useSegmentPlayback.ts:30-140`
- Modify: `frontend/src/components/TurnSegments.tsx:80-140`
- Modify: `frontend/src/components/ActionConsole.tsx:1-90`
- Test: `pkg/gui/streaming_tts_test.go`
- Test: `frontend/src/hooks/useSegmentPlayback.test.ts`

- [ ] **Step 1: Define `AudioProgressDTO` in backend and frontend types**

In `pkg/gui/types.go`:
```go
type AudioProgressDTO struct {
	TurnNumber    int    `json:"turn_number"`
	Sequence      int    `json:"sequence"`
	TotalSegments int    `json:"total_segments"`
	Stage         string `json:"stage"` // "waiting" | "synthesizing" | "encoding" | "ready" | "failed"
	ReadyCount    int    `json:"ready_count"`
	AudioKey      string `json:"audio_key,omitempty"`
	AudioURL      string `json:"audio_url,omitempty"`
}
```
Add `AudioProgress *AudioProgressDTO` to `TurnEvent` in `pkg/gui/types.go`.

In `frontend/src/types.ts`:
```ts
export interface AudioProgressEvent {
  turn_number: number;
  sequence: number;
  total_segments: number;
  stage: 'waiting' | 'synthesizing' | 'encoding' | 'ready' | 'failed';
  ready_count: number;
  audio_key?: string;
  audio_url?: string;
}
```
Add `audio_progress?: AudioProgressEvent;` to `TurnEvent` in `frontend/src/types.ts`.

- [ ] **Step 2: Write failing test in `pkg/gui/streaming_tts_test.go` for audio progress emission**

```go
func TestStreamerEmitsAudioProgressEvents(t *testing.T) {
	var stages []string
	streamer := newSentenceStreamer(context.Background(), mockPipeline, nil, nil, 1, nil)
	streamer.SetProgressObserver(func(progress AudioProgressDTO) {
		stages = append(stages, progress.Stage)
	})

	streamer.FeedSegment(turnstream.Event{Kind: turnstream.KindNarration, Text: "The castle gates creak open."})
	streamer.Flush()
	streamer.Wait()

	if len(stages) == 0 || stages[len(stages)-1] != "ready" {
		t.Fatalf("expected progress reaching 'ready', got: %v", stages)
	}
}
```

- [ ] **Step 3: Implement progress tracking in `sentenceStreamer` (`pkg/gui/streaming_tts.go`)**

In `pkg/gui/streaming_tts.go`:
- Add `progressObserver func(AudioProgressDTO)` to `sentenceStreamer`.
- When a job is enqueued: emit `stage: "waiting"`.
- In `worker()`: before calling `Synthesize*`: emit `stage: "synthesizing"`.
- When audio returns before writing to cache: emit `stage: "encoding"`.
- When cache file is written: emit `stage: "ready"`.
- On error: emit `stage: "failed"`.
In `service.go:TurnSession.Run`:
Wire `streamer.SetProgressObserver` to `announce(TurnEvent{Type: "audio_progress", AudioProgress: &p})`.

- [ ] **Step 4: Update `useStreamedSpeech` and `useSegmentPlayback` for eager sequential playback**

In `frontend/src/hooks/useStreamedSpeech.ts`:
- Track items by sequence number.
- As soon as sequence #0 arrives, start playback immediately.
- If sequence #0 finishes and sequence #1 is in the queue, immediately play sequence #1.
- If sequence #1 is not ready, pause draining and wait until sequence #1 arrives.

In `frontend/src/hooks/useSegmentPlayback.ts`:
- Expose `segmentProgress: Record<number, string>` (mapping segment index to stage).
- Play sequence 0 as soon as it has a URL, advancing sequentially.

- [ ] **Step 5: Add audio progress pill in `ActionConsole.tsx` and segment card status in `TurnSegments.tsx`**

In `frontend/src/components/ActionConsole.tsx`:
Render a subtle status badge when audio is in flight:
```tsx
{audioProgress && audioProgress.ready_count < audioProgress.total_segments && (
  <div className="flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-sans font-medium bg-amber-500/10 text-amber-300 border border-amber-500/20 anim-fade-in">
    <span className="w-1.5 h-1.5 rounded-full bg-amber-400 animate-pulse" />
    <span>Audio: {audioProgress.ready_count}/{audioProgress.total_segments} ready ({audioProgress.stage})</span>
  </div>
)}
```

In `frontend/src/components/TurnSegments.tsx`:
Display a status indicator for segments whose audio is currently processing.

- [ ] **Step 6: Run frontend and backend tests**

Run: `mise run test`  
Expected: ALL PASS

- [ ] **Step 7: Commit**

```bash
git add pkg/gui/ frontend/
git commit -m "feat(audio): add progressive audio status feedback and eager sequential playback"
```

---

### Task 5: Background Character Portrait Generation Notification & Reactive Chronicle Lightbox

**Files:**
- Modify: `pkg/gui/types.go:40-60`
- Modify: `pkg/gui/service.go:420-465, 1850-1910`
- Modify: `pkg/engine/portrait_worker.go:50-145`
- Modify: `frontend/src/types.ts:40-60`
- Modify: `frontend/src/App.tsx:60-120, 310-360`
- Modify: `frontend/src/components/TurnSegments.tsx:140-165`
- Test: `pkg/gui/character_portrait_test.go`
- Test: `pkg/engine/portrait_worker_test.go`

- [ ] **Step 1: Write test for `HasCustomPortrait` in `segmentDTOs` and portrait notification hook**

In `pkg/gui/character_portrait_test.go`:
```go
func TestSegmentDTO_HasCustomPortraitFlag(t *testing.T) {
	// When portrait file does not exist on disk, HasCustomPortrait should be false
	seg := entity.TurnSegment{
		Kind:      "speech",
		Speaker:   "Kaelen",
		SpeakerID: "kaelen",
		Text:      "Hello traveler.",
	}
	dtos := segmentDTOs([]entity.TurnSegment{seg}, "test-game", clipPlan{}, nil)
	if len(dtos) == 0 || dtos[0].HasCustomPortrait {
		t.Fatalf("expected HasCustomPortrait=false for character without custom portrait file, got %v", dtos[0].HasCustomPortrait)
	}
}
```

- [ ] **Step 2: Add `HasCustomPortrait` to `SegmentDTO` in `pkg/gui/types.go` and populate in `service.go`**

In `pkg/gui/types.go`:
```go
type SegmentDTO struct {
	Kind              string   `json:"kind"`
	Speaker           string   `json:"speaker,omitempty"`
	SpeakerID         string   `json:"speaker_id,omitempty"`
	Text              string   `json:"text"`
	PortraitURL       string   `json:"portrait_url,omitempty"`
	HasCustomPortrait bool     `json:"has_custom_portrait"`
	CheckRef          string   `json:"check_ref,omitempty"`
	Player            bool     `json:"player,omitempty"`
	Duration          float64  `json:"duration"`
	AudioURLs         []string `json:"audio_urls,omitempty"`
	ClipGroup         string   `json:"clip_group,omitempty"`
}
```

In `pkg/gui/service.go:segmentDTOs`:
Check if `ent.Portrait != ""` and file exists in game dir. Set `dto.HasCustomPortrait = true` if so, `false` otherwise.

- [ ] **Step 3: Add `OnPortraitReady` callback to `PortraitWorker` and emit `portrait` event from Service**

In `pkg/engine/portrait_worker.go`:
Add callback:
```go
type PortraitWorker struct {
	// ... existing fields ...
	onReady func(gameID, characterID, relPath string)
}

func (w *PortraitWorker) SetOnReady(fn func(gameID, characterID, relPath string)) {
	w.mu.Lock()
	w.onReady = fn
	w.mu.Unlock()
}
```
In `writePortrait`:
After writing portrait file and updating note:
```go
	w.mu.Lock()
	cb := w.onReady
	w.mu.Unlock()
	if cb != nil {
		cb(gameID, ent.ID, relPath)
	}
```

In `pkg/gui/service.go`:
When initializing `PortraitWorker`:
```go
portraitWorker.SetOnReady(func(gameID, characterID, relPath string) {
	s.broadcastPortraitReady(gameID, characterID, relPath)
})
```
Define `broadcastPortraitReady` to announce `TurnEvent{Type: "portrait", CharacterID: characterID, PortraitURL: fmt.Sprintf("/api/game/%s/character/%s/portrait?t=%d", gameID, characterID, time.Now().UnixMilli()), HasCustomPortrait: true}` to active turn sessions.

- [ ] **Step 4: Update `frontend/src/types.ts` and `frontend/src/App.tsx`**

In `frontend/src/types.ts`:
Add `has_custom_portrait?: boolean;` to `TurnSegment`.
Add `PortraitEvent` type:
```ts
export interface PortraitEvent {
  character_id: string;
  portrait_url: string;
  has_custom_portrait: boolean;
}
```
In `frontend/src/App.tsx`:
Maintain reactive portrait map:
```tsx
const [characterPortraits, setCharacterPortraits] = useState<Record<string, { url: string; hasCustom: boolean }>>({});
```
On `event.type === 'portrait'`:
```tsx
} else if (event.type === 'portrait' && event.portrait) {
  const p = event.portrait;
  setCharacterPortraits((prev) => ({
    ...prev,
    [p.character_id]: { url: p.portrait_url, hasCustom: p.has_custom_portrait }
  }));
}
```
Pass `characterPortraits` to `ChronicleView`.

- [ ] **Step 5: Update `TurnSegments.tsx` to disable lightbox for placeholder portraits and reactively swap images**

In `frontend/src/components/TurnSegments.tsx`:
Resolve portrait state:
```tsx
const portraitState = segment.speaker_id && characterPortraits?.[segment.speaker_id]
  ? characterPortraits[segment.speaker_id]
  : { url: segment.portrait_url, hasCustom: !!segment.has_custom_portrait };

const portraitURL = portraitState.url;
const hasCustomPortrait = portraitState.hasCustom;
```
In the avatar render:
```tsx
{portraitURL && (
  <div
    onClick={() => {
      if (hasCustomPortrait) {
        openLightbox(portraitURL, segment.speaker || 'Portrait');
      }
    }}
    className={`w-9 h-9 rounded-full overflow-hidden shrink-0 border-2 shadow-md transition-transform ${
      hasCustomPortrait ? 'cursor-zoom-in hover:scale-105' : 'cursor-default opacity-85'
    } ${segment.player ? 'border-sky-400/80' : 'border-purple-400/80'}`}
    title={hasCustomPortrait ? `View portrait of ${segment.speaker || 'character'}` : (segment.speaker || 'Character')}
  >
    <img
      src={portraitURL}
      alt={segment.speaker || 'Speaker portrait'}
      className="w-full h-full object-cover"
      loading="lazy"
    />
  </div>
)}
```

- [ ] **Step 6: Run tests and verify**

Run: `mise run test`  
Expected: ALL PASS

- [ ] **Step 7: Commit**

```bash
git add pkg/gui/ pkg/engine/ frontend/
git commit -m "feat(portrait): emit background portrait events, swap chronicle avatars inline and disable placeholder lightbox"
```

---

### Task 6: Full Integration Verification & Regressional Checks

**Files:**
- Verify: Full codebase

- [ ] **Step 1: Run frontend typecheck**

Run: `mise run test:frontend`  
Expected: PASS with 0 errors.

- [ ] **Step 2: Run backend tests**

Run: `mise run test:backend`  
Expected: ALL PASS with 0 failures.

- [ ] **Step 3: Run full linter suite**

Run: `mise run lint`  
Expected: PASS (markdownlint, goreleaser check, go vet clean).

- [ ] **Step 4: Update embedded docs if config or catalogue changed**

Run: `go test ./pkg/gui -update-docs`  
Expected: PASS (docs in sync).

- [ ] **Step 5: Build binary and verify assets packaging**

Run: `mise run build`  
Expected: Successful build of `bin/localrpg` with embedded frontend assets.

- [ ] **Step 6: Final commit**

```bash
git commit --allow-empty -m "chore: verify full test and build pass for audio, portrait and gui polish"
```
