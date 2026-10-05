# Opening Scene Restatement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make a campaign's first turn restate the opening scene before continuing, so the hooks it writes are readable without the player scrolling back to the Prologue.

**Architecture:** The opening scene stops travelling as the turn's *action* (which is why the GM responds to it instead of restating it) and becomes its own non-droppable prompt section, `## OPENING SCENE`, modelled on the existing `actionEchoSection`. An `Opening` turn always establishes the scene; a quiet variant (`scene_only`) restates it and adds no hooks, so the player's own first action can follow as turn 2.

**Tech Stack:** Go 1.27 (stdlib `testing`, no testify), React 19 + TypeScript, `modernc.org/sqlite`.

**Spec:** `docs/superpowers/specs/2026-10-05-opening-scene-restatement-design.md`

## Global Constraints

- Go tests use the standard library only (`testing`, `t.TempDir()`); never add testify.
- Errors are wrapped with `fmt.Errorf("...: %w", err)`.
- Use `interface{}`, never `any`; `go vet ./...` must stay clean.
- Conventional Commits with a scope; subject under 72 characters.
- No comments that restate what the code does; comment only the *why*.
- The opening scene section must never be droppable: a turn whose purpose is to establish the scene must not lose it to the token budget.
- The recorded turn mode for both scene flavours is `engine.OpeningMode` (`"Opening"`); the chronicle renders `[turn.mode]`, so no new label may appear.
- The opening scene is `settings.opening_prompt` (`engine.OpeningPromptSetting`). No new storage is introduced.

---

### Task 1: The opening scene prompt section

**Files:**
- Modify: `pkg/harness/context.go` (`ContextRequest` at line 78, `actionEchoSection` at line 218, `buildSections` at line 247)
- Test: `pkg/harness/context_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `ContextRequest.OpeningScene string`, `ContextRequest.OpeningHooks bool`, and `openingSceneSection(scene string, hooks bool) string`.

- [ ] **Step 1: Write the failing tests**

Append to `pkg/harness/context_test.go`:

```go
func TestOpeningSceneSectionRestatesTheScene(t *testing.T) {
	assembler := NewContextAssembler(newTestEntityStore(t))

	const scene = "Fire rains down as the wreckage of the Dawnbreaker tumbles past the palace window."

	result, err := assembler.Assemble(ContextRequest{
		OpeningScene: scene,
		OpeningHooks: true,
		TurnNumber:   1,
		Mode:         "Opening",
	})
	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}
	if !strings.Contains(result.Prompt, "## OPENING SCENE") {
		t.Fatalf("expected an opening scene section:\n%s", result.Prompt)
	}
	if !strings.Contains(result.Prompt, scene) {
		t.Fatalf("expected the scene text in the prompt:\n%s", result.Prompt)
	}
	if !strings.Contains(result.Prompt, "restating the scene below") {
		t.Fatalf("expected the restatement instruction:\n%s", result.Prompt)
	}
	if !strings.Contains(result.Prompt, "invites the protagonist") {
		t.Fatalf("expected the hooks instruction:\n%s", result.Prompt)
	}
}

func TestOpeningSceneHooksAreOptional(t *testing.T) {
	assembler := NewContextAssembler(newTestEntityStore(t))

	const scene = "Fire rains down over the market."
	quiet, err := assembler.Assemble(ContextRequest{OpeningScene: scene, OpeningHooks: false})
	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}
	if !strings.Contains(quiet.Prompt, scene) {
		t.Fatalf("expected the scene in the quiet turn:\n%s", quiet.Prompt)
	}
	if strings.Contains(quiet.Prompt, "invites the protagonist") {
		t.Fatalf("a quiet scene turn must add no hooks:\n%s", quiet.Prompt)
	}

	ordinary, err := assembler.Assemble(ContextRequest{Action: "I look around"})
	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}
	if strings.Contains(ordinary.Prompt, "## OPENING SCENE") {
		t.Fatalf("expected no opening scene section for an ordinary turn:\n%s", ordinary.Prompt)
	}
}

