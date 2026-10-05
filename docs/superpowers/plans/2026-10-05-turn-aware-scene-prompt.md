# Turn-Aware Scene Prompt Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Compose a scene image prompt from the turn's narration, action, cast, place, and outcome tone.

**Architecture:** A `ScenePromptContext` replaces the three-argument `BuildScenePrompt` (kept as a wrapper); the enqueue site fills it from the recorded turn; each part is bounded.

**Tech Stack:** Go standard library.

**Spec:** `docs/superpowers/specs/2026-10-05-turn-aware-scene-prompt-design.md`

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `interface{}`, not `any`, in Go.
- Caps: narration excerpt 200 chars, action 160, entities 4.
- The old three-argument call must produce the previous prompt for the same inputs.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The context and the builder

**Files:**
- Modify: `pkg/engine/scene_worker.go:17-35`
- Test: `pkg/engine/scene_prompt_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `ScenePromptContext`, `BuildScenePrompt(ctx ScenePromptContext) string`, and the old wrapper `BuildScenePromptFor(cue string, loc *entity.Entity, style string) string`.

- [ ] **Step 1: Write the failing test**

```go
func TestBuildScenePromptIncludesParts(t *testing.T) {
	got := BuildScenePrompt(ScenePromptContext{
		Action: "pick the lock", Entities: []string{"Kaelen", "Garrick"},
		Location: "The Drowned Hall", Style: "grim fantasy", Outcome: "miss",
	})
	for _, want := range []string{"pick the lock", "Kaelen", "The Drowned Hall", "grim fantasy", "ominous"} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt missing %q: %s", want, got)
		}
	}
}

func TestBuildScenePromptCaps(t *testing.T) {
	got := BuildScenePrompt(ScenePromptContext{Narration: strings.Repeat("x", 500)})
	if len(got) > 400 {
		t.Fatalf("prompt too long: %d", len(got))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/engine/ -run TestBuildScenePrompt -v`
Expected: FAIL, `undefined: ScenePromptContext`.

- [ ] **Step 3: Write minimal implementation**

Add `ScenePromptContext` and `BuildScenePrompt` composing the six parts from the spec §4.2 with the
caps. Rename the existing `BuildScenePrompt` to `BuildScenePromptFor` and keep its behaviour.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/engine/ -run TestBuildScenePrompt -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/scene_worker.go pkg/engine/scene_prompt_test.go
git commit -m "feat(engine): compose a scene prompt from a turn context"
```

---

### Task 2: The outcome tone

**Files:**
- Modify: `pkg/engine/scene_worker.go`
- Test: `pkg/engine/scene_prompt_test.go` (append)

**Interfaces:**
- Consumes: the outcome label.
- Produces: `func outcomeToneWords(outcome string) string`.

- [ ] **Step 1: Write the failing test**

```go
func TestOutcomeToneWords(t *testing.T) {
	if outcomeToneWords("strong") == "" || outcomeToneWords("miss") == "" {
		t.Fatal("known outcomes should map to tone words")
	}
	if outcomeToneWords("") != "" || outcomeToneWords("nonsense") != "" {
		t.Fatal("an unknown outcome should add nothing")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/engine/ -run TestOutcomeToneWords -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Map the common outcome families (strong/success/pass, weak/partial, miss/fail) to short tone phrases,
and return "" for anything else, so a system with a custom vocabulary does not get a wrong mood.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/engine/ -run TestOutcomeToneWords -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/scene_worker.go pkg/engine/scene_prompt_test.go
git commit -m "feat(engine): map a check outcome to prompt tone words"
```

---

### Task 3: The enqueue builds the context

**Files:**
- Modify: `pkg/engine/orchestrator.go:1462-1473`
- Test: `pkg/engine/scene_prompt_test.go` (append)

**Interfaces:**
- Consumes: `turn.Narration`, the mode input, `turn.Checks`, the location entity, the world style, the turn's mentions.
- Produces: the enqueue passes a filled `ScenePromptContext`.

- [ ] **Step 1: Write the failing test**

```go
func TestEnqueueUsesTurnContext(t *testing.T) {
	// Capture the prompt the worker receives and assert it contains the action
	// and the outcome tone.
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/engine/ -run TestEnqueueUsesTurnContext -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

At the enqueue site, build the context: cue from `ExtractSceneCue(turn.Narration)` (or the
extractor's `VisualCue`), narration from `turn.Narration`, action from the mode input, entities from
the turn's mentions/speakers, location name and appearance from the location entity, style from
`o.worldArtStyle`, and outcome from the first check in `turn.Checks`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/engine/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/orchestrator.go pkg/engine/scene_prompt_test.go
git commit -m "feat(engine): feed the turn context to the scene worker"
```

---

### Task 4: Verification

- [ ] **Step 1: Regression guard**

Add a test asserting `BuildScenePromptFor` produces the previous prompt for the same inputs.

- [ ] **Step 2: Determinism test**

Add a test that the same context produces the same prompt twice.

- [ ] **Step 3: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 4: Confirm the acceptance criteria**

- The prompt includes narration, action, cast, place, style, and outcome tone when present.
- Each part is capped.
- The old builder is unchanged.
- The same turn produces the same prompt.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "test: guard the scene prompt against the previous behaviour"
```
