# Desktop Turn Submission Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the desktop app play a turn: stream the narrator's prose as it is written, record the turn through the existing pipeline, serialise concurrent turns, and make the action console actually submit.

**Architecture:** `TurnOrchestrator.ProcessActionStream` becomes the one turn pipeline and `ProcessAction` becomes its wrapper, so streaming cannot drift from the TUI's behaviour. The GUI gains a preflighted `RunTurn` behind a per-game lock, framed as newline-delimited JSON over a single POST, and the console consumes it with a `fetch` reader that is correct whether or not the client delivers the body progressively.

**Tech Stack:** Go 1.27.1, the existing `harness` streaming providers, React 19 + TypeScript, `mise` tasks.

**Spec:** `docs/superpowers/specs/2026-09-21-desktop-turn-submission-design.md`

## Global Constraints

- One turn pipeline. `ProcessAction` must call `ProcessActionStream`, never duplicate it.
- Nothing is persisted for a cancelled, failed, or disconnected turn: the timeline only ever holds completed turns.
- Turns are serialised per campaign; a second in-flight turn gets `409`, and the lock is released on every path.
- Provider setup lives in `pkg/harness`, shared by the CLI and the GUI. Nothing in `package main` is reachable from the GUI.
- A live turn and a replayed turn must be the same DTO shape, produced by one mapping function.
- The client must not depend on progressive delivery: a buffered body must produce the same events in the same order.
- Tests use the standard library only (`testing`, `t.TempDir()`); no testify. Use `interface{}`, never `any`.
- `go vet ./...`, `go test -count=1 ./...`, and `cd frontend && npx tsc --noEmit` must pass. Every commit must build standalone: `git worktree add --detach /tmp/verify <sha> && (cd /tmp/verify && go build ./...)`.

## Scope & Splitting

Phase 1 (streaming) and Phase 2 (shared factories) are prerequisites for the endpoint and are independently shippable: Phase 1 also makes the TUI's turns stream through the provider layer, and Phase 2 removes a duplication that already exists. Phase 3 (the endpoint) is the feature. Phase 4 (the console) is what makes it visible. This plan assumes the attribution, location, and playback work has shipped: it consumes `Turn.Location`, `Turn.Segments`, `Turn.Outcome`, `TurnDTO`, `Timeline.SetVoiceProfiles`, `media` playback routes, and the auto-created player note.

---

## Phase 1: One Streaming Pipeline

### Task 1: Stream the narration through the existing turn pipeline

**Files:**
- Modify: `pkg/engine/orchestrator.go`
- Test: `pkg/engine/orchestrator_stream_test.go`

**Interfaces:**
- Consumes: `harness.Router.StreamForRole`, `harness.StreamChunk`
- Produces: `(*TurnOrchestrator).ProcessActionStream(ctx context.Context, mode, actionInput string, onChunk func(text string) error) (*Turn, error)`; `(*TurnOrchestrator).generate(ctx context.Context, prompt string, onChunk func(string) error) (string, error)`

- [x] **Step 1: Write the failing test**

`pkg/engine/orchestrator_stream_test.go`:

```go
package engine

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// scriptedStreamProvider streams a fixed set of chunks, optionally failing or
// blocking, so the orchestrator's streaming behaviour is testable without a model.
type scriptedStreamProvider struct {
	chunks []string
	err    error
	block  bool
}

func (p *scriptedStreamProvider) ID() string { return "scripted" }

func (p *scriptedStreamProvider) Generate(ctx context.Context, req harness.GenerateRequest) (*harness.GenerateResponse, error) {
	var sb strings.Builder
	for _, chunk := range p.chunks {
		sb.WriteString(chunk)
	}
	return &harness.GenerateResponse{Text: sb.String()}, p.err
}

func (p *scriptedStreamProvider) Stream(ctx context.Context, req harness.GenerateRequest, out chan<- harness.StreamChunk) error {
	defer close(out)

	if p.block {
		<-ctx.Done()
		return ctx.Err()
	}

	for _, chunk := range p.chunks {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- harness.StreamChunk{Text: chunk}:
		}
	}
	return p.err
}

func streamingOrchestrator(t *testing.T, provider harness.ModelProvider) (*TurnOrchestrator, *Timeline, *storage.Store) {
	t.Helper()

	tempDir := t.TempDir()
	store := newTestStore(t)
	timeline := NewTimeline(core.NewPathResolver(tempDir), store, NewHistoryLogger(filepath.Join(tempDir, "history.jsonl")), "campaign-01")

	entitiesDir := timeline.EntitiesDir()
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "alden-tavern", Name: "Alden Tavern", Type: "location", Body: "Warm."})
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "player", Name: "Sean", Type: "character", Body: "A traveller.", Location: "[[alden-tavern]]"})
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		t.Fatal(err)
	}

	router := harness.NewRouter()
	router.RegisterProvider(provider)
	router.AssignRole("gm", provider.ID())

	return NewTurnOrchestrator(store, timeline, nil, router, "alden-tavern", "player"), timeline, store
}

func TestProcessActionStreamDeliversChunksInOrder(t *testing.T) {
	provider := &scriptedStreamProvider{chunks: []string{"The docks ", "reek of ", "brine."}}
	orchestrator, timeline, _ := streamingOrchestrator(t, provider)

	var received []string
	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I look around", func(text string) error {
		received = append(received, text)
		return nil
	})
	if err != nil {
		t.Fatalf("ProcessActionStream failed: %v", err)
	}

	if strings.Join(received, "") != "The docks reek of brine." {
		t.Errorf("chunks = %v, want the narration in order", received)
	}
	if turn.Narration != strings.Join(received, "") {
		t.Errorf("Narration = %q, want the concatenated chunks", turn.Narration)
	}

	recorded, err := timeline.history.LoadHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(recorded) != 1 || recorded[0].Narration != turn.Narration {
		t.Errorf("expected the streamed turn recorded, got %+v", recorded)
	}
}

func TestProcessActionMatchesTheStreamingPipeline(t *testing.T) {
	provider := &scriptedStreamProvider{chunks: []string{"Steel ", "rings."}}

	streamed, streamedTimeline, _ := streamingOrchestrator(t, provider)
	streamTurn, err := streamed.ProcessActionStream(context.Background(), "Do", "I swing", nil)
	if err != nil {
		t.Fatalf("ProcessActionStream failed: %v", err)
	}

	plain, plainTimeline, _ := streamingOrchestrator(t, provider)
	plainTurn, err := plain.ProcessAction(context.Background(), "Do", "I swing")
	if err != nil {
		t.Fatalf("ProcessAction failed: %v", err)
	}

	if streamTurn.Narration != plainTurn.Narration || streamTurn.Mode != plainTurn.Mode {
		t.Errorf("pipelines disagree: streamed %+v, plain %+v", streamTurn, plainTurn)
	}
	if streamTurn.Location != plainTurn.Location {
		t.Errorf("Location differs: %q vs %q", streamTurn.Location, plainTurn.Location)
	}
	if len(streamTurn.Entities) != len(plainTurn.Entities) {
		t.Errorf("involvement differs: %+v vs %+v", streamTurn.Entities, plainTurn.Entities)
	}

	for _, timeline := range []*Timeline{streamedTimeline, plainTimeline} {
		turns, err := timeline.history.LoadHistory()
		if err != nil {
			t.Fatal(err)
		}
		if len(turns) != 1 {
			t.Errorf("expected one recorded turn, got %d", len(turns))
		}
	}
}

func TestProcessActionStreamEmitsNothingForShortCircuitModes(t *testing.T) {
	provider := &scriptedStreamProvider{chunks: []string{"should not be used"}}
	orchestrator, _, _ := streamingOrchestrator(t, provider)

	calls := 0
	if _, err := orchestrator.ProcessActionStream(context.Background(), "System", "/go Alden Tavern", func(string) error {
		calls++
		return nil
	}); err != nil {
		t.Fatalf("/go failed: %v", err)
	}
	if calls != 0 {
		t.Errorf("/go produced %d chunks, want none", calls)
	}

	if _, err := orchestrator.ProcessActionStream(context.Background(), "System", "/undo", func(string) error {
		calls++
		return nil
	}); err == nil {
		t.Errorf("expected /undo without turns to fail")
	}
	if calls != 0 {
		t.Errorf("/undo produced %d chunks, want none", calls)
	}
}
```

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test -run "TestProcessActionStream|TestProcessActionMatchesTheStreaming" -count=1 ./pkg/engine/`
Expected: FAIL — `orchestrator.ProcessActionStream undefined`

- [x] **Step 3: Implement**

In `pkg/engine/orchestrator.go`, split generation out and make the non-streaming entry point a wrapper:

```go
// ProcessActionStream runs a turn, reporting narration deltas as they arrive. It
// is the implementation; ProcessAction is the same pipeline without a listener.
// A non-nil error from onChunk aborts before anything is recorded, which is how a
// client that has disconnected stops generation rather than letting it finish into
// nothing.
func (o *TurnOrchestrator) ProcessActionStream(ctx context.Context, mode, actionInput string, onChunk func(text string) error) (*Turn, error) {
	// … the entire body of ProcessAction, unchanged, up to and including context
	// assembly, with the generation call replaced by:

	narration, err := o.generate(ctx, contextPrompt, onChunk)
	if err != nil {
		return nil, fmt.Errorf("gm generation failed: %w", err)
	}
	// … and `Narration: narration` on the constructed turn.
}

