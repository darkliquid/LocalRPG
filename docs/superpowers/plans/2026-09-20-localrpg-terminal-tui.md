# LocalRPG Terminal TUI & Turn Orchestration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the complete headless interactive terminal gameplay experience (`localrpg play <game-id>`): turn history persistence in `history.jsonl`, end-to-end turn orchestration (mechanics check, context assembly, GM generation, background entity extraction), GM director steering (`/gm`), turn undo/rewind, and an interactive Bubbletea terminal UI with Glamour markdown rendering and inspection drawers.

**Architecture:** A decoupled `TurnOrchestrator` manages the lifecycle of each game turn, coordinating between the rules engine, storage, context assembler, and model harness router. Turns are written append-only to `games/<game-id>/history.jsonl`. An interactive Bubbletea TUI displays the story chronicle, accepts player actions in various modes (`Do`, `Say`, `Story`, `Roll`, `/gm`), and provides overlay modals for inspecting character stats, living world narrative arcs, and lore notes.

**Tech Stack:** Go 1.27, `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/lipgloss`, `github.com/charmbracelet/glamour`, `github.com/darkliquid/localrpg/pkg/engine`, `github.com/darkliquid/localrpg/pkg/harness`, `github.com/darkliquid/localrpg/pkg/rules`, `github.com/darkliquid/localrpg/pkg/storage`.

---

### File Structure Map

```text
LocalRPG/
├── cmd/
│   └── localrpg/
│       ├── main.go               # Updated with "play" subcommand
│       ├── play.go               # CLI handler for localrpg play <game-id>
│       └── play_test.go          # Play command argument validation tests
├── pkg/
│   ├── engine/
│   │   ├── history.go            # Turn struct, append-only history.jsonl logger & undo
│   │   ├── history_test.go       # History serialization & rewind tests
│   │   ├── orchestrator.go       # End-to-end turn orchestrator & GM steering
│   │   └── orchestrator_test.go  # Turn lifecycle & /gm correction tests
│   └── tui/
│       ├── styles.go             # Lipgloss color palettes and UI frames
│       ├── render.go             # Glamour markdown story renderer
│       ├── render_test.go        # Markdown formatting tests
│       ├── app.go                # Bubbletea interactive model & key handlers
│       └── app_test.go           # TUI message handling & state transitions
```

---

### Task 1: Turn History Logger & Snapshot Rewind

**Files:**
- Create: `pkg/engine/history.go`
- Test: `pkg/engine/history_test.go`

- [x] **Step 1: Write the failing test for Turn History**

```go
// pkg/engine/history_test.go
package engine

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/rules"
)

func TestTurnHistoryAppendAndLoad(t *testing.T) {
	tempDir := t.TempDir()
	historyPath := filepath.Join(tempDir, "history.jsonl")

	hl := NewHistoryLogger(historyPath)

	t1 := Turn{
		Number:    1,
		Timestamp: time.Now(),
		Mode:      "Do",
		Input:     "I enter the tavern and look for Evelyn",
		Roll:      &rules.RollResult{Notation: "1d20", Total: 15},
		Output:    "The tavern is warm and bustling. Lady Evelyn sits in the corner.",
	}

	if err := hl.AppendTurn(t1); err != nil {
		t.Fatalf("AppendTurn failed: %v", err)
	}

	t2 := Turn{
		Number:    2,
		Timestamp: time.Now(),
		Mode:      "Say",
		Input:     "Greetings, my lady.",
		Output:    "Evelyn looks up with guarded eyes.",
	}

	if err := hl.AppendTurn(t2); err != nil {
		t.Fatalf("AppendTurn failed: %v", err)
	}

	turns, err := hl.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory failed: %v", err)
	}

	if len(turns) != 2 {
		t.Fatalf("expected 2 turns, got %d", len(turns))
	}
	if turns[0].Input != t1.Input || turns[1].Input != t2.Input {
		t.Errorf("history mismatch: %+v", turns)
	}

	// Test Rewind to Turn 1
	if err := hl.RewindToTurn(1); err != nil {
		t.Fatalf("RewindToTurn failed: %v", err)
	}

	rewound, err := hl.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory after rewind failed: %v", err)
	}
	if len(rewound) != 1 || rewound[0].Number != 1 {
		t.Errorf("expected 1 turn after rewind, got %d", len(rewound))
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/engine/... -v -run TestTurnHistoryAppendAndLoad`  
Expected: FAIL (NewHistoryLogger not defined)

