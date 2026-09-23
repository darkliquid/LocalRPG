# Kokoro Voice Alignment and Codex Save Safety Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the shipped Kokoro voice set match the model that is actually downloaded, stop a codex save from being able to remove or hide another note, and make the codex voice picker use the same profiles as Settings.

**Architecture:** Move the Kokoro speaker-to-SID table into a model-variant-keyed structure in `pkg/media` and trim `pkg/config.KokoroVoiceProfiles` to the 11 v0.19 speakers. In `pkg/gui`, validate and identity-normalise `SaveEntity`, degrade malformed notes into visible `parse_error` listings, and make merge an explicit confirmed operation. In `frontend`, replace the inline merge `<select>` with a modal, consume the configured `voice_profiles` in `CodexDrawer`, and make quick-create collision-safe.

**Tech Stack:** Go 1.27.1, `github.com/k2-fsa/sherpa-onnx-go`, React 19, TypeScript, Tailwind v4.

**Spec:** `docs/superpowers/specs/2026-09-22-tts-voice-and-codex-fixes-design.md`

---

## File Structure Map

| File Path | Responsibility |
| :--- | :--- |
| `pkg/media/kokoro_voices.go` | New: variant-keyed Kokoro speaker table, `KokoroSpeakersForModel`, `ResolveKokoroSpeakerID` |
| `pkg/media/kokoro_voices_test.go` | New: SID table, fallback, and config cross-check tests |
| `pkg/media/sherpa_tts.go` | Remove the hardcoded map; carry `modelID`; call the scoped resolver |
| `pkg/media/sherpa_tts_test.go` | Update SID expectations |
| `pkg/media/providers.go` | Construct the client with the pinned variant |
| `pkg/models/manager.go` | Record the speaker-map variant on the `kokoro-tts` spec |
| `pkg/config/presets.go` | Trim `KokoroVoiceProfiles` to 11 aligned voices |
| `pkg/config/presets_test.go` | Expect 11; add alignment assertions |
| `pkg/gui/service.go` | Save validation + id forcing; malformed-note listing; merge confirm invariant |
| `pkg/gui/service_test.go` | Save/merge regression tests |
| `pkg/gui/server.go` | Save 400s; merge `confirm` requirement |
| `pkg/gui/types.go` | Add `ParseError` to the entity DTOs; merge request `Confirm` |
| `frontend/src/types.ts` | Mirror `parse_error` and merge `confirm` |
| `frontend/src/api/client.ts` | Send `confirm` on merge |
| `frontend/src/templates/providerPresets.ts` | Trim `KOKORO_VOICE_PROFILES` to 11 |
| `frontend/src/components/SettingsStudio.tsx` | Update the Kokoro button copy |
| `frontend/src/components/CodexDrawer.tsx` | `voiceProfiles` prop; merge modal; save error banner |
| `frontend/src/App.tsx` | Pass configured profiles; id-guarded save; collision-safe quick-create |

---

### Task 1: Add the model-scoped Kokoro speaker table

**Files:**
- Create: `pkg/media/kokoro_voices.go`
- Create: `pkg/media/kokoro_voices_test.go`

- [ ] **Step 1: Write the failing test**

Create `pkg/media/kokoro_voices_test.go`:

