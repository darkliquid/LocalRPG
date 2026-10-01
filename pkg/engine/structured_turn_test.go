package engine

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/storage"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// runLoopForTest drives the generation loop with a minimal assembled context.
func runLoopForTest(o *TurnOrchestrator) (streamResult, error) {
	o.logger = trace.Nop()
	return o.runGenerationLoop(context.Background(), &harness.AssembleResult{Prompt: "context"}, "", nil, nil, "auto", nil)
}

func TestLoopResolvesCheckThenSubmits(t *testing.T) {
	provider := &toolScriptProvider{replies: []toolReply{
		{tools: []harness.ToolCall{{ID: "1", Name: "request_check", Arguments: `{"actor":"player","check_kind":"skill","stakes":"jump","outcomes":{"pass":"clear","fail":"fall"}}`}}},
		{tools: []harness.ToolCall{{ID: "2", Name: "submit_turn", Arguments: `{"action_verdict":{"feasibility":"uncertain","reason":"a gap"},"segments":[{"kind":"narration","text":"You leap.","check_ref":"1"}]}`}}},
	}}
	o, _ := toolLoopOrchestrator(t, provider)
	o.SetTools(&fakeExecutor{}, "yes")
	o.SetCheckResolver(defaultCheckResolver{})

	result, err := runLoopForTest(o)
	if err != nil {
		t.Fatalf("loop: %v", err)
	}
	if result.Submission == nil {
		t.Fatal("no submission")
	}
	if len(result.Checks) != 1 {
		t.Fatalf("checks = %d, want 1", len(result.Checks))
	}
	if result.Checks[0].CheckID == "" {
		t.Fatal("check has no id")
	}
}

type trackingLogger struct {
	mu     sync.Mutex
	events map[string][]map[string]interface{}
}

func newTrackingLogger() *trackingLogger {
	return &trackingLogger{events: make(map[string][]map[string]interface{})}
}

func (l *trackingLogger) Enabled(level trace.Level) bool { return true }
func (l *trackingLogger) SetGame(gameID string)          {}

func (l *trackingLogger) Event(name string, fields map[string]interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events[name] = append(l.events[name], fields)
}

func (l *trackingLogger) hasEvent(name string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.events[name]) > 0
}

type mockStructuredGM struct {
	mu        sync.Mutex
	capable   bool
	response  string
	toolCalls []harness.ToolCall
	lastReq   harness.GenerateRequest
	calls     int
}

func (m *mockStructuredGM) ID() string                       { return "mock-gm" }
func (m *mockStructuredGM) StructuredOutputCapable() bool    { return m.capable }

func (m *mockStructuredGM) Generate(ctx context.Context, req harness.GenerateRequest) (*harness.GenerateResponse, error) {
	m.mu.Lock()
	m.lastReq = req
	m.calls++
	m.mu.Unlock()
	return &harness.GenerateResponse{Text: m.response}, nil
}

func (m *mockStructuredGM) Stream(ctx context.Context, req harness.GenerateRequest, out chan<- harness.StreamChunk) error {
	defer close(out)
	m.mu.Lock()
	m.lastReq = req
	m.calls++
	m.mu.Unlock()
	out <- harness.StreamChunk{Text: m.response, Done: true, FinishReason: "stop", ToolCalls: m.toolCalls}
	return nil
}

type countingExtractorModel struct {
	calls int
}

func (m *countingExtractorModel) ID() string { return "counting-extractor" }

func (m *countingExtractorModel) Generate(ctx context.Context, req harness.GenerateRequest) (*harness.GenerateResponse, error) {
	m.calls++
	return &harness.GenerateResponse{Text: `[]`}, nil
}

func (m *countingExtractorModel) Stream(ctx context.Context, req harness.GenerateRequest, out chan<- harness.StreamChunk) error {
	defer close(out)
	m.calls++
	out <- harness.StreamChunk{Text: `[]`, Done: true}
	return nil
}