// ProcessAction runs a turn without reporting narration as it arrives.
func (o *TurnOrchestrator) ProcessAction(ctx context.Context, mode, actionInput string) (*Turn, error) {
	return o.ProcessActionStream(ctx, mode, actionInput, nil)
}

// generate streams the GM's reply, forwarding each delta and accumulating the text.
func (o *TurnOrchestrator) generate(ctx context.Context, prompt string, onChunk func(string) error) (string, error) {
	chunks := make(chan harness.StreamChunk, 32)

	streamErr := make(chan error, 1)
	go func() {
		streamErr <- o.router.StreamForRole(ctx, "gm", harness.GenerateRequest{Prompt: prompt}, chunks)
	}()

	var sb strings.Builder
	for chunk := range chunks {
		if chunk.Error != nil {
			<-streamErr
			return "", chunk.Error
		}
		if chunk.Text == "" {
			continue
		}

		sb.WriteString(chunk.Text)
		if onChunk != nil {
			if err := onChunk(chunk.Text); err != nil {
				return "", err
			}
		}
	}

	if err := <-streamErr; err != nil {
		return "", err
	}
	return sb.String(), nil
}
```

The `ProcessAction` doc comment that described the pipeline moves onto `ProcessActionStream`; nothing else in the pipeline changes, which is the point of the task.

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/engine/ ./pkg/tui/`
Expected: PASS, including the TUI's existing turn tests, which now exercise the streaming path.

- [x] **Step 5: Commit**

```bash
git add pkg/engine/orchestrator.go pkg/engine/orchestrator_stream_test.go
git commit -m "feat(engine): stream the narrator's prose through the turn pipeline"
```

### Task 2: An interrupted turn leaves nothing behind

**Files:**
- Modify: `pkg/engine/orchestrator.go`
- Test: `pkg/engine/orchestrator_stream_test.go`

**Interfaces:**
- Consumes: `Timeline.RecordTurn`, `storage.Store.CountTurns`
- Produces: cancellation and failure checks before `RecordTurn`

- [x] **Step 1: Write the failing tests**

Append to `pkg/engine/orchestrator_stream_test.go`:

```go
func TestCancellationRecordsNothing(t *testing.T) {
	provider := &scriptedStreamProvider{block: true}
	orchestrator, timeline, store := streamingOrchestrator(t, provider)

	ctx, cancel := context.WithCancel(context.Background())
	go cancel()

	if _, err := orchestrator.ProcessActionStream(ctx, "Do", "I wait", nil); err == nil {
		t.Fatalf("expected a cancelled turn to fail")
	}

	turns, err := timeline.history.LoadHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 0 {
		t.Errorf("expected no recorded turn, got %+v", turns)
	}

	if count, err := store.CountTurns(); err != nil || count != 0 {
		t.Errorf("CountTurns = %d, %v; want 0", count, err)
	}

	player, err := store.GetEntity("player")
	if err != nil {
		t.Fatal(err)
	}
	if len(player.History) != 0 {
		t.Errorf("expected no involvement recorded, got %v", player.History)
	}
}

func TestChunkFailureAbortsBeforeRecording(t *testing.T) {
	provider := &scriptedStreamProvider{chunks: []string{"The docks ", "reek "}}
	orchestrator, timeline, store := streamingOrchestrator(t, provider)

	clientGone := errors.New("client disconnected")
	if _, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I look", func(string) error {
		return clientGone
	}); !errors.Is(err, clientGone) {
		t.Fatalf("expected the listener's error, got %v", err)
	}

	turns, err := timeline.history.LoadHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 0 {
		t.Errorf("expected nothing recorded, got %+v", turns)
	}
	if count, _ := store.CountTurns(); count != 0 {
		t.Errorf("CountTurns = %d, want 0", count)
	}
}

func TestMidStreamProviderFailureRecordsNothing(t *testing.T) {
	provider := &scriptedStreamProvider{chunks: []string{"Steel "}, err: errors.New("model exploded")}
	orchestrator, timeline, _ := streamingOrchestrator(t, provider)

	if _, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I swing", nil); err == nil {
		t.Fatalf("expected the provider failure to surface")
	}

	turns, err := timeline.history.LoadHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 0 {
		t.Errorf("expected nothing recorded, got %+v", turns)
	}
}
```

