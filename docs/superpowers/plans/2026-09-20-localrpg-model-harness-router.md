# LocalRPG Model & Harness Router Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the model execution and harness routing layer for LocalRPG: external CLI harnesses (`claude`, `codex`, `agy`, `opencode`), local Ollama & OpenAI-compatible HTTP streaming, role-based provider routing with failover, the 4-layer living world context assembler, and the background entity extractor.

**Architecture:** A unified `ModelProvider` interface abstracts streaming across CLI subprocesses, local HTTP servers (Ollama/llama.cpp), and cloud endpoints. A role-based router dispatches distinct sub-tasks (`gm`, `extractor`, `world_sim`) to configured engines. The 4-layer context assembler gathers immediate scene entities, living world narrative arcs, 1-hop graph neighbors, and vector/historical facts into a prioritized prompt. The extractor parses model responses to update Markdown entity notes in real time.

**Tech Stack:** Go 1.27, standard library (`os/exec`, `net/http`, `context`), `github.com/darkliquid/localrpg/pkg/core`, `github.com/darkliquid/localrpg/pkg/entity`, `github.com/darkliquid/localrpg/pkg/storage`.

---

### File Structure Map

```text
LocalRPG/
├── cmd/
│   └── localrpg/
│       ├── main.go               # Updated with "prompt" subcommand
│       ├── prompt.go             # CLI test runner for model harnesses
│       └── prompt_test.go        # CLI prompt integration tests
├── pkg/
│   └── harness/
│       ├── types.go              # ModelProvider interface, request/response structs
│       ├── cli_provider.go       # CLI subprocess runner (claude, codex, agy, custom)
│       ├── cli_provider_test.go  # CLI streaming & execution tests
│       ├── http_provider.go      # Local Ollama & OpenAI-compatible SSE client
│       ├── http_provider_test.go # HTTP streaming & error handling tests
│       ├── router.go             # Role-based provider router & failover manager
│       ├── router_test.go        # Role dispatch & fallback tests
│       ├── context.go            # 4-layer context assembler (living world + graph)
│       ├── context_test.go       # Context assembly & token budgeting tests
│       ├── extractor.go          # Background entity & fact extractor
│       └── extractor_test.go     # Structured extraction & markdown sync tests
```

---

### Task 1: Core Harness Types & Streaming Interface

**Files:**
- Create: `pkg/harness/types.go`
- Test: `pkg/harness/types_test.go`

- [x] **Step 1: Write the failing test for Harness Types**

```go
// pkg/harness/types_test.go
package harness

import (
	"testing"
)

func TestProviderConfigValidation(t *testing.T) {
	cfg := ProviderConfig{
		Type:     "cli",
		Command:  "claude",
		Args:     []string{"--print"},
		Endpoint: "",
	}

	if cfg.Type != "cli" || cfg.Command != "claude" {
		t.Errorf("unexpected config: %+v", cfg)
	}

	roleCfg := RoleRoutingConfig{
		Roles: map[string]ProviderConfig{
			"gm": {Type: "cli", Command: "claude"},
			"extractor": {Type: "http", Endpoint: "http://localhost:11434", Model: "qwen2.5:7b"},
		},
	}

	if len(roleCfg.Roles) != 2 {
		t.Errorf("expected 2 roles, got %d", len(roleCfg.Roles))
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/harness/... -v`  
Expected: FAIL (package/harness not defined)

- [x] **Step 3: Implement Core Harness Types**