- [x] **Step 3: Implement History Logger**

Write `pkg/engine/history.go`:
```go
package engine

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/darkliquid/localrpg/pkg/rules"
)

type Turn struct {
	Number    int               `json:"number"`
	Timestamp time.Time         `json:"timestamp"`
	Mode      string            `json:"mode"` // "Do", "Say", "Story", "Roll", "GM"
	Input     string            `json:"input"`
	Roll      *rules.RollResult `json:"roll,omitempty"`
	Output    string            `json:"output"`
	AudioRefs []string          `json:"audio_refs,omitempty"`
}

type HistoryLogger struct {
	mu   sync.RWMutex
	path string
}

func NewHistoryLogger(path string) *HistoryLogger {
	return &HistoryLogger{path: path}
}

func (h *HistoryLogger) AppendTurn(t Turn) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	f, err := os.OpenFile(h.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("open history file: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("marshal turn: %w", err)
	}

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write turn: %w", err)
	}
	return nil
}

func (h *HistoryLogger) LoadHistory() ([]Turn, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	f, err := os.Open(h.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Turn{}, nil
		}
		return nil, fmt.Errorf("open history file: %w", err)
	}
	defer f.Close()

	var turns []Turn
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var t Turn
		if err := json.Unmarshal(line, &t); err == nil {
			turns = append(turns, t)
		}
	}
	return turns, scanner.Err()
}

func (h *HistoryLogger) RewindToTurn(targetTurnNumber int) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	turns, err := h.loadHistoryUnlocked()
	if err != nil {
		return err
	}

	var kept []Turn
	for _, t := range turns {
		if t.Number <= targetTurnNumber {
			kept = append(kept, t)
		}
	}

	f, err := os.Create(h.path)
	if err != nil {
		return fmt.Errorf("rewrite history file: %w", err)
	}
	defer f.Close()

	for _, t := range kept {
		data, err := json.Marshal(t)
		if err != nil {
			return err
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			return err
		}
	}
	return nil
}

func (h *HistoryLogger) loadHistoryUnlocked() ([]Turn, error) {
	f, err := os.Open(h.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Turn{}, nil
		}
		return nil, err
	}
	defer f.Close()

	var turns []Turn
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var t Turn
		if err := json.Unmarshal(line, &t); err == nil {
			turns = append(turns, t)
		}
	}
	return turns, scanner.Err()
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/engine/... -v -run TestTurnHistoryAppendAndLoad`  
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/engine/history.go pkg/engine/history_test.go
git commit -m "feat(engine): implement append-only turn history logger and rewind"
```

---

### Task 2: Turn Orchestrator & GM Steering

**Files:**
- Create: `pkg/engine/orchestrator.go`
- Test: `pkg/engine/orchestrator_test.go`

- [x] **Step 1: Write failing test for Turn Orchestrator**

```go
// pkg/engine/orchestrator_test.go
package engine

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/rules"
	"github.com/darkliquid/localrpg/pkg/storage"
)

type mockOrchestratorModel struct {
	lastPrompt string
	response   string
}

func (m *mockOrchestratorModel) ID() string { return "mock-gm" }
func (m *mockOrchestratorModel) Generate(ctx context.Context, req harness.GenerateRequest) (*harness.GenerateResponse, error) {
	m.lastPrompt = req.Prompt
	return &harness.GenerateResponse{Text: m.response}, nil
}
func (m *mockOrchestratorModel) Stream(ctx context.Context, req harness.GenerateRequest, out chan<- harness.StreamChunk) error {
	defer close(out)
	m.lastPrompt = req.Prompt
	out <- harness.StreamChunk{Text: m.response, Done: true}
	return nil
}

