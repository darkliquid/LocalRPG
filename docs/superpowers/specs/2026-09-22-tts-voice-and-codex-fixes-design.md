# Design Spec: Kokoro Voice Alignment and Codex Save Safety

**Date:** 2026-09-22
**Status:** Draft
**Target:** `pkg/config`, `pkg/media`, `pkg/gui`, `frontend` (`CodexDrawer`, `SettingsStudio`, `App`)

---

## 1. Executive Summary

Three defects reported after the Sherpa-ONNX TTS and codex usability work need to be closed:

1. **Kokoro voice mismatch.** The pinned model (`kokoro-en-v0_19`) has **11 speakers** (IDs `0-10`), but `pkg/config.KokoroVoiceProfiles` ships **27** profiles and `pkg/media.kokoroSpeakerMap` encodes the **53-speaker v1.0** layout. Every British (`bf_*`, `bm_*`) voice therefore resolves to the wrong speaker, and 17 profiles cannot be synthesised at all.
2. **Codex save can remove a note.** A "save" has been observed deleting a different note. The only code path that removes a note is `MergeEntities`, which is wired to an inline `<select onChange>` guarded by a native `window.confirm`; the same drawer also lets saves silently rewrite a note's canonical identity because the file name and the frontmatter `id` are allowed to diverge.
3. **Codex voice archetypes use a hardcoded list.** `CodexDrawer` applies `DEFAULT_VOICE_PROFILES` (4 built-in fantasy archetypes) instead of the voice profiles configured in Settings, so a campaign running Kokoro cannot pick any Kokoro voice from the codex.

This spec defines the corrected speaker data, the save/merge safety model, and the single source of truth for voice profiles in the UI.

---

## 2. Issue 1 - Kokoro Voice Alignment

### 2.1 Findings

`pkg/models/manager.go` pins `kokoro-tts` to `kokoro-en-v0_19.tar.bz2`. The official mapping for that release (sherpa-onnx `kokoro.rst`, section *kokoro-en-v0_19 (English, 11 speakers)*) is:

| SID | Speaker |
| :-- | :------ |
| 0 | `af` |
| 1 | `af_bella` |
| 2 | `af_nicole` |
| 3 | `af_sarah` |
| 4 | `af_sky` |
| 5 | `am_adam` |
| 6 | `am_michael` |
| 7 | `bf_emma` |
| 8 | `bf_isabella` |
| 9 | `bm_george` |
| 10 | `bm_lewis` |

The current `kokoroSpeakerMap` (`pkg/media/sherpa_tts.go:20`) instead assumes the 53-speaker v1.0 layout, which inserts `am_santa` at 19 and shifts `bf_alice` onward by one. Consequences:

- `af_bella` resolves to SID `2` but SID `2` is `af_nicole`.
- `bm_george` resolves to `25` (`bm_fable` in v1.0) and SID `25` does not exist in an 11-speaker model, so sherpa-onnx clamps/rejects it.
- 17 of the 27 profiles (`af_alloy`, `af_aoede`, `af_heart`, `af_jessica`, `af_kore`, `af_nova`, `af_river`, `am_echo`, `am_eric`, `am_fenrir`, `am_liam`, `am_onyx`, `am_puck`, `bf_alice`, `bf_lily`, `bm_daniel`, `bm_fable`) have no speaker in this model.

The user count of "10 voices" is correct for the *named* profiles: the model exposes the 10 profiles we already authored (`af_bella`..`bm_lewis`) plus the unnamed `af` default voice, for 11 speakers total.

### 2.2 Decision

**Keep the pinned English `kokoro-en-v0_19` model and align both the profile list and the SID map to its 11 speakers.** The alternative - upgrading to `kokoro-multi-lang-v1_0` (53 speakers, ~2x download, and requires lexicon files plus Chinese text handling) purely to retain 17 unused voices - is out of scope and would contradict the local-first, small-footprint intent of the model manager. A future model swap remains possible because the speaker map is tied to a model variant (below).

### 2.3 Model-scoped speaker map

The SID table is a property of the model, not of the app. Introduce a variant-keyed map so a future `ModelSpec` cannot silently inherit the wrong table:

```go
// pkg/media/kokoro_voices.go
type KokoroSpeaker struct {
    Name string
    SID  int
}

const KokoroModelV019 = "kokoro-en-v0_19"

var kokoroSpeakers = map[string][]KokoroSpeaker{
    KokoroModelV019: {
        {"af", 0}, {"af_bella", 1}, {"af_nicole", 2}, {"af_sarah", 3},
        {"af_sky", 4}, {"am_adam", 5}, {"am_michael", 6}, {"bf_emma", 7},
        {"bf_isabella", 8}, {"bm_george", 9}, {"bm_lewis", 10},
    },
}

func KokoroSpeakersForModel(modelID string) []KokoroSpeaker
func ResolveKokoroSpeakerID(modelID, voiceID string) int    // fallback 0 (af)
```

`SherpaTTSClient` gains a `modelID` (default `KokoroModelV019`) supplied by the factory, and calls `ResolveKokoroSpeakerID(s.modelID, voice.voiceID)`. The `ModelSpec` for `kokoro-tts` records the same variant id so the two cannot drift.

To keep the map and the profile list honest, `pkg/media` also asserts:
- every `config.KokoroVoiceProfiles` entry's `VoiceID` resolves to a valid SID for the pinned variant;
- no profile references a speaker absent from the model.

### 2.4 Profile list

`KokoroVoiceProfiles` (`pkg/config/presets.go:152`) is reduced to the 11 speakers the model actually has. The 10 named profiles keep their existing prose, tags and defaults; one new entry is added for the unnamed default:

| ID | Name | VoiceID | SID |
| :-- | :--- | :------ | :-- |
| `af` | Default (American Female) | `af` | 0 |
| `af_bella` | Bella (American Female) | `af_bella` | 1 |
| `af_nicole` | Nicole (American Female) | `af_nicole` | 2 |
| `af_sarah` | Sarah (American Female) | `af_sarah` | 3 |
| `af_sky` | Sky (American Female) | `af_sky` | 4 |
| `am_adam` | Adam (American Male) | `am_adam` | 5 |
| `am_michael` | Michael (American Male) | `am_michael` | 6 |
| `bf_emma` | Emma (British Female) | `bf_emma` | 7 |
| `bf_isabella` | Isabella (British Female) | `bf_isabella` | 8 |
| `bm_george` | George (British Male) | `bm_george` | 9 |
| `bm_lewis` | Lewis (British Male) | `bm_lewis` | 10 |

`pkg/config.TTSPresets["sherpa-onnx"].DefaultVoice` stays `af_bella` (SID 1), which now resolves correctly.

The frontend mirror `KOKORO_VOICE_PROFILES` (`frontend/src/templates/providerPresets.ts:84`) is updated to the same 11 entries, and the Settings button copy changes from "Load Kokoro Voices (27 Profiles)" to "(11 Profiles)".

### 2.5 Guarding against the next model swap

`AssignVoiceProfile` (`pkg/harness/extractor.go:60`) already folds its deterministic FNV fallback into `len(profiles)`, so trimming the list needs no change there; it simply runs the fallback more often, which is acceptable. The variant constant lives in `pkg/media` and is duplicated as the `kokoro-tts` spec's variant so the two can be asserted equal in a test.

---

## 3. Issue 2 - Codex Note Save Safety

### 3.1 Findings

A "save" removing a note has exactly one destructive path and several enabling defects.

**A. Merge is an accidental-click hazard (primary).** `CodexDrawer` renders the "Merge into..." `<select>` immediately left of the Save button, wired to `onChange`. Selecting any option fires `onMerge(entity.id, target)` with no intermediate step; the only protection is a native `window.confirm` whose OK button/Enter key is frequently pressed reflexively. Any wheel/keyboard interaction while aiming for Save can therefore remove the open note after one Enter press. This is the most plausible mechanism behind "I was just saving it".

**B. File name and frontmatter identity are allowed to diverge (primary).** `Service.SaveEntity` (`pkg/gui/service.go:544`) writes whatever text the user typed to `<entityID>.md` verbatim, then `Syncer.SyncFile` indexes the note under `Entity.ID`, which comes from the frontmatter `id` field (`pkg/entity/entity.go:66`). `ListEntities` and `GetEntity` key on the *file name*. If a save carries a frontmatter `id` that differs from the file name - a hand edit, or a starter template copied from another note - the index is written under the wrong primary key and can overwrite a different note's row. The file survives, so the codex listing hides the corruption until the graph or history disagrees.