func TestDirectJSONParsedWithoutCallingExtractor(t *testing.T) {
	jsonPayload := `{
		"action_verdict": {
			"feasibility": "automatic",
			"reason": "You are skilled."
		},
		"segments": [
			{"kind": "narration", "text": "You leap smoothly across the rooftop."},
			{"kind": "speech", "speaker": "Garrick", "text": "Quick as always."}
		],
		"personae": [
			{"name": "Garrick", "type": "character", "description": "An old acquaintance."}
		]
	}`

	gm := &mockStructuredGM{
		capable:  true,
		response: jsonPayload,
	}

	router := harness.NewRouter()
	router.RegisterProvider(gm)
	router.AssignRole("gm", "mock-gm")

	tempDir := t.TempDir()
	store := newTestStore(t)
	timeline := NewTimeline(core.NewPathResolver(tempDir), store, NewHistoryLogger(filepath.Join(tempDir, "history.jsonl")), "campaign-test")

	entitiesDir := timeline.EntitiesDir()
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "tavern", Name: "Alden Tavern", Type: "location", Body: "Cozy."})
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "player", Name: "Sean", Type: "character", Body: "A traveller."})
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		t.Fatal(err)
	}

	orchestrator := NewTurnOrchestrator(store, timeline, nil, router, "tavern", "player")
	logger := newTrackingLogger()
	orchestrator.SetLogger(logger)

	extractorModel := &countingExtractorModel{}
	orchestrator.SetExtractor(harness.NewExtractor(extractorModel))

	turn, err := orchestrator.ProcessAction(context.Background(), "Do", "I leap across the rooftop")
	if err != nil {
		t.Fatalf("ProcessAction failed: %v", err)
	}

	// Verify structured submission populated
	if turn.Verdict == nil {
		t.Fatal("expected turn.Verdict to be populated")
	}
	if turn.Verdict.Feasibility != harness.FeasibilityAutomatic {
		t.Errorf("expected feasibility %q, got %q", harness.FeasibilityAutomatic, turn.Verdict.Feasibility)
	}
	if len(turn.Segments) == 0 {
		t.Fatal("expected segments to be populated from submission")
	}
	if len(turn.Personae) == 0 || turn.Personae[0] != "garrick" {
		t.Errorf("expected personae [garrick], got %v", turn.Personae)
	}

	// Verify extractor was NOT called
	if extractorModel.calls != 0 {
		t.Errorf("expected extractor to be skipped (0 calls), got %d calls", extractorModel.calls)
	}

	// Verify response schema was requested on terminal generation request
	gm.mu.Lock()
	lastReq := gm.lastReq
	gm.mu.Unlock()
	if lastReq.ResponseSchema == nil {
		t.Error("expected ResponseSchema to be set on request")
	} else if lastReq.ResponseSchema.Name != "turn_submission" {
		t.Errorf("expected ResponseSchema Name 'turn_submission', got %q", lastReq.ResponseSchema.Name)
	}

	// Verify telemetry event
	if !logger.hasEvent("turn.structured_generation") {
		t.Error("expected turn.structured_generation event to be logged")
	}
}

func TestToolCallSubmitTurnParsedWithoutCallingExtractor(t *testing.T) {
	submitArgs := `{
		"action_verdict": {
			"feasibility": "automatic",
			"reason": "The door is unlocked."
		},
		"segments": [
			{"kind": "narration", "text": "You push the door open and step inside."}
		]
	}`

	gm := &mockStructuredGM{
		capable:   false,
		toolCalls: []harness.ToolCall{{ID: "call-1", Name: "submit_turn", Arguments: submitArgs}},
	}

	router := harness.NewRouter()
	router.RegisterProvider(gm)
	router.AssignRole("gm", "mock-gm")

	tempDir := t.TempDir()
	store := newTestStore(t)
	timeline := NewTimeline(core.NewPathResolver(tempDir), store, NewHistoryLogger(filepath.Join(tempDir, "history.jsonl")), "campaign-test")

	entitiesDir := timeline.EntitiesDir()
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "tavern", Name: "Alden Tavern", Type: "location", Body: "Cozy."})
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "player", Name: "Sean", Type: "character", Body: "A traveller."})
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		t.Fatal(err)
	}

	orchestrator := NewTurnOrchestrator(store, timeline, nil, router, "tavern", "player")
	logger := newTrackingLogger()
	orchestrator.SetLogger(logger)
	orchestrator.SetTools(&fakeExecutor{}, "yes")

	extractorModel := &countingExtractorModel{}
	orchestrator.SetExtractor(harness.NewExtractor(extractorModel))

	turn, err := orchestrator.ProcessAction(context.Background(), "Do", "I open the door")
	if err != nil {
		t.Fatalf("ProcessAction failed: %v", err)
	}

	if turn.Verdict == nil {
		t.Fatal("expected turn.Verdict to be populated")
	}
	if turn.Verdict.Feasibility != harness.FeasibilityAutomatic {
		t.Errorf("expected feasibility %q, got %q", harness.FeasibilityAutomatic, turn.Verdict.Feasibility)
	}

	// Verify extractor was NOT called
	if extractorModel.calls != 0 {
		t.Errorf("expected extractor to be skipped (0 calls), got %d calls", extractorModel.calls)
	}

	// Verify telemetry event
	if !logger.hasEvent("turn.structured_generation") {
		t.Error("expected turn.structured_generation event to be logged")
	}
}