Add `"errors"` to the test file's imports. (`scriptedStreamProvider` returns `p.err` after its chunks, which is exactly the mid-stream failure the router forwards.)

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test -run "TestCancellationRecordsNothing|TestChunkFailure|TestMidStreamProviderFailure" -count=1 ./pkg/engine/`
Expected: FAIL — a cancelled stream still records a turn with empty narration, because nothing checks the context before `RecordTurn`.

- [x] **Step 3: Implement**

In `ProcessActionStream`, immediately before the timeline write:

```go
	// Nothing is persisted for a cancelled or failed turn: the timeline only ever
	// holds completed turns, so a disconnect is a no-op on disk.
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("turn cancelled: %w", err)
	}

	if err := o.timeline.RecordTurn(&turn, extraction.Entities); err != nil {
		return nil, fmt.Errorf("record turn: %w", err)
	}
```

Also guard the empty-narration case, which is what a provider returning nothing looks like:

```go
	if strings.TrimSpace(turn.Narration) == "" {
		return nil, fmt.Errorf("gm returned no narration")
	}
```

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/engine/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/engine/orchestrator.go pkg/engine/orchestrator_stream_test.go
git commit -m "fix(engine): record nothing when a turn is interrupted"
```

---

## Phase 2: Shared Provider Setup

### Task 3: Build the router from config in one place

**Files:**
- Modify: `pkg/harness/factory.go`
- Modify: `cmd/localrpg/play.go`
- Test: `pkg/harness/factory_test.go`

**Interfaces:**
- Consumes: `NewModelProvider`, `config.AgentsConfig`
- Produces: `harness.RouterFromConfig(cfg *config.Config) (*Router, error)`

- [x] **Step 1: Write the failing test**

Append to `pkg/harness/factory_test.go`:

```go
func TestRouterFromConfigBuildsEachRole(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agents.Roles["narrator"] = config.AgentRoleConfig{Type: "cli", Command: "echo"}
	cfg.Agents.Fallbacks = map[string]string{"gm": "narrator"}

	router, err := RouterFromConfig(cfg)
	if err != nil {
		t.Fatalf("RouterFromConfig failed: %v", err)
	}

	if _, err := router.GetProviderForRole("gm"); err != nil {
		t.Errorf("expected the gm role to resolve: %v", err)
	}
	if _, err := router.GetProviderForRole("narrator"); err != nil {
		t.Errorf("expected the narrator role to resolve: %v", err)
	}
}

func TestRouterFromConfigSkipsInheritedRoles(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agents.Roles[config.RoleExtractor] = config.AgentRoleConfig{
		Type:        "inherit",
		InheritFrom: config.RoleGM,
	}

	router, err := RouterFromConfig(cfg)
	if err != nil {
		t.Fatalf("RouterFromConfig failed: %v", err)
	}

	// An inherited role is resolved through another role, not registered itself.
	if _, err := router.GetProviderForRole(config.RoleExtractor); err == nil {
		t.Errorf("expected the inherited role to have no provider of its own")
	}
	if ExtractorFromConfig(cfg, router) == nil {
		t.Errorf("expected extraction to resolve through the inherited role")
	}
}

func TestRouterFromConfigFallsBackToEchoForGM(t *testing.T) {
	cfg := config.DefaultConfig()
	delete(cfg.Agents.Roles, "gm")

	router, err := RouterFromConfig(cfg)
	if err != nil {
		t.Fatalf("RouterFromConfig failed: %v", err)
	}

	provider, err := router.GetProviderForRole("gm")
	if err != nil {
		t.Fatalf("expected an echo fallback for gm: %v", err)
	}
	if provider.ID() != "default-echo" {
		t.Errorf("provider = %q, want default-echo", provider.ID())
	}
}
```

Add the `config` import to the test file.

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test -run TestRouterFromConfig -count=1 ./pkg/harness/`
Expected: FAIL — `undefined: RouterFromConfig`

- [x] **Step 3: Implement**

In `pkg/harness/factory.go`:

```go
// RouterFromConfig builds the role-routed provider registry a turn needs: one
// provider per configured role, the configured fallbacks, and an echo default for
// gm when nothing else is set up. Inherited roles are skipped because they resolve
// through the role they name.
func RouterFromConfig(cfg *config.Config) (*Router, error) {
	if cfg == nil {
		return nil, fmt.Errorf("build router: no config")
	}

	router := NewRouter()

	for role, roleCfg := range cfg.Agents.Roles {
		if roleCfg.Type == "inherit" {
			continue
		}

		provider, err := NewModelProvider(role, ProviderConfig{
			Type:        roleCfg.Type,
			BuiltinName: roleCfg.BuiltinName,
			Command:     roleCfg.Command,
			Args:        roleCfg.Args,
			Endpoint:    roleCfg.Endpoint,
			Model:       roleCfg.Model,
			APIKey:      roleCfg.APIKey,
			Temperature: roleCfg.Temperature,
			MaxTokens:   roleCfg.MaxTokens,
		})
		if err != nil {
			continue
		}

		router.RegisterProvider(provider)
		router.AssignRole(role, role)
	}

	for role, fallback := range cfg.Agents.Fallbacks {
		if fallback != "" {
			router.SetFallback(role, fallback)
		}
	}

	if _, err := router.GetProviderForRole(config.RoleGM); err != nil {
		router.RegisterProvider(NewCLIProvider("default-echo", "echo", []string{}))
		router.AssignRole(config.RoleGM, "default-echo")
	}

	return router, nil
}
```

In `cmd/localrpg/play.go`, replace the role loop, the fallback loop, and the gm fallback block with:

```go
	router, err := harness.RouterFromConfig(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error building model router: %v\n", err)
		os.Exit(1)
	}
```

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/harness/ ./cmd/localrpg/ ./pkg/tui/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/harness/factory.go pkg/harness/factory_test.go cmd/localrpg/play.go
git commit -m "refactor(harness): build the model router from config in one place"
```

### Task 4: Resolve the extractor role in one place

**Files:**
- Modify: `pkg/harness/factory.go`
- Modify: `cmd/localrpg/play.go` (delete `resolveExtractor`)
- Move: `cmd/localrpg/play_resolver_test.go` → `pkg/harness/extractor_config_test.go`
- Test: `pkg/harness/extractor_config_test.go`

**Interfaces:**
- Consumes: `RouterFromConfig`, `Router.GetProviderForRole`
- Produces: `harness.ExtractorFromConfig(cfg *config.Config, router *Router) *Extractor`

- [x] **Step 1: Move the failing test**

Move `cmd/localrpg/play_resolver_test.go` to `pkg/harness/extractor_config_test.go`, changing the package to `harness`, dropping the `config.`/`harness.` qualifiers, and repointing `resolveExtractor` at `ExtractorFromConfig`. Its `routerWithGM` helper becomes:

```go
func routerWithGM(t *testing.T) *Router {
	t.Helper()

	router := NewRouter()
	router.RegisterProvider(NewCLIProvider("gm", "echo", []string{}))
	router.AssignRole(config.RoleGM, "gm")
	return router
}
```

Run: `go test -run TestExtractorFromConfig -count=1 ./pkg/harness/`
Expected: FAIL — `undefined: ExtractorFromConfig`

- [x] **Step 2: Implement**

In `pkg/harness/factory.go`:

```go
// ExtractorFromConfig resolves the per-turn extractor role. An absent role still
// inherits gm, so configuration written before the role existed keeps working;
// `disabled` opts out; `inherit` follows the named role, which is what stops
// extraction silently pointing at a stale copy of gm.
func ExtractorFromConfig(cfg *config.Config, router *Router) *Extractor {
	if cfg == nil || router == nil {
		return nil
	}

	roleCfg, configured := cfg.Agents.Roles[config.RoleExtractor]
	if !configured {
		roleCfg = config.AgentRoleConfig{Type: "inherit", InheritFrom: config.RoleGM}
	}

	switch roleCfg.Type {
	case "disabled":
		return nil
	case "inherit", "":
		source := roleCfg.InheritFrom
		if source == "" {
			source = config.RoleGM
		}

		provider, err := router.GetProviderForRole(source)
		if err != nil {
			return nil
		}
		return NewExtractor(provider)
	}

	provider, err := NewModelProvider(config.RoleExtractor, ProviderConfig{
		Type:        roleCfg.Type,
		BuiltinName: roleCfg.BuiltinName,
		Command:     roleCfg.Command,
		Args:        roleCfg.Args,
		Endpoint:    roleCfg.Endpoint,
		Model:       roleCfg.Model,
		APIKey:      roleCfg.APIKey,
		Temperature: roleCfg.Temperature,
		MaxTokens:   roleCfg.MaxTokens,
	})
	if err != nil {
		return nil
	}
	return NewExtractor(provider)
}
```

In `cmd/localrpg/play.go`, replace `orchestrator.SetExtractor(resolveExtractor(cfg, router))` with `orchestrator.SetExtractor(harness.ExtractorFromConfig(cfg, router))` and delete `resolveExtractor`.

- [x] **Step 3: Verify**

Run: `go test -count=1 ./pkg/harness/ ./cmd/localrpg/`
Expected: PASS, with the relocated resolver tests running in their new home.

- [x] **Step 4: Commit**

```bash
git add pkg/harness/factory.go pkg/harness/extractor_config_test.go cmd/localrpg/play.go
git rm --cached cmd/localrpg/play_resolver_test.go
git commit -m "refactor(harness): resolve the extractor role for every client"
```

<!-- PLAN-CONTINUES -->

---

## Phase 3: The Endpoint

### Task 5: One turn mapping for live and replayed turns

**Files:**
- Modify: `pkg/gui/service.go` (`turnDTO` extracted from `GetChronicle`)
- Test: `pkg/gui/service_test.go`

**Interfaces:**
- Consumes: `engine.Turn`, `TurnDTO`, `segmentDTOs`
- Produces: `(*Service).turnDTO(turn engine.Turn, store *storage.Store, cfg *config.Config, gameID string) TurnDTO`

- [x] **Step 1: Write the failing test**

Append to `pkg/gui/service_test.go`:

```go
func TestChronicleTurnsCarryLocationAndPacing(t *testing.T) {
	gameID, svc := setupTestGame(t)

	record := `{"number":1,"timestamp":"2026-09-21T10:00:00Z","mode":"Do","input":"I look around","narration":"The harbour is quiet.","location":"aldon-harbour","outcome":"clean_look","segments":[{"kind":"narration","text":"The harbour is quiet."}],"entities":[{"id":"player-elena","mention":"player"},{"id":"aldon-harbour","mention":"location"}]}` + "\n"
	if err := os.WriteFile(filepath.Join(svc.GetResolver().GameDir(gameID), "history.jsonl"), []byte(record), 0644); err != nil {
		t.Fatal(err)
	}

	turns, err := svc.GetChronicle(context.Background(), gameID)
	if err != nil {
		t.Fatalf("GetChronicle failed: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(turns))
	}

	turn := turns[0]
	if turn.LocationID != "aldon-harbour" || turn.LocationName != "Aldon Harbour" {
		t.Errorf("expected the location resolved, got %q / %q", turn.LocationID, turn.LocationName)
	}
	if turn.Outcome != "clean_look" {
		t.Errorf("Outcome = %q", turn.Outcome)
	}
	if turn.LocationArtURL == "" {
		t.Errorf("expected an art URL when the built-in generator is available")
	}
	if len(turn.Segments) != 1 || turn.Segments[0].Duration < scene.MinimumBeatDuration.Seconds() {
		t.Errorf("expected a paced segment, got %+v", turn.Segments)
	}
}
```

The test's `aldon-harbour` location and the config's media settings come from the fixtures the earlier plans added to `setupTestGame`.

- [x] **Step 2: Run the test to verify it fails**

Run: `go test -run TestChronicleTurnsCarryLocationAndPacing -count=1 ./pkg/gui/`
Expected: FAIL if the chronicle's mapping is still inline and incomplete for these fields; PASS means the fields already exist and this test is the pin for the refactor.

- [x] **Step 3: Implement**

Extract the mapping in `pkg/gui/service.go`:

```go
// turnDTO maps a persisted turn for the API. GetChronicle and the turn endpoint
// share it so a live turn and a replayed one are the same shape, which is what
// lets the client render both with one code path.
func (s *Service) turnDTO(turn engine.Turn, store *storage.Store, cfg *config.Config, gameID string) TurnDTO {
	audioAvailable := cfg.Media.TTS.Type != "" && cfg.Media.TTS.Type != "disabled"
	artAvailable := cfg.Media.Image.BuiltinFallback || cfg.Media.Image.Type != "disabled"

	dto := TurnDTO{
		TurnNumber:  turn.Number,
		InputText:   turn.Input,
		Mode:        turn.Mode,
		Prose:       turn.Prose(),
		Outcome:     turn.Outcome,
		EntitiesHit: mentionIDs(turn.Entities),
		Segments:    segmentDTOs(turn.Segments, gameID, turn.Number, audioAvailable),
	}

	if turn.Location != "" {
		dto.LocationID = turn.Location
		if store != nil {
			if location, err := store.GetEntity(turn.Location); err == nil && location != nil {
				dto.LocationName = location.Name
			}
		}
		if artAvailable {
			dto.LocationArtURL = "/api/game/" + gameID + "/location/" + turn.Location + "/art"
		}
	}

	return dto
}
```

`GetChronicle` loads configuration and the pooled store once, then maps each turn through it:

```go
	cfg := s.configMgr.Get()
	store, err := s.store(gameID)
	if err != nil {
		store = nil // location names and art URLs are decoration, not prerequisites
	}

	dtos := make([]TurnDTO, len(turns))
	for i, turn := range turns {
		dtos[i] = s.turnDTO(turn, store, cfg, gameID)
	}
```

