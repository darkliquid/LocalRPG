# GM-Invented Character Voices Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ensure a character the GM invents is written to the codex with a voice chosen from its description and attributes, and that the character's own lines are read in that voice in the same turn.

**Architecture:** Introduce one character-type predicate, use it wherever voice assignment is gated, widen the profile-matching corpus to include appearance and aliases, voice matched-but-unvoiced characters on their next appearance, and route all speaker-to-voice lookups through the existing `harness.ResolveSpeakerID` so a display-name-only segment still resolves.

**Tech Stack:** Go 1.27.1, no new dependencies.

**Spec:** `docs/superpowers/specs/2026-09-22-gm-invented-character-voices-design.md`

---

## File Structure Map

| File Path | Responsibility |
| :--- | :--- |
| `pkg/entity/entity.go` | `IsCharacterType` predicate and its test |
| `pkg/engine/timeline.go` | Voice assignment gate; voice matched-but-unvoiced characters |
| `pkg/harness/extractor.go` | Predicate in `AssignVoiceProfile`; widened corpus; `ResolveSpeakerVoice` |
| `pkg/gui/service.go` | `voiceFor` uses `ResolveSpeakerVoice` |
| `pkg/export/script.go` | `speechResolver.voiceFor` uses `ResolveSpeakerVoice` |
| `pkg/engine/orchestrator.go` | Document and test the persistence-before-synthesis ordering |

---

### Task 1: Add the character-type predicate

**Files:**
- Modify: `pkg/entity/entity.go`
- Modify: `pkg/entity/entity_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestIsCharacterType(t *testing.T) {
	for _, want := range []string{"character", "Character", "npc", "NPC", "person", " creature "} {
		if !IsCharacterType(want) {
			t.Errorf("IsCharacterType(%q) = false, want true", want)
		}
	}
	for _, got := range []string{"location", "item", "faction", "arc", ""} {
		if IsCharacterType(got) {
			t.Errorf("IsCharacterType(%q) = true, want false", got)
		}
	}
}
```

- [ ] **Step 2: Run and confirm failure**

Run: `go test -run TestIsCharacterType ./pkg/entity/`
Expected: FAIL, undefined `IsCharacterType`.

- [ ] **Step 3: Implement**

```go
// IsCharacterType reports whether a type names a speaking being. Content authors,
// extractor models, and the graph all use different spellings for the same idea.
func IsCharacterType(t string) bool {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "character", "npc", "person", "creature":
		return true
	}
	return false
}
```

- [ ] **Step 4: Run the test**

Run: `go test -run TestIsCharacterType ./pkg/entity/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/entity/entity.go pkg/entity/entity_test.go
git commit -m "feat(entity): recognise npc and other character type spellings"
```

---

### Task 2: Widen the voice-matching corpus

**Files:**
- Modify: `pkg/harness/extractor.go`
- Modify: `pkg/harness/extractor_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestAssignVoiceProfileUsesAppearance(t *testing.T) {
	profiles := []config.VoiceProfile{
		{ID: "gruff", Name: "Gruff", VoiceID: "am_adam", Tags: []string{"deep", "gravelly"}},
		{ID: "soft", Name: "Soft", VoiceID: "bf_emma", Tags: []string{"gentle", "soft"}},
	}
	ent := &entity.Entity{ID: "sera", Name: "Sera", Type: "npc", Appearance: "A deep, gravelly voice; broad shouldered."}
	AssignVoiceProfile(ent, profiles)
	if ent.Voice == nil || ent.Voice.VoiceID != "am_adam" {
		t.Fatalf("expected appearance to select am_adam, got %+v", ent.Voice)
	}
}

func TestAssignVoiceProfileAcceptsNPCStyleType(t *testing.T) {
	profiles := []config.VoiceProfile{{ID: "p", Name: "P", VoiceID: "af_bella", Tags: []string{"female"}}}
	ent := &entity.Entity{ID: "x", Name: "X", Type: "npc", Body: "A female guard."}
	AssignVoiceProfile(ent, profiles)
	if ent.Voice == nil {
		t.Fatal("expected an npc-typed character to receive a voice")
	}
}
```

- [ ] **Step 2: Run and confirm failure**

Run: `go test -run TestAssignVoiceProfile ./pkg/harness/`
Expected: FAIL.

- [ ] **Step 3: Implement**

In `AssignVoiceProfile`:
- replace `ent.Type != "character"` with `!entity.IsCharacterType(ent.Type)`;
- build the corpus from name, body, appearance, and aliases:

```go
corpus := strings.ToLower(strings.Join([]string{
    ent.Name,
    ent.Body,
    ent.Appearance,
    strings.Join(ent.Aliases, " "),
}, " "))
```

- [ ] **Step 4: Run the harness tests**

Run: `go test -count=1 ./pkg/harness/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/harness/extractor.go pkg/harness/extractor_test.go
git commit -m "fix(harness): choose a voice from a character's full description"
```

---

### Task 3: Voice every character-like entity the turn produces

**Files:**
- Modify: `pkg/engine/timeline.go`
- Modify: `pkg/engine/timeline_test.go`

- [ ] **Step 1: Write the failing test**

Add a test that feeds `RecordTurn` an extraction containing an `npc` entity and asserts the written note parses back with a non-nil `Voice`.