func TestTurnOrchestrator(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	store, err := storage.NewStore(filepath.Join(tempDir, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Seed location and player
	store.SaveEntity(&entity.Entity{ID: "tavern", Name: "Alden Tavern", Type: "location", Body: "Cozy tavern."})
	player := &entity.Entity{ID: "player", Name: "Sean", Type: "character"}
	player.InitState(map[string]interface{}{"hp": 25})
	store.SaveEntity(player)

	history := NewHistoryLogger(filepath.Join(tempDir, "history.jsonl"))
	bridge := rules.NewHostBridge(store)
	jsEngine := rules.NewJSEngine(bridge)

	model := &mockOrchestratorModel{response: "You step inside the warm tavern."}
	router := harness.NewRouter()
	router.RegisterProvider(model)
	router.AssignRole("gm", "mock-gm")

	orchestrator := NewTurnOrchestrator(store, history, jsEngine, router, "tavern", "player")

	// 1. Play standard turn
	turn, err := orchestrator.ProcessAction(ctx, "Do", "I open the door")
	if err != nil {
		t.Fatalf("ProcessAction failed: %v", err)
	}

	if turn.Number != 1 || turn.Output != "You step inside the warm tavern." {
		t.Errorf("unexpected turn outcome: %+v", turn)
	}

	// 2. Test GM director steering (/gm command)
	model.response = "Correction: The door was locked, but you pick it open."
	corrected, err := orchestrator.ProcessAction(ctx, "GM", "/gm The door was supposed to be locked.")
	if err != nil {
		t.Fatalf("ProcessAction GM steering failed: %v", err)
	}

	if !strings.Contains(corrected.Output, "Correction: The door was locked") {
		t.Errorf("expected corrected output, got %q", corrected.Output)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/engine/... -v -run TestTurnOrchestrator`  
Expected: FAIL (NewTurnOrchestrator not defined)

- [x] **Step 3: Implement Turn Orchestrator**

Write `pkg/engine/orchestrator.go`:
```go
package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/rules"
	"github.com/darkliquid/localrpg/pkg/storage"
)

type TurnOrchestrator struct {
	store       *storage.Store
	history     *HistoryLogger
	rulesEngine *rules.JSEngine
	router      *harness.Router
	locationID  string
	playerID    string
	assembler   *harness.ContextAssembler
}

func NewTurnOrchestrator(
	store *storage.Store,
	history *HistoryLogger,
	rulesEngine *rules.JSEngine,
	router *harness.Router,
	locationID string,
	playerID string,
) *TurnOrchestrator {
	return &TurnOrchestrator{
		store:       store,
		history:     history,
		rulesEngine: rulesEngine,
		router:      router,
		locationID:  locationID,
		playerID:    playerID,
		assembler:   harness.NewContextAssembler(store),
	}
}

func (o *TurnOrchestrator) ProcessAction(ctx context.Context, mode, actionInput string) (*Turn, error) {
	pastTurns, err := o.history.LoadHistory()
	if err != nil {
		return nil, fmt.Errorf("load history: %w", err)
	}
	turnNum := len(pastTurns) + 1

	var rollRes *rules.RollResult
	var gmDirective string

	// Handle /undo command
	if strings.HasPrefix(strings.TrimSpace(actionInput), "/undo") {
		if len(pastTurns) == 0 {
			return nil, fmt.Errorf("no turns to undo")
		}
		if err := o.history.RewindToTurn(len(pastTurns) - 1); err != nil {
			return nil, fmt.Errorf("undo failed: %w", err)
		}
		return &Turn{
			Number:    len(pastTurns) - 1,
			Timestamp: time.Now(),
			Mode:      "System",
			Input:     "/undo",
			Output:    "Undid previous turn.",
		}, nil
	}

	// Handle /gm director note or mode
	isCorrection := mode == "GM" || strings.HasPrefix(actionInput, "/gm ")
	if isCorrection {
		directiveText := strings.TrimPrefix(actionInput, "/gm ")
		gmDirective = fmt.Sprintf("[DIRECTOR CORRECTION DIRECTIVE: %s]", directiveText)
	} else if strings.EqualFold(mode, "Roll") {
		// Evaluate dice roll
		r, err := rules.EvaluateRoll(actionInput)
		if err == nil {
			rollRes = r
			actionInput = fmt.Sprintf("I rolled %s with result %d", r.Notation, r.Total)
		}
	} else if o.rulesEngine != nil {
		// Run action through mechanics hook if available
		res, err := o.rulesEngine.ExecuteAction(strings.ToLower(mode), map[string]interface{}{
			"action": actionInput,
			"player": o.playerID,
		})
		if err == nil && res != nil {
			rollRes = res.Roll
			if res.Message != "" {
				gmDirective = fmt.Sprintf("[MECHANICS RESULT: %s]", res.Message)
			}
		}
	}

	// Assemble 4-layer context
	contextPrompt, err := o.assembler.AssembleContext(o.locationID, o.playerID, actionInput)
	if err != nil {
		return nil, fmt.Errorf("assemble context: %w", err)
	}

	if gmDirective != "" {
		contextPrompt = gmDirective + "\n\n" + contextPrompt
	}

	// Generate story response from GM model
	req := harness.GenerateRequest{
		Prompt: contextPrompt,
	}

	resp, err := o.router.GenerateForRole(ctx, "gm", req)
	if err != nil {
		return nil, fmt.Errorf("gm generation failed: %w", err)
	}

	turn := Turn{
		Number:    turnNum,
		Timestamp: time.Now(),
		Mode:      mode,
		Input:     actionInput,
		Roll:      rollRes,
		Output:    resp.Text,
	}

	// Append to history
	if err := o.history.AppendTurn(turn); err != nil {
		return nil, fmt.Errorf("append turn: %w", err)
	}

	// Trigger post-turn hooks
	if o.rulesEngine != nil {
		_ = o.rulesEngine.ExecuteTurnEnd(map[string]interface{}{"turn": turnNum})
	}

	return &turn, nil
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/engine/... -v -run TestTurnOrchestrator`  
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/engine/orchestrator.go pkg/engine/orchestrator_test.go
git commit -m "feat(engine): implement end-to-end turn orchestrator with GM steering"
```

---

### Task 3: Terminal TUI Styles & Glamour Markdown Renderer

**Files:**
- Create: `pkg/tui/styles.go`
- Create: `pkg/tui/render.go`
- Test: `pkg/tui/render_test.go`

- [x] **Step 1: Write failing test for Glamour Markdown Renderer**

```go
// pkg/tui/render_test.go
package tui

import (
	"strings"
	"testing"
)

func TestRenderMarkdown(t *testing.T) {
	input := "# The Tavern\n\nA quiet room with **Lady Evelyn** in the corner."
	rendered, err := RenderMarkdown(input, 80)
	if err != nil {
		t.Fatalf("RenderMarkdown failed: %v", err)
	}

	if !strings.Contains(rendered, "The Tavern") || !strings.Contains(rendered, "Lady Evelyn") {
		t.Errorf("expected rendered markdown, got: %s", rendered)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/tui/... -v -run TestRenderMarkdown`  
Expected: FAIL (package/tui not defined)

- [x] **Step 3: Implement Styles and Markdown Renderer**

Write `pkg/tui/styles.go`:
```go
package tui

import "github.com/charmbracelet/lipgloss"

var (
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 1)

	StatusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")).
			Italic(true)

	PromptStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#04B575"))

	DialogueStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(lipgloss.Color("#7D56F4")).
			PaddingLeft(1).
			Italic(true)

	ModalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7D56F4")).
			Padding(1, 2).
			Width(70)
)
```

Write `pkg/tui/render.go`:
```go
package tui

import (
	"fmt"

	"github.com/charmbracelet/glamour"
)

func RenderMarkdown(content string, wordWrap int) (string, error) {
	if wordWrap <= 0 {
		wordWrap = 80
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(wordWrap),
	)
	if err != nil {
		return "", fmt.Errorf("new term renderer: %w", err)
	}

	out, err := r.Render(content)
	if err != nil {
		return "", fmt.Errorf("render markdown: %w", err)
	}

	return out, nil
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/tui/... -v -run TestRenderMarkdown`  
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/tui/styles.go pkg/tui/render.go pkg/tui/render_test.go
git commit -m "feat(tui): implement terminal UI styles and Glamour markdown renderer"
```

---

### Task 4: Interactive Bubbletea TUI Model

**Files:**
- Create: `pkg/tui/app.go`
- Test: `pkg/tui/app_test.go`

- [x] **Step 1: Write failing test for Bubbletea Model message handling**

```go
// pkg/tui/app_test.go
package tui

import (
	"context"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/darkliquid/localrpg/pkg/engine"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/storage"
)

type mockTUIModel struct {
	output string
}

func (m *mockTUIModel) ID() string { return "tui-mock" }
func (m *mockTUIModel) Generate(ctx context.Context, req harness.GenerateRequest) (*harness.GenerateResponse, error) {
	return &harness.GenerateResponse{Text: m.output}, nil
}
func (m *mockTUIModel) Stream(ctx context.Context, req harness.GenerateRequest, out chan<- harness.StreamChunk) error {
	defer close(out)
	out <- harness.StreamChunk{Text: m.output, Done: true}
	return nil
}

func TestTUIModelInitializationAndInput(t *testing.T) {
	tempDir := t.TempDir()
	store, _ := storage.NewStore(filepath.Join(tempDir, "index.db"))
	defer store.Close()

	store.SaveEntity(&entity.Entity{ID: "tavern", Name: "Alden Tavern", Type: "location", Body: "Quiet place."})
	store.SaveEntity(&entity.Entity{ID: "player", Name: "Sean", Type: "character"})

	history := engine.NewHistoryLogger(filepath.Join(tempDir, "history.jsonl"))
	router := harness.NewRouter()
	router.RegisterProvider(&mockTUIModel{output: "Welcome traveler."})
	router.AssignRole("gm", "tui-mock")

	orch := engine.NewTurnOrchestrator(store, history, nil, router, "tavern", "player")

	app := NewAppModel(orch, 80, 24)
	if app.mode != "Do" {
		t.Errorf("expected initial mode 'Do', got %q", app.mode)
	}

	// Test switching mode via Tab
	msg := tea.KeyMsg{Type: tea.KeyTab}
	newModel, _ := app.Update(msg)
	updatedApp := newModel.(*AppModel)

	if updatedApp.mode != "Say" {
		t.Errorf("expected mode 'Say' after Tab, got %q", updatedApp.mode)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/tui/... -v -run TestTUIModel`  
Expected: FAIL (NewAppModel not defined)

- [x] **Step 3: Implement Bubbletea App Model**

Write `pkg/tui/app.go`:
```go
package rules_tui

package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/darkliquid/localrpg/pkg/engine"
)

type turnResultMsg struct {
	turn *engine.Turn
	err  error
}

type AppModel struct {
	orchestrator *engine.TurnOrchestrator
	history      []engine.Turn
	mode         string   // "Do", "Say", "Story", "Roll", "GM"
	modes        []string // cycle list
	inputBuffer  string
	modalContent string
	showModal    bool
	width        int
	height       int
	isWaiting    bool
	statusMsg    string
}

func NewAppModel(orchestrator *engine.TurnOrchestrator, width, height int) *AppModel {
	return &AppModel{
		orchestrator: orchestrator,
		history:      make([]engine.Turn, 0),
		mode:         "Do",
		modes:        []string{"Do", "Say", "Story", "Roll", "GM"},
		inputBuffer:  "",
		width:        width,
		height:       height,
		isWaiting:    false,
		statusMsg:    "Ready. (Tab to cycle modes, Esc to clear, Enter to submit)",
	}
}

func (m *AppModel) Init() tea.Cmd {
	return nil
}

func (m *AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case turnResultMsg:
		m.isWaiting = false
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", msg.err)
			return m, nil
		}
		if msg.turn != nil {
			m.history = append(m.history, *msg.turn)
			m.statusMsg = fmt.Sprintf("Turn %d completed.", msg.turn.Number)
		}
		return m, nil

	case tea.KeyMsg:
		if m.showModal {
			if msg.Type == tea.KeyEsc || msg.String() == "q" {
				m.showModal = false
				return m, nil
			}
		}

		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit

		case tea.KeyTab:
			// Cycle modes
			for i, mode := range m.modes {
				if mode == m.mode {
					m.mode = m.modes[(i+1)%len(m.modes)]
					break
				}
			}
			return m, nil

		case tea.KeyEsc:
			m.inputBuffer = ""
			m.showModal = false
			return m, nil

		case tea.KeyBackspace:
			if len(m.inputBuffer) > 0 {
				m.inputBuffer = m.inputBuffer[:len(m.inputBuffer)-1]
			}
			return m, nil

		case tea.KeyEnter:
			if strings.TrimSpace(m.inputBuffer) == "" || m.isWaiting {
				return m, nil
			}
			userInput := m.inputBuffer
			currentMode := m.mode
			m.inputBuffer = ""
			m.isWaiting = true
			m.statusMsg = "GM is thinking..."

			return m, func() tea.Cmd {
				return func() tea.Msg {
					turn, err := m.orchestrator.ProcessAction(context.Background(), currentMode, userInput)
					return turnResultMsg{turn: turn, err: err}
				}
			}()

		case tea.KeyRunes:
			m.inputBuffer += string(msg.Runes)
			return m, nil
		}
	}

	return m, nil
}

func (m *AppModel) View() string {
	var sb strings.Builder

	// Header
	header := TitleStyle.Render(" LocalRPG Chronicles ")
	sb.WriteString(header + "\n\n")

	// Story chronicle history
	for _, turn := range m.history {
		sb.WriteString(PromptStyle.Render(fmt.Sprintf("[%s] %s", turn.Mode, turn.Input)) + "\n")
		renderedStory, err := RenderMarkdown(turn.Output, m.width-4)
		if err == nil {
			sb.WriteString(renderedStory + "\n")
		} else {
			sb.WriteString(turn.Output + "\n\n")
		}
	}

	if m.showModal {
		sb.WriteString(ModalStyle.Render(m.modalContent) + "\n")
	}

	// Bottom Status & Action Bar
	sb.WriteString(StatusStyle.Render(m.statusMsg) + "\n")
	modeTag := fmt.Sprintf("[%s]", m.mode)
	sb.WriteString(PromptStyle.Render(modeTag+" > ") + m.inputBuffer)

	return sb.String()
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/tui/... -v -run TestTUIModel`  
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/tui/app.go pkg/tui/app_test.go
git commit -m "feat(tui): implement interactive Bubbletea TUI application model"
```

---

### Task 5: CLI Play Subcommand (`localrpg play <game-id>`)

**Files:**
- Create: `cmd/localrpg/play.go`
- Modify: `cmd/localrpg/main.go`
- Test: `cmd/localrpg/play_test.go`

- [x] **Step 1: Write CLI play argument test**

```go
// cmd/localrpg/play_test.go
package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestCLIPlayMissingArg(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "play")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected error on missing game-id argument")
	}

	if !strings.Contains(string(out), "Usage: localrpg play <game-id>") {
		t.Errorf("unexpected output: %s", string(out))
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/localrpg/... -v -run TestCLIPlayMissingArg`  
Expected: FAIL (play subcommand not implemented)

- [x] **Step 3: Implement CLI Play Subcommand**

Write `cmd/localrpg/play.go`:
```go
package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/engine"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/rules"
	"github.com/darkliquid/localrpg/pkg/storage"
	"github.com/darkliquid/localrpg/pkg/tui"
)

func handlePlayCommand(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: localrpg play <game-id>")
		os.Exit(1)
	}

	gameID := args[0]
	baseDir := "."
	paths := core.NewPathResolver(baseDir)
	gameDir := paths.GameDir(gameID)

	manifestPath := filepath.Join(gameDir, "game.yaml")
	manifest, err := core.LoadGameManifest(manifestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading game manifest %q: %v\n", manifestPath, err)
		os.Exit(1)
	}

	dbPath := filepath.Join(gameDir, "cache", "index.db")
	store, err := storage.NewStore(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening game database %q: %v\n", dbPath, err)
		os.Exit(1)
	}
	defer store.Close()

	historyPath := filepath.Join(gameDir, "history.jsonl")
	history := engine.NewHistoryLogger(historyPath)

	bridge := rules.NewHostBridge(store)
	jsEngine := rules.NewJSEngine(bridge)

	ruleLoader := rules.NewRuleLoader(paths, jsEngine)
	_ = ruleLoader.LoadRules(manifest.SystemID, manifest.WorldID)

	// Setup model router
	router := harness.NewRouter()
	router.RegisterProvider(harness.NewCLIProvider("default-echo", "echo", []string{}))
	router.AssignRole("gm", "default-echo")

	orchestrator := engine.NewTurnOrchestrator(
		store,
		history,
		jsEngine,
		router,
		"tavern",
		manifest.Player,
	)

	app := tui.NewAppModel(orchestrator, 80, 24)
	p := tea.NewProgram(app, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
}
```

Update `cmd/localrpg/main.go` to route `case "play": handlePlayCommand(args[1:])`.

- [x] **Step 4: Run all package tests across workspace**

Run: `go test -count=1 ./... -v`  
Expected: All package tests PASS

- [x] **Step 5: Commit**

```bash
git add cmd/localrpg/play.go cmd/localrpg/main.go cmd/localrpg/play_test.go
git commit -m "feat(cli): add 'play' subcommand to launch terminal TUI"
```
