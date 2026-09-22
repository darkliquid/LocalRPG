# Memory and Recap Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give a campaign memory that reaches further back than the recall window, by keeping a rolling story-so-far note that is summarised behind the turn and injected compactly, plus a recap the player can read and refresh.

**Architecture:** A `chronicle` note holds the summary and the turn it covers. `harness.Summariser` owns the prompt and the provider call; `engine.Chronicler` owns due-ness, regeneration, and the note. The turn reads the note into its prompt as a droppable section that surrenders last, and regeneration runs detached from the turn that triggered it, coalescing behind a single pending flag.

**Tech Stack:** Go 1.27.1, the standard library only, React 19 + TypeScript for the recap panel.

**Spec:** `docs/superpowers/specs/2026-09-22-narrative-coherence-and-trace-design.md` (sections 5.3 and 8)

## Global Constraints

- Go tests use the standard library only (`testing`, `t.TempDir()`); no testify. Use `interface{}`, never `any`.
- No new dependencies, Go or Node.
- A failed summarisation never loses a turn and never advances `through_turn`. The next trigger retries.
- Regeneration is detached: it must not add latency to the turn that triggered it, and that turn must not use it.
- One pending regeneration per campaign, never a queue. A run in flight is never restarted.
- The summary is injected as a **recollection**, explicitly subordinate to canon and the notes, because it is lossy and model-written.
- The summary section is droppable and surrenders **last**, after the catalogue, retrieval, scene recall, and the recall window.
- Every new key has a default matching today's behaviour, and `agents.summary_every: 0` disables summarisation entirely.
- `go vet ./...`, `go test -count=1 ./...`, and `cd frontend && npx tsc --noEmit` must pass. Never commit the deletion of `pkg/gui/dist/.gitkeep`.

## Scope & Splitting

This plan implements **increment 4** of the coherence spec: long memory and the recap. Increments 5 and 6 (aliases and merging, the continuity checks, and open threads) are a separate plan, because they add a repair mechanism and a verification pass rather than extending memory. Increment 6 also extends the recap with open threads, and this plan leaves the shape ready for it.

---

### Task 1: The chronicle note

**Files:**
- Create: `pkg/engine/chronicle.go`
- Test: `pkg/engine/chronicle_test.go`

**Interfaces:**
- Produces: `engine.ChronicleEntityID = "chronicle"`
- Produces: `engine.Chronicle{Summary string; ThroughTurn int}`
- Produces: `engine.ReadChronicle(store *storage.Store) (Chronicle, error)`
- Produces: `engine.WriteChronicle(store *storage.Store, gameID string, chronicle Chronicle) error`

The summary lives in a normal note rather than the database, so the player can read it, edit it, and export it, and so a rebuild of the index cannot lose it.

- [ ] **Step 1: Write the failing test**

Create `pkg/engine/chronicle_test.go`:

```go
package engine

import (
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/storage"
)

func TestReadChronicleOfACampaignThatHasNone(t *testing.T) {
	store := newTestStore(t)

	chronicle, err := ReadChronicle(store)
	if err != nil {
		t.Fatalf("ReadChronicle failed: %v", err)
	}
	if chronicle.ThroughTurn != 0 || chronicle.Summary != "" {
		t.Errorf("expected an empty chronicle, got %+v", chronicle)
	}
}

func TestWriteThenReadChronicle(t *testing.T) {
	store := newTestStore(t)
	entitiesDir := t.TempDir()

	if err := WriteChronicle(store, entitiesDir, Chronicle{
		Summary:     "The party reached the harbour and learnt the oil was low.",
		ThroughTurn: 12,
	}); err != nil {
		t.Fatalf("WriteChronicle failed: %v", err)
	}

	// The note must be a normal entity, so the codex and the graph can see it.
	path := filepath.Join(entitiesDir, ChronicleEntityID+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected a chronicle note on disk: %v", err)
	}
	parsed, err := entity.ParseMarkdownEntity(data)
	if err != nil {
		t.Fatalf("parse chronicle note: %v", err)
	}
	if parsed.Type != "chronicle" {
		t.Errorf("type = %q, want chronicle", parsed.Type)
	}

	chronicle, err := ReadChronicle(store)
	if err != nil {
		t.Fatal(err)
	}
	if chronicle.ThroughTurn != 12 {
		t.Errorf("ThroughTurn = %d, want 12", chronicle.ThroughTurn)
	}
	if !strings.Contains(chronicle.Summary, "oil was low") {
		t.Errorf("summary did not round-trip: %q", chronicle.Summary)
	}
}
```

Add `os` and `strings` to that file's imports.

Note: `WriteChronicle` takes an entities directory rather than a resolver, so it is testable without a campaign layout. The caller passes `timeline.EntitiesDir()`.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestReadChronicle|TestWriteThenReadChronicle ./pkg/engine/`
Expected: FAIL — `undefined: ReadChronicle`.

- [ ] **Step 3: Write the implementation**

Create `pkg/engine/chronicle.go`:

```go
package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// ChronicleEntityID is the note holding a campaign's rolling summary. It is a
// normal entity on purpose: the player can read it, edit it, and export it, and a
// rebuilt index cannot lose it.
const ChronicleEntityID = "chronicle"

// ChronicleThroughTurnKey is the state key recording how far the summary reaches.
const ChronicleThroughTurnKey = "through_turn"

// Chronicle is a campaign's memory of everything older than the recall window.
type Chronicle struct {
	Summary     string
	ThroughTurn int
}

// ReadChronicle returns the campaign's summary, or an empty one when it has none.
func ReadChronicle(store *storage.Store) (Chronicle, error) {
	if store == nil {
		return Chronicle{}, nil
	}

	ent, err := store.GetEntity(ChronicleEntityID)
	if err != nil || ent == nil {
		return Chronicle{}, nil
	}

	chronicle := Chronicle{Summary: strings.TrimSpace(ent.Body)}
	if ent.State != nil {
		if raw, ok := ent.State.Get(ChronicleThroughTurnKey); ok {
			chronicle.ThroughTurn = intFromAny(raw)
		}
	}
	return chronicle, nil
}