```go
package media

import "testing"

func TestKokoroV019SpeakerTable(t *testing.T) {
	want := map[string]int{
		"af": 0, "af_bella": 1, "af_nicole": 2, "af_sarah": 3, "af_sky": 4,
		"am_adam": 5, "am_michael": 6, "bf_emma": 7, "bf_isabella": 8,
		"bm_george": 9, "bm_lewis": 10,
	}
	speakers := KokoroSpeakersForModel(KokoroModelV019)
	if len(speakers) != 11 {
		t.Fatalf("expected 11 v0.19 speakers, got %d", len(speakers))
	}
	for _, s := range speakers {
		if want[s.Name] != s.SID {
			t.Errorf("speaker %q: got SID %d, want %d", s.Name, s.SID, want[s.Name])
		}
	}
	for name, sid := range want {
		if got := ResolveKokoroSpeakerID(KokoroModelV019, name); got != sid {
			t.Errorf("ResolveKokoroSpeakerID(%q) = %d, want %d", name, got, sid)
		}
	}
}

func TestKokoroUnknownVoiceAndModelFallback(t *testing.T) {
	if got := ResolveKokoroSpeakerID(KokoroModelV019, "af_alloy"); got != 0 {
		t.Errorf("unsupported voice should fall back to SID 0, got %d", got)
	}
	if got := ResolveKokoroSpeakerID("no-such-model", "af_bella"); got != 0 {
		t.Errorf("unknown model should fall back to SID 0, got %d", got)
	}
}
```

- [ ] **Step 2: Run the test and confirm it fails**

Run: `go test -run TestKokoroV019 ./pkg/media/`
Expected: FAIL with undefined `KokoroSpeakersForModel` / `KokoroModelV019`.

- [ ] **Step 3: Implement `pkg/media/kokoro_voices.go`**

```go
package media

// KokoroModelV019 is the English Kokoro model pinned by pkg/models. Its speaker
// order comes from the sherpa-onnx release docs and must never be reused for a
// different model variant: v1.0 inserts am_santa at SID 19 and shifts every
// British voice by one.
const KokoroModelV019 = "kokoro-en-v0_19"

type KokoroSpeaker struct {
	Name string
	SID  int
}

var kokoroSpeakersByModel = map[string][]KokoroSpeaker{
	KokoroModelV019: {
		{"af", 0},
		{"af_bella", 1},
		{"af_nicole", 2},
		{"af_sarah", 3},
		{"af_sky", 4},
		{"am_adam", 5},
		{"am_michael", 6},
		{"bf_emma", 7},
		{"bf_isabella", 8},
		{"bm_george", 9},
		{"bm_lewis", 10},
	},
}

func KokoroSpeakersForModel(modelID string) []KokoroSpeaker {
	return kokoroSpeakersByModel[modelID]
}

// ResolveKokoroSpeakerID maps an authored voice ID to the numeric style id the
// model expects, falling back to the first speaker so an unknown voice still
// speaks rather than erroring the turn.
func ResolveKokoroSpeakerID(modelID, voiceID string) int {
	for _, s := range kokoroSpeakersByModel[modelID] {
		if s.Name == voiceID {
			return s.SID
		}
	}
	return 0
}
```

- [ ] **Step 4: Run the test to confirm it passes**

Run: `go test -run TestKokoro ./pkg/media/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/media/kokoro_voices.go pkg/media/kokoro_voices_test.go
git commit -m "fix(media): pin Kokoro speaker ids to the bundled model variant"
```

---

### Task 2: Use the scoped table in the Sherpa client

**Files:**
- Modify: `pkg/media/sherpa_tts.go`
- Modify: `pkg/media/sherpa_tts_test.go`
- Modify: `pkg/media/providers.go`

- [ ] **Step 1: Update the failing test**

Replace `TestKokoroVoiceMapping` in `pkg/media/sherpa_tts_test.go` with:

```go
func TestKokoroVoiceMapping(t *testing.T) {
	tests := []struct {
		voiceID string
		wantSid int
	}{
		{"af", 0},
		{"af_bella", 1},
		{"af_nicole", 2},
		{"am_adam", 5},
		{"am_michael", 6},
		{"bf_emma", 7},
		{"bm_george", 9},
		{"bm_lewis", 10},
		{"unknown_voice", 0},
		{"", 0},
	}

	for _, tt := range tests {
		got := ResolveKokoroSpeakerID(KokoroModelV019, tt.voiceID)
		if got != tt.wantSid {
			t.Errorf("ResolveKokoroSpeakerID(%q) = %d; want %d", tt.voiceID, got, tt.wantSid)
		}
	}
}
```

- [ ] **Step 2: Run the test and confirm it fails**