```go
func TestRecordTurnVoicesInventedNPC(t *testing.T) {
	// build a timeline with profiles via SetVoiceProfiles
	// extraction entity: {Name: "Old Sera", Type: "npc", Body: "A gravelly female veteran."}
	// after RecordTurn, read the note from disk and assert Voice != nil
}
```

- [ ] **Step 2: Run and confirm failure**

Run: `go test -run TestRecordTurnVoicesInventedNPC ./pkg/engine/`
Expected: FAIL (no voice assigned for `npc`).

- [ ] **Step 3: Implement**

In `recordEntities`, replace the gate:

```go
if entity.IsCharacterType(ent.Type) {
    harness.AssignVoiceProfile(ent, t.voiceProfiles)
}
```

Then extend the reconciliation so a **matched** character-like entity that has no voice receives one. The loop already has `ent` in hand; after `MergeExtractedEntity`, if `entity.IsCharacterType(ent.Type) && ent.Voice == nil`, call `AssignVoiceProfile` and add `ent` to `pending` so the voice is written back. Keep `AssignVoiceProfile`'s existing no-overwrite rule intact.

- [ ] **Step 4: Run the engine tests**

Run: `go test -count=1 ./pkg/engine/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/timeline.go pkg/engine/timeline_test.go
git commit -m "fix(engine): give invented and previously unvoiced characters a voice"
```

---

### Task 4: Resolve a speaker reference to its voice everywhere

**Files:**
- Modify: `pkg/harness/extractor.go`
- Modify: `pkg/gui/service.go`
- Modify: `pkg/export/script.go`
- Modify: `pkg/harness/extractor_test.go`

- [ ] **Step 1: Add the failing test**

```go
func TestResolveSpeakerVoiceByDisplayName(t *testing.T) {
	// store with an entity id "lady-evelyn", name "Lady Evelyn", voice af_bella
	got := ResolveSpeakerVoice(store, "Lady Evelyn")
	if got == nil || got.VoiceID != "af_bella" {
		t.Fatalf("expected Lady Evelyn's voice, got %+v", got)
	}
}
```

- [ ] **Step 2: Run and confirm failure**

Run: `go test -run TestResolveSpeakerVoice ./pkg/harness/`
Expected: FAIL.

- [ ] **Step 3: Implement `ResolveSpeakerVoice`**

```go
// ResolveSpeakerVoice finds the configured voice for a speaker reference, which
// may be an entity ID or a written display name. Returns nil when no entity or no
// voice matches, so callers fall back to the narrator voice.
func ResolveSpeakerVoice(store *storage.Store, speakerRef string) *entity.VoiceConfig {
	if store == nil || strings.TrimSpace(speakerRef) == "" {
		return nil
	}
	if ent, err := store.GetEntity(speakerRef); err == nil && ent != nil {
		return ent.Voice
	}
	if id := ResolveSpeakerID(store, speakerRef); id != "" {
		if ent, err := store.GetEntity(id); err == nil && ent != nil {
			return ent.Voice
		}
	}
	return nil
}
```

- [ ] **Step 4: Use it in the GUI**

Replace the body of `Service.voiceFor`'s closure with `return harness.ResolveSpeakerVoice(store, speakerID)`. Confirm `harness` is already imported (`pkg/gui/service.go` imports it).

- [ ] **Step 5: Use it in the export resolver**

Replace `pkg/export/script.go`'s `speechResolver.voiceFor` body with the same call, importing `harness` if not already present.

- [ ] **Step 6: Run tests**

Run: `go test -count=1 ./pkg/harness/ ./pkg/gui/ ./pkg/export/`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add pkg/harness/extractor.go pkg/gui/service.go pkg/export/script.go pkg/harness/extractor_test.go
git commit -m "fix(harness): resolve a speaker's voice from their display name too"
```

---

### Task 5: Prove the persistence-before-synthesis ordering

**Files:**
- Modify: `pkg/engine/orchestrator.go`
- Modify: `pkg/gui/turn_test.go`

- [ ] **Step 1: Write the failing test**

Add a test that runs a turn whose extraction creates a character and asserts the character's note exists on disk (and is indexed) by the time `ProcessAction` returns. Then add a GUI-level test that resolves the character's voice from the store immediately after the turn event and asserts it is non-nil.

- [ ] **Step 2: Run and confirm the test passes**

If the ordering is already correct the test passes immediately; keep it as a regression guard. If it fails, fix `Run` so playback cannot start before `RecordTurn` has completed.

- [ ] **Step 3: Document the invariant**

Add a comment at the `RecordTurn` call in `ProcessActionStream` and at the playback launch in `Service.Run` stating that entities must be persisted and voiced before any segment is synthesised.

- [ ] **Step 4: Commit**

```bash
git add pkg/engine/orchestrator.go pkg/gui/turn_test.go
git commit -m "test(engine): guard the persist-before-synthesis ordering for new characters"
```

---

### Task 6: Verification

- [ ] **Step 1: Backend gate**

Run: `mise run test:backend` and `mise run lint`
Expected: all tests pass, `go vet` clean.

- [ ] **Step 2: Manual smoke**

Run a campaign turn in which the GM invents a character typed `npc`, confirm the note appears in the codex with a `voice` block, and confirm the character's line plays in that voice rather than the narrator's.