Write `pkg/harness/types.go`:
```go
package harness

import (
	"context"
)

type StreamChunk struct {
	Text  string
	Done  bool
	Error error
}

type GenerateRequest struct {
	Prompt      string                 `json:"prompt"`
	System      string                 `json:"system,omitempty"`
	Temperature float64                `json:"temperature,omitempty"`
	MaxTokens   int                    `json:"max_tokens,omitempty"`
	Extra       map[string]interface{} `json:"extra,omitempty"`
}

type GenerateResponse struct {
	Text string `json:"text"`
}

type ModelProvider interface {
	ID() string
	Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error)
	Stream(ctx context.Context, req GenerateRequest, out chan<- StreamChunk) error
}

type ProviderConfig struct {
	Type        string   `yaml:"type"` // "cli", "http", "mock"
	Command     string   `yaml:"command,omitempty"`
	Args        []string `yaml:"args,omitempty"`
	Endpoint    string   `yaml:"endpoint,omitempty"`
	Model       string   `yaml:"model,omitempty"`
	APIKey      string   `yaml:"api_key,omitempty"`
	Temperature float64  `yaml:"temperature,omitempty"`
}

type RoleRoutingConfig struct {
	Roles map[string]ProviderConfig `yaml:"roles"`
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/harness/... -v`  
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/harness/types.go pkg/harness/types_test.go
git commit -m "feat(harness): define core ModelProvider interface and role configs"
```

---

### Task 2: CLI Subprocess Harness Runner

**Files:**
- Create: `pkg/harness/cli_provider.go`
- Test: `pkg/harness/cli_provider_test.go`

- [x] **Step 1: Write failing test for CLI Provider**

```go
// pkg/harness/cli_provider_test.go
package harness

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestCLIProviderExecution(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use standard echo/sh command to test CLI harness runner
	provider := NewCLIProvider("test-cli", "sh", []string{"-c", "echo 'Hello from CLI harness: ' $1", "--"})

	req := GenerateRequest{
		Prompt: "Adventurer",
	}

	res, err := provider.Generate(ctx, req)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if !strings.Contains(res.Text, "Hello from CLI harness: Adventurer") {
		t.Errorf("unexpected output: %q", res.Text)
	}
}

func TestCLIProviderStreaming(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	provider := NewCLIProvider("stream-cli", "sh", []string{"-c", "printf 'Line1 '; sleep 0.05; printf 'Line2'", "--"})

	req := GenerateRequest{Prompt: "test"}
	out := make(chan StreamChunk, 10)

	errCh := make(chan error, 1)
	go func() {
		errCh <- provider.Stream(ctx, req, out)
	}()

	var received []string
	for chunk := range out {
		if chunk.Error != nil {
			t.Fatalf("stream error: %v", chunk.Error)
		}
		if chunk.Text != "" {
			received = append(received, chunk.Text)
		}
	}

	if err := <-errCh; err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	full := strings.Join(received, "")
	if full != "Line1 Line2" {
		t.Errorf("expected 'Line1 Line2', got %q", full)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/harness/... -v -run TestCLIProviderExecution`  
Expected: FAIL (NewCLIProvider not defined)

- [x] **Step 3: Implement CLI Subprocess Provider**

Write `pkg/harness/cli_provider.go`:
```go
package harness

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

type CLIProvider struct {
	id      string
	command string
	args    []string
}

func NewCLIProvider(id, command string, args []string) *CLIProvider {
	return &CLIProvider{
		id:      id,
		command: command,
		args:    args,
	}
}

func (c *CLIProvider) ID() string {
	return c.id
}

func (c *CLIProvider) buildCmd(ctx context.Context, req GenerateRequest) *exec.Cmd {
	args := append([]string{}, c.args...)
	args = append(args, req.Prompt)

	cmd := exec.CommandContext(ctx, c.command, args...)
	if req.System != "" {
		cmd.Env = append(cmd.Environ(), "SYSTEM_PROMPT="+req.System)
	}
	return cmd
}

func (c *CLIProvider) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	cmd := c.buildCmd(ctx, req)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("cli provider %q failed: %w (stderr: %s)", c.id, err, stderr.String())
	}

	return &GenerateResponse{
		Text: strings.TrimSpace(stdout.String()),
	}, nil
}

func (c *CLIProvider) Stream(ctx context.Context, req GenerateRequest, out chan<- StreamChunk) error {
	defer close(out)

	cmd := c.buildCmd(ctx, req)
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start cli command: %w", err)
	}

	reader := bufio.NewReader(stdoutPipe)
	buf := make([]byte, 256)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			out <- StreamChunk{Text: string(buf[:n])}
		}
		if err != nil {
			if err != io.EOF {
				out <- StreamChunk{Error: err}
			}
			break
		}
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("cli process finished with error: %w (stderr: %s)", err, stderr.String())
	}

	out <- StreamChunk{Done: true}
	return nil
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/harness/... -v -run TestCLIProvider`  
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/harness/cli_provider.go pkg/harness/cli_provider_test.go
git commit -m "feat(harness): implement CLI subprocess harness runner with streaming"
```