Run: `go test -run TestKokoroVoiceMapping ./pkg/media/`
Expected: FAIL on the `am_adam`/`bm_george` cases.

- [ ] **Step 3: Delete the old map and wire `modelID`**

In `pkg/media/sherpa_tts.go`:
- Delete the `kokoroSpeakerMap` var and the package-level `ResolveKokoroSpeakerID(voiceID string)` (superseded by Task 1).
- Add a `modelID string` field to `SherpaTTSClient`.
- Change the constructor:

```go
func NewSherpaTTSClient(modelDir string) *SherpaTTSClient {
	return &SherpaTTSClient{modelDir: modelDir, modelID: KokoroModelV019}
}
```

- In `Synthesize`, change the call to `sid = ResolveKokoroSpeakerID(s.modelID, voice.VoiceID)`.

- [ ] **Step 4: Run the media tests**

Run: `go test -count=1 ./pkg/media/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/media/sherpa_tts.go pkg/media/sherpa_tts_test.go
git commit -m "fix(media): resolve Kokoro speakers against the loaded model"
```

---

### Task 3: Trim the profile list to the model's voices

**Files:**
- Modify: `pkg/config/presets.go`
- Modify: `pkg/config/presets_test.go`
- Modify: `frontend/src/templates/providerPresets.ts`
- Modify: `frontend/src/components/SettingsStudio.tsx`

- [ ] **Step 1: Update the failing tests**

Replace `TestKokoroVoiceProfilesPreset` in `pkg/config/presets_test.go`:

```go
func TestKokoroVoiceProfilesPreset(t *testing.T) {
	if len(config.KokoroVoiceProfiles) != 11 {
		t.Fatalf("expected 11 Kokoro voice profiles, got %d", len(config.KokoroVoiceProfiles))
	}
	for _, p := range config.KokoroVoiceProfiles {
		if p.ID == "" || p.VoiceID == "" || len(p.Tags) == 0 {
			t.Errorf("invalid profile: %+v", p)
		}
	}

	preset, ok := config.GetTTSPreset("sherpa-onnx")
	if !ok {
		t.Fatal("missing sherpa-onnx preset")
	}
	if len(preset.VoiceProfiles) != 11 {
		t.Errorf("expected sherpa-onnx preset to have 11 voice profiles, got %d", len(preset.VoiceProfiles))
	}
}
```

- [ ] **Step 2: Run and confirm failure**

Run: `go test -run TestKokoroVoiceProfilesPreset ./pkg/config/`
Expected: FAIL, "expected 11 ... got 27".

- [ ] **Step 3: Replace `KokoroVoiceProfiles` in `pkg/config/presets.go`**

Keep the existing prose/tags for the ten retained voices and add `af`. The order must match the SID table:

```go
var KokoroVoiceProfiles = []VoiceProfile{
	{ID: "af", Name: "Default (American Female)", VoiceID: "af", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "female", "default", "neutral"}, Description: "The model's stock American female voice."},
	{ID: "af_bella", Name: "Bella (American Female)", VoiceID: "af_bella", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "female", "bella", "warm", "friendly"}, Description: "American female voice, warm, approachable, and pleasant."},
	{ID: "af_nicole", Name: "Nicole (American Female)", VoiceID: "af_nicole", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "female", "nicole", "youthful", "energetic"}, Description: "American female voice, brisk, youthful, and direct."},
	{ID: "af_sarah", Name: "Sarah (American Female)", VoiceID: "af_sarah", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "female", "sarah", "poised", "narrative"}, Description: "American female voice, polished, measured, and story-oriented."},
	{ID: "af_sky", Name: "Sky (American Female)", VoiceID: "af_sky", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "female", "sky", "light", "airy"}, Description: "American female voice, light, gentle, and breathy."},
	{ID: "am_adam", Name: "Adam (American Male)", VoiceID: "am_adam", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "male", "adam", "deep", "authoritative"}, Description: "American male voice, deep, steady, and commanding."},
	{ID: "am_michael", Name: "Michael (American Male)", VoiceID: "am_michael", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "male", "michael", "commanding", "formal"}, Description: "American male voice, disciplined, authoritative, and formal."},
	{ID: "bf_emma", Name: "Emma (British Female)", VoiceID: "bf_emma", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"british", "female", "emma", "gentle", "poised"}, Description: "British female voice, elegant, gentle, and softly spoken."},
	{ID: "bf_isabella", Name: "Isabella (British Female)", VoiceID: "bf_isabella", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"british", "female", "isabella", "noble", "melodic"}, Description: "British female voice, aristocratic, melodic, and graceful."},
	{ID: "bm_george", Name: "George (British Male)", VoiceID: "bm_george", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"british", "male", "george", "mature", "distinguished"}, Description: "British male voice, mature, distinguished, and resonant."},
	{ID: "bm_lewis", Name: "Lewis (British Male)", VoiceID: "bm_lewis", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"british", "male", "lewis", "thoughtful", "refined"}, Description: "British male voice, measured, polite, and reflective."},
}
```

