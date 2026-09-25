# Gameplay Experience — Unblock Play Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make a turn always end, make generation honour the parameters that are already configured, and make the protagonist entered at setup actually exist and be found, so a campaign created today can be played.

**Architecture:** Providers gain the sampling parameters they currently drop and report their terminal `finish_reason`; the orchestrator gains an idle watchdog and the GUI a wall-clock deadline so a stalled provider fails visibly and records nothing. Player identity is reconciled from the manifest and the entity index through one resolver that both clients share, repairing legacy campaigns on first open.

**Tech Stack:** Go 1.27.1, the existing `harness` providers, `gopkg.in/yaml.v3` for manifest writes, the standard library for tests, `mise` tasks.

**Spec:** `docs/superpowers/specs/2026-09-22-gameplay-experience-design.md`

## Global Constraints

- Go tests use the standard library only (`testing`, `t.TempDir()`); no testify. Use `interface{}`, never `any`.
- Errors are wrapped with `fmt.Errorf("...: %w", err)`.
- No new dependencies, Go or Node.
- Nothing is persisted for a cancelled, stalled, or failed turn: `RecordTurn` is only reached after the cancellation check.
- A live turn and a replayed turn are the same `TurnDTO` shape; the shared `turnDTO` mapper is not duplicated.
- The engine stays schema-agnostic: no HP, mana, classes, or stats are hardcoded.
- New config keys must be safe to omit. A zero value means "use the documented default", because existing `config.yaml` files will not have them.
- `go vet ./...`, `go test -count=1 ./...`, and `cd frontend && npx tsc --noEmit` must pass. Every commit must build standalone.
- Frontend build gotcha: never commit the deletion of `pkg/gui/dist/.gitkeep`.

## Scope & Splitting

This plan implements increment 1 ("Unblock play") of the spec: provider generation parameters and completion, turn deadlines, player identity, and campaign metadata. It is entirely Go. Increments 2-5 (opening, markdown, speech, draft UX, polish) are separate plans. Tasks are ordered: Task 2 builds on Task 1's `FinishReason`, and Task 4 builds on Task 3's `InitOptions`.

## File Structure

- `pkg/harness/types.go` — `GenerationOptions`, `StreamChunk.FinishReason`.
- `pkg/harness/http_provider.go` — OpenAI-compatible body parameters and finish-reason parsing.
- `pkg/harness/cli_provider.go` — generation options as environment, finish reason on clean exit.
- `pkg/harness/factory.go` — pass role sampling config into providers.
- `pkg/config/types.go` — turn and chunk timeout settings with accessors and defaults.
- `pkg/engine/orchestrator.go` — idle watchdog and `ErrGenerationStalled`.
- `pkg/engine/player.go` — `ResolvePlayerID` and `RepairPlayerIdentity`.
- `pkg/engine/game.go` — `InitOptions`, slugged player ID, display name, campaign title.
- `pkg/engine/startlocation.go` — resolve the player through the shared resolver.
- `pkg/gui/service.go` — deadlines when running a turn, player resolution, title, latest-first listing.
- `cmd/localrpg/play.go` — repair identity for the TUI.

---

## Phase 1: Turns That Always End

### Task 1: Providers honour generation options and report completion

**Files:**
- Modify: `pkg/harness/types.go`
- Modify: `pkg/harness/http_provider.go`
- Modify: `pkg/harness/cli_provider.go`
- Modify: `pkg/harness/factory.go`
- Test: `pkg/harness/http_provider_test.go`
- Test: `pkg/harness/cli_provider_test.go`

**Interfaces:**
- Produces: `harness.GenerationOptions{Temperature float64; MaxTokens int; Stop []string}`
- Produces: `harness.StreamChunk.FinishReason string`
- Produces: `harness.NewHTTPProviderWithOptions(id, endpoint, model, apiKey string, opts GenerationOptions) *HTTPProvider`
- Produces: `harness.NewCLIProviderWithOptions(id, command string, args []string, opts GenerationOptions) *CLIProvider`
- Consumes: nothing from earlier tasks.

The existing `NewHTTPProvider` and `NewCLIProvider` signatures are kept as wrappers so every current call site (`cmd/localrpg/prompt.go`, tests) keeps compiling.

- [x] **Step 1: Write the failing HTTP test**

Append to `pkg/harness/http_provider_test.go` (and add `"encoding/json"` to its import block):

```go
func TestHTTPProviderSendsGenerationOptionsAndReportsFinish(t *testing.T) {
	var gotBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected flusher")
		}
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hi\"},\"finish_reason\":null}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"length\"}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	provider := NewHTTPProviderWithOptions("mock-ollama", server.URL, "llama3", "", GenerationOptions{
		Temperature: 0.4,
		MaxTokens:   256,
	})

	out := make(chan StreamChunk, 10)
	errCh := make(chan error, 1)
	go func() {
		errCh <- provider.Stream(context.Background(), GenerateRequest{Prompt: "Tell a story"}, out)
	}()

	var finishReason string
	for chunk := range out {
		if chunk.FinishReason != "" {
			finishReason = chunk.FinishReason
		}
	}
	if err := <-errCh; err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	if got := gotBody["max_tokens"]; got != float64(256) {
		t.Errorf("max_tokens = %v, want 256", got)
	}
	if got := gotBody["temperature"]; got != 0.4 {
		t.Errorf("temperature = %v, want 0.4", got)
	}
	if finishReason != "length" {
		t.Errorf("finish reason = %q, want length", finishReason)
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

Run: `go test -run TestHTTPProviderSendsGenerationOptionsAndReportsFinish -v ./pkg/harness/`
Expected: FAIL — `undefined: NewHTTPProviderWithOptions` and `unknown field FinishReason` in `StreamChunk`.

- [x] **Step 3: Add the shared types**

In `pkg/harness/types.go`, replace `StreamChunk` and add `GenerationOptions`:

```go
type StreamChunk struct {
	Text         string
	Done         bool
	FinishReason string
	Error        error
}