The existing chronicle tests are the regression pin for this refactor.

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/gui/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/gui/service.go pkg/gui/service_test.go
git commit -m "refactor(gui): map a turn the same way live and replayed"
```

### Task 6: A prepared turn session

**Files:**
- Modify: `pkg/gui/service.go` (`BeginTurn`, `TurnSession`, per-game locks, errors)
- Test: `pkg/gui/turn_test.go`

**Interfaces:**
- Consumes: `engine.NewTimeline`, `engine.NewTurnOrchestrator`, `harness.RouterFromConfig`, `harness.ExtractorFromConfig`, `rules.NewJSEngine`, `rules.NewHostBridge`, `core.LoadSystemManifest`, `core.LoadWorldManifest`
- Produces: `gui.TurnSession`, `(*Service).BeginTurn(gameID string) (*TurnSession, error)`, `(*TurnSession).Run(ctx context.Context, req TurnRequest, emit func(TurnEvent) error) error`, `(*TurnSession).Close()`, `gui.ErrTurnInFlight`, `gui.ErrCampaignNotPlayable`

- [x] **Step 1: Write the failing test**

`pkg/gui/turn_test.go`:

```go
package gui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/engine"
)

// turnFixture builds a playable campaign: a config whose gm provider is the
// built-in echo engine, a game, a system, and a world.
func turnFixture(t *testing.T) (string, *Service) {
	t.Helper()

	root := t.TempDir()
	configYAML := "agents:\n  roles:\n    gm:\n      type: builtin\n"
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	svc := NewService(root)
	paths := svc.GetResolver()

	sysDir := paths.SystemDir("freeform")
	if err := os.MkdirAll(sysDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sysDir, "system.yaml"), []byte("id: freeform\nname: Freeform\nversion: 1.0\n"), 0644); err != nil {
		t.Fatal(err)
	}

	worldDir := paths.WorldDir("harbour-realm")
	if err := os.MkdirAll(filepath.Join(worldDir, "entities"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worldDir, "world.yaml"), []byte("id: harbour-realm\nname: Harbour Realm\n"), 0644); err != nil {
		t.Fatal(err)
	}

	session, err := engine.InitGame(paths, "campaign-01", "freeform", "harbour-realm", "Sean")
	if err != nil {
		t.Fatalf("InitGame failed: %v", err)
	}
	_ = session.Close()

	return "campaign-01", svc
}

func TestBeginTurnSerialisesTurns(t *testing.T) {
	gameID, svc := turnFixture(t)

	first, err := svc.BeginTurn(gameID)
	if err != nil {
		t.Fatalf("first BeginTurn failed: %v", err)
	}
	defer first.Close()

	if _, err := svc.BeginTurn(gameID); !errors.Is(err, ErrTurnInFlight) {
		t.Fatalf("expected ErrTurnInFlight, got %v", err)
	}

	// The lock is released on Close, not before.
	first.Close()

	second, err := svc.BeginTurn(gameID)
	if err != nil {
		t.Fatalf("BeginTurn after Close failed: %v", err)
	}
	second.Close()
}

func TestBeginTurnRejectsAnUnplayableCampaign(t *testing.T) {
	root := t.TempDir()
	svc := NewService(root)

	if _, err := svc.BeginTurn("absent-campaign"); !errors.Is(err, ErrCampaignNotPlayable) {
		t.Errorf("expected ErrCampaignNotPlayable, got %v", err)
	}
}