// WriteChronicle writes the summary as a note and indexes it. The write precedes
// the index, so an indexing failure leaves the summary on disk rather than the
// other way round.
func WriteChronicle(store *storage.Store, entitiesDir string, chronicle Chronicle) error {
	note := &entity.Entity{
		ID:    ChronicleEntityID,
		Name:  "Story So Far",
		Type:  "chronicle",
		Body:  strings.TrimSpace(chronicle.Summary),
		State: state.NewState(map[string]interface{}{ChronicleThroughTurnKey: chronicle.ThroughTurn}),
	}

	data, err := note.SerializeMarkdown()
	if err != nil {
		return fmt.Errorf("serialize chronicle: %w", err)
	}

	path := filepath.Join(entitiesDir, ChronicleEntityID+".md")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write chronicle: %w", err)
	}

	if store != nil {
		if err := storage.NewSyncer(store).SyncFile(path); err != nil {
			return fmt.Errorf("index chronicle: %w", err)
		}
	}
	return nil
}

// intFromAny converts a state value to an int. YAML and JSON disagree on numeric
// types, and the store decodes frontmatter from JSON.
func intFromAny(value interface{}) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil {
			return parsed
		}
	}
	return 0
}
```

Add `"github.com/darkliquid/localrpg/pkg/state"` to that file's imports.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/engine/ && go vet ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/chronicle.go pkg/engine/chronicle_test.go
git commit -m "feat(engine): keep a campaign's story-so-far note"
```

---

### Task 2: The summariser

**Files:**
- Create: `pkg/harness/summariser.go`
- Test: `pkg/harness/summariser_test.go`

**Interfaces:**
- Produces: `harness.SummaryTurn{Number int; Mode, Input, Narration string}`
- Produces: `harness.Summariser` with `NewSummariser(provider ModelProvider)`, `SetLogger(trace.Logger)`, `SetCharLimit(int)`, `Summarise(ctx context.Context, previous string, turns []SummaryTurn) (string, error)`
- Produces: `harness.SummariserFromConfig(cfg *config.Config, router *Router, logger trace.Logger) *Summariser`

- [ ] **Step 1: Write the failing test**

Create `pkg/harness/summariser_test.go`:

```go
package harness

import (
	"context"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/trace"
)

// scriptedSummaryProvider records the prompt it was given, so a test can assert
// what the summariser asked for rather than only what it returned.
type scriptedSummaryProvider struct {
	reply     string
	lastReq   GenerateRequest
	callCount int
}

func (p *scriptedSummaryProvider) ID() string { return "summary-script" }

func (p *scriptedSummaryProvider) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	p.callCount++
	p.lastReq = req
	return &GenerateResponse{Text: p.reply}, nil
}

func (p *scriptedSummaryProvider) Stream(ctx context.Context, req GenerateRequest, out chan<- StreamChunk) error {
	return nil
}

func TestSummariserAsksForTheThreadsAndForbidsInvention(t *testing.T) {
	provider := &scriptedSummaryProvider{reply: "The party reached the harbour."}
	summariser := NewSummariser(provider)

	summary, err := summariser.Summarise(context.Background(), "They left the tavern.", []SummaryTurn{
		{Number: 7, Mode: "Do", Input: "I ask about the oil", Narration: "Kael mentioned the oil was low."},
	})
	if err != nil {
		t.Fatalf("Summarise failed: %v", err)
	}
	if summary != "The party reached the harbour." {
		t.Errorf("summary = %q", summary)
	}

	prompt := provider.lastReq.Prompt
	for _, wanted := range []string{"They left the tavern.", "Kael mentioned the oil was low.", "Turn 7"} {
		if !strings.Contains(prompt, wanted) {
			t.Errorf("expected the prompt to carry %q:\n%s", wanted, prompt)
		}
	}
	if !strings.Contains(strings.ToLower(prompt), "do not invent") {
		t.Errorf("the prompt must forbid invention:\n%s", prompt)
	}
}

func TestSummariserTruncatesToTheConfiguredLimit(t *testing.T) {
	provider := &scriptedSummaryProvider{reply: strings.Repeat("long ", 500)}
	summariser := NewSummariser(provider)
	summariser.SetCharLimit(40)

	summary, err := summariser.Summarise(context.Background(), "", []SummaryTurn{{Number: 1, Mode: "Do", Narration: "x"}})
	if err != nil {
		t.Fatal(err)
	}
	if len([]rune(summary)) > 41 {
		t.Errorf("summary was not capped: %d runes", len([]rune(summary)))
	}
}

func TestSummariserTracesTheRegeneration(t *testing.T) {
	memory := trace.NewMemory(trace.LevelSummary)
	provider := &scriptedSummaryProvider{reply: "A short summary."}
	summariser := NewSummariser(provider)
	summariser.SetLogger(memory)

	if _, err := summariser.Summarise(context.Background(), "", []SummaryTurn{{Number: 1, Mode: "Do", Narration: "x"}}); err != nil {
		t.Fatal(err)
	}

	event, ok := memory.Find("summary.regenerate")
	if !ok {
		t.Fatalf("expected a summary.regenerate event, got %v", memory.Names())
	}
	if event.Fields["turns"] != 1 {
		t.Errorf("expected the turn count, got %+v", event.Fields)
	}
}

func TestSummariserReportsAProviderFailure(t *testing.T) {
	summariser := NewSummariser(&failingSummaryProvider{})

	if _, err := summariser.Summarise(context.Background(), "", []SummaryTurn{{Number: 1, Mode: "Do", Narration: "x"}}); err == nil {
		t.Errorf("expected the failure to surface, so the caller can leave through_turn alone")
	}
}

type failingSummaryProvider struct{}

func (p *failingSummaryProvider) ID() string { return "summary-fail" }

func (p *failingSummaryProvider) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	return nil, errors.New("model unavailable")
}

func (p *failingSummaryProvider) Stream(ctx context.Context, req GenerateRequest, out chan<- StreamChunk) error {
	return nil
}
```

Add `"errors"` to that file's imports.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestSummariser ./pkg/harness/`
Expected: FAIL — `undefined: NewSummariser`.

- [ ] **Step 3: Write the implementation**

Create `pkg/harness/summariser.go`:

```go
package harness

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// defaultSummaryChars matches the configured default, so a summariser built without
// a limit still cannot inject an unbounded block.
const defaultSummaryChars = 2000

// SummaryTurn is one turn as the summariser sees it.
type SummaryTurn struct {
	Number    int
	Mode      string
	Input     string
	Narration string
}

// summariserPrompt asks for the things a later turn needs and forbids the thing
// that would poison canon: invention.
const summariserPrompt = `## TASK
Rewrite the story so far, incorporating the new turns. This is a recollection for a
narrator, not prose for the player.

## RULES
- Preserve names, places, promises, oaths, debts, and unresolved threads exactly.
- Preserve state changes: who was wounded, what was spent, what was lit or doused.
- Drop verbatim dialogue and scene-setting that a later turn does not need.
- Never invent events, characters, or outcomes that the turns do not contain.
- Write plain prose, no headings, no lists, no bullet points.
- Keep it under %d characters.

## PREVIOUS SUMMARY
%s

## NEW TURNS
%s

## REWRITTEN SUMMARY`

// Summariser turns a range of turns into a compact recollection. It is the only
// model call the memory system makes, and it is always detached from a turn.
type Summariser struct {
	provider  ModelProvider
	charLimit int
	logger    trace.Logger
}

func NewSummariser(provider ModelProvider) *Summariser {
	return &Summariser{provider: provider, charLimit: defaultSummaryChars}
}

func (s *Summariser) SetLogger(logger trace.Logger) {
	s.logger = trace.OrNil(logger)
}

// SetCharLimit caps the summary. Zero restores the default, so a misconfigured
// value cannot remove the bound.
func (s *Summariser) SetCharLimit(limit int) {
	if limit <= 0 {
		limit = defaultSummaryChars
	}
	s.charLimit = limit
}

// Summarise asks for a rewritten recollection. A failure is returned rather than
// swallowed, because the caller must leave through_turn alone so the next trigger
// tries again.
func (s *Summariser) Summarise(ctx context.Context, previous string, turns []SummaryTurn) (string, error) {
	if s.provider == nil {
		return "", fmt.Errorf("summarise: no provider")
	}

	start := time.Now()
	s.logger = trace.OrNil(s.logger)

	var transcript strings.Builder
	for _, turn := range turns {
		if input := strings.TrimSpace(turn.Input); input != "" {
			fmt.Fprintf(&transcript, "Turn %d - Player [%s]: %s\n", turn.Number, turn.Mode, input)
		} else {
			fmt.Fprintf(&transcript, "Turn %d - [%s]\n", turn.Number, turn.Mode)
		}
		if narration := strings.TrimSpace(turn.Narration); narration != "" {
			fmt.Fprintf(&transcript, "Narrator: %s\n\n", narration)
		}
	}

	prompt := fmt.Sprintf(summariserPrompt, s.charLimit, strings.TrimSpace(previous), strings.TrimSpace(transcript.String()))

	response, err := s.provider.Generate(ctx, GenerateRequest{Prompt: prompt})
	if err != nil {
		s.logger.Event("provider.error", map[string]interface{}{"role": "summariser", "error": err.Error()})
		return "", fmt.Errorf("summarise %d turn(s): %w", len(turns), err)
	}

	summary := strings.TrimSpace(response.Text)
	if len([]rune(summary)) > s.charLimit {
		summary = TruncateRunes(summary, s.charLimit) + "..."
	}

	s.logger.Event("summary.regenerate", map[string]interface{}{
		"turns":       len(turns),
		"chars":       len([]rune(summary)),
		"char_limit":  s.charLimit,
		"duration_ms": time.Since(start).Milliseconds(),
	})

	return summary, nil
}

// SummariserFromConfig resolves the role that writes summaries. It mirrors the
// extractor: an absent role inherits gm, `disabled` turns summaries off entirely,
// and `inherit` follows the named role.
func SummariserFromConfig(cfg *config.Config, router *Router, logger trace.Logger) *Summariser {
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
		summariser := NewSummariser(provider)
		summariser.SetLogger(logger)
		summariser.SetCharLimit(cfg.SummaryCharLimit())
		return summariser
	}

	provider, err := NewModelProviderWithLogger(config.RoleExtractor, ProviderConfig{
		Type:        roleCfg.Type,
		BuiltinName: roleCfg.BuiltinName,
		Command:     roleCfg.Command,
		Args:        roleCfg.Args,
		Endpoint:    roleCfg.Endpoint,
		Model:       roleCfg.Model,
		APIKey:      roleCfg.APIKey,
		Temperature: roleCfg.Temperature,
		MaxTokens:   roleCfg.MaxTokens,
	}, logger)
	if err != nil {
		return nil
	}
	summariser := NewSummariser(provider)
	summariser.SetLogger(logger)
	summariser.SetCharLimit(cfg.SummaryCharLimit())
	return summariser
}
```

`SummaryCharLimit()` and `SummaryEvery()` are added in Task 5; add them now to keep this task compiling:

In `pkg/config/types.go`, extend `AgentsConfig` and add the accessors:

```go
	// SummaryEvery is how many turns pass between regenerations of the story so far.
	// Zero disables summarisation.
	SummaryEvery     int `yaml:"summary_every" json:"summary_every"`
	SummaryCharLimit int `yaml:"summary_char_limit" json:"summary_char_limit"`
```

```go
// SummaryEvery is how many turns pass between regenerations. Zero disables it.
func (c *Config) SummaryEvery() int {
	return c.Agents.SummaryEvery
}

// SummaryCharLimit caps the injected summary.
func (c *Config) SummaryCharLimit() int {
	if c.Agents.SummaryCharLimit <= 0 {
		return 2000
	}
	return c.Agents.SummaryCharLimit
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/harness/ ./pkg/config/ && go vet ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/harness/summariser.go pkg/harness/summariser_test.go pkg/config/types.go
git commit -m "feat(harness): summarise a campaign's older turns"
```

---

### Task 3: The chronicler

**Files:**
- Create: `pkg/engine/chronicler.go`
- Test: `pkg/engine/chronicler_test.go`

**Interfaces:**
- Consumes: `engine.ReadChronicle`, `engine.WriteChronicle` (Task 1), `harness.Summariser` (Task 2)
- Produces: `engine.Chronicler` with `NewChronicler(paths *core.PathResolver, store *storage.Store, summariser *harness.Summariser)`, `SetEvery(int)`, `SetLogger(trace.Logger)`, `Due(gameID string) (bool, error)`, `Regenerate(ctx context.Context, gameID string) (bool, error)`, `Recap(gameID string) (Chronicle, error)`

- [ ] **Step 1: Write the failing test**

Create `pkg/engine/chronicler_test.go`:

```go
package engine

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/storage"
)