**C. Malformed saves vanish silently.** `ListEntities` (`pkg/gui/service.go:466`) `continue`s past any file whose frontmatter fails to parse. A save that breaks the `---` fences or YAML makes the note disappear from the codex with no error, which reads as deletion.

**D. Quick-create can overwrite.** `handleQuickCreateEntity` and `handleEditInCodexEntity` (`frontend/src/App.tsx:250,266`) compute a slug with a local regex that differs from `entity.Slugify`, then PUT a starter template. If the slug collides with an existing note (for example quick-registering "Evelyn" when `evelyn.md` exists), the existing note is replaced by an empty template.

**E. Late save responses clobber selection.** `handleSaveEntity` awaits `getEntity` and then `setSelectedEntity(updated)` unconditionally, so a save that finishes after the user selected another note re-selects the wrong one.

### 3.2 Decision

Make destructive actions deliberate, and make one file mean exactly one identity.

**Merge becomes a two-step, explicit action.**
- Replace the inline `<select>` with a `Merge note…` button that opens a small modal.
- The modal contains a target picker, a plain-language description of what moves, and a destructive button labelled `Merge and delete "<source name>"`.
- No native `window.confirm`; the confirm is a real button in the modal.
- The button is disabled until a target is chosen, and the source and target names are shown side by side.

**Save enforces identity.**
- `Service.SaveEntity` parses the submitted markdown before writing.
- If it does not parse, return `400` and leave the file untouched; the client surfaces the error.
- On success, force the frontmatter `id` to equal `entityID` (the file name is authoritative) and re-serialise before writing, so the index key can never diverge from the file name.
- If the caller wants to deliberately rename a note, that is a separate future operation; it must not happen implicitly from the textarea.

**Malformed notes stay visible.**
- `ListEntities` includes a note even when parsing fails, falling back to the file name and an empty type, and marks it (`parse_error: true`) so the codex shows a repair prompt instead of hiding it.
- `GetEntity` returns the raw markdown and a parse error flag, so the note can be edited back to validity.

**Quick-create never overwrites.**
- The client checks for an existing note first (reusing `ListEntities`, which the codex already holds) and opens it instead of writing a template.
- Slug derivation is centralised: expose the backend `entity.Slugify` result by creating notes through a create endpoint that generates and returns the id, rather than trusting a local regex. Until then, the local helper is aligned to `entity.Slugify` and a collision test is added.

**Save responses are id-guarded.**
- `handleSaveEntity` only applies the refreshed note when `updated.id === ` the currently selected id; otherwise it just refreshes the corpus.

### 3.3 Server-side merge invariants

`MergeEntities` already writes the survivor before removing the source, which is the correct order. Add these invariants and tests:

- Refuse when the source and target resolve to the same file after slug comparison.
- Refuse when the target file does not exist (currently `MergeEntities` reads both and errors on a missing target, but the HTTP layer maps this through `writeGameError`; verify it is a 4xx, not a 500).
- Never remove a file when the survivor write fails (already true; add a regression test).
- The merge endpoint requires an explicit `into` (already true); additionally require the body to carry `confirm: true` so a stray POST cannot fold notes.

---

## 4. Issue 3 - One Voice-Profile Source of Truth

### 4.1 Findings

Voice profiles exist in three places:

| Location | Content | Used by |
| :-- | :-- | :-- |
| `frontend/src/templates/providerPresets.ts:DEFAULT_VOICE_PROFILES` | 4 fantasy archetypes | `CodexDrawer` voice archetype picker, Settings "Load Fantasy Defaults" |
| `frontend/src/templates/providerPresets.ts:KOKORO_VOICE_PROFILES` | 27 Kokoro voices | Settings "Load Kokoro Voices" |
| `pkg/config.TTSPresets["sherpa-onnx"].VoiceProfiles` / config default | same two lists | persisted config, `Timeline.SetVoiceProfiles` |

`CodexDrawer` hardcodes `DEFAULT_VOICE_PROFILES`, so it ignores whatever the campaign actually configured. A Kokoro campaign cannot apply a Kokoro voice from the codex, and the codex list disagrees with Settings.

### 4.2 Decision

The configured list wins. The codex consumes `config.media.tts.voice_profiles` (already loaded in `App` as `config`):