// GenerationOptions are the sampling parameters a provider applies to a call.
// They come from the role's configuration and are merged with any per-request
// values, which win.
type GenerationOptions struct {
	Temperature float64
	MaxTokens   int
	Stop        []string
}
```

- [x] **Step 4: Teach the HTTP provider to send and report them**

In `pkg/harness/http_provider.go`:

Add the field and the options constructor:

```go
type HTTPProvider struct {
	id       string
	endpoint string
	model    string
	apiKey   string
	opts     GenerationOptions
	client   *http.Client
}

func NewHTTPProvider(id, endpoint, model, apiKey string) *HTTPProvider {
	return NewHTTPProviderWithOptions(id, endpoint, model, apiKey, GenerationOptions{})
}

func NewHTTPProviderWithOptions(id, endpoint, model, apiKey string, opts GenerationOptions) *HTTPProvider {
	return &HTTPProvider{
		id:       id,
		endpoint: strings.TrimRight(endpoint, "/"),
		model:    model,
		apiKey:   apiKey,
		opts:     opts,
		client:   &http.Client{},
	}
}
```

Extend the request and chunk structs:

```go
type openAIChatRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	Stream      bool            `json:"stream"`
	Temperature float64         `json:"temperature,omitempty"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Stop        []string        `json:"stop,omitempty"`
}

type openAIChatChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}
```

Build the payload from merged options:

```go
	temperature := req.Temperature
	if temperature == 0 {
		temperature = h.opts.Temperature
	}
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = h.opts.MaxTokens
	}

	payload := openAIChatRequest{
		Model:       h.model,
		Messages:    messages,
		Stream:      true,
		Temperature: temperature,
		MaxTokens:   maxTokens,
		Stop:        h.opts.Stop,
	}
```

Replace the streaming loop's tail so the finish reason is captured and `Done` is always sent:

```go
	var finishReason string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		eventData := strings.TrimPrefix(line, "data: ")
		if strings.TrimSpace(eventData) == "[DONE]" {
			break
		}

		var chunk openAIChatChunk
		if err := json.Unmarshal([]byte(eventData), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) > 0 {
			if chunk.Choices[0].Delta.Content != "" {
				out <- StreamChunk{Text: chunk.Choices[0].Delta.Content}
			}
			if chunk.Choices[0].FinishReason != "" {
				finishReason = chunk.Choices[0].FinishReason
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}
	if finishReason == "" {
		finishReason = "stop"
	}
	out <- StreamChunk{Done: true, FinishReason: finishReason}
	return nil
```

- [x] **Step 5: Run the HTTP test to verify it passes**

Run: `go test -run TestHTTPProvider -v ./pkg/harness/`
Expected: PASS, including the pre-existing `TestHTTPProviderStreaming`.

- [x] **Step 6: Write the failing CLI test**

Append to `pkg/harness/cli_provider_test.go`:

```go
func TestCLIProviderReportsCompletionAndExposesOptions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	provider := NewCLIProviderWithOptions("stream-cli", "sh",
		[]string{"-c", "printf '%s-%s' \"$LOCALRPG_MAX_TOKENS\" \"$LOCALRPG_TEMPERATURE\"", "--"},
		GenerationOptions{Temperature: 0.5, MaxTokens: 512})

	out := make(chan StreamChunk, 10)
	errCh := make(chan error, 1)
	go func() {
		errCh <- provider.Stream(ctx, GenerateRequest{Prompt: "x"}, out)
	}()

	var received strings.Builder
	var done bool
	var finishReason string
	for chunk := range out {
		received.WriteString(chunk.Text)
		if chunk.Done {
			done = true
		}
		if chunk.FinishReason != "" {
			finishReason = chunk.FinishReason
		}
	}
	if err := <-errCh; err != nil {
		t.Fatalf("Stream failed: %v", err)
	}
	if !done || finishReason != "stop" {
		t.Errorf("done = %v, finish reason = %q; want true and stop", done, finishReason)
	}
	if got := received.String(); got != "512-0.5" {
		t.Errorf("options did not reach the process, got %q", got)
	}
}
```

- [x] **Step 7: Run the CLI test to verify it fails**

Run: `go test -run TestCLIProviderReportsCompletionExposesOptions -v ./pkg/harness/`
Expected: FAIL — `undefined: NewCLIProviderWithOptions`.

(If the run name above does not match, run `go test -run TestCLIProvider -v ./pkg/harness/`; the new test will fail to compile.)

- [x] **Step 8: Implement the CLI options**

In `pkg/harness/cli_provider.go`:

```go
type CLIProvider struct {
	id      string
	command string
	args    []string
	opts    GenerationOptions
}

func NewCLIProvider(id, command string, args []string) *CLIProvider {
	return NewCLIProviderWithOptions(id, command, args, GenerationOptions{})
}

func NewCLIProviderWithOptions(id, command string, args []string, opts GenerationOptions) *CLIProvider {
	return &CLIProvider{
		id:      id,
		command: command,
		args:    args,
		opts:    opts,
	}
}
```