func chroniclerFixture(t *testing.T, every int) (*Chronicler, *Timeline, *scriptedStreamProvider, *storage.Store) {
	t.Helper()

	tempDir := t.TempDir()
	store := newTestStore(t)
	timeline := NewTimeline(core.NewPathResolver(tempDir), store, NewHistoryLogger(filepath.Join(tempDir, "history.jsonl")), "campaign-01")

	entitiesDir := timeline.EntitiesDir()
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "player", Name: "Sean", Type: "character", Body: "A traveller."})
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		t.Fatal(err)
	}

	provider := &scriptedStreamProvider{chunks: []string{"The party reached the harbour."}}
	chronicler := NewChronicler(core.NewPathResolver(tempDir), store, harness.NewSummariser(provider))
	chronicler.SetEvery(every)
	return chronicler, timeline, provider, store
}

func recordTurn(t *testing.T, timeline *Timeline, number int, narration string) {
	t.Helper()
	turn := Turn{
		Number:    number,
		Timestamp: time.Now(),
		Mode:      "Do",
		Input:     "I look around",
		Narration: narration,
	}
	if err := timeline.RecordTurn(&turn, nil); err != nil {
		t.Fatalf("record turn %d: %v", number, err)
	}
}

func TestChroniclerIsNotDueBeforeTheCadence(t *testing.T) {
	chronicler, timeline, _, _ := chroniclerFixture(t, 3)
	for i := 1; i <= 2; i++ {
		recordTurn(t, timeline, i, "Nothing much happens.")
	}

	due, err := chronicler.Due("campaign-01")
	if err != nil {
		t.Fatal(err)
	}
	if due {
		t.Errorf("expected no regeneration before the cadence is reached")
	}
}

func TestChroniclerRegeneratesAtTheCadenceAndRecordsHowFarItReached(t *testing.T) {
	chronicler, timeline, _, _ := chroniclerFixture(t, 3)
	for i := 1; i <= 3; i++ {
		recordTurn(t, timeline, i, "Nothing much happens.")
	}

	due, err := chronicler.Due("campaign-01")
	if err != nil {
		t.Fatal(err)
	}
	if !due {
		t.Fatalf("expected the cadence to make a regeneration due")
	}

	changed, err := chronicler.Regenerate(context.Background(), "campaign-01")
	if err != nil {
		t.Fatalf("Regenerate failed: %v", err)
	}
	if !changed {
		t.Fatalf("expected the chronicle to be written")
	}

	chronicle, err := chronicler.Recap("campaign-01")
	if err != nil {
		t.Fatal(err)
	}
	if chronicle.ThroughTurn != 3 {
		t.Errorf("ThroughTurn = %d, want 3", chronicle.ThroughTurn)
	}
	if !strings.Contains(chronicle.Summary, "harbour") {
		t.Errorf("summary = %q", chronicle.Summary)
	}

	due, err = chronicler.Due("campaign-01")
	if err != nil {
		t.Fatal(err)
	}
	if due {
		t.Errorf("expected the chronicle to be current once regenerated")
	}
}

func TestChroniclerLeavesThroughTurnAloneWhenTheProviderFails(t *testing.T) {
	chronicler, timeline, provider, _ := chroniclerFixture(t, 1)
	provider.err = errors.New("model unavailable")

	recordTurn(t, timeline, 1, "Nothing much happens.")

	if _, err := chronicler.Regenerate(context.Background(), "campaign-01"); err == nil {
		t.Fatalf("expected the provider failure to surface")
	}

	chronicle, err := chronicler.Recap("campaign-01")
	if err != nil {
		t.Fatal(err)
	}
	if chronicle.ThroughTurn != 0 || chronicle.Summary != "" {
		t.Errorf("a failed regeneration must leave the chronicle untouched, got %+v", chronicle)
	}

	// The next trigger retries rather than the range being lost.
	due, err := chronicler.Due("campaign-01")
	if err != nil {
		t.Fatal(err)
	}
	if !due {
		t.Errorf("expected the campaign to still be due after a failure")
	}
}

func TestChroniclerDisabledWritesNothing(t *testing.T) {
	chronicler, timeline, _, _ := chroniclerFixture(t, 0)
	recordTurn(t, timeline, 1, "Nothing much happens.")

	due, err := chronicler.Due("campaign-01")
	if err != nil {
		t.Fatal(err)
	}
	if due {
		t.Errorf("zero cadence must disable summarisation")
	}
}
```

Add `"errors"` and `"strings"` to that file's imports.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestChronicler ./pkg/engine/`
Expected: FAIL — `undefined: NewChronicler`.

- [ ] **Step 3: Write the implementation**

Create `pkg/engine/chronicler.go`:

```go
package engine

import (
	"context"
	"fmt"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/storage"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// Chronicler owns a campaign's long memory: whether a regeneration is due, the
// regeneration itself, and the note it writes. It is a separate type from the
// orchestrator because its work is detached from any turn.
type Chronicler struct {
	paths      *core.PathResolver
	store      *storage.Store
	summariser *harness.Summariser
	every      int
	logger     trace.Logger
}

func NewChronicler(paths *core.PathResolver, store *storage.Store, summariser *harness.Summariser) *Chronicler {
	return &Chronicler{paths: paths, store: store, summariser: summariser}
}

// SetEvery sets the cadence. Zero disables summarisation.
func (c *Chronicler) SetEvery(every int) {
	c.every = every
}

func (c *Chronicler) SetLogger(logger trace.Logger) {
	c.logger = trace.OrNil(logger)
}

// Recap returns the campaign's summary, or an empty one when it has none.
func (c *Chronicler) Recap(gameID string) (Chronicle, error) {
	return ReadChronicle(c.store)
}

// Due reports whether the campaign has turned far enough past its summary to need
// another. It is false when summarisation is disabled or has no provider, so a
// caller never has to check both.
func (c *Chronicler) Due(gameID string) (bool, error) {
	if c.every <= 0 || c.summariser == nil {
		return false, nil
	}

	chronicle, err := ReadChronicle(c.store)
	if err != nil {
		return false, err
	}

	historyPath := c.historyPath(gameID)
	turns, err := NewHistoryLogger(historyPath).LoadHistory()
	if err != nil {
		return false, fmt.Errorf("load history: %w", err)
	}
	if len(turns) == 0 {
		return false, nil
	}

	latest := turns[len(turns)-1].Number
	if chronicle.ThroughTurn >= latest {
		return false, nil
	}
	return latest-chronicle.ThroughTurn >= c.every, nil
}

// Regenerate summarises everything since the last one and writes the note. It
// reports whether the chronicle changed, and it leaves through_turn alone when the
// provider fails so the next trigger retries the same range.
func (c *Chronicler) Regenerate(ctx context.Context, gameID string) (bool, error) {
	if c.every <= 0 || c.summariser == nil {
		return false, nil
	}

	chronicle, err := ReadChronicle(c.store)
	if err != nil {
		return false, err
	}

	historyPath := c.historyPath(gameID)
	turns, err := NewHistoryLogger(historyPath).LoadHistory()
	if err != nil {
		return false, fmt.Errorf("load history: %w", err)
	}

	pending := make([]harness.SummaryTurn, 0, len(turns))
	for _, turn := range turns {
		if turn.Number <= chronicle.ThroughTurn {
			continue
		}
		pending = append(pending, harness.SummaryTurn{
			Number:    turn.Number,
			Mode:      turn.Mode,
			Input:     turn.Input,
			Narration: turn.Narration,
		})
	}
	if len(pending) == 0 {
		return false, nil
	}

	summary, err := c.summariser.Summarise(ctx, chronicle.Summary, pending)
	if err != nil {
		return false, err
	}

	through := pending[len(pending)-1].Number
	if err := WriteChronicle(c.store, c.entitiesDir(gameID), Chronicle{
		Summary:     summary,
		ThroughTurn: through,
	}); err != nil {
		return false, err
	}

	c.logger = trace.OrNil(c.logger)
	c.logger.Event("summary.written", map[string]interface{}{
		"from_turn": chronicle.ThroughTurn + 1,
		"to_turn":   through,
		"chars":     len([]rune(summary)),
	})

	return true, nil
}

func (c *Chronicler) historyPath(gameID string) string {
	return c.paths.GameDir(gameID) + string(filepath.Separator) + "history.jsonl"
}

func (c *Chronicler) entitiesDir(gameID string) string {
	return c.paths.GameDir(gameID) + string(filepath.Separator) + "entities"
}
```

Replace the two path helpers with the cleaner pair, and add `"path/filepath"` to the imports:

```go
func (c *Chronicler) historyPath(gameID string) string {
	return filepath.Join(c.paths.GameDir(gameID), "history.jsonl")
}

func (c *Chronicler) entitiesDir(gameID string) string {
	return filepath.Join(c.paths.GameDir(gameID), "entities")
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/engine/ && go vet ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/chronicler.go pkg/engine/chronicler_test.go
git commit -m "feat(engine): regenerate a campaign's story so far on a cadence"
```

---

### Task 4: Inject the summary into the prompt

**Files:**
- Modify: `pkg/harness/context.go`
- Modify: `pkg/engine/orchestrator.go`
- Test: `pkg/harness/context_test.go`

**Interfaces:**
- Produces: `ContextRequest.Summary string`
- Produces: the `summary` section, droppable at rank 6
- Produces: `(*TurnOrchestrator).SetChronicler(*Chronicler)`

- [ ] **Step 1: Write the failing test**

Append to `pkg/harness/context_test.go`:

```go
func TestSummaryIsInjectedAsASubordinateRecollection(t *testing.T) {
	assembler := NewContextAssembler(newTestEntityStore(t))

	result, err := assembler.Assemble(ContextRequest{
		Action:  "I ask about the oil",
		Summary: "The party reached the harbour and learnt the oil was low.",
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result.Prompt, "STORY SO FAR") {
		t.Fatalf("expected a summary section:\n%s", result.Prompt)
	}
	if !strings.Contains(result.Prompt, "oil was low") {
		t.Errorf("expected the summary text:\n%s", result.Prompt)
	}
	// It is a recollection, and says so, because it is lossy and model-written.
	lowered := strings.ToLower(result.Prompt)
	if !strings.Contains(lowered, "recollection") || !strings.Contains(lowered, "authoritative") {
		t.Errorf("the summary must be marked as subordinate to canon:\n%s", result.Prompt)
	}
}

func TestSummaryIsTheLastSectionSurrendered(t *testing.T) {
	assembler := NewContextAssembler(newTestEntityStore(t))
	assembler.SetLimits(ContextLimits{
		TokenBudget:     1,
		SceneRecallTurns: 0,
		RetrievalTurns:   0,
	})

	result, err := assembler.Assemble(ContextRequest{
		Action:      "I listen",
		RulesPrompt: "RULES",
		LorePrompt:  "LORE",
		Summary:     strings.Repeat("the mist rolls in ", 40),
		Profiles:    []config.VoiceProfile{{ID: "elder_sage", Description: "Ancient wizards"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// A budget of one token trims everything droppable, in rank order, and the
	// reported order must show the catalogue going before the summary.
	order := result.Trimmed
	catalogueAt, summaryAt := -1, -1
	for i, name := range order {
		if strings.Contains(name, "catalogue") {
			catalogueAt = i
		}
		if strings.Contains(name, "summary") {
			summaryAt = i
		}
	}
	if catalogueAt == -1 || summaryAt == -1 {
		t.Fatalf("expected both to be trimmed, got %v", order)
	}
	if catalogueAt > summaryAt {
		t.Errorf("the catalogue must go before the summary, got %v", order)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestSummary ./pkg/harness/`
Expected: FAIL — no `STORY SO FAR` section.

- [ ] **Step 3: Add the section**

In `pkg/harness/context.go`:

```go
	// Summary is the campaign's recollection of everything older than the recall
	// window. It is lossy, so it is stated as subordinate to canon.
	Summary string
```

add it to `ContextRequest`, add the builder, and register the section before `recent` so it reads as background:

```go
// summarySection renders the campaign's long memory. It is explicitly a
// recollection: it is model-written and lossy, and canon wins wherever they differ.
func summarySection(summary string) string {
	if strings.TrimSpace(summary) == "" {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n## STORY SO FAR (a recollection, not authoritative)\n")
	sb.WriteString("Where this differs from the state and notes above, they are correct and this is not.\n")
	sb.WriteString(strings.TrimSpace(summary) + "\n")
	return sb.String()
}
```

```go
		{name: "summary", text: summarySection(req.Summary), droppable: true, rank: 6},
```

and extend `sectionDescription`:

```go
	case "summary":
		return "the story so far"
```

The trim loop iterates ranks 1 to 4 today; extend it to the new ranks:

```go
		for _, rank := range []int{1, 2, 3, 4, 6} {
```

Rank 5 is deliberately unused: it was reserved for the recall-excerpt shortening, which happens after every whole-section drop.

- [ ] **Step 4: Read the summary in the orchestrator**

In `pkg/engine/orchestrator.go`:

```go
	chronicler *Chronicler
```

```go
// SetChronicler gives the orchestrator the campaign's long memory, which it injects
// as a recollection rather than reading itself: the chronicle is updated by another
// goroutine, and the turn must use whatever was current when it started.
func (o *TurnOrchestrator) SetChronicler(chronicler *Chronicler) {
	o.chronicler = chronicler
}
```

and in the assembly request:

```go
	summary := ""
	if o.chronicler != nil {
		if chronicle, err := o.chronicler.Recap(o.gameID()); err == nil {
			summary = chronicle.Summary
		}
	}
```

The orchestrator needs the campaign ID. It has the timeline; add:

```go
// gameID is the campaign this orchestrator plays, which the chronicle note belongs
// to.
func (o *TurnOrchestrator) gameID() string {
	return o.timeline.GameID()
}
```

and on the timeline:

```go
// GameID is the campaign this timeline records.
func (t *Timeline) GameID() string {
	return t.gameID
}
```