- `App` passes `config?.media.tts.voice_profiles ?? []` to `CodexDrawer` as a `voiceProfiles` prop.
- `CodexDrawer` drops the `DEFAULT_VOICE_PROFILES` import and renders `voiceProfiles`.
- When the configured list is empty, the picker is disabled with a hint pointing at Settings rather than falling back to a private list.
- The applied snippet writes the profile's `voice_id`, `pitch`, `speech_rate`, and `provider` when set, so a profile is reproducible from the note alone.
- The frontend template constants (`DEFAULT_VOICE_PROFILES`, `KOKORO_VOICE_PROFILES`) remain, but only as Settings *seed* data; they are no longer a rendering source for the codex.

This also makes issue 1 visible and testable end-to-end: with the trimmed 11-profile Kokoro list loaded in Settings, the codex picker shows exactly those 11 voices.

---

## 5. Testing Strategy

**Go**
- `pkg/media`: SID table test for all 11 v0.19 speakers, unknown-voice fallback, and a cross-check that every `config.KokoroVoiceProfiles[].VoiceID` resolves within the pinned variant.
- `pkg/config`: preset count is 11, `sherpa-onnx.DefaultVoice` resolves, and every profile's `VoiceID` is present in the v0.19 speaker table.
- `pkg/gui`: `SaveEntity` rejects unparseable markdown with 400 and leaves the file byte-identical; `SaveEntity` forces the frontmatter `id` to the file name; `ListEntities` still lists a malformed note; merge refuses `confirm: false` and a missing target without deleting anything.

**Frontend**
- `tsc --noEmit` clean; `CodexDrawer` renders the passed `voiceProfiles` and the picker is disabled when empty.
- Manual: applying a Kokoro voice from the codex writes the expected frontmatter; merge requires the modal confirm; editing and saving a note leaves every other note intact.

---

## 6. File Map

| Action | Path | Description |
| :--- | :--- | :--- |
| Create | `pkg/media/kokoro_voices.go` | Variant-keyed Kokoro speaker table and resolver |
| Modify | `pkg/media/sherpa_tts.go` | Use model-scoped resolver; carry `modelID` |
| Modify | `pkg/media/sherpa_tts_test.go` | Correct SID expectations; profile/model cross-check |
| Modify | `pkg/media/providers.go` | Pass the pinned model variant into `NewSherpaTTSClient` |
| Modify | `pkg/models/manager.go` | Record the speaker-map variant on the `kokoro-tts` spec |
| Modify | `pkg/config/presets.go` | Trim `KokoroVoiceProfiles` to 11 aligned voices |
| Modify | `pkg/config/types.go` | Update default voice-profile seed if it references a removed voice |
| Modify | `pkg/config/presets_test.go` | Expect 11 profiles; assert model alignment |
| Modify | `pkg/gui/service.go` | Save validation + id forcing; listing malformed notes; merge invariants |
| Modify | `pkg/gui/server.go` | Surface save 400s; require `confirm` on merge |
| Modify | `pkg/gui/service_test.go` | Save/merge regression tests |
| Modify | `frontend/src/templates/providerPresets.ts` | Trim `KOKORO_VOICE_PROFILES` to 11 |
| Modify | `frontend/src/components/SettingsStudio.tsx` | "11 Profiles" copy |
| Modify | `frontend/src/components/CodexDrawer.tsx` | Configured profiles prop; merge modal; save error surfacing |
| Modify | `frontend/src/components/Drawers.tsx` | (Only if the merge modal needs a nested-dialog slot) |
| Modify | `frontend/src/App.tsx` | Pass configured profiles; id-guarded save; collision-safe quick-create |
| Modify | `frontend/src/types.ts` / `api/client.ts` | Parse-error flag on entity DTOs; merge `confirm` body |

---

## 7. Acceptance Criteria

1. Every one of the 11 pinned-model speakers is reachable by its correct SID, verified by test.
2. The Settings Kokoro seed contains exactly the voices the model can synthesise, and the codex picker shows the configured list.
3. Saving a note cannot alter, hide, or remove any other note.
4. Merging a note requires an explicit target and an explicit confirm, and cannot delete anything if the survivor write fails.
5. `go vet ./...`, `go test -count=1 ./...`, and `npx tsc --noEmit` all pass.