func TestTurnSessionRunsAndRecordsATurn(t *testing.T) {
	gameID, svc := turnFixture(t)

	session, err := svc.BeginTurn(gameID)
	if err != nil {
		t.Fatalf("BeginTurn failed: %v", err)
	}
	defer session.Close()

	var chunks []string
	var final *TurnDTO
	err = session.Run(context.Background(), TurnRequest{Mode: "Do", Input: "I look around"}, func(event TurnEvent) error {
		switch event.Type {
		case "chunk":
			chunks = append(chunks, event.Text)
		case "turn":
			final = event.Turn
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if len(chunks) == 0 {
		t.Errorf("expected streamed chunks from the built-in provider")
	}
	if final == nil {
		t.Fatalf("expected a final turn event")
	}
	if !strings.Contains(final.Prose, "Echo:") {
		t.Errorf("Prose = %q, want the provider's narration", final.Prose)
	}
	if final.LocationID == "" {
		t.Errorf("expected the turn to record where it happened")
	}

	// The live DTO and the replayed one agree, which is the whole point of the
	// shared mapping.
	chronicle, err := svc.GetChronicle(context.Background(), gameID)
	if err != nil {
		t.Fatal(err)
	}
	if len(chronicle) != 1 {
		t.Fatalf("expected 1 recorded turn, got %d", len(chronicle))
	}
	if chronicle[0].Prose != final.Prose || chronicle[0].LocationID != final.LocationID {
		t.Errorf("live and replayed turns differ:\n%+v\n%+v", *final, chronicle[0])
	}
}

func TestTurnSessionRecordsNothingWhenEmitFails(t *testing.T) {
	gameID, svc := turnFixture(t)

	session, err := svc.BeginTurn(gameID)
	if err != nil {
		t.Fatalf("BeginTurn failed: %v", err)
	}
	defer session.Close()

	clientGone := errors.New("client disconnected")
	if err := session.Run(context.Background(), TurnRequest{Mode: "Do", Input: "I look"}, func(TurnEvent) error {
		return clientGone
	}); !errors.Is(err, clientGone) {
		t.Fatalf("expected the emit error, got %v", err)
	}

	chronicle, err := svc.GetChronicle(context.Background(), gameID)
	if err != nil {
		t.Fatal(err)
	}
	if len(chronicle) != 0 {
		t.Errorf("expected nothing recorded, got %+v", chronicle)
	}
}
```

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test -run "TestBeginTurn|TestTurnSession" -count=1 ./pkg/gui/`
Expected: FAIL — `svc.BeginTurn undefined`

- [x] **Step 3: Implement**

In `pkg/gui/service.go`, add to `Service` a lock map beside `indexed`, initialised in `NewService`:

```go
	locks map[string]*sync.Mutex
```

```go
// ErrTurnInFlight means another turn is already running for this campaign.
var ErrTurnInFlight = errors.New("a turn is already in flight")

// ErrCampaignNotPlayable means the campaign's files are not ready for a turn, so
// the caller can answer before any bytes are sent.
var ErrCampaignNotPlayable = errors.New("campaign cannot be prepared")

// gameLock returns the campaign's turn lock, creating it on first use.
func (s *Service) gameLock(gameID string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.locks == nil {
		s.locks = make(map[string]*sync.Mutex)
	}
	if lock, ok := s.locks[gameID]; ok {
		return lock
	}

	lock := &sync.Mutex{}
	s.locks[gameID] = lock
	return lock
}

// TurnSession is one prepared turn: the campaign's timeline and orchestrator,
// wired exactly as the CLI wires them, holding the campaign's turn lock until
// Close. Preparing up front is what lets the caller answer 409 or 503 as a status
// code rather than as an event after streaming has begun.
type TurnSession struct {
	service      *Service
	gameID       string
	cfg          *config.Config
	store        *storage.Store
	timeline     *engine.Timeline
	orchestrator *engine.TurnOrchestrator
	release      func()
}

func (s *Service) BeginTurn(gameID string) (*TurnSession, error) {
	lock := s.gameLock(gameID)
	if !lock.TryLock() {
		return nil, ErrTurnInFlight
	}
	release := lock.Unlock

	session, err := s.prepareTurn(gameID)
	if err != nil {
		release()
		// Both the sentinel and the cause are wrapped, so the route can tell an
		// unknown game (404) from a campaign that exists but cannot be played (503).
		return nil, fmt.Errorf("%w: %w", ErrCampaignNotPlayable, err)
	}

	session.release = release
	return session, nil
}

// Close releases the campaign's turn lock. It is safe to call twice.
func (t *TurnSession) Close() {
	if t.release == nil {
		return
	}
	t.release()
	t.release = nil
}

// prepareTurn assembles everything a turn needs. It is built per turn on purpose:
// a cached orchestrator would miss settings changes and note edits, which is a
// failure this codebase has already produced twice.
func (s *Service) prepareTurn(gameID string) (*TurnSession, error) {
	s.ensureIndexed(gameID)

	gameDir := s.resolver.GameDir(gameID)
	manifest, err := core.LoadGameManifest(filepath.Join(gameDir, "game.yaml"))
	if err != nil {
		return nil, fmt.Errorf("load game manifest: %w", err)
	}
	if _, err := core.LoadSystemManifest(filepath.Join(s.resolver.SystemDir(manifest.SystemID), "system.yaml")); err != nil {
		return nil, fmt.Errorf("load system %q: %w", manifest.SystemID, err)
	}
	if _, err := core.LoadWorldManifest(filepath.Join(s.resolver.WorldDir(manifest.WorldID), "world.yaml")); err != nil {
		return nil, fmt.Errorf("load world %q: %w", manifest.WorldID, err)
	}

	store, err := s.store(gameID)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}

	cfg := s.configMgr.Get()
	timeline := engine.NewTimeline(s.resolver, store, engine.NewHistoryLogger(filepath.Join(gameDir, "history.jsonl")), gameID)
	timeline.SetVoiceProfiles(cfg.Media.TTS.VoiceProfiles)

	router, err := harness.RouterFromConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build router: %w", err)
	}

	jsEngine := rules.NewJSEngine(rules.NewHostBridge(store, timeline, manifest.Player))

	startLocation := ""
	if pinned, ok := manifest.Settings[engine.StartLocationSetting].(string); ok {
		startLocation = pinned
	}

	orchestrator := engine.NewTurnOrchestrator(store, timeline, jsEngine, router, startLocation, manifest.Player)
	orchestrator.SetExtractor(harness.ExtractorFromConfig(cfg, router))
	orchestrator.LoadPrompts(s.resolver, manifest.SystemID, manifest.WorldID)

	return &TurnSession{
		service:      s,
		gameID:       gameID,
		cfg:          cfg,
		store:        store,
		timeline:     timeline,
		orchestrator: orchestrator,
	}, nil
}

// Run plays one turn, emitting events as they happen. An emit failure cancels the
// turn, which is how a disconnected client stops generation rather than paying for
// a turn nobody will see.
func (t *TurnSession) Run(ctx context.Context, req TurnRequest, emit func(TurnEvent) error) error {
	turn, err := t.orchestrator.ProcessActionStream(ctx, req.Mode, req.Input, func(text string) error {
		return emit(TurnEvent{Type: "chunk", Text: text})
	})
	if err != nil {
		return err
	}

	dto := t.service.turnDTO(*turn, t.store, t.cfg, t.gameID)
	return emit(TurnEvent{Type: "turn", Turn: &dto})
}
```

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/gui/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/gui/service.go pkg/gui/turn_test.go
git commit -m "feat(gui): prepare and run one turn per campaign under a lock"
```

### Task 7: The turn endpoint

**Files:**
- Modify: `pkg/gui/types.go` (`TurnRequest`, `TurnEvent`, mode validation)
- Modify: `pkg/gui/server.go` (route, NDJSON framing, status mapping)
- Test: `pkg/gui/turn_test.go`, `pkg/gui/server_test.go`

**Interfaces:**
- Consumes: `BeginTurn`, `TurnSession.Run`, `ErrTurnInFlight`, `ErrCampaignNotPlayable`
- Produces: `gui.TurnRequest`, `gui.TurnEvent`, `(*TurnRequest).validate() error`; route `POST /api/game/{id}/turn`

- [x] **Step 1: Write the failing tests**

Append to `pkg/gui/server_test.go`:

```go
func TestTurnEndpointStreamsNDJSON(t *testing.T) {
	gameID, svc := turnFixture(t)
	server := NewServer(svc, http.NotFoundHandler())

	req := httptest.NewRequest("POST", "/api/game/"+gameID+"/turn", strings.NewReader(`{"mode":"do","input":"I look around"}`))
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/x-ndjson" {
		t.Errorf("Content-Type = %q, want application/x-ndjson", ct)
	}

	var last TurnEvent
	lines := 0
	for _, line := range strings.Split(strings.TrimSpace(rec.Body.String()), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines++

		var event TurnEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("line %d is not JSON: %v\n%s", lines, err, line)
		}
		last = event
	}

	if lines == 0 {
		t.Fatalf("expected at least one event")
	}
	if last.Type != "turn" || last.Turn == nil {
		t.Fatalf("expected a final turn event, got %+v", last)
	}
	if last.Turn.TurnNumber != 1 {
		t.Errorf("TurnNumber = %d, want 1", last.Turn.TurnNumber)
	}
}

func TestTurnEndpointRejectsBadRequests(t *testing.T) {
	gameID, svc := turnFixture(t)
	server := NewServer(svc, http.NotFoundHandler())

	cases := []struct {
		name       string
		path       string
		body       string
		wantStatus int
	}{
		{"unknown mode", "/api/game/" + gameID + "/turn", `{"mode":"dance","input":"hello"}`, http.StatusBadRequest},
		{"empty input", "/api/game/" + gameID + "/turn", `{"mode":"do","input":"   "}`, http.StatusBadRequest},
		{"malformed body", "/api/game/" + gameID + "/turn", `{`, http.StatusBadRequest},
		{"unknown game", "/api/game/absent-campaign/turn", `{"mode":"do","input":"hi"}`, http.StatusNotFound},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", tc.path, strings.NewReader(tc.body))
			rec := httptest.NewRecorder()
			server.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d: %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestTurnEndpointReportsAnUnplayableCampaign(t *testing.T) {
	gameID, svc := turnFixture(t)
	server := NewServer(svc, http.NotFoundHandler())

	// A campaign whose manifest names a system that does not exist: playable
	// campaign files are missing, but the game itself is there.
	brokenDir := svc.GetResolver().GameDir("broken-campaign")
	if err := os.MkdirAll(brokenDir, 0755); err != nil {
		t.Fatal(err)
	}
	manifest := "id: broken-campaign\nname: Broken\nsystem: ghost-system\nworld: harbour-realm\nplayer: sean\n"
	if err := os.WriteFile(filepath.Join(brokenDir, "game.yaml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("POST", "/api/game/broken-campaign/turn", strings.NewReader(`{"mode":"do","input":"hello"}`))
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "ghost-system") {
		t.Errorf("expected the missing system named in the error, got %s", rec.Body.String())
	}

	_ = gameID
}

func TestTurnEndpointConflictsWhileATurnIsInFlight(t *testing.T) {
	gameID, svc := turnFixture(t)
	server := NewServer(svc, http.NotFoundHandler())

	// Hold the campaign's lock the way an in-flight turn would.
	session, err := svc.BeginTurn(gameID)
	if err != nil {
		t.Fatalf("BeginTurn failed: %v", err)
	}
	defer session.Close()

	req := httptest.NewRequest("POST", "/api/game/"+gameID+"/turn", strings.NewReader(`{"mode":"do","input":"I wait"}`))
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Errorf("expected a Retry-After header")
	}
}
```

Add `"encoding/json"` to `server_test.go`'s imports.

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test -run TestTurnEndpoint -count=1 ./pkg/gui/`
Expected: FAIL — the route does not exist, so these requests 404.

- [x] **Step 3: Implement**

In `pkg/gui/types.go`:

```go
// TurnRequest is a player action as submitted from a client.
type TurnRequest struct {
	Mode  string `json:"mode"`
	Input string `json:"input"`
}

// TurnEvent is one NDJSON line sent while a turn runs.
type TurnEvent struct {
	Type    string   `json:"type"`              // "chunk", "turn", or "error"
	Text    string   `json:"text,omitempty"`    // narration delta
	Turn    *TurnDTO `json:"turn,omitempty"`    // the persisted turn
	Message string   `json:"message,omitempty"` // failure detail
}

// turnModes maps the mode names a client may send to the engine's casing.
var turnModes = map[string]string{
	"do": "Do", "say": "Say", "story": "Story", "roll": "Roll", "gm": "GM", "system": "System",
}

// validate normalises a submitted turn and rejects one the engine cannot run.
func (r *TurnRequest) validate() error {
	mode, ok := turnModes[strings.ToLower(strings.TrimSpace(r.Mode))]
	if !ok {
		return fmt.Errorf("unknown mode %q", r.Mode)
	}
	r.Mode = mode

	if strings.TrimSpace(r.Input) == "" {
		return fmt.Errorf("input is required")
	}
	return nil
}
```

In `pkg/gui/server.go`, add the case to `handleGameRoutes`'s switch, sharing it with the segment-audio route the export work adds:

```go
	case "turn":
		// POST /api/game/{id}/turn submits a turn. GET
		// /api/game/{id}/turn/{n}/segment/{i}/audio serves one beat's clip; both
		// hang off a turn, so they share this case.
		if r.Method == http.MethodPost && len(parts) == 2 {
			s.handleTurnSubmit(w, r, gameID)
			return
		}
		// If the export work has landed, its branch for
		// parts[1] == "segment" && parts[3] == "audio" follows here; without it,
		// this case handles POST only and a GET falls through to the switch's
		// default (404).
```

and the handler:

```go
// maxTurnBody bounds a player action so a runaway paste cannot allocate without
// limit. It is far above any plausible action.
const maxTurnBody = 64 << 10

// handleTurnSubmit streams a turn as newline-delimited JSON. Status codes can only
// be chosen before the first byte, so the campaign is prepared first: 409 for a
// turn already in flight, 503 for a campaign that cannot be played, and an `error`
// event for anything that happens once streaming has begun.
func (s *Server) handleTurnSubmit(w http.ResponseWriter, r *http.Request, gameID string) {
	var req TurnRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnBody)).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := req.validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	session, err := s.service.BeginTurn(gameID)
	switch {
	case errors.Is(err, ErrTurnInFlight):
		w.Header().Set("Retry-After", "1")
		http.Error(w, err.Error(), http.StatusConflict)
		return
	case errors.Is(err, fs.ErrNotExist):
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	case err != nil:
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer session.Close()

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported by this client", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	encoder := json.NewEncoder(w)
	writeEvent := func(event TurnEvent) error {
		if err := encoder.Encode(event); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	if err := session.Run(r.Context(), req, writeEvent); err != nil {
		_ = writeEvent(TurnEvent{Type: "error", Message: err.Error()})
	}
}
```

Add `"errors"` to `server.go`'s imports.

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/gui/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/gui/types.go pkg/gui/server.go pkg/gui/turn_test.go pkg/gui/server_test.go
git commit -m "feat(gui): play a turn from the desktop app"
```

---

## Phase 4: The Console

### Task 8: Read the stream in the client

**Files:**
- Modify: `frontend/src/types.ts` (`TurnEvent`)
- Modify: `frontend/src/api/client.ts` (`streamTurn`)
- Test: `cd frontend && npx tsc --noEmit`

**Interfaces:**
- Consumes: the NDJSON body from Task 7
- Produces: `APIClient.streamTurn(gameID, body, onEvent, signal): Promise<void>`

- [x] **Step 1: Implement**

In `frontend/src/types.ts`:

```typescript
export interface TurnEvent {
  type: 'chunk' | 'turn' | 'error';
  text?: string;
  turn?: Turn;
  message?: string;
}
```

In `frontend/src/api/client.ts`:

```typescript
  // streamTurn posts a player action and reports each NDJSON line as it arrives.
  // It must not assume the body arrives progressively: a client that buffers the
  // response produces the same events in the same order.
  static async streamTurn(
    gameID: string,
    body: { mode: string; input: string },
    onEvent: (event: TurnEvent) => void,
    signal?: AbortSignal
  ): Promise<void> {
    const res = await fetch(`/api/game/${gameID}/turn`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
      signal,
    });

    if (!res.ok) {
      throw new Error(`streamTurn: ${res.status} ${await res.text()}`);
    }
    if (!res.body) {
      throw new Error('streamTurn: response has no body');
    }

    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';

    for (;;) {
      const { value, done } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });

      let newline = buffer.indexOf('\n');
      while (newline !== -1) {
        const line = buffer.slice(0, newline).trim();
        buffer = buffer.slice(newline + 1);
        if (line) onEvent(JSON.parse(line) as TurnEvent);
        newline = buffer.indexOf('\n');
      }
    }

    const tail = buffer.trim();
    if (tail) onEvent(JSON.parse(tail) as TurnEvent);
  }