- [ ] **Step 4: Mirror the list in `frontend/src/templates/providerPresets.ts`**

Replace `KOKORO_VOICE_PROFILES` with the same 11 entries in camelCase (`voice_id`, `speech_rate`). Update the shelf copy in `frontend/src/components/SettingsStudio.tsx` from "Load Kokoro Voices (27 Profiles)" / "Autofill all 27 ..." to "(11 Profiles)" / "11".

- [ ] **Step 5: Add a cross-check test**

In `pkg/media/kokoro_voices_test.go` add a test that imports `pkg/config` and asserts every `KokoroVoiceProfiles` entry resolves to a non-fallback SID and that the counts match:

```go
func TestKokoroProfilesMatchPinnedModel(t *testing.T) {
	speakers := KokoroSpeakersForModel(KokoroModelV019)
	if len(config.KokoroVoiceProfiles) != len(speakers) {
		t.Fatalf("profile count %d != speaker count %d", len(config.KokoroVoiceProfiles), len(speakers))
	}
	known := make(map[string]bool, len(speakers))
	for _, s := range speakers {
		known[s.Name] = true
	}
	for _, p := range config.KokoroVoiceProfiles {
		if !known[p.VoiceID] {
			t.Errorf("profile %q uses voice %q absent from %s", p.ID, p.VoiceID, KokoroModelV019)
		}
	}
}
```

Add the `pkg/config` import to that test file.

- [ ] **Step 6: Run tests**

Run: `go test -count=1 ./pkg/config/ ./pkg/media/`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add pkg/config/presets.go pkg/config/presets_test.go pkg/media/kokoro_voices_test.go frontend/src/templates/providerPresets.ts frontend/src/components/SettingsStudio.tsx
git commit -m "fix(config): align Kokoro profiles with the downloaded model"
```

---

### Task 4: Make note saves identity-safe and non-destructive

**Files:**
- Modify: `pkg/gui/service.go`
- Modify: `pkg/gui/service_test.go`
- Modify: `pkg/gui/server.go`

- [ ] **Step 1: Write the failing tests**

Add to `pkg/gui/service_test.go` (use the helpers already present for creating a game/service; mirror the style of the existing tests):

```go
func TestSaveEntityRejectsMalformedMarkdown(t *testing.T) {
	// create game, write a valid note, then attempt a save with broken frontmatter
	// assert: error returned, and the on-disk bytes are unchanged
}