Replace `buildCmd` so options travel as environment (a CLI harness has no portable flag convention):

```go
func (c *CLIProvider) buildCmd(ctx context.Context, req GenerateRequest) *exec.Cmd {
	args := append([]string{}, c.args...)
	args = append(args, req.Prompt)

	cmd := exec.CommandContext(ctx, c.command, args...)

	env := cmd.Environ()
	if req.System != "" {
		env = append(env, "SYSTEM_PROMPT="+req.System)
	}
	if c.opts.MaxTokens > 0 {
		env = append(env, fmt.Sprintf("LOCALRPG_MAX_TOKENS=%d", c.opts.MaxTokens))
	}
	if c.opts.Temperature > 0 {
		env = append(env, fmt.Sprintf("LOCALRPG_TEMPERATURE=%g", c.opts.Temperature))
	}
	cmd.Env = env

	return cmd
}
```

Set the finish reason in `Stream`:

```go
	out <- StreamChunk{Done: true, FinishReason: "stop"}
	return nil
```

- [x] **Step 9: Run the CLI tests to verify they pass**

Run: `go test -run TestCLIProvider -v ./pkg/harness/`
Expected: PASS.

- [x] **Step 10: Wire the options through the factory**

In `pkg/harness/factory.go`, replace the `cli` and `http` branches of `NewModelProvider`:

```go
	case "cli":
		return NewCLIProviderWithOptions(id, cfg.Command, cfg.Args, GenerationOptions{
			Temperature: cfg.Temperature,
			MaxTokens:   cfg.MaxTokens,
		}), nil
	case "http":
		return NewHTTPProviderWithOptions(id, cfg.Endpoint, cfg.Model, cfg.APIKey, GenerationOptions{
			Temperature: cfg.Temperature,
			MaxTokens:   cfg.MaxTokens,
		}), nil
	case "builtin", "mock", "":
		if cfg.BuiltinName == "narrative-oracle" {
			return NewNarrativeOracleProvider(id), nil
		}
		if cfg.Command != "" {
			return NewCLIProviderWithOptions(id, cfg.Command, cfg.Args, GenerationOptions{
				Temperature: cfg.Temperature,
				MaxTokens:   cfg.MaxTokens,
			}), nil
		}
		return &builtinEchoModelProvider{id: id}, nil
```

- [x] **Step 11: Run the harness suite and vet**

Run: `go test -count=1 ./pkg/harness/ && go vet ./pkg/harness/`
Expected: PASS and clean vet.

- [x] **Step 12: Commit**

```bash
git add pkg/harness/types.go pkg/harness/http_provider.go pkg/harness/cli_provider.go pkg/harness/factory.go pkg/harness/http_provider_test.go pkg/harness/cli_provider_test.go
git commit -m "fix(harness): send configured sampling and report model completion"
```

---

### Task 2: Bound a turn so a stalled provider fails visibly

**Files:**
- Modify: `pkg/config/types.go`
- Create: `pkg/config/types_test.go`
- Modify: `pkg/engine/orchestrator.go`
- Modify: `pkg/gui/service.go`
- Test: `pkg/engine/orchestrator_stream_test.go`

**Interfaces:**
- Consumes: `harness.StreamChunk.FinishReason` (Task 1)
- Produces: `config.Config.TurnTimeout() time.Duration`, `config.Config.ChunkTimeout() time.Duration`
- Produces: `engine.ErrGenerationStalled`
- Produces: `(*engine.TurnOrchestrator).SetChunkTimeout(timeout time.Duration)`

- [x] **Step 1: Write the failing config test**

Create `pkg/config/types_test.go`:

```go
package config

import (
	"testing"
	"time"
)

func TestTurnAndChunkTimeoutsHaveDefaults(t *testing.T) {
	empty := &Config{}

	if got := empty.TurnTimeout(); got != 300*time.Second {
		t.Errorf("TurnTimeout() = %v, want 300s for an omitted setting", got)
	}
	if got := empty.ChunkTimeout(); got != 60*time.Second {
		t.Errorf("ChunkTimeout() = %v, want 60s for an omitted setting", got)
	}

	configured := &Config{Agents: AgentsConfig{TurnTimeoutSeconds: 45, ChunkTimeoutSeconds: 5}}
	if got := configured.TurnTimeout(); got != 45*time.Second {
		t.Errorf("TurnTimeout() = %v, want 45s", got)
	}
	if got := configured.ChunkTimeout(); got != 5*time.Second {
		t.Errorf("ChunkTimeout() = %v, want 5s", got)
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

Run: `go test -run TestTurnAndChunkTimeoutsHaveDefaults -v ./pkg/config/`
Expected: FAIL — `unknown field TurnTimeoutSeconds`.

- [x] **Step 3: Add the settings and accessors**

In `pkg/config/types.go`, add `"time"` to the import block (create the block if there is none).

Extend `AgentsConfig`:

```go
type AgentsConfig struct {
	DefaultRole string                     `yaml:"default_role" json:"default_role"`
	Roles       map[string]AgentRoleConfig `yaml:"roles" json:"roles"`
	Fallbacks   map[string]string          `yaml:"fallbacks,omitempty" json:"fallbacks,omitempty"`
	// TurnTimeoutSeconds bounds a whole turn; ChunkTimeoutSeconds bounds the
	// silence tolerated between narration deltas. Zero means "use the default",
	// so configuration written before these keys existed keeps working.
	TurnTimeoutSeconds  int `yaml:"turn_timeout_seconds" json:"turn_timeout_seconds"`
	ChunkTimeoutSeconds int `yaml:"chunk_timeout_seconds" json:"chunk_timeout_seconds"`
}
```

Add the accessors at the end of the file:

```go
// TurnTimeout is the wall-clock budget for one turn.
func (c *Config) TurnTimeout() time.Duration {
	seconds := c.Agents.TurnTimeoutSeconds
	if seconds <= 0 {
		seconds = 300
	}
	return time.Duration(seconds) * time.Second
}