```

Add `TurnEvent` to the file's type imports.

- [x] **Step 2: Verify**

Run: `cd frontend && npx tsc --noEmit`
Expected: PASS

- [x] **Step 3: Commit**

```bash
git add frontend/src/types.ts frontend/src/api/client.ts
git commit -m "feat(frontend): read a streamed turn"
```

### Task 9: Make the console submit

**Files:**
- Modify: `frontend/src/components/ActionConsole.tsx`
- Modify: `frontend/src/App.tsx` (`handleActionSubmit`, transient prose, Stop)
- Test: `cd frontend && npx tsc --noEmit && npm run build`

**Interfaces:**
- Consumes: `APIClient.streamTurn`, `TurnSegments`
- Produces: `ActionConsole` props `streaming?: boolean` and `onStop?: () => void`

- [x] **Step 1: Implement the console controls**

`ActionConsole`'s existing `onSubmit(mode, text)` contract is unchanged; it gains the streaming affordances:

```tsx
interface ActionConsoleProps {
  onSubmit: (mode: string, text: string) => void;
  disabled?: boolean;
  streaming?: boolean;
  onStop?: () => void;
}
```

The mode tabs and the input take `disabled={disabled || streaming}`, and the submit button is replaced by a Stop control while streaming:

```tsx
        {streaming ? (
          <button
            type="button"
            onClick={onStop}
            className="flex items-center gap-1.5 px-3.5 py-2.5 rounded-xl bg-stone-800 hover:bg-stone-700 text-stone-200 text-sm font-cinzel cursor-pointer"
          >
            <Square className="w-4 h-4" />
            <span>STOP</span>
          </button>
        ) : (
          // Keep the existing submit button markup and classes unchanged, so the
          // Stop control is the only visual addition.
          <button type="submit" disabled={disabled}>
            <Send className="w-4 h-4" />
          </button>
        )}