---

### Task 3: Local HTTP & OpenAI-Compatible Client (Ollama / vLLM)

**Files:**
- Create: `pkg/harness/http_provider.go`
- Test: `pkg/harness/http_provider_test.go`

- [x] **Step 1: Write failing test with mock HTTP server**

```go
// pkg/harness/http_provider_test.go
package harness

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPProviderStreaming(t *testing.T) {
	// Mock OpenAI/Ollama SSE server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected flusher")
		}

		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Once upon \"}}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"a time.\"}}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	provider := NewHTTPProvider("mock-ollama", server.URL, "llama3", "")
	req := GenerateRequest{Prompt: "Tell a story"}

	out := make(chan StreamChunk, 10)
	errCh := make(chan error, 1)
	go func() {
		errCh <- provider.Stream(ctx, req, out)
	}()

	var received []string
	for chunk := range out {
		if chunk.Text != "" {
			received = append(received, chunk.Text)
		}
	}

	if err := <-errCh; err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	result := strings.Join(received, "")
	if result != "Once upon a time." {
		t.Errorf("expected 'Once upon a time.', got %q", result)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/harness/... -v -run TestHTTPProviderStreaming`  
Expected: FAIL (NewHTTPProvider not defined)

- [x] **Step 3: Implement HTTP Provider**

Write `pkg/harness/http_provider.go`:
```go
package harness

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type HTTPProvider struct {
	id       string
	endpoint string
	model    string
	apiKey   string
	client   *http.Client
}

func NewHTTPProvider(id, endpoint, model, apiKey string) *HTTPProvider {
	return &HTTPProvider{
		id:       id,
		endpoint: strings.TrimRight(endpoint, "/"),
		model:    model,
		apiKey:   apiKey,
		client:   &http.Client{},
	}
}

func (h *HTTPProvider) ID() string {
	return h.id
}

type openAIChatRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

func (h *HTTPProvider) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	out := make(chan StreamChunk, 20)
	var sb strings.Builder

	errCh := make(chan error, 1)
	go func() {
		errCh <- h.Stream(ctx, req, out)
	}()

	for chunk := range out {
		if chunk.Error != nil {
			return nil, chunk.Error
		}
		sb.WriteString(chunk.Text)
	}

	if err := <-errCh; err != nil {
		return nil, err
	}

	return &GenerateResponse{Text: sb.String()}, nil
}

func (h *HTTPProvider) Stream(ctx context.Context, req GenerateRequest, out chan<- StreamChunk) error {
	defer close(out)

	messages := make([]openAIMessage, 0, 2)
	if req.System != "" {
		messages = append(messages, openAIMessage{Role: "system", Content: req.System})
	}
	messages = append(messages, openAIMessage{Role: "user", Content: req.Prompt})

	payload := openAIChatRequest{
		Model:    h.model,
		Messages: messages,
		Stream:   true,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	url := h.endpoint
	if !strings.HasSuffix(url, "/chat/completions") && !strings.HasSuffix(url, "/v1") {
		url = url + "/v1/chat/completions"
	} else if strings.HasSuffix(url, "/v1") {
		url = url + "/chat/completions"
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("new http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if h.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+h.apiKey)
	}

	resp, err := h.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http error %s from %s", resp.Status, url)
	}

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
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			out <- StreamChunk{Text: chunk.Choices[0].Delta.Content}
		}
	}

	out <- StreamChunk{Done: true}
	return scanner.Err()
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/harness/... -v -run TestHTTPProviderStreaming`  
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/harness/http_provider.go pkg/harness/http_provider_test.go
git commit -m "feat(harness): implement local HTTP and OpenAI-compatible SSE streaming provider"
```

---

### Task 4: Role-Based Router & Fallback Dispatcher

**Files:**
- Create: `pkg/harness/router.go`
- Test: `pkg/harness/router_test.go`

- [x] **Step 1: Write failing test for Role Router**

```go
// pkg/harness/router_test.go
package harness