func TestOpeningSceneSectionIsNotDroppable(t *testing.T) {
	assembler := NewContextAssembler(newTestEntityStore(t))
	assembler.SetLimits(ContextLimits{TokenBudget: 1})

	const scene = "Fire rains down over the market."
	result, err := assembler.Assemble(ContextRequest{OpeningScene: scene, OpeningHooks: true})
	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}
	if !strings.Contains(result.Prompt, scene) {
		t.Fatalf("the opening scene must survive trimming:\n%s", result.Prompt)
	}
}
```

Assertions use short phrases that cannot be split by a line wrap in the instruction strings ("restating the scene below", "invites the protagonist"). Keep that in mind when editing the copy.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test -run 'TestOpeningScene' ./pkg/harness/`
Expected: FAIL to compile, `OpeningScene` is not a field of `ContextRequest`.

- [ ] **Step 3: Add the request fields**

In `pkg/harness/context.go`, immediately after the `ActionEcho bool` field of `ContextRequest`:

```go
	// OpeningScene is the campaign's opening prose, carried on the first turn so
	// the narrator can restate it before continuing.
	OpeningScene string
	// OpeningHooks asks the first turn to extend the scene with events that invite
	// the protagonist to act. False for the quiet scene turn that precedes the
	// player's own first action.
	OpeningHooks bool
```

- [ ] **Step 4: Add the instructions and the section renderer**

In `pkg/harness/context.go`, immediately after `actionEchoSection` (which ends at line 223):

```go
// openingRestateInstruction tells the narrator to restate the campaign's opening
// scene before continuing, so the first turn never starts mid-consequence.
const openingRestateInstruction = `## OPENING SCENE
This is the campaign's first turn. Open the narration by restating the scene below
in your own words, keeping its facts, imagery, and mood, so the player can tell
where they are and what is already happening from this turn alone. Do not skip
straight to the consequences.

Do not decide the protagonist's actions, thoughts, or feelings.
`

// openingEstablishInstruction is the same framing for a campaign with no opening
// prompt: the scene comes from the world, the rules, and the lore.
const openingEstablishInstruction = `## OPENING SCENE
This is the campaign's first turn. Establish the opening of this campaign: describe
where the protagonist is and what they can perceive.

Do not decide the protagonist's actions, thoughts, or feelings.
`

// openingHooksInstruction asks for the developments that draw the player in.
const openingHooksInstruction = `
Once the scene is established, continue with one thing that invites the protagonist
to act, and at most one present character, named as they are already known. Stop
there; do not resolve the protagonist's response.
`

// openingSceneSection renders the first turn's scene framing, or nothing when the
// turn is not an opening one. The scene is restated when the campaign has an
// opening prompt, and established from the world when it does not.
func openingSceneSection(scene string, hooks bool) string {
	trimmed := strings.TrimSpace(scene)
	if trimmed == "" && !hooks {
		return ""
	}

	var sb strings.Builder
	if trimmed != "" {
		sb.WriteString(openingRestateInstruction)
		sb.WriteString("\nOpening scene:\n")
		sb.WriteString(trimmed)
		sb.WriteString("\n")
	} else {
		sb.WriteString(openingEstablishInstruction)
	}
	if hooks {
		sb.WriteString(openingHooksInstruction)
	}
	return sb.String()
}
```

- [ ] **Step 5: Emit the section**

In `buildSections`, in the returned slice, add a section immediately after the `action_echo` entry. It carries no `droppable` flag, which is what keeps it through trimming:

```go
		{name: "opening_scene", source: "opening_scene", text: openingSceneSection(req.OpeningScene, req.OpeningHooks)},
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test -run 'TestOpeningScene' ./pkg/harness/`
Expected: PASS

- [ ] **Step 7: Run the whole package to check for regressions**

Run: `go test ./pkg/harness/`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add pkg/harness/context.go pkg/harness/context_test.go
git commit -m "feat(harness): give the opening scene its own prompt section"
```

---

### Task 2: An action-less opening prompt

**Files:**
- Modify: `pkg/harness/context.go` (`turnFramingInstruction` at line 227, `buildSections` at line 272)
- Test: `pkg/harness/context_test.go` (update the message in `TestAssembleReportsEverySectionAndKeepsTheActionLast` at line 307)

**Interfaces:**
- Consumes: `ContextRequest.OpeningScene`, `ContextRequest.OpeningHooks` from Task 1.
- Produces: an `## OPENING SCENE` turn whose prompt contains no `## PLAYER ACTION` section, while still teaching the `> Speaker:` reply format.

- [ ] **Step 1: Write the failing test**

Append to `pkg/harness/context_test.go`:

```go
func TestPromptOmitsThePlayerActionWhenThereIsNone(t *testing.T) {
	assembler := NewContextAssembler(newTestEntityStore(t))

	result, err := assembler.Assemble(ContextRequest{
		OpeningScene: "Fire rains down over the market.",
		OpeningHooks: true,
	})
	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}
	if strings.Contains(result.Prompt, "## PLAYER ACTION") {
		t.Fatalf("an opening turn has no action to present:\n%s", result.Prompt)
	}
	if !strings.Contains(result.Prompt, "MUST start with '> Speaker:") {
		t.Fatalf("the reply format must still be taught:\n%s", result.Prompt)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestPromptOmitsThePlayerActionWhenThereIsNone ./pkg/harness/`
Expected: FAIL, the prompt contains `## PLAYER ACTION`.

- [ ] **Step 3: Hoist the format reminder into the framing section**

In `pkg/harness/context.go`, change `turnFramingInstruction` so the reminder lives here rather than in the action block:

```go
const turnFramingInstruction = `## TURN FORMAT
Format reminder: Every spoken line or dialogue beat MUST start with '> Speaker: "utterance"'. Plain prose without '>' is for narration only. Use '@persona {"name":"...", "reveals":"Old Name"}' when an unknown identity is revealed.

Write the turn as prose. Mark each spoken line as a blockquote whose speaker is
named before a colon, on its own line:
```

Leave the rest of the constant unchanged.

- [ ] **Step 4: Gate the action section**

In `buildSections`, replace the block that builds `actionText` (currently lines 272-282) with:

```go
	var actionRefs []Ref
	if req.PlayerID != "" {
		actionRefs = append(actionRefs, Ref{Kind: RefEntity, ID: req.PlayerID, Relation: "action"})
	}

	// An opening turn has no action to answer, so the section is absent rather than
	// empty: presenting it made the GM respond to the scene it was given.
	actionText := ""
	if strings.TrimSpace(req.Action) != "" || strings.TrimSpace(req.PlayerName) != "" {
		actionText = "\n## PLAYER ACTION\n"
		if name := strings.TrimSpace(req.PlayerName); name != "" {
			actionText += name + ": "
		}
		actionText += req.Action + "\n"
	}
```

The `> Format reminder:` lines that were concatenated here are now gone, because Step 3 moved them.

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test -run TestPromptOmitsThePlayerActionWhenThereIsNone ./pkg/harness/`
Expected: PASS

- [ ] **Step 6: Update the stale assertion message**

In `TestAssembleReportsEverySectionAndKeepsTheActionLast`, the reminder now lives in the framing section. Change its message:

```go
	if !strings.Contains(result.Prompt, "Format reminder: Every spoken line or dialogue beat MUST start with '> Speaker:") {
		t.Errorf("expected the format reminder in the framing section")
	}
```

- [ ] **Step 7: Run the whole package**