// ChunkTimeout is the silence tolerated between narration deltas.
func (c *Config) ChunkTimeout() time.Duration {
	seconds := c.Agents.ChunkTimeoutSeconds
	if seconds <= 0 {
		seconds = 60
	}
	return time.Duration(seconds) * time.Second
}
```

Add the defaults to `DefaultConfig`'s `Agents` literal:

```go
		Agents: AgentsConfig{
			DefaultRole:         "gm",
			TurnTimeoutSeconds:  300,
			ChunkTimeoutSeconds: 60,
			Roles: map[string]AgentRoleConfig{
```

- [x] **Step 4: Run the config test to verify it passes**

Run: `go test -run TestTurnAndChunkTimeoutsHaveDefaults -v ./pkg/config/`
Expected: PASS.

- [x] **Step 5: Write the failing orchestrator test**

Append to `pkg/engine/orchestrator_stream_test.go`:

```go
func TestGenerationStallsWhenNoChunkArrives(t *testing.T) {
	provider := &scriptedStreamProvider{block: true}
	orchestrator, timeline, store := streamingOrchestrator(t, provider)
	orchestrator.SetChunkTimeout(20 * time.Millisecond)

	_, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I wait", nil)
	if !errors.Is(err, ErrGenerationStalled) {
		t.Fatalf("expected ErrGenerationStalled, got %v", err)
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
}
```

Add `"time"` to that file's import block.

- [x] **Step 6: Run the test to verify it fails**

Run: `go test -run TestGenerationStallsWhenNoChunkArrives -v ./pkg/engine/`
Expected: FAIL — `undefined: ErrGenerationStalled` or `SetChunkTimeout`.

- [x] **Step 7: Implement the idle watchdog**

In `pkg/engine/orchestrator.go`, add `"errors"` to the import block.

Add the sentinel and the default next to the type:

```go
// ErrGenerationStalled reports that the gm provider stopped sending deltas for
// longer than the configured chunk timeout.
var ErrGenerationStalled = errors.New("gm generation stalled")

// defaultChunkTimeout is the silence tolerated between deltas when a caller sets none.
const defaultChunkTimeout = 60 * time.Second
```

Add the field to `TurnOrchestrator`:

```go
	extractor     *harness.Extractor
	chunkTimeout  time.Duration
```

Add the setter after `SetExtractor`:

```go
// SetChunkTimeout bounds the silence tolerated between narration deltas. Zero
// restores the default, so a misconfigured value cannot disable the watchdog.
func (o *TurnOrchestrator) SetChunkTimeout(timeout time.Duration) {
	if timeout <= 0 {
		timeout = defaultChunkTimeout
	}
	o.chunkTimeout = timeout
}
```

Replace `generate` entirely:

```go
// generate streams the GM's reply, forwarding each delta and accumulating the text.
// A provider that goes silent for longer than the chunk timeout is abandoned: the
// stream context is cancelled, the provider's goroutines are drained, and the turn
// fails without being recorded, which is what keeps a hung model from holding the
// campaign forever.
func (o *TurnOrchestrator) generate(ctx context.Context, prompt string, onChunk func(string) error) (string, error) {
	timeout := o.chunkTimeout
	if timeout <= 0 {
		timeout = defaultChunkTimeout
	}

	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	chunks := make(chan harness.StreamChunk, 32)
	streamErr := make(chan error, 1)
	go func() {
		streamErr <- o.router.StreamForRole(streamCtx, "gm", harness.GenerateRequest{Prompt: prompt}, chunks)
	}()

	idle := time.NewTimer(timeout)
	defer idle.Stop()

	var sb strings.Builder
	for {
		select {
		case <-idle.C:
			cancel()
			// Draining until the provider closes lets its goroutines exit rather
			// than block forever on a channel nobody reads.
			go func() {
				for range chunks {
				}
			}()
			<-streamErr
			return "", fmt.Errorf("%w after %s", ErrGenerationStalled, timeout)

		case chunk, ok := <-chunks:
			if !ok {
				if err := <-streamErr; err != nil {
					return "", err
				}
				return sb.String(), nil
			}
			if chunk.Error != nil {
				<-streamErr
				return "", chunk.Error
			}
			if !idle.Stop() {
				select {
				case <-idle.C:
				default:
				}
			}
			idle.Reset(timeout)

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
	}
}
```

- [x] **Step 8: Run the orchestrator suite to verify it passes**

Run: `go test -count=1 ./pkg/engine/`
Expected: PASS, including the existing cancellation and streaming tests.

- [x] **Step 9: Apply the deadlines in the service**

In `pkg/gui/service.go`, in `prepareTurn`, after `orchestrator.LoadPrompts(...)`:

```go
	orchestrator.SetChunkTimeout(cfg.ChunkTimeout())
```

In `TurnSession.Run`, wrap the context:

```go
func (t *TurnSession) Run(ctx context.Context, req TurnRequest, emit func(TurnEvent) error) error {
	runCtx, cancel := context.WithTimeout(ctx, t.cfg.TurnTimeout())
	defer cancel()

	turn, err := t.orchestrator.ProcessActionStream(runCtx, req.Mode, req.Input, func(text string) error {
		return emit(TurnEvent{Type: "chunk", Text: text})
	})
	if err != nil {
		return err
	}

	dto := t.service.turnDTO(*turn, t.store, t.cfg, t.gameID)
	return emit(TurnEvent{Type: "turn", Turn: &dto})
}
```

- [x] **Step 10: Run the GUI suite and vet**

Run: `go test -count=1 ./pkg/gui/ && go vet ./...`
Expected: PASS and clean vet. The existing `TestTurnSessionRunsAndRecordsATurn` still passes because it finishes well inside the 300s default.

- [x] **Step 11: Commit**

```bash
git add pkg/config/types.go pkg/config/types_test.go pkg/engine/orchestrator.go pkg/engine/orchestrator_stream_test.go pkg/gui/service.go
git commit -m "fix(engine): fail a turn whose narrator goes silent"
```

---

## Phase 2: The Player Exists

### Task 3: Reconcile the protagonist's identity

**Files:**
- Modify: `pkg/core/types.go`
- Create: `pkg/engine/player.go`
- Create: `pkg/engine/player_test.go`
- Modify: `pkg/engine/game.go`
- Modify: `pkg/engine/startlocation.go`
- Modify: `pkg/gui/service.go`
- Modify: `cmd/localrpg/play.go`
- Modify: `pkg/engine/game_test.go`
- Modify: `pkg/gui/turn_test.go`

**Interfaces:**
- Produces: `core.GameManifest.PlayerName string`
- Produces: `engine.InitOptions{GameID, SystemID, WorldID, PlayerName, PlayerDetails string}`
- Produces: `engine.InitGame(paths *core.PathResolver, opts InitOptions) (*Session, error)`
- Produces: `engine.ResolvePlayerID(store *storage.Store, manifest *core.GameManifest) (string, error)`
- Produces: `engine.RepairPlayerIdentity(paths *core.PathResolver, store *storage.Store, manifest *core.GameManifest) (string, error)`

- [x] **Step 1: Write the failing resolver test**

Create `pkg/engine/player_test.go`:

```go
package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// legacyPlayerCampaign writes a campaign the way versions before player_name did:
// the manifest holds the display name while the note is named by its slug.
func legacyPlayerCampaign(t *testing.T, player string) (*core.PathResolver, *storage.Store) {
	t.Helper()

	paths := writeTestCampaignScaffold(t, t.TempDir(), nil)
	session, err := InitGame(paths, InitOptions{
		GameID:     "campaign-legacy",
		SystemID:   "d20-test",
		WorldID:    "fantasy-realm",
		PlayerName: player,
	})
	if err != nil {
		t.Fatalf("InitGame failed: %v", err)
	}
	_ = session.Close()

	// Rewrite the manifest into the legacy shape: display name in player, no
	// player_name, note still under the slug.
	legacy := "id: campaign-legacy\nname: campaign-legacy\nsystem: d20-test\nworld: fantasy-realm\nplayer: " + player + "\n"
	if err := os.WriteFile(filepath.Join(paths.GameDir("campaign-legacy"), "game.yaml"), []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}

	store, err := storage.OpenGameStore(paths, "campaign-legacy")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return paths, store
}

func TestResolvePlayerIDReadsALegacyDisplayName(t *testing.T) {
	paths, store := legacyPlayerCampaign(t, "Elena Nightshade")
	defer store.Close()

	manifest, err := core.LoadGameManifest(filepath.Join(paths.GameDir("campaign-legacy"), "game.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	id, err := ResolvePlayerID(store, manifest)
	if err != nil {
		t.Fatalf("ResolvePlayerID failed: %v", err)
	}
	if id != "elena-nightshade" {
		t.Errorf("ResolvePlayerID = %q, want elena-nightshade", id)
	}

	persisted, err := RepairPlayerIdentity(paths, store, manifest)
	if err != nil {
		t.Fatalf("RepairPlayerIdentity failed: %v", err)
	}
	if persisted != "elena-nightshade" {
		t.Errorf("RepairPlayerIdentity = %q, want elena-nightshade", persisted)
	}

	reloaded, err := core.LoadGameManifest(filepath.Join(paths.GameDir("campaign-legacy"), "game.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Player != "elena-nightshade" {
		t.Errorf("persisted player = %q, want the entity ID", reloaded.Player)
	}
	if reloaded.PlayerName != "Elena Nightshade" {
		t.Errorf("persisted player_name = %q, want the display name", reloaded.PlayerName)
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

Run: `go test -run TestResolvePlayerIDReadsALegacyDisplayName -v ./pkg/engine/`
Expected: FAIL — `undefined: InitOptions`.

- [x] **Step 3: Add the display name to the manifest**

In `pkg/core/types.go`, extend `GameManifest`:

```go
type GameManifest struct {
	ID         string                 `yaml:"id"`
	Name       string                 `yaml:"name"`
	SystemID   string                 `yaml:"system"`
	WorldID    string                 `yaml:"world"`
	Player     string                 `yaml:"player"`
	PlayerName string                 `yaml:"player_name,omitempty"`
	Settings   map[string]interface{} `yaml:"settings,omitempty"`
}
```

- [x] **Step 4: Replace `InitGame`'s signature and write IDs**

In `pkg/engine/game.go`, add `"strings"` to the imports and introduce the options struct above `InitGame`:

```go
// InitOptions describes a campaign to create. It is a struct rather than a
// parameter list because campaign creation grows new optional fields, and a
// positional signature would break every caller each time one is added.
type InitOptions struct {
	GameID        string
	SystemID      string
	WorldID       string
	PlayerName    string
	PlayerDetails string
}
```

Change the signature and the manifest literal:

```go
func InitGame(paths *core.PathResolver, opts InitOptions) (*Session, error) {
	gameID := opts.GameID
	systemID := opts.SystemID
	worldID := opts.WorldID

	// Verify system & world exist
	sysManifest, err := core.LoadSystemManifest(filepath.Join(paths.SystemDir(systemID), "system.yaml"))
	if err != nil {
		return nil, fmt.Errorf("load system %q: %w", systemID, err)
	}

	worldManifest, err := core.LoadWorldManifest(filepath.Join(paths.WorldDir(worldID), "world.yaml"))
	if err != nil {
		return nil, fmt.Errorf("load world %q: %w", worldID, err)
	}
```

```go
	playerID := entity.Slugify(opts.PlayerName)
	if playerID == "" {
		playerID = "player"
	}

	manifest := &core.GameManifest{
		ID:         gameID,
		Name:       gameID,
		SystemID:   systemID,
		WorldID:    worldID,
		Player:     playerID,
		PlayerName: opts.PlayerName,
		Settings:   make(map[string]interface{}),
	}
```

Pass the details to the note:

```go
	if err := ensurePlayerNote(paths, store, gameID, opts.PlayerName, opts.PlayerDetails, startLocation); err != nil {
		return nil, fmt.Errorf("create player note: %w", err)
	}
```

Update `ensurePlayerNote`:

```go
func ensurePlayerNote(paths *core.PathResolver, store *storage.Store, gameID, playerName, details, locationID string) error {
	id := entity.Slugify(playerName)
	if id == "" {
		id = "player"
	}

	path := filepath.Join(paths.GameDir(gameID), "entities", id+".md")
	if _, err := os.Stat(path); err == nil {
		return nil
	}

	body := strings.TrimSpace(details)
	if body == "" {
		body = "The player character."
	}

	player := &entity.Entity{
		ID:   id,
		Name: playerName,
		Type: "character",
		Body: body,
	}
```

- [x] **Step 5: Create the resolver**

Create `pkg/engine/player.go`:

```go
package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/storage"
	"gopkg.in/yaml.v3"
)

// ResolvePlayerID finds the protagonist's entity ID. A manifest's Player field
// may hold an entity ID, or a display name written by a version that named the
// note by its slug instead. Both are tried, then the player name, then a
// case-insensitive match over indexed character entities.
func ResolvePlayerID(store *storage.Store, manifest *core.GameManifest) (string, error) {
	if store == nil || manifest == nil {
		return "", nil
	}

	for _, candidate := range []string{manifest.Player, entity.Slugify(manifest.PlayerName), entity.Slugify(manifest.Player)} {
		if candidate == "" {
			continue
		}
		if ent, err := store.GetEntity(candidate); err == nil && ent != nil {
			return ent.ID, nil
		}
	}

	name := manifest.PlayerName
	if name == "" {
		name = manifest.Player
	}
	if name == "" {
		return "", nil
	}

	summaries, err := store.ListEntities()
	if err != nil {
		return "", fmt.Errorf("list entities: %w", err)
	}
	for _, summary := range summaries {
		if summary.Type == "character" && strings.EqualFold(summary.Name, name) {
			return summary.ID, nil
		}
	}

	return "", nil
}

// RepairPlayerIdentity reconciles a legacy manifest whose Player field holds a
// display name, rewriting game.yaml so every later read is exact. It returns the
// resolved ID, which is empty when the campaign has no player note.
func RepairPlayerIdentity(paths *core.PathResolver, store *storage.Store, manifest *core.GameManifest) (string, error) {
	id, err := ResolvePlayerID(store, manifest)
	if err != nil || id == "" {
		return id, err
	}

	if manifest.Player == id && manifest.PlayerName != "" {
		return id, nil
	}

	if manifest.PlayerName == "" && manifest.Player != "" && manifest.Player != id {
		manifest.PlayerName = manifest.Player
	}
	manifest.Player = id

	data, err := yaml.Marshal(manifest)
	if err != nil {
		return "", fmt.Errorf("marshal game manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(paths.GameDir(manifest.ID), "game.yaml"), data, 0644); err != nil {
		return "", fmt.Errorf("write game.yaml: %w", err)
	}
	return id, nil
}
```

- [x] **Step 6: Run the resolver test to verify it passes**

Run: `go test -run TestResolvePlayerIDReadsALegacyDisplayName -v ./pkg/engine/`
Expected: PASS.

- [x] **Step 7: Update the existing callers that no longer compile**

In `pkg/engine/game_test.go`, replace each `InitGame(paths, "campaign-0N", "d20-test", "fantasy-realm", name)` with:

```go
	InitGame(paths, InitOptions{
		GameID:     "campaign-01",
		SystemID:   "d20-test",
		WorldID:    "fantasy-realm",
		PlayerName: "Sean",
	})
```

Use the matching game ID and player name at each site (`campaign-01`/`Sean`, `campaign-02`/`Sean O'Neill`, `campaign-03`/`Sean`).

Update the assertion in `TestGameInitAndLoad`:

```go
	if session.Manifest.Player != "sean" {
		t.Errorf("expected the player entity ID sean, got %q", session.Manifest.Player)
	}
	if session.Manifest.PlayerName != "Sean" {
		t.Errorf("expected the player display name Sean, got %q", session.Manifest.PlayerName)
	}
```

In `pkg/gui/turn_test.go`, replace the call at line 44 with:

```go
	session, err := engine.InitGame(paths, engine.InitOptions{
		GameID:     "campaign-01",
		SystemID:   "freeform",
		WorldID:    "harbour-realm",
		PlayerName: "Sean",
	})
```

- [x] **Step 8: Use the resolver in `ResolveStartLocation`**

In `pkg/engine/startlocation.go`, replace the direct player lookup:

```go
	if id, err := ResolvePlayerID(store, manifest); err == nil && id != "" {
		if player, err := store.GetEntity(id); err == nil && player != nil {
			// existing body that reads player.Location / wikilinks
		}
	}
```

Keep the body that inspects the player's location and wikilinks exactly as it is; only the lookup changes.

- [x] **Step 9: Use the resolver in the GUI service**

In `pkg/gui/service.go`, `GetGameState` currently reads the file by `gameManifest.Player`:

```go
	playerID, err := engine.ResolvePlayerID(s.storeOrNil(gameID), gameManifest)
	if err != nil {
		return nil, fmt.Errorf("resolve player: %w", err)
	}
	if playerID == "" {
		return nil, fmt.Errorf("campaign %q has no player note", gameID)
	}
	playerFile := filepath.Join(gameDir, "entities", playerID+".md")
```

Add the small helper next to `store`:

```go
// storeOrNil opens a campaign's index, returning nil rather than an error so a
// caller that can fall back does not have to branch on the error value.
func (s *Service) storeOrNil(gameID string) *storage.Store {
	store, err := s.store(gameID)
	if err != nil {
		return nil
	}
	return store
}
```

In `prepareTurn`, after `store, err := s.store(gameID)` succeeds and before the orchestrator is built:

```go
	playerID := manifest.Player
	if resolved, err := engine.RepairPlayerIdentity(s.resolver, store, manifest); err == nil && resolved != "" {
		playerID = resolved
	}
```

Then use `playerID` instead of `manifest.Player` in the bridge and orchestrator:

```go
	jsEngine := rules.NewJSEngine(rules.NewHostBridge(store, timeline, playerID))
	...
	orchestrator := engine.NewTurnOrchestrator(store, timeline, jsEngine, router, startLocation, playerID)
```

- [x] **Step 10: Use the resolver in the TUI**

In `cmd/localrpg/play.go`, after the entity sync and before the bridge:

```go
	playerID := manifest.Player
	if resolved, err := engine.RepairPlayerIdentity(paths, store, manifest); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not reconcile player identity: %v\n", err)
	} else if resolved != "" {
		playerID = resolved
	}
```

Replace `manifest.Player` with `playerID` in `rules.NewHostBridge(...)` and `engine.NewTurnOrchestrator(...)`.

- [x] **Step 11: Add a GUI regression test**

Append to `pkg/gui/service_test.go`:

```go
func TestGetGameStateFindsALegacyDisplayNamePlayer(t *testing.T) {
	gameID, svc := setupTestGame(t)

	// Rewrite the manifest the way a pre-player_name build wrote it: the display
	// name in player:, the note still named by its slug.
	manifestPath := filepath.Join(svc.GetResolver().GameDir(gameID), "game.yaml")
	legacy := "id: test-campaign\nname: Test Campaign\nsystem: core-d20\nworld: shadow-realm\nplayer: Elena Nightshade\n"
	if err := os.WriteFile(manifestPath, []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}

	state, err := svc.GetGameState(context.Background(), gameID)
	if err != nil {
		t.Fatalf("GetGameState failed for a legacy manifest: %v", err)
	}
	if state.Player.Name != "Elena Nightshade" {
		t.Errorf("Player.Name = %q, want Elena Nightshade", state.Player.Name)
	}
}
```

Note: `setupTestGame` names the player note `player-elena.md` while its frontmatter reads `name: Elena Nightshade`. The resolver's name-match fallback finds it, so the assertion holds; no fixture change is needed.

- [x] **Step 12: Run the engine and GUI suites**

Run: `go test -count=1 ./pkg/engine/ ./pkg/gui/ && go vet ./...`
Expected: PASS and clean vet.

- [x] **Step 13: Commit**

```bash
git add pkg/core/types.go pkg/engine/player.go pkg/engine/player_test.go pkg/engine/game.go pkg/engine/game_test.go pkg/engine/startlocation.go pkg/gui/service.go pkg/gui/service_test.go pkg/gui/turn_test.go cmd/localrpg/play.go
git commit -m "fix(engine): resolve the protagonist across legacy player identifiers"
```

---

### Task 4: Persist the campaign title and resume the latest campaign

**Files:**
- Modify: `pkg/engine/game.go`
- Modify: `pkg/gui/service.go`
- Modify: `pkg/gui/service_test.go`

**Interfaces:**
- Consumes: `engine.InitOptions` (Task 3)
- Produces: `engine.InitOptions.Name string`

- [x] **Step 1: Write the failing test**

Append to `pkg/gui/service_test.go`:

```go
func TestCampaignTitleIsPersistedAndLatestIsFirst(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(tmpDir)

	sysDir := svc.GetResolver().SystemDir("freeform")
	if err := os.MkdirAll(sysDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sysDir, "system.yaml"), []byte("id: freeform\nname: Freeform\nversion: 1.0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	worldDir := svc.GetResolver().WorldDir("harbour-realm")
	if err := os.MkdirAll(worldDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worldDir, "world.yaml"), []byte("id: harbour-realm\nname: Harbour Realm\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.CreateGame(context.Background(), CreateGameRequestDTO{
		Name:       "The Salt Road",
		SystemID:   "freeform",
		WorldID:    "harbour-realm",
		PlayerName: "Elena Nightshade",
	}); err != nil {
		t.Fatalf("CreateGame failed: %v", err)
	}

	games, err := svc.ListGames(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 1 {
		t.Fatalf("expected 1 campaign, got %d", len(games))
	}
	if games[0].Name != "The Salt Road" {
		t.Errorf("Name = %q, want the entered title", games[0].Name)
	}
	if games[0].PlayerName != "Elena Nightshade" {
		t.Errorf("PlayerName = %q, want the display name", games[0].PlayerName)
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

Run: `go test -run TestCampaignTitleIsPersistedAndLatestIsFirst -v ./pkg/gui/`
Expected: FAIL — `unknown field Name` on `CreateGameRequestDTO`, or `Name` equals the slug.

- [x] **Step 3: Add the title to the request and the init options**

In `pkg/gui/types.go`, extend `CreateGameRequestDTO` (around line 118):

```go
	PlayerName string `json:"player_name"`
```

Confirm the struct already has `Name`; if it does, no change is needed there.

In `pkg/engine/game.go`, add `Name` to `InitOptions`:

```go
type InitOptions struct {
	GameID        string
	Name          string
	SystemID      string
	WorldID       string
	PlayerName    string
	PlayerDetails string
}
```

Use it in the manifest with a slug fallback:

```go
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = gameID
	}

	manifest := &core.GameManifest{
		ID:         gameID,
		Name:       name,
		SystemID:   systemID,
		WorldID:    worldID,
		Player:     playerID,
		PlayerName: opts.PlayerName,
		Settings:   make(map[string]interface{}),
	}
```

- [x] **Step 4: Pass the title and return the display name from `CreateGame`**

In `pkg/gui/service.go`, replace the `InitGame` call:

```go
	session, err := engine.InitGame(s.resolver, engine.InitOptions{
		GameID:     gameID,
		Name:       req.Name,
		SystemID:   req.SystemID,
		WorldID:    req.WorldID,
		PlayerName: req.PlayerName,
	})
```

- [x] **Step 5: Sort the campaign list by most recently played**

Add `"sort"` to `pkg/gui/service.go`'s imports.

At the end of `ListGames`, before `return summaries, nil`:

```go
	sort.SliceStable(summaries, func(i, j int) bool {
		return summaries[i].LastPlayed > summaries[j].LastPlayed
	})
	return summaries, nil
```

Populate the display name in the summary:

```go
		playerName := m.PlayerName
		if playerName == "" {
			playerName = m.Player
		}

		summaries = append(summaries, GameSummaryDTO{
			ID:         gameID,
			Name:       name,
			SystemID:   m.SystemID,
			WorldID:    m.WorldID,
			PlayerName: playerName,
			TurnCount:  turnCount,
			LastPlayed: lastPlayed,
		})
```

- [x] **Step 6: Run the GUI suite to verify it passes**

Run: `go test -count=1 ./pkg/gui/`
Expected: PASS.

- [x] **Step 7: Run the full verification gate**

Run: `go vet ./... && go test -count=1 ./... && (cd frontend && npx tsc --noEmit)`
Expected: all clean. No frontend files changed, but the repo gate is the same command.

- [x] **Step 8: Commit**

```bash
git add pkg/engine/game.go pkg/gui/service.go pkg/gui/service_test.go
git commit -m "fix(gui): keep the campaign title and resume the latest campaign"
```

---

## Self-Review

**Spec coverage (increment 1: "Unblock play"):**

- Spec §5.1 send configured generation parameters → Task 1.
- Spec §5.2 propagate completion (`finish_reason`) → Task 1.
- Spec §5.3 deadlines (idle watchdog + wall clock) → Task 2.
- Spec §5.4 status/progress events and §5.5 client timeouts → deferred to increment 4, which is where the streaming UX changes; the spec's increment list puts them there.
- Spec §8.1 player identity fix and legacy migration → Task 3.
- Spec §8.4 campaign title and latest-first resume → Task 4.
- Spec §8.2 protagonist details: `InitOptions.PlayerDetails` and `ensurePlayerNote` accept them in Task 3; the wizard field that supplies them is increment 2. The plan ships the plumbing without the UI, which is why the parameter exists now rather than forcing a second signature change.
- The Characters-drawer recovery action (§8.3) is a frontend change and belongs to a later increment; Task 3's resolver removes the observed "no character loaded" cause for correctly created campaigns.

**Placeholders:** none. Every step names a file, shows the code, and gives a runnable command and expected result. Task 3 Step 11 intentionally asks the implementer to read `pkg/gui/service_test.go:31-43` before asserting a name, because the fixture's note filename determines the expected value; that is a verification instruction with a concrete fallback, not a placeholder.

**Type consistency:** `GenerationOptions`, `StreamChunk.FinishReason`, `NewHTTPProviderWithOptions`, `NewCLIProviderWithOptions`, `InitOptions`, `ResolvePlayerID`, `RepairPlayerIdentity`, `ErrGenerationStalled`, `SetChunkTimeout`, `Config.TurnTimeout`, `Config.ChunkTimeout`, and `GameManifest.PlayerName` are each defined once and used with the same signatures throughout.