```

`Square` comes from `lucide-react` alongside the icons already imported.

- [x] **Step 2: Replace the stub handler in `App.tsx`**

`handleActionSubmit` currently appends an optimistic turn reading "The storyteller ponders your directive..." and does nothing else. Replace it with a streaming submit:

```tsx
  const [turnInFlight, setTurnInFlight] = useState(false);
  const [streamedProse, setStreamedProse] = useState('');
  const abortRef = useRef<AbortController | null>(null);

  const handleActionSubmit = async (mode: string, text: string) => {
    if (!client || !activeGameID || turnInFlight) return;

    setTurnInFlight(true);
    setStreamedProse('');

    const controller = new AbortController();
    abortRef.current = controller;

    try {
      await APIClient.streamTurn(
        activeGameID,
        { mode, input: text },
        (event) => {
          if (event.type === 'chunk') {
            setStreamedProse((prev) => prev + (event.text ?? ''));
          } else if (event.type === 'turn' && event.turn) {
            const turn = event.turn;
            setChronicle((prev) => [...prev, turn]);
            setStreamedProse('');
          } else if (event.type === 'error') {
            console.error('turn failed:', event.message);
          }
        },
        controller.signal
      );
    } catch (err) {
      console.error('turn failed:', err);
    } finally {
      abortRef.current = null;
      setTurnInFlight(false);
    }
  };

  const handleStopTurn = () => {
    abortRef.current?.abort();
    setStreamedProse('');
  };
```

Render the transient prose above the console so the narrator's words appear as they are written, using the existing segment renderer with a single narration segment:

```tsx
        {streamedProse && (
          <TurnSegments
            segments={[{ kind: 'narration', text: streamedProse }]}
            fallback={streamedProse}
            onEntityClick={onWikilinkClick}
          />
        )}
```

and pass the streaming props to the console:

```tsx
              <ActionConsole
                onSubmit={handleActionSubmit}
                streaming={turnInFlight}
                onStop={handleStopTurn}
              />
```

`onEntityClick` is the same handler `App` already passes to the chronicle as `onWikilinkClick`; reuse that binding rather than inventing a second one. Add `useRef` to the React import.

- [x] **Step 3: Verify the type check and the build**

Run: `cd frontend && npx tsc --noEmit && npm run build`
Expected: PASS

- [x] **Step 4: Commit**

```bash
git add frontend/src/App.tsx frontend/src/components/ActionConsole.tsx
git commit -m "feat(frontend): play turns from the action console"
```

---

## Phase 5: Documentation

### Task 10: Record the endpoint and the manual check

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/specs/2026-09-21-desktop-turn-submission-design.md` (the confirmed webview behaviour)

**Interfaces:**
- Consumes: the behaviour observed in Task 9's manual check
- Produces: documentation only

- [x] **Step 1: Update `AGENTS.md`**

Add to the GUI paragraph: the desktop app plays turns through `POST /api/game/{id}/turn`, which streams newline-delimited JSON; turns are serialised per campaign by `Service.BeginTurn` (409 while one is in flight); nothing is persisted for a cancelled or disconnected turn; and provider setup lives in `pkg/harness.RouterFromConfig`/`ExtractorFromConfig` rather than in `package main`. Replace the gotcha that says the GUI has no turn-submission endpoint.

- [x] **Step 2: Run the manual desktop check**

With a local provider configured, run `bin/localrpg gui`, open a campaign, type an action, and record two observations in the spec's §12: whether the prose appears progressively or in one burst, and that a new turn lands in the chronicle with its audio controls. Then press Stop mid-generation and confirm no turn was recorded.

- [x] **Step 3: Verify nothing else regressed**

Run: `go vet ./... && go test -count=1 ./... && (cd frontend && npx tsc --noEmit)`
Expected: PASS

- [x] **Step 4: Commit**

```bash
git add AGENTS.md docs/superpowers/specs/2026-09-21-desktop-turn-submission-design.md
git commit -m "docs: record how the desktop app plays a turn"
```

---

## Spec Coverage

| Spec section | Tasks |
| --- | --- |
| §3 the endpoint, framing, status codes, mode validation | 7 |
| §4 streaming the turn pipeline, cancellation, `ProcessAction` as a wrapper | 1, 2 |
| §5 shared provider setup | 3, 4 |
| §6 `RunTurn`, serialisation, per-request construction, shared `turnDTO` | 5, 6 |
| §7 the client, console wiring, Stop, error handling | 8, 9 |
| §8 file map | all |
| §9 compatibility and edge cases | 2, 7 |
| §10 verification plan | every task's Step 1, plus 10's manual check |
| §12 the webview fact, confirmed rather than assumed | 10 |

### Deviations

- **The chunk callback returns an error** (`onChunk func(text string) error`), amended in the spec before implementation. A callback that cannot fail leaves a disconnected client with a model running to completion into nothing; the error return lets the orchestrator abort generation immediately.
- **`503` means "the campaign cannot be prepared"**, not "no gm provider is configured", also amended in the spec. `RouterFromConfig` always registers an echo fallback for `gm`, so a missing provider cannot happen at startup; a `disabled` provider is a turn-time failure and therefore streams an `error` event.
- **`BeginTurn` returns a prepared `TurnSession` rather than a bare release function.** The preflight has to do the preparation anyway to know whether the campaign is playable, and returning the session means that work happens once instead of twice.
- **The route case is shared with the export work.** `POST /api/game/{id}/turn` and the segment-audio route both hang off a turn, so `case "turn"` branches on the method and the path length; the export plan's command indices are applied after the `gameID` re-slice.

### Execution notes

- Phase 1 alone changes the TUI's behaviour in one observable way: generation now goes through `StreamForRole` rather than `GenerateForRole`, so a provider whose `Stream` misbehaves now affects the TUI too. All four shipped providers implement `Stream`, and `Router.StreamForRole` already falls back when the first chunk fails, so this is the intended consolidation rather than a new risk.
- Tasks 5 and 7 need the GUI test fixtures the earlier plans extend (`setupTestGame` with a location, media config, and a player note). Task 6's `turnFixture` builds its own playable campaign instead, so it does not depend on those fixtures.
- The manual desktop check in Task 10 is the only step that cannot be automated here; it requires a display and a configured provider.