import (
	"context"
	"testing"
)

type mockProvider struct {
	id     string
	output string
	fail   bool
}

func (m *mockProvider) ID() string { return m.id }
func (m *mockProvider) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	if m.fail {
		return nil, context.DeadlineExceeded
	}
	return &GenerateResponse{Text: m.output}, nil
}
func (m *mockProvider) Stream(ctx context.Context, req GenerateRequest, out chan<- StreamChunk) error {
	defer close(out)
	if m.fail {
		out <- StreamChunk{Error: context.DeadlineExceeded}
		return context.DeadlineExceeded
	}
	out <- StreamChunk{Text: m.output, Done: true}
	return nil
}

func TestRouterRoleDispatchAndFallback(t *testing.T) {
	ctx := context.Background()
	router := NewRouter()

	primary := &mockProvider{id: "primary-claude", fail: true}
	fallback := &mockProvider{id: "fallback-ollama", output: "Fallback story response"}

	router.RegisterProvider(primary)
	router.RegisterProvider(fallback)

	router.AssignRole("gm", "primary-claude")
	router.SetFallback("gm", "fallback-ollama")

	res, err := router.GenerateForRole(ctx, "gm", GenerateRequest{Prompt: "hello"})
	if err != nil {
		t.Fatalf("GenerateForRole failed: %v", err)
	}

	if res.Text != "Fallback story response" {
		t.Errorf("expected fallback response, got %q", res.Text)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/harness/... -v -run TestRouterRoleDispatchAndFallback`  
Expected: FAIL (NewRouter not defined)

- [x] **Step 3: Implement Router**

Write `pkg/harness/router.go`:
```go
package harness

import (
	"context"
	"fmt"
	"sync"
)

type Router struct {
	mu        sync.RWMutex
	providers map[string]ModelProvider
	roleMap   map[string]string // role -> providerID
	fallbacks map[string]string // role -> fallback providerID
}

func NewRouter() *Router {
	return &Router{
		providers: make(map[string]ModelProvider),
		roleMap:   make(map[string]string),
		fallbacks: make(map[string]string),
	}
}

func (r *Router) RegisterProvider(p ModelProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.ID()] = p
}

func (r *Router) AssignRole(role, providerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.roleMap[role] = providerID
}

func (r *Router) SetFallback(role, fallbackProviderID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fallbacks[role] = fallbackProviderID
}

func (r *Router) GetProviderForRole(role string) (ModelProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	providerID, exists := r.roleMap[role]
	if !exists {
		return nil, fmt.Errorf("no provider assigned to role %q", role)
	}

	p, exists := r.providers[providerID]
	if !exists {
		return nil, fmt.Errorf("provider %q for role %q not registered", providerID, role)
	}
	return p, nil
}

func (r *Router) GenerateForRole(ctx context.Context, role string, req GenerateRequest) (*GenerateResponse, error) {
	primary, err := r.GetProviderForRole(role)
	if err != nil {
		return nil, err
	}

	res, err := primary.Generate(ctx, req)
	if err == nil {
		return res, nil
	}

	// Try fallback if available
	r.mu.RLock()
	fallbackID, hasFallback := r.fallbacks[role]
	var fallback ModelProvider
	if hasFallback {
		fallback = r.providers[fallbackID]
	}
	r.mu.RUnlock()

	if fallback != nil {
		return fallback.Generate(ctx, req)
	}

	return nil, fmt.Errorf("primary role %q failed: %w", role, err)
}

func (r *Router) StreamForRole(ctx context.Context, role string, req GenerateRequest, out chan<- StreamChunk) error {
	primary, err := r.GetProviderForRole(role)
	if err != nil {
		close(out)
		return err
	}

	// Attempt primary stream
	tempOut := make(chan StreamChunk, 20)
	errCh := make(chan error, 1)

	go func() {
		errCh <- primary.Stream(ctx, req, tempOut)
	}()

	firstChunk, ok := <-tempOut
	if !ok || (firstChunk.Error != nil) {
		// Fallback
		r.mu.RLock()
		fallbackID, hasFallback := r.fallbacks[role]
		var fallback ModelProvider
		if hasFallback {
			fallback = r.providers[fallbackID]
		}
		r.mu.RUnlock()

		if fallback != nil {
			return fallback.Stream(ctx, req, out)
		}
		close(out)
		return <-errCh
	}

	// Forward stream
	go func() {
		defer close(out)
		out <- firstChunk
		for chunk := range tempOut {
			out <- chunk
		}
	}()

	return <-errCh
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/harness/... -v -run TestRouterRoleDispatchAndFallback`  
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/harness/router.go pkg/harness/router_test.go
git commit -m "feat(harness): implement role-based model router with failover"
```

---

### Task 5: 4-Layer Context Assembler (Living World & Graph)

**Files:**
- Create: `pkg/harness/context.go`
- Test: `pkg/harness/context_test.go`

- [x] **Step 1: Write failing test for Context Assembler**

```go
// pkg/harness/context_test.go
package harness

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/storage"
)

func TestContextAssembler(t *testing.T) {
	tempDir := t.TempDir()
	store, err := storage.NewStore(filepath.Join(tempDir, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// 1. Scene location
	tavern := &entity.Entity{
		ID:        "alden-tavern",
		Name:      "Alden Tavern",
		Type:      "location",
		Body:      "A warm tavern smelling of ale.",
		Wikilinks: []string{"iron-pact"},
		Hash:      "hash-tavern",
	}
	store.SaveEntity(tavern)

	// 2. Active NPC
	npc := &entity.Entity{
		ID:       "lady-evelyn",
		Name:     "Lady Evelyn",
		Type:     "character",
		Location: "[[alden-tavern]]",
		Body:     "Guarded former lieutenant.",
		Hash:     "hash-evelyn",
	}
	store.SaveEntity(npc)

	// 3. Living World Arc
	arc := &entity.Entity{
		ID:   "arc-siege",
		Name: "The Iron Siege",
		Type: "arc",
		Body: "Food supplies are depleted in the lower quarter.",
		Hash: "hash-arc",
	}
	store.SaveEntity(arc)

	assembler := NewContextAssembler(store)
	ctxPrompt, err := assembler.AssembleContext("alden-tavern", "player", "I speak with Evelyn")
	if err != nil {
		t.Fatalf("AssembleContext failed: %v", err)
	}

	// Verify all layers are assembled
	if !strings.Contains(ctxPrompt, "Alden Tavern") {
		t.Errorf("missing location in context")
	}
	if !strings.Contains(ctxPrompt, "Lady Evelyn") {
		t.Errorf("missing NPC in context")
	}
	if !strings.Contains(ctxPrompt, "The Iron Siege") {
		t.Errorf("missing living world arc in context")
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/harness/... -v -run TestContextAssembler`  
Expected: FAIL (NewContextAssembler not defined)

- [x] **Step 3: Implement Context Assembler**

Write `pkg/harness/context.go`:
```go
package harness

import (
	"fmt"
	"strings"

	"github.com/darkliquid/localrpg/pkg/storage"
)

type ContextAssembler struct {
	store *storage.Store
}

func NewContextAssembler(store *storage.Store) *ContextAssembler {
	return &ContextAssembler{store: store}
}

func (c *ContextAssembler) AssembleContext(locationID, playerID, playerAction string) (string, error) {
	var sb strings.Builder

	// Layer 1: Immediate Scene Scope
	sb.WriteString("## IMMEDIATE SCENE\n")
	if loc, err := c.store.GetEntity(locationID); err == nil && loc != nil {
		sb.WriteString(fmt.Sprintf("**Current Location:** %s\n%s\n\n", loc.Name, loc.Body))
	}

	if player, err := c.store.GetEntity(playerID); err == nil && player != nil {
		sb.WriteString(fmt.Sprintf("**Player Character:** %s\n", player.Name))
		if player.State != nil {
			sb.WriteString(fmt.Sprintf("State: %+v\n\n", player.State.Raw()))
		}
	}

	// Layer 2: Living World Arcs & Background Agendas
	sb.WriteString("## LIVING WORLD & BACKGROUND ARCS\n")
	// Query entities of type "arc"
	rows, err := c.store.GetEdgesFrom(locationID)
	if err == nil {
		for _, edge := range rows {
			if ent, err := c.store.GetEntity(edge.TargetID); err == nil && ent.Type == "arc" {
				sb.WriteString(fmt.Sprintf("### Arc: %s\n%s\n\n", ent.Name, ent.Body))
			}
		}
	}

	// Layer 3: Present Actors
	sb.WriteString("## PRESENT CHARACTERS & NOTABLE BEINGS\n")
	edges, err := c.store.GetEdgesFrom(locationID)
	if err == nil {
		for _, edge := range edges {
			if ent, err := c.store.GetEntity(edge.TargetID); err == nil && ent.Type == "character" {
				sb.WriteString(fmt.Sprintf("- **%s**: %s\n", ent.Name, ent.Body))
			}
		}
	}

	sb.WriteString("\n## PLAYER ACTION\n")
	sb.WriteString(playerAction + "\n")

	return sb.String(), nil
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/harness/... -v -run TestContextAssembler`  
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/harness/context.go pkg/harness/context_test.go
git commit -m "feat(harness): implement 4-layer living world context assembler"
```

---

### Task 6: Background Entity & Fact Extractor

**Files:**
- Create: `pkg/harness/extractor.go`
- Test: `pkg/harness/extractor_test.go`

- [x] **Step 1: Write failing test for Entity Extractor**

```go
// pkg/harness/extractor_test.go
package harness

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/storage"
)

func TestExtractAndSyncEntities(t *testing.T) {
	tempDir := t.TempDir()
	store, err := storage.NewStore(filepath.Join(tempDir, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Mock model provider that outputs structured JSON entity discovery
	mockModel := &mockProvider{
		id: "extractor-model",
		output: `[
			{
				"id": "garrick-the-fence",
				"name": "Garrick the Fence",
				"type": "character",
				"location": "[[alden-tavern]]",
				"body": "A shadowy broker dealing in stolen trinkets."
			}
		]`,
	}

	extractor := NewEntityExtractor(mockModel, store)
	count, err := extractor.ExtractFromTurn(context.Background(), "You meet Garrick in the corner of the tavern.")
	if err != nil {
		t.Fatalf("ExtractFromTurn failed: %v", err)
	}

	if count != 1 {
		t.Errorf("expected 1 entity extracted, got %d", count)
	}

	// Verify Garrick is in storage
	garrick, err := store.GetEntity("garrick-the-fence")
	if err != nil || garrick.Name != "Garrick the Fence" {
		t.Errorf("expected Garrick in store, got %+v", garrick)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/harness/... -v -run TestExtractAndSyncEntities`  
Expected: FAIL (NewEntityExtractor not defined)

- [x] **Step 3: Implement Entity Extractor**

Write `pkg/harness/extractor.go`:
```go
package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/storage"
)

type ExtractedEntity struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Location string `json:"location,omitempty"`
	Faction  string `json:"faction,omitempty"`
	Body     string `json:"body"`
}

type EntityExtractor struct {
	model ModelProvider
	store *storage.Store
}

func NewEntityExtractor(model ModelProvider, store *storage.Store) *EntityExtractor {
	return &EntityExtractor{
		model: model,
		store: store,
	}
}

const extractorSystemPrompt = `You are a world-state extractor. Read the narrative turn and return a JSON list of any newly discovered or updated characters, locations, items, factions, or plot arcs. Format:
[
  {
    "id": "kebab-case-id",
    "name": "Full Name",
    "type": "character|location|item|faction|arc",
    "location": "[[Optional-Location]]",
    "body": "Description and known facts."
  }
]
If nothing new is discovered, return []`

func (e *EntityExtractor) ExtractFromTurn(ctx context.Context, narrativeOutput string) (int, error) {
	req := GenerateRequest{
		System: extractorSystemPrompt,
		Prompt: narrativeOutput,
	}

	res, err := e.model.Generate(ctx, req)
	if err != nil {
		return 0, fmt.Errorf("extractor model failed: %w", err)
	}

	cleaned := strings.TrimSpace(res.Text)
	if idx := strings.Index(cleaned, "["); idx != -1 {
		cleaned = cleaned[idx:]
	}
	if idx := strings.LastIndex(cleaned, "]"); idx != -1 {
		cleaned = cleaned[:idx+1]
	}

	var extracted []ExtractedEntity
	if err := json.Unmarshal([]byte(cleaned), &extracted); err != nil {
		return 0, fmt.Errorf("parse extracted json %q: %w", cleaned, err)
	}

	savedCount := 0
	for _, raw := range extracted {
		if raw.ID == "" || raw.Name == "" {
			continue
		}

		ent := &entity.Entity{
			ID:        raw.ID,
			Name:      raw.Name,
			Type:      raw.Type,
			Location:  raw.Location,
			Faction:   raw.Faction,
			Body:      raw.Body,
			Wikilinks: make([]string, 0),
			Hash:      fmt.Sprintf("extracted-%s", raw.ID),
		}

		if err := e.store.SaveEntity(ent); err == nil {
			savedCount++
		}
	}

	return savedCount, nil
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/harness/... -v -run TestExtractAndSyncEntities`  
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/harness/extractor.go pkg/harness/extractor_test.go
git commit -m "feat(harness): implement background entity extractor and sync pipeline"
```

---

### Task 7: CLI Test Harness Command (`localrpg prompt`)

**Files:**
- Create: `cmd/localrpg/prompt.go`
- Modify: `cmd/localrpg/main.go`
- Test: `cmd/localrpg/prompt_test.go`

- [x] **Step 1: Write integration test for CLI Prompt command**

```go
// cmd/localrpg/prompt_test.go
package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestCLIPromptCommand(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "prompt", "--cmd", "echo", "Hello adventurer")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command failed: %v, output: %s", err, string(out))
	}

	if !strings.Contains(string(out), "Hello adventurer") {
		t.Errorf("expected prompt output, got: %s", string(out))
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/localrpg/... -v -run TestCLIPromptCommand`  
Expected: FAIL (subcommand prompt not handled)

- [x] **Step 3: Implement CLI Prompt Subcommand**

Write `cmd/localrpg/prompt.go`:
```go
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/darkliquid/localrpg/pkg/harness"
)

func handlePromptCommand(args []string) {
	fs := flag.NewFlagSet("prompt", flag.ExitOnError)
	cliCmd := fs.String("cmd", "", "CLI command harness to use (e.g. echo, claude, agy)")
	httpEndpoint := fs.String("endpoint", "", "HTTP endpoint for Ollama / OpenAI")
	model := fs.String("model", "llama3", "Model name for HTTP provider")

	fs.Parse(args)
	promptText := fs.Arg(0)
	if promptText == "" {
		fmt.Fprintln(os.Stderr, "Usage: localrpg prompt [--cmd <command> | --endpoint <url>] <prompt text>")
		os.Exit(1)
	}

	var provider harness.ModelProvider
	if *cliCmd != "" {
		provider = harness.NewCLIProvider("cli-harness", *cliCmd, []string{})
	} else if *httpEndpoint != "" {
		provider = harness.NewHTTPProvider("http-harness", *httpEndpoint, *model, "")
	} else {
		// Default to echo
		provider = harness.NewCLIProvider("default-echo", "echo", []string{})
	}

	ctx := context.Background()
	res, err := provider.Generate(ctx, harness.GenerateRequest{Prompt: promptText})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Prompt failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(res.Text)
}
```

Update `cmd/localrpg/main.go` to dispatch `case "prompt": handlePromptCommand(args[1:])`.

- [x] **Step 4: Run all package tests across project**

Run: `go test -count=1 ./... -v`  
Expected: All package tests PASS

- [x] **Step 5: Commit**

```bash
git add cmd/localrpg/prompt.go cmd/localrpg/main.go cmd/localrpg/prompt_test.go
git commit -m "feat(cli): add 'prompt' subcommand to test model harnesses"
```