func TestSaveEntityForcesFrontmatterIDToFileName(t *testing.T) {
	// save markdown whose frontmatter id is "someone-else"
	// assert: reading the file back yields id == the requested entityID
}
```

- [ ] **Step 2: Run and confirm failure**

Run: `go test -run 'TestSaveEntity' ./pkg/gui/`
Expected: FAIL.

- [ ] **Step 3: Harden `Service.SaveEntity`**

In `pkg/gui/service.go`:

```go
func (s *Service) SaveEntity(ctx context.Context, gameID, entityID, rawMarkdown string) error {
	ent, err := entity.ParseMarkdownEntity([]byte(rawMarkdown))
	if err != nil {
		return fmt.Errorf("save entity %q: %w", entityID, err)
	}
	// The file name is the note's identity. Normalise the frontmatter id so a
	// hand-edited or copied id can never index a note under another note's key.
	ent.ID = entityID
	normalised, err := ent.SerializeMarkdown()
	if err != nil {
		return fmt.Errorf("normalise entity %q: %w", entityID, err)
	}

	gameDir := s.resolver.GameDir(gameID)
	path := filepath.Join(gameDir, "entities", entityID+".md")
	if err := os.WriteFile(path, normalised, 0644); err != nil {
		return fmt.Errorf("write entity file: %w", err)
	}

	store, err := s.store(gameID)
	if err != nil {
		return nil // Non-fatal if db sync fails temporarily
	}

	syncer := storage.NewSyncer(store)
	return syncer.SyncFile(path)
}
```

Confirm `service.go` already imports `entity`, `storage`, `os`, `filepath`, and `fmt` (it does).

- [ ] **Step 4: Map the error to HTTP 400**

In `pkg/gui/server.go`, in the `entity` PUT branch, distinguish validation failures:

```go
if err := s.service.SaveEntity(r.Context(), gameID, entityID, body.Markdown); err != nil {
	http.Error(w, err.Error(), http.StatusBadRequest)
	return
}
```

- [ ] **Step 5: Run tests**

Run: `go test -count=1 ./pkg/gui/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/gui/service.go pkg/gui/server.go pkg/gui/service_test.go
git commit -m "fix(gui): stop a note save from rewriting another note's identity"
```

---

### Task 5: Keep malformed notes visible instead of hiding them

**Files:**
- Modify: `pkg/gui/service.go`
- Modify: `pkg/gui/types.go`
- Modify: `frontend/src/types.ts`

- [ ] **Step 1: Add the DTO fields**

In `pkg/gui/types.go`, add `ParseError bool `json:"parse_error,omitempty"`` to `EntitySummaryDTO` and `EntityDTO`. Mirror as `parse_error?: boolean` in `frontend/src/types.ts` on `EntitySummary` and `EntityNote`.

- [ ] **Step 2: Degrade instead of skipping in `ListEntities`**

When `entity.ParseMarkdownEntity` fails, still append a summary with `ID` from the file name, `Name` from the file name (or the `name:` line if trivially extractable), and `ParseError: true`, instead of `continue`.

- [ ] **Step 3: Flag parse failure in `GetEntity`**

Return the raw markdown with `ParseError: true` rather than a hard error, so the codex can open the note and repair it. Keep the error only when the file cannot be read at all.

- [ ] **Step 4: Test**

Add a `TestListEntitiesIncludesMalformedNote` case asserting a file with bad YAML appears once with `ParseError` set.

Run: `go test -run TestListEntities ./pkg/gui/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/service.go pkg/gui/types.go frontend/src/types.ts pkg/gui/service_test.go
git commit -m "fix(gui): surface unparsable notes instead of dropping them from the codex"
```

---

### Task 6: Make merge explicit and confirmed

**Files:**
- Modify: `pkg/gui/types.go`
- Modify: `pkg/gui/server.go`
- Modify: `frontend/src/types.ts`
- Modify: `frontend/src/api/client.ts`
- Modify: `frontend/src/components/CodexDrawer.tsx`
- Modify: `frontend/src/App.tsx`

- [ ] **Step 1: Require `confirm` on the server**

Add `Confirm bool `json:"confirm"`` to `MergeEntityRequestDTO` in `pkg/gui/types.go`. In the `merge` branch of `server.go`, reject with `400` when `!req.Confirm` before calling `MergeEntities`.

- [ ] **Step 2: Send `confirm` from the client**

In `frontend/src/api/client.ts`, add `confirm: true` to the merge body.

- [ ] **Step 3: Replace the inline select with a modal**

In `frontend/src/components/CodexDrawer.tsx`:
- Remove the `<select>` merge control entirely.
- Add a `Merge note…` button with local state `isMergeOpen` and `mergeTarget`.
- Render a modal listing eligible targets (all entities except the open one), a summary of what moves, and a destructive `Merge and delete "<source name>"` button disabled until a target is chosen.
- On confirm, call `onMerge(entity.id, mergeTarget)` and close the modal. Remove the `window.confirm` from `App.handleMergeEntity` since the modal is now the confirmation.

- [ ] **Step 4: Verify TypeScript**

Run: `cd frontend && npx tsc --noEmit`
Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/types.go pkg/gui/server.go frontend/src/types.ts frontend/src/api/client.ts frontend/src/components/CodexDrawer.tsx frontend/src/App.tsx
git commit -m "fix(frontend): require an explicit confirmation before merging notes"
```