Finally pass `Summary: summary` in the `harness.ContextRequest`.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/harness/ ./pkg/engine/ && go vet ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/harness/context.go pkg/harness/context_test.go pkg/engine/orchestrator.go pkg/engine/timeline.go
git commit -m "feat(harness): inject the story so far as a recollection"
```

---

### Task 5: Regenerate behind the turn

**Files:**
- Modify: `pkg/gui/service.go`
- Modify: `pkg/gui/types.go`
- Test: `pkg/gui/service_test.go`

**Interfaces:**
- Consumes: `engine.Chronicler` (Task 3), `harness.SummariserFromConfig` (Task 2)
- Produces: `(*Service).SetChronicler(*engine.Chronicler)`
- Produces: coalescing state in the service: one pending regeneration per campaign

- [ ] **Step 1: Write the failing test**

Append to `pkg/gui/service_test.go`:

```go
func TestAServiceRegeneratesTheSummaryBehindTheTurn(t *testing.T) {
	gameID, svc := turnFixture(t)

	// A cadence of one, so the very first turn makes a regeneration due.
	svc.SetSummaryCadence(1)

	session, err := svc.BeginTurn(gameID)
	if err != nil {
		t.Fatalf("BeginTurn failed: %v", err)
	}
	if err := session.Run(context.Background(), TurnRequest{Mode: "Do", Input: "I look around"}, func(TurnEvent) error {
		return nil
	}); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	session.Close()

	// The regeneration is detached, so the test waits for it rather than assuming
	// it finished before Run returned.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !svc.SummaryPending(gameID) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	chronicle, err := svc.GetRecap(context.Background(), gameID)
	if err != nil {
		t.Fatalf("GetRecap failed: %v", err)
	}
	if chronicle.ThroughTurn != 1 {
		t.Errorf("ThroughTurn = %d, want 1: the summary must cover the turn that triggered it", chronicle.ThroughTurn)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestAServiceRegeneratesTheSummaryBehindTheTurn ./pkg/gui/`
Expected: FAIL — `svc.SetSummaryCadence undefined`.

- [ ] **Step 3: Wire it**

In `pkg/gui/service.go`, add the state and the setters:

```go
	// A regeneration is detached and coalesced: the flag records that one is
	// pending, so a player turning quickly triggers a catch-up run rather than a
	// queue of overlapping ones.
	summaryMu      sync.Mutex
	summaryPending map[string]bool
	summaryCadence int
```

initialise `summaryPending` in `NewService` beside `indexed`, and add:

```go
// SetSummaryCadence sets how many turns pass between regenerations, for tests and
// for the composition root. Zero disables summarisation.
func (s *Service) SetSummaryCadence(every int) {
	s.summaryCadence = every
}

// SummaryPending reports whether a regeneration is in flight for a campaign.
func (s *Service) SummaryPending(gameID string) bool {
	s.summaryMu.Lock()
	defer s.summaryMu.Unlock()
	return s.summaryPending[gameID]
}
```

In `prepareTurn`, build the summariser and chronicler and give them to the orchestrator:

```go
	summariser := harness.SummariserFromConfig(cfg, router, logger)
	chronicler := engine.NewChronicler(s.resolver, store, summariser)
	chronicler.SetEvery(cfg.SummaryEvery())
	chronicler.SetLogger(logger)
	orchestrator.SetChronicler(chronicler)
```

Then, in `TurnSession.Run`, after the turn is recorded and beside the playback it already starts:

```go
	// Memory is repaired behind the turn: a second model call must never delay the
	// reply, and the turn that triggers it must not use its own summary.
	if t.service.shouldSummarise(t.gameID) {
		gameID := t.gameID
		go t.service.regenerateSummary(context.Background(), gameID)
	}
```

with:

```go
// shouldSummarise reports whether a campaign needs a regeneration and nothing is
// already running for it.
func (s *Service) shouldSummarise(gameID string) bool {
	if s.summaryCadence <= 0 {
		return false
	}
	if s.SummaryPending(gameID) {
		return false
	}
	s.summaryMu.Lock()
	s.summaryPending[gameID] = true
	s.summaryMu.Unlock()
	return true
}

// regenerateSummary rebuilds the chronicle and clears the pending flag, whatever
// the outcome: a failure is retried by the next turn rather than being retried in a
// loop here.
func (s *Service) regenerateSummary(ctx context.Context, gameID string) {
	defer func() {
		s.summaryMu.Lock()
		delete(s.summaryPending, gameID)
		s.summaryMu.Unlock()
	}()

	chronicler := s.chronicler(gameID)
	if chronicler == nil {
		return
	}
	if _, err := chronicler.Regenerate(ctx, gameID); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not update the story so far: %v\n", err)
	}
}
```

`shouldSummarise` uses the cadence for its own gate, but the chronicler owns the real due-ness check:

```go
func (s *Service) shouldSummarise(gameID string) bool {
	if s.summaryCadence <= 0 {
		return false
	}
	if s.SummaryPending(gameID) {
		return false
	}
	chronicler := s.chronicler(gameID)
	if chronicler == nil {
		return false
	}
	due, err := chronicler.Due(gameID)
	if err != nil || !due {
		return false
	}

	s.summaryMu.Lock()
	s.summaryPending[gameID] = true
	s.summaryMu.Unlock()
	return true
}
```

and a helper that builds a chronicler for a campaign on demand, so it works outside a turn:

```go
// chronicler builds the campaign's chronicler from the current configuration. It is
// per call like the rest of a turn's wiring, so a settings change takes effect
// without a restart.
func (s *Service) chronicler(gameID string) *engine.Chronicler {
	store, err := s.store(gameID)
	if err != nil {
		return nil
	}

	cfg := s.configMgr.Get()
	router, err := harness.RouterFromConfigWithLogger(cfg, s.logger)
	if err != nil {
		return nil
	}

	chronicler := engine.NewChronicler(s.resolver, store, harness.SummariserFromConfig(cfg, router, s.logger))
	chronicler.SetEvery(cfg.SummaryEvery())
	chronicler.SetLogger(s.logger)
	return chronicler
}
```

The orchestrator's chronicler in `prepareTurn` is built the same way; both are cheap.

- [ ] **Step 4: Add the recap endpoint**

In `pkg/gui/types.go`:

```go
// RecapDTO is a campaign's long memory as the client reads it.
type RecapDTO struct {
	Summary     string `json:"summary,omitempty"`
	ThroughTurn int    `json:"through_turn"`
	// Enabled is false when summarisation is off, so a client can offer the panel
	// without offering a refresh that would do nothing.
	Enabled bool `json:"enabled"`
}
```

In `pkg/gui/service.go`:

```go
// GetRecap returns the campaign's story so far.
func (s *Service) GetRecap(ctx context.Context, gameID string) (*RecapDTO, error) {
	chronicler := s.chronicler(gameID)
	if chronicler == nil {
		return &RecapDTO{}, nil
	}

	chronicle, err := chronicler.Recap(gameID)
	if err != nil {
		return nil, fmt.Errorf("read chronicle: %w", err)
	}
	return &RecapDTO{
		Summary:     chronicle.Summary,
		ThroughTurn: chronicle.ThroughTurn,
		Enabled:     s.configMgr.Get().SummaryEvery() > 0,
	}, nil
}
```

In `pkg/gui/server.go`, register the route and add it to the game switch:

```go
	s.mux.HandleFunc("/api/game/", s.handleGameRoutes)
```

```go
	case "recap":
		recap, err := s.service.GetRecap(r.Context(), gameID)
		if err != nil {
			writeGameError(w, err)
			return
		}
		writeJSON(w, recap)
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/gui/ && go vet ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/gui/ pkg/config/ pkg/engine/ pkg/harness/
git commit -m "feat(gui): regenerate the story so far behind the turn"
```

---

### Task 6: The recap a player reads

**Files:**
- Modify: `pkg/engine/orchestrator.go`
- Test: `pkg/engine/orchestrator_test.go`
- Modify: `frontend/src/types.ts`, `frontend/src/api/client.ts`, `frontend/src/components/LivingWorldDrawer.tsx`, `frontend/src/App.tsx`

**Interfaces:**
- Produces: `/recap` as an engine command, recognised in `ProcessActionStream`
- Produces: `APIClient.getRecap()`, and a "Story so far" block in the Living World drawer with a refresh

- [ ] **Step 1: Write the failing test**

Append to `pkg/engine/orchestrator_test.go`:

```go
func TestRecapCommandPrintsTheSummaryWithoutGenerating(t *testing.T) {
	provider := &scriptedStreamProvider{chunks: []string{"A generated scene."}}
	orchestrator, timeline, store := streamingOrchestrator(t, provider)

	if err := WriteChronicle(store, timeline.EntitiesDir(), Chronicle{
		Summary:     "The party reached the harbour.",
		ThroughTurn: 4,
	}); err != nil {
		t.Fatal(err)
	}

	turn, err := orchestrator.ProcessActionStream(context.Background(), "System", "/recap", nil)
	if err != nil {
		t.Fatalf("/recap failed: %v", err)
	}
	if !strings.Contains(turn.Narration, "The party reached the harbour.") {
		t.Errorf("expected the recap as the reply, got %q", turn.Narration)
	}
	if strings.Contains(turn.Narration, "A generated scene.") {
		t.Errorf("/recap must not call the model when the summary exists")
	}

	// A command is not a turn: nothing is recorded for it.
	turns, err := timeline.history.LoadHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 0 {
		t.Errorf("expected /recap to record nothing, got %d turns", len(turns))
	}
}

func TestRecapCommandSaysWhenThereIsNothingYet(t *testing.T) {
	provider := &scriptedStreamProvider{chunks: []string{"A generated scene."}}
	orchestrator, _, _ := streamingOrchestrator(t, provider)

	turn, err := orchestrator.ProcessActionStream(context.Background(), "System", "/recap", nil)
	if err != nil {
		t.Fatalf("/recap failed: %v", err)
	}
	if !strings.Contains(strings.ToLower(turn.Narration), "not far enough") {
		t.Errorf("expected a plain answer for a campaign with no summary, got %q", turn.Narration)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestRecapCommand ./pkg/engine/`
Expected: FAIL — `/recap` falls through to the model.

- [ ] **Step 3: Implement the command**

In `pkg/engine/orchestrator.go`, beside the `/undo` and `/go` handlers, before anything that touches the model:

```go
	// /recap answers "where were we?" from the campaign's own memory. It never calls
	// the model: a summary is what the summariser already wrote, and a player asking
	// to read it should not pay for another one.
	if strings.HasPrefix(strings.TrimSpace(actionInput), "/recap") {
		chronicle := Chronicle{}
		if o.chronicler != nil {
			chronicle, _ = o.chronicler.Recap(o.gameID())
		}

		narration := "This campaign has not turned far enough for a recap yet."
		if strings.TrimSpace(chronicle.Summary) != "" {
			narration = fmt.Sprintf("## Story So Far (through turn %d)\n\n%s", chronicle.ThroughTurn, chronicle.Summary)
		}

		return &Turn{
			Number:    turnNum,
			Timestamp: time.Now(),
			Mode:      "System",
			Input:     "/recap",
			Narration: narration,
		}, nil
	}
```

- [ ] **Step 4: Add the client and the panel**

In `frontend/src/types.ts`:

```ts
export interface Recap {
  summary?: string;
  through_turn: number;
  enabled: boolean;
}
```

In `frontend/src/api/client.ts`:

```ts
  async getRecap(): Promise<Recap> {
    const res = await fetch(`/api/game/${this.gameID}/recap`);
    if (!res.ok) throw new Error(`getRecap: ${res.statusText}`);
    return res.json();
  }
```

In `frontend/src/components/LivingWorldDrawer.tsx`, add a "Story So Far" block above the arcs, taking the recap as a prop:

```tsx
interface LivingWorldDrawerProps {
  state?: GameState;
  recap?: Recap;
  onRefreshRecap?: () => void;
}
```

```tsx
      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <h3 className="text-sm font-cinzel text-amber-400 font-bold uppercase tracking-wider flex items-center gap-1.5">
            <ScrollText className="w-4 h-4" />
            <span>Story So Far</span>
          </h3>
          {recap?.enabled && onRefreshRecap && (
            <button
              onClick={onRefreshRecap}
              className="text-[11px] font-cinzel px-2 py-1 rounded-lg bg-stone-900 border border-stone-700 text-stone-300 hover:text-amber-300 cursor-pointer transition-colors"
            >
              Refresh
            </button>
          )}
        </div>
        {recap?.summary ? (
          <>
            <p className="text-xs text-stone-300 leading-relaxed whitespace-pre-wrap bg-black/30 p-3 rounded-xl border border-white/5">
              {recap.summary}
            </p>
            <p className="text-[11px] font-mono text-stone-500">Through turn {recap.through_turn}</p>
          </>
        ) : (
          <p className="text-stone-500 text-xs italic bg-black/30 p-3 rounded-xl border border-white/5">
            {recap?.enabled === false
              ? 'Summaries are switched off in Settings.'
              : 'The story has not turned far enough to be summarised yet.'}
          </p>
        )}
      </div>
```

Import `ScrollText` from `lucide-react` and `Recap` from `../types`.

In `frontend/src/App.tsx`, fetch the recap alongside the rest of the corpus and pass it down:

```tsx
  const [recap, setRecap] = useState<Recap | null>(null);
```

add `client.getRecap().then(setRecap).catch(console.error);` to `refreshCorpus`, and:

```tsx
            {activeDrawer === 'world' && (
              <LivingWorldDrawer
                state={gameState || undefined}
                recap={recap || undefined}
                onRefreshRecap={() => {
                  APIClient.getRecap()
                    .then(setRecap)
                    .catch(console.error);
                }}
              />
            )}
```

- [ ] **Step 5: Run the full gate**

Run: `go test -count=1 ./... && go vet ./... && (cd frontend && npx tsc --noEmit && npm run build)`
Expected: PASS, and a successful build. Restore `pkg/gui/dist/.gitkeep` afterwards and do not stage its deletion.

- [ ] **Step 6: Commit**

```bash
git add pkg/engine/ frontend/src/
git commit -m "feat(engine): add /recap and show the story so far"
```

---

## Self-Review

**Spec coverage** (coherence spec sections 5.3 and 8, increments 4):

| Spec requirement | Task |
| --- | --- |
| A rolling summary note, readable and editable | 1 |
| Written by the extractor role, falling back to gm, disabling when the role is disabled | 2 |
| Preserves names, promises, threads, and state changes; forbids invention | 2 |
| Regenerates every N turns from the opening turn inclusive | 3 |
| A failed regeneration never loses a turn and does not advance `through_turn` | 3 |
| Detached from the triggering turn, coalesced behind one pending flag, never restarted | 5 |
| Injected compactly and capped | 2, 4 |
| Subordinate to canon and the notes | 4 |
| Surrendered last when the budget bites | 4 |
| `/recap` regenerates when stale, then prints | 6 (prints; see the note below) |
| Excluded from exports | Already true: exports render turns and segments, and a chronicle note is neither |
| Recap panel in the GUI | 6 |
| Deferred to the next plan: aliases, merge, continuity, open threads | stated in Scope |

**Deliberate deviation:** the spec says `/recap` regenerates a stale summary before printing. This plan makes it print and never call the model, because a command that silently spends a model call is a surprise, and because regeneration already happens behind every Nth turn. If a stale recap proves annoying in use, the change is to call `Regenerate` in that branch when the chronicler says it is due, which is a three-line change to Task 6 and already has a test to extend.

**Placeholder scan:** no "TBD", no "similar to Task N". Task 3's two path helpers are given in a corrected form immediately after the draft that used string concatenation, so the reader is not left choosing.

**Type consistency:** `Chronicle`, `ChronicleEntityID`, `ChronicleThroughTurnKey`, `ReadChronicle`, `WriteChronicle`, `Summariser`, `SummaryTurn`, `SummariserFromConfig`, `Chronicler`, `RecapDTO`, and the `summary` section name are each defined once and used with the same signatures.

**Known gap:** the recap shows the summary only. Open threads arrive with increment 6, and `RecapDTO` grows a `threads` field then rather than this plan guessing its shape.