func TestMalformedSubmissionFallsBackToRawTextAndCallsExtractor(t *testing.T) {
	// Raw text narration that is not valid JSON
	rawNarration := "The old clocktower strikes midnight as rain pours down."

	gm := &mockStructuredGM{
		capable:  true,
		response: rawNarration,
	}

	router := harness.NewRouter()
	router.RegisterProvider(gm)
	router.AssignRole("gm", "mock-gm")

	tempDir := t.TempDir()
	store := newTestStore(t)
	timeline := NewTimeline(core.NewPathResolver(tempDir), store, NewHistoryLogger(filepath.Join(tempDir, "history.jsonl")), "campaign-test")

	entitiesDir := timeline.EntitiesDir()
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "tavern", Name: "Alden Tavern", Type: "location", Body: "Cozy."})
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "player", Name: "Sean", Type: "character", Body: "A traveller."})
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		t.Fatal(err)
	}

	orchestrator := NewTurnOrchestrator(store, timeline, nil, router, "tavern", "player")
	logger := newTrackingLogger()
	orchestrator.SetLogger(logger)

	extractorModel := &countingExtractorModel{}
	orchestrator.SetExtractor(harness.NewExtractor(extractorModel))

	turn, err := orchestrator.ProcessAction(context.Background(), "Do", "I listen into the night")
	if err != nil {
		t.Fatalf("ProcessAction failed: %v", err)
	}

	// Structured fields should be nil/empty on raw text fallback
	if turn.Verdict != nil {
		t.Errorf("expected nil Verdict on fallback, got %+v", turn.Verdict)
	}

	// Narration should contain raw text
	if !strings.Contains(turn.Narration, "The old clocktower strikes midnight") {
		t.Errorf("expected raw narration in turn, got %q", turn.Narration)
	}

	// Extractor WAS called
	if extractorModel.calls == 0 {
		t.Error("expected extractor to be called on raw text fallback, got 0 calls")
	}

	// Verify structured parse failure was logged
	if !logger.hasEvent("turn.structured_parse_failed") {
		t.Error("expected turn.structured_parse_failed event to be logged")
	}

	// Verify structured generation was NOT logged
	if logger.hasEvent("turn.structured_generation") {
		t.Error("turn.structured_generation should not be logged on fallback")
	}
}

func TestMarkdownFencedJSONParsedIntoStructuredTurn(t *testing.T) {
	fencedJSON := "```json\n" + `{
		"action_verdict": {
			"feasibility": "automatic",
			"reason": "Clear path."
		},
		"segments": [
			{"kind": "narration", "text": "You walk along the quiet canal."}
		]
	}` + "\n```"

	gm := &mockStructuredGM{
		capable:  true,
		response: fencedJSON,
	}

	router := harness.NewRouter()
	router.RegisterProvider(gm)
	router.AssignRole("gm", "mock-gm")

	tempDir := t.TempDir()
	store := newTestStore(t)
	timeline := NewTimeline(core.NewPathResolver(tempDir), store, NewHistoryLogger(filepath.Join(tempDir, "history.jsonl")), "campaign-test")

	entitiesDir := timeline.EntitiesDir()
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "tavern", Name: "Alden Tavern", Type: "location", Body: "Cozy."})
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "player", Name: "Sean", Type: "character", Body: "A traveller."})
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		t.Fatal(err)
	}

	orchestrator := NewTurnOrchestrator(store, timeline, nil, router, "tavern", "player")
	extractorModel := &countingExtractorModel{}
	orchestrator.SetExtractor(harness.NewExtractor(extractorModel))

	turn, err := orchestrator.ProcessAction(context.Background(), "Do", "I walk by the canal")
	if err != nil {
		t.Fatalf("ProcessAction failed: %v", err)
	}

	if turn.Verdict == nil {
		t.Fatal("expected turn.Verdict to be populated from code-fenced JSON")
	}
	if turn.Verdict.Feasibility != harness.FeasibilityAutomatic {
		t.Errorf("expected feasibility %q, got %q", harness.FeasibilityAutomatic, turn.Verdict.Feasibility)
	}
	if !strings.Contains(turn.Narration, "You walk along the quiet canal.") {
		t.Errorf("expected narration from segments, got %q", turn.Narration)
	}
	if extractorModel.calls != 0 {
		t.Errorf("expected extractor to be skipped (0 calls), got %d calls", extractorModel.calls)
	}
}

func TestCheckPreservedOnParseFallback(t *testing.T) {
	provider := &toolScriptProvider{replies: []toolReply{
		{tools: []harness.ToolCall{{ID: "c1", Name: "request_check", Arguments: `{"actor":"player","check_kind":"skill","stakes":"jump","outcomes":{"pass":"clear","fail":"fall"}}`}}},
		{text: "not valid json but ordinary narration describing the jump"},
	}}
	orchestrator, _ := toolLoopOrchestrator(t, provider)
	orchestrator.SetTools(&fakeExecutor{}, "yes")
	orchestrator.SetCheckResolver(defaultCheckResolver{})

	result, err := runLoopForTest(orchestrator)
	if err != nil {
		t.Fatalf("loop: %v", err)
	}
	if result.Submission != nil {
		t.Fatal("expected nil submission on parse failure")
	}
	if len(result.Checks) != 1 {
		t.Fatalf("expected 1 check to be preserved on fallback, got %d", len(result.Checks))
	}
	if result.Checks[0].CheckID != "c1" {
		t.Errorf("expected check ID 'c1', got %q", result.Checks[0].CheckID)
	}
}