Run: `go test ./pkg/harness/`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add pkg/harness/context.go pkg/harness/context_test.go
git commit -m "fix(harness): stop presenting a scene as a player action"
```

---

### Task 3: The first turn carries the scene

**Files:**
- Modify: `pkg/engine/orchestrator.go` (`openingDirective` at line 60, `SetOpeningPrompt` at line 414, `ProcessActionStream` at line 555, the `Assemble` call at line 802)
- Modify: `pkg/engine/orchestrator_stream_test.go` (`scriptedStreamProvider.Stream` at line 48)
- Test: `pkg/engine/orchestrator_stream_test.go`

**Interfaces:**
- Consumes: `ContextRequest.OpeningScene`, `ContextRequest.OpeningHooks` from Task 1.
- Produces: `func (o *TurnOrchestrator) SetSceneOnly(sceneOnly bool)`.

- [ ] **Step 1: Make the test provider record prompts on the streaming path**

`scriptedStreamProvider.onRequest` is documented as "called with each request the provider is given", but only `Generate` calls it. Add the call to `Stream`, immediately after `defer close(out)`:

```go
func (p *scriptedStreamProvider) Stream(ctx context.Context, req harness.GenerateRequest, out chan<- harness.StreamChunk) error {
	defer close(out)

	if p.onRequest != nil {
		p.onRequest(req)
	}
```

- [ ] **Step 2: Write the failing tests**

Append to `pkg/engine/orchestrator_stream_test.go`:

```go
func TestOpeningTurnRestatesTheScene(t *testing.T) {
	const scene = "Fire rains down as the wreckage of the Dawnbreaker tumbles past the palace window."

	var prompt string
	provider := &scriptedStreamProvider{
		chunks:    []string{"Smoke blots the dawn."},
		onRequest: func(req harness.GenerateRequest) { prompt = req.Prompt },
	}
	orchestrator, timeline, _ := streamingOrchestrator(t, provider)
	orchestrator.SetOpeningPrompt(scene)

	turn, err := orchestrator.ProcessActionStream(context.Background(), "Opening", "", nil)
	if err != nil {
		t.Fatalf("opening turn failed: %v", err)
	}
	if turn.Number != 1 || turn.Mode != OpeningMode {
		t.Fatalf("Number/Mode = %d/%q, want 1/%q", turn.Number, turn.Mode, OpeningMode)
	}
	if !strings.Contains(prompt, "## OPENING SCENE") {
		t.Errorf("expected an opening scene section:\n%s", prompt)
	}
	if !strings.Contains(prompt, scene) {
		t.Errorf("expected the scene in the prompt:\n%s", prompt)
	}
	if strings.Contains(prompt, "## PLAYER ACTION") {
		t.Errorf("the opening turn must not present the scene as an action:\n%s", prompt)
	}
	if !strings.Contains(prompt, "invites the protagonist") {
		t.Errorf("the opening turn should invite action:\n%s", prompt)
	}

	turns, err := timeline.history.LoadHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 {
		t.Fatalf("expected one recorded turn, got %d", len(turns))
	}
}

func TestQuietSceneTurnAddsNoHooks(t *testing.T) {
	const scene = "Fire rains down over the market."

	var prompt string
	provider := &scriptedStreamProvider{
		chunks:    []string{"The market burns."},
		onRequest: func(req harness.GenerateRequest) { prompt = req.Prompt },
	}
	orchestrator, _, _ := streamingOrchestrator(t, provider)
	orchestrator.SetOpeningPrompt(scene)
	orchestrator.SetSceneOnly(true)

	if _, err := orchestrator.ProcessActionStream(context.Background(), "Opening", "", nil); err != nil {
		t.Fatalf("quiet scene turn failed: %v", err)
	}
	if !strings.Contains(prompt, scene) {
		t.Errorf("expected the scene restated:\n%s", prompt)
	}
	if strings.Contains(prompt, "invites the protagonist") {
		t.Errorf("a quiet scene turn must add no hooks:\n%s", prompt)
	}
}

func TestOpeningSceneIsNotCarriedAfterTurnOne(t *testing.T) {
	const scene = "Fire rains down over the market."

	var prompts []string
	provider := &scriptedStreamProvider{
		chunks:    []string{"The market burns."},
		onRequest: func(req harness.GenerateRequest) { prompts = append(prompts, req.Prompt) },
	}
	orchestrator, _, _ := streamingOrchestrator(t, provider)
	orchestrator.SetOpeningPrompt(scene)

	if _, err := orchestrator.ProcessActionStream(context.Background(), "Opening", "", nil); err != nil {
		t.Fatalf("opening turn failed: %v", err)
	}
	if _, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I run", nil); err != nil {
		t.Fatalf("second turn failed: %v", err)
	}
	if len(prompts) != 2 {
		t.Fatalf("expected two prompts, got %d", len(prompts))
	}
	if strings.Contains(prompts[1], "## OPENING SCENE") {
		t.Errorf("only the first turn carries the scene:\n%s", prompts[1])
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test -run 'TestOpeningTurnRestatesTheScene|TestQuietSceneTurnAddsNoHooks|TestOpeningSceneIsNotCarriedAfterTurnOne' ./pkg/engine/`
Expected: FAIL to compile, `SetSceneOnly` is undefined.

- [ ] **Step 4: Add the flag and its setter**

In `pkg/engine/orchestrator.go`, add a field beside `openingPrompt` in the `TurnOrchestrator` struct:

```go
	// sceneOnly marks the next turn as a quiet scene turn: it restates the
	// campaign's opening scene and adds no hooks. Consumed once per turn.
	sceneOnly bool
```

Add the setter immediately after `SetOpeningPrompt`:

```go
// SetSceneOnly marks the next turn as a quiet scene turn. It is meaningful only
// for an Opening turn, and the engine consumes it once.
func (o *TurnOrchestrator) SetSceneOnly(sceneOnly bool) {
	o.sceneOnly = sceneOnly
}
```

- [ ] **Step 5: Consume the flag and stop passing the scene as the action**

In `ProcessActionStream`, immediately after `turnNum := len(pastTurns) + 1` (line 560):

```go
	sceneOnly := o.sceneOnly
	o.sceneOnly = false
```

Replace the opening branch (lines 697-704) with:

```go
	isOpening := strings.EqualFold(mode, OpeningMode)
	if isOpening {
		if len(pastTurns) > 0 {
			return nil, fmt.Errorf("the campaign has already begun")
		}
		mode = OpeningMode
		// The scene is the turn's frame, not the player's action: handing it over
		// as the action made the GM respond to it instead of restating it.
		generationPrompt = ""
	}
```

- [ ] **Step 6: Carry the scene on the turn**

Immediately before the `assembly, err := o.assembler.Assemble(...)` call (line 802), add:

```go
	// The opening turn restates the campaign's scene. A quiet scene turn suppresses
	// the hooks only when there is a scene to restate; without one the turn still
	// establishes a scene from the world.
	openingScene := ""
	openingHooks := false
	if isOpening {
		openingScene = o.openingPrompt
		openingHooks = !sceneOnly || strings.TrimSpace(openingScene) == ""
	}
```

Add two fields to the `harness.ContextRequest` literal, beside `Action`:

```go
		OpeningScene:     openingScene,
		OpeningHooks:     openingHooks,
```

- [ ] **Step 7: Delete the old directive**

Delete `openingDirective` and its doc comment (lines 60-74). Nothing calls it after Step 5.

- [ ] **Step 8: Run the tests to verify they pass**

Run: `go test -run 'TestOpening|TestQuietSceneTurnAddsNoHooks' ./pkg/engine/`
Expected: PASS

- [ ] **Step 9: Run the whole package**

Run: `go test ./pkg/engine/`
Expected: PASS. `TestOpeningModeEstablishesTheFirstTurnOnly` asserts the turn's number and mode and that a second opening turn is refused; it does not read the prompt, so it passes unchanged.

- [ ] **Step 10: Commit**

```bash
git add pkg/engine/orchestrator.go pkg/engine/orchestrator_stream_test.go
git commit -m "feat(engine): restate the opening scene in the first turn"
```

---

### Task 4: The turn request carries the quiet flag

**Files:**
- Modify: `pkg/gui/types.go` (`TurnRequest` at line 573)
- Modify: `pkg/gui/service.go` (the `ProcessActionStream` call at line 2043)
- Test: `pkg/gui/turn_test.go`

**Interfaces:**
- Consumes: `func (o *TurnOrchestrator) SetSceneOnly(bool)` from Task 3.
- Produces: `TurnRequest.SceneOnly bool` with JSON key `scene_only`.

- [ ] **Step 1: Write the failing test**

Append to `pkg/gui/turn_test.go`:

```go
func TestQuietSceneTurnPrecedesTheFirstAction(t *testing.T) {
	gameID, svc := turnFixture(t)

	const scene = "Fire rains down as the wreckage of the Dawnbreaker tumbles past the palace window."
	if err := svc.UpdateGameSettings(context.Background(), gameID, map[string]interface{}{
		engine.OpeningPromptSetting: scene,
	}); err != nil {
		t.Fatalf("UpdateGameSettings failed: %v", err)
	}

	sceneSession, err := svc.BeginTurn(gameID)
	if err != nil {
		t.Fatalf("BeginTurn failed: %v", err)
	}
	var sceneTurn *TurnDTO
	if err := sceneSession.Run(context.Background(), TurnRequest{Mode: "Opening", SceneOnly: true}, func(event TurnEvent) error {
		if event.Type == "turn" {
			sceneTurn = event.Turn
		}
		return nil
	}); err != nil {
		t.Fatalf("quiet scene turn failed: %v", err)
	}
	sceneSession.Close()

	if sceneTurn == nil {
		t.Fatal("expected the scene turn to be recorded")
	}
	if sceneTurn.TurnNumber != 1 || sceneTurn.Mode != engine.OpeningMode {
		t.Errorf("scene turn = %d/%q, want 1/%q", sceneTurn.TurnNumber, sceneTurn.Mode, engine.OpeningMode)
	}
	// The fixture's gm is the builtin echo provider, so the prompt is in the prose.
	if !strings.Contains(sceneTurn.Prose, scene) {
		t.Errorf("expected the scene in the turn prose:\n%s", sceneTurn.Prose)
	}
	if strings.Contains(sceneTurn.Prose, "invites the protagonist") {
		t.Errorf("a quiet scene turn must add no hooks:\n%s", sceneTurn.Prose)
	}

	actionSession, err := svc.BeginTurn(gameID)
	if err != nil {
		t.Fatalf("second BeginTurn failed: %v", err)
	}
	defer actionSession.Close()

	var actionTurn *TurnDTO
	if err := actionSession.Run(context.Background(), TurnRequest{Mode: "Do", Input: "I run for the gate"}, func(event TurnEvent) error {
		if event.Type == "turn" {
			actionTurn = event.Turn
		}
		return nil
	}); err != nil {
		t.Fatalf("action turn failed: %v", err)
	}
	if actionTurn == nil || actionTurn.TurnNumber != 2 {
		t.Fatalf("expected the player's action to be turn 2, got %+v", actionTurn)
	}
}
```

Add `"strings"` to the test file's imports if it is not already there.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestQuietSceneTurnPrecedesTheFirstAction ./pkg/gui/`
Expected: FAIL to compile, `TurnRequest` has no field `SceneOnly`.

- [ ] **Step 3: Add the request field**

In `pkg/gui/types.go`, replace `TurnRequest`:

```go
type TurnRequest struct {
	Mode  string `json:"mode"`
	Input string `json:"input"`
	// SceneOnly asks an Opening turn to restate the campaign's scene and add no
	// hooks, so the player's own first action can follow it.
	SceneOnly bool `json:"scene_only,omitempty"`
}
```

- [ ] **Step 4: Pass it to the orchestrator**

In `pkg/gui/service.go`, immediately before the `ProcessActionStream` call in `TurnSession.Run`:

```go
	t.orchestrator.SetSceneOnly(req.SceneOnly)
	turn, err := t.orchestrator.ProcessActionStream(runCtx, req.Mode, req.Input, func(text string) error {
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test -run TestQuietSceneTurnPrecedesTheFirstAction ./pkg/gui/`
Expected: PASS

- [ ] **Step 6: Run the whole package**

Run: `go test ./pkg/gui/`
Expected: PASS. `TestTurnSessionRunsAnOpeningTurn` submits `{Mode: "Opening", Input: ""}` with no scene flag and no opening prompt; it still records turn 1 in `Opening` mode.

- [ ] **Step 7: Commit**

```bash
git add pkg/gui/types.go pkg/gui/service.go pkg/gui/turn_test.go
git commit -m "feat(gui): let a turn ask for the quiet scene restatement"
```

---

### Task 5: The Prologue submits the quiet scene turn

**Files:**
- Modify: `frontend/src/api/client.ts` (`streamTurn` at line 678)
- Modify: `frontend/src/App.tsx` (`handleActionSubmit` at line 389, `handleBeginWithAction` at line 543)
- Modify: `frontend/src/components/ProloguePanel.tsx`

**Interfaces:**
- Consumes: the `scene_only` JSON field from Task 4.
- Produces: nothing consumed by later tasks.

- [ ] **Step 1: Accept the field in the fetch layer**

In `frontend/src/api/client.ts`, change `streamTurn`'s body type:

```ts
    body: { mode: string; input: string; pending_check_ref?: string; scene_only?: boolean },
```

- [ ] **Step 2: Send it from the submit helper**

In `frontend/src/App.tsx`, change `handleActionSubmit`'s signature and body:

```ts
  const handleActionSubmit = async (mode: string, text: string, pendingCheckRef?: string, sceneOnly = false) => {
```

and the request it posts:

```ts
      await APIClient.streamTurn(
        activeGameID,
        { mode, input: text, pending_check_ref: pendingCheckRef, scene_only: sceneOnly || undefined },
```

- [ ] **Step 3: Establish the scene before the player's first step**

Replace `handleBeginWithAction` in `frontend/src/App.tsx`:

```ts
  // Taking the first step still establishes the scene first: a quiet scene turn
  // restates the opening prompt, then the console takes the player's own first
  // step as turn 2.
  const handleBeginWithAction = async () => {
    if (gameState?.opening_prompt && !turnInFlight) {
      await handleActionSubmit('Opening', '', undefined, true);
    }
    document.getElementById('action-console-input')?.focus();
  };
```

- [ ] **Step 4: Note the restatement in the Prologue copy**

In `frontend/src/components/ProloguePanel.tsx`, add a line under the existing explanation paragraph, inside the header block:

```tsx
          <p className="text-xs text-stone-500">
            The opening scene is restated at the start of the first turn, so you can
            always see where the story begins.
          </p>
```

- [ ] **Step 5: Type-check and build**

Run: `cd frontend && npm run build`
Expected: PASS. `tsc` has `noUnusedLocals` and `noUnusedParameters`, so an unused import or parameter fails the build.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/api/client.ts frontend/src/App.tsx frontend/src/components/ProloguePanel.tsx
git commit -m "feat(frontend): set the opening scene before the first step"
```

---

### Task 6: The TUI establishes the scene

**Files:**
- Modify: `pkg/tui/app.go`
- Modify: `cmd/localrpg/play.go` (wiring at line 134, the app construction at line 160)
- Test: `pkg/tui/app_test.go`

**Interfaces:**
- Consumes: `SetSceneOnly`, `SetOpeningPrompt`, `engine.OpeningMode`, `engine.OpeningPrompt`, `(*engine.HistoryLogger).LoadHistory`, `(*TurnOrchestrator).ProcessAction`.
- Produces: `func (m *AppModel) Seed(turns []engine.Turn)`.

- [ ] **Step 1: Write the failing test**

Append to `pkg/tui/app_test.go`:

```go
func TestSeedShowsAnEarlierTurn(t *testing.T) {
	model := NewAppModel(nil, 80, 24)
	model.Seed([]engine.Turn{{Number: 1, Mode: "Opening", Narration: "The market burns."}})

	if len(model.history) != 1 {
		t.Fatalf("history = %d turns, want 1", len(model.history))
	}
	if model.history[0].Mode != "Opening" {
		t.Errorf("Mode = %q, want %q", model.history[0].Mode, "Opening")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestSeedShowsAnEarlierTurn ./pkg/tui/`
Expected: FAIL to compile, `Seed` is undefined.

- [ ] **Step 3: Add the seeding method**

In `pkg/tui/app.go`, after `NewAppModel`:

```go
// Seed replaces the model's history, so a campaign that opened with a scene turn
// shows it before the first input.
func (m *AppModel) Seed(turns []engine.Turn) {
	m.history = append(m.history[:0], turns...)
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -run TestSeedShowsAnEarlierTurn ./pkg/tui/`
Expected: PASS

- [ ] **Step 5: Hand the engine the campaign's opening prompt**

In `cmd/localrpg/play.go`, immediately after `orchestrator.SetActionEcho(cfg.ActionEcho())`:

```go
	orchestrator.SetOpeningPrompt(engine.OpeningPrompt(manifest))
```

- [ ] **Step 6: Establish the scene before the first input**

In `cmd/localrpg/play.go`, replace the `app := tui.NewAppModel(...)` line:

```go
	app := tui.NewAppModel(orchestrator, 80, 24)

	// A campaign with an opening prompt and no history opens with the quiet scene
	// turn, so the TUI restates the scene the GUI shows in its Prologue.
	if engine.OpeningPrompt(manifest) != "" {
		if turns, err := history.LoadHistory(); err == nil && len(turns) == 0 {
			orchestrator.SetSceneOnly(true)
			if scene, err := orchestrator.ProcessAction(context.Background(), engine.OpeningMode, ""); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not establish the opening scene: %v\n", err)
			} else if scene != nil {
				app.Seed([]engine.Turn{*scene})
			}
		}
	}
```

- [ ] **Step 7: Build and run the whole test suite**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add pkg/tui/app.go pkg/tui/app_test.go cmd/localrpg/play.go
git commit -m "feat(tui): open a campaign by restating its scene"
```

---

## Manual verification

After Task 5, with a model provider configured:

1. `mise run dev:gui`, create a campaign with an opening prompt such as the airship wreckage sentence.
2. **Begin the story** — the first turn restates the wreckage, then adds a hook.
3. Create a second campaign with the same prompt and press **I'll take the first step** — turn 1 restates the wreckage and stops; the action typed afterwards is turn 2.
4. Create a third campaign with a blank opening prompt and press **I'll take the first step** — turn 1 is the player's action, with no scene turn.