---

### Task 7: Make quick-create collision-safe

**Files:**
- Modify: `frontend/src/App.tsx`

- [ ] **Step 1: Align and guard the slug helper**

Add a single `slugify(name)` helper in `App.tsx` matching `entity.Slugify` semantics (lowercase; keep `[a-z0-9]`; collapse spaces/`-`/`_` to a single `-`; trim trailing `-`). Use it in `handleQuickCreateEntity` and `handleEditInCodexEntity`.

- [ ] **Step 2: Do not overwrite an existing note**

Before writing the template, check the already-loaded `entities` list for a matching id; if found, open that note (fetch it and select it) and mark the finding addressed instead of writing. Otherwise create as today.

- [ ] **Step 3: Verify TypeScript**

Run: `cd frontend && npx tsc --noEmit`
Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/App.tsx
git commit -m "fix(frontend): stop quick-registering a note from overwriting an existing one"
```

---

### Task 8: Use the configured voice profiles in the codex

**Files:**
- Modify: `frontend/src/components/CodexDrawer.tsx`
- Modify: `frontend/src/App.tsx`

- [ ] **Step 1: Add the `voiceProfiles` prop**

In `CodexDrawer.tsx`, add `voiceProfiles?: VoiceProfile[]` to `CodexDrawerProps`, remove the `DEFAULT_VOICE_PROFILES` import, and render `voiceProfiles ?? []` in the archetype `<select>`. `applyVoiceArchetype` looks the profile up in `voiceProfiles`. When the list is empty, render the select disabled with the option text `Configure voices in Settings`.

- [ ] **Step 2: Pass the configured list**

In `App.tsx`, pass `voiceProfiles={config?.media.tts.voice_profiles ?? []}` to `CodexDrawer`.

- [ ] **Step 3: Include the provider in the snippet when set**

Write `provider:` into the emitted `voice:` block when `profile.provider` is defined, so a note reproduces the profile's provider as well as its voice.

- [ ] **Step 4: Verify TypeScript**

Run: `cd frontend && npx tsc --noEmit`
Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/CodexDrawer.tsx frontend/src/App.tsx
git commit -m "fix(frontend): use the configured voice profiles in the codex picker"
```

---

### Task 9: Full verification

- [ ] **Step 1: Backend**

Run: `mise run test:backend` and `mise run lint`
Expected: `go test` passes; `go vet` clean (the pre-existing "sherpa-onnx-go should be direct" gopls hint may remain).

- [ ] **Step 2: Frontend**

Run: `mise run test:frontend`
Expected: clean `tsc --noEmit`.

- [ ] **Step 3: Manual smoke**

1. Settings: load Kokoro voices and confirm 11 profiles.
2. Codex: the archetype picker lists the same 11 voices; apply one and confirm the frontmatter.
3. Save a note and confirm every other note is unchanged; break the frontmatter and confirm the note stays listed as needing repair.
4. Merge requires the modal confirm and removes exactly the chosen source.

- [ ] **Step 4: Update `todo.txt`**

Clear the three resolved bullets.
