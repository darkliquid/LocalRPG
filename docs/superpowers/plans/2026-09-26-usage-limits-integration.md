# Usage & Limits Integration Implementation Plan

> **Status:** In progress as of 2026-09-27. Tasks 1, 2, and 3 are done, including image and transcription recording. Studio work (asset previews, world assets, provider tests) lands in a shared ledger at `<cache>/usage.db` under the `global` scope; game assets record against their campaign; and a creation flow can hold spend under a `pending:<token>` scope with `RecordUsageDeferred`, then `CommitDeferredUsage(token, gameID)` on creation or `DiscardDeferredUsage` to leave it shared. `CreateGame` already commits the new campaign's deferred rows using the campaign id as the token, so a client that generates previews for a new campaign should pass that id as the token. Task 4 (rate-limit blocks and funds failures), Task 5 (usage API), and Task 6 (Usage UI, which must surface the shared scope and per-campaign drilldown) remain. Branch: `feat/usage-limits-recording`.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make providers report usage, record it with cost into the ledger, enforce provider+role rate-limit blocks, surface funds failures, and add the Usage UI.

**Architecture:** Providers parse the usage fields their APIs already return (with two request changes), the media layer reports characters/tokens, the service owns a per-turn `UsageContext` that stamps records, and the GUI exposes usage and limit state plus inline status. Consumes the foundation plan.

**Tech Stack:** Go 1.27 (stdlib `testing`), React 19 frontend. No new dependencies.

**Spec:** `docs/superpowers/specs/2026-09-26-usage-cost-and-provider-limits-design.md`

**Requires:** `docs/superpowers/plans/2026-09-26-usage-limits-foundation.md` (Tasks 1-5) merged first.

## Global Constraints

- Go tests use only `testing` and `t.TempDir()`; no testify, no mocks.
- Use `interface{}`, never `any`; wrap errors; `go vet ./...` clean.
- No new dependencies; integer micros for money.
- Conventional Commits with a scope; subject under 72 chars.
- Verification: `mise run test`, `mise run lint`, `mise run build`.
- Known pre-existing `pkg/gui` TempDir flake — re-run before treating a failure as real.

---

### Task 1: LLM and embedding providers report usage

**Files:**
- Modify: `pkg/provider/openaichat/http.go` (request body ~line 220-228, chunk struct ~line 118-134, parse loop ~line 328-347, `Generate`/`Stream` ~line 175-201)
- Modify: `pkg/provider/geminillm/provider.go` (genai `Generate` ~line 465-508, `GenerateContentStream` ~line 510-578)
- Modify: `pkg/provider/openaiembedding/openai.go` (response struct ~line 97-102, read ~line 147-160)
- Test: `pkg/provider/openaichat/usage_test.go`, `pkg/provider/geminillm/usage_test.go`, `pkg/provider/openaiembedding/usage_test.go`

**Interfaces:**
- Consumes: `harness.Usage`, `harness.StreamChunk.Usage`, `harness.GenerateResponse.Usage`.
- Produces: providers populate usage; no signature changes.

- [ ] **Step 1: Write the failing tests**

For `openaichat`, build a stream from a `httptest.Server` that emits two SSE chunks: one text chunk with no usage and a final chunk `{"choices":[],"usage":{"prompt_tokens":11,"completion_tokens":4,"total_tokens":15}}`. Assert the last `StreamChunk` has `Usage.InputTokens == 11` and `Usage.OutputTokens == 4`, and that the request body contained `"stream_options":{"include_usage":true}`.

```go
func TestStreamReportsTokenUsage(t *testing.T) {
	// server records body, returns the two-chunk SSE above
	// ... construct provider with endpoint = server.URL
	chunks := make(chan harness.StreamChunk, 8)
	if err := provider.Stream(context.Background(), harness.GenerateRequest{Prompt: "hi"}, chunks); err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var last *harness.Usage
	for chunk := range chunks {
		if chunk.Usage != nil {
			last = chunk.Usage
		}
	}
	if last == nil || last.InputTokens != 11 || last.OutputTokens != 4 {
		t.Fatalf("usage = %+v, want 11/4", last)
	}
	if !strings.Contains(recordedBody, `"include_usage":true`) {
		t.Fatalf("request omitted stream_options: %s", recordedBody)
	}
}
```

For `geminillm`, a unit test on the usage mapping function: `usageFromMetadata(&genai.GenerateContentResponse{UsageMetadata: &genai.GenerateContentResponseUsageMetadata{PromptTokenCount: 7, CandidatesTokenCount: 3}})` returns `Usage{InputTokens: 7, OutputTokens: 3}`.

For `openaiembedding`, a test that a response body `{"data":[],"usage":{"prompt_tokens":9,"total_tokens":9}}` surfaces into whatever the client returns (add a `LastUsage()` on the embedding client or return usage from `Embed`).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run TestStreamReportsTokenUsage ./pkg/provider/openaichat/ -v` and the gemini/embedding tests.
Expected: FAIL / behaviour absent.

- [ ] **Step 3: openaichat**

Add to the request body struct and marshal an options object:

```go
type openAIChatRequest struct {
	Model         string                   `json:"model"`
	Messages      []openAIChatMessage      `json:"messages"`
	Stream        bool                     `json:"stream"`
	Temperature   float64                  `json:"temperature,omitempty"`
	MaxTokens     int                      `json:"max_tokens,omitempty"`
	Stop          []string                 `json:"stop,omitempty"`
	Tools         []openAITool             `json:"tools,omitempty"`
	StreamOptions *openAIChatStreamOptions `json:"stream_options,omitempty"`
}

type openAIChatStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}
```

Set `StreamOptions: &openAIChatStreamOptions{IncludeUsage: true}` when streaming. Add to the chunk struct:

```go
type openAIChatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}
```

with `Usage *openAIChatUsage \`json:"usage,omitempty"\`` on `openAIChatChunk`. In the parse loop, when a chunk carries usage, emit a `StreamChunk{Done: true, Usage: &harness.Usage{Provider: "openaichat", Model: providerModel, InputTokens: …, OutputTokens: …}}` (do not emit empty text). This final chunk may be lost if the stream is cancelled; that is expected.

- [ ] **Step 4: geminillm**

Add a mapper:

```go
func usageFromMetadata(model string, meta *genai.GenerateContentResponseUsageMetadata) *harness.Usage {
	if meta == nil {
		return nil
	}
	return &harness.Usage{
		Provider:     "gemini",
		Model:        model,
		InputTokens:  int(meta.PromptTokenCount),
		OutputTokens: int(meta.CandidatesTokenCount),
	}
}
```

Use it in the non-stream genai `Generate` (set `resp.Usage` on the returned `GenerateResponse`) and in `GenerateContentStream`: capture `resp.UsageMetadata` per chunk and emit the final non-nil one as a `StreamChunk{Usage: …}`.

- [ ] **Step 5: openaiembedding**

Add a `usage` field to the response struct and expose it:

```go
type embeddingResponse struct {
	Data  []embeddingData `json:"data"`
	Error *apiError       `json:"error,omitempty"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}
```

Return the usage from the client (add `LastUsage() harness.Usage` on the embedding client, mirroring `MeteredProvider`). Embedding usage is attributed to role `embedding` in Task 3.

- [ ] **Step 6: Run tests and vet**

Run: `go test ./pkg/provider/... && mise run lint`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add pkg/provider/openaichat pkg/provider/geminillm pkg/provider/openaiembedding
git commit -m "feat(usage): report LLM and embedding token usage"
```

---

### Task 2: Media providers report usage

**Files:**
- Modify: `pkg/media/capabilities.go` (mirror `MeteredProvider` discovery ~line 33-34)
- Modify: `pkg/media/tts.go` (`SynthesizeUtteranceForce` ~line 263-355)
- Modify: `pkg/provider/ttsgemini/client.go` (`Synthesize` ~line 219-246)
- Modify: `pkg/provider/ttselevenlabs/client.go` (`Synthesize` ~line 151-154, `do` ~line 185-209)
- Modify: `pkg/provider/sttwhisperhttp/client.go` (request ~line 44-66, response ~line 79-81)
- Modify: `pkg/provider/imagehttp/client.go` (response decode ~line 302-348)
- Test: `pkg/media/usage_test.go`, plus provider tests

**Interfaces:**
- Produces: `media.UsageReporter interface { LastUsage() harness.Usage }` — but `pkg/media` does not import `pkg/harness`. Use a media-local shape instead:

```go
// pkg/media
type UsageReporter interface {
	LastUsage() Usage
}
type Usage struct {
	Characters   int
	InputTokens  int
	OutputTokens int
	Requests     int
	Estimated    bool
}
```

and convert to `harness.Usage` at the service boundary (Task 3). This keeps `pkg/media` free of `pkg/harness`.

- [ ] **Step 1: Write the failing tests**

`pkg/media/usage_test.go`: a stub TTS client implementing `Synthesize` and `LastUsage() Usage{Characters: 42}`; after one `pipeline.SynthesizeUtterance`, assert the pipeline exposes the usage (add `(p *TTSPipeline) LastUsage() Usage` that returns the last reported client usage, zero on a cache hit).

```go
func TestPipelineReportsUsageOnMissOnly(t *testing.T) {
	client := &usageTTS{chars: 42}
	pipeline := NewTTSPipeline(client, NewContentCache(t.TempDir()))
	voice := &entity.VoiceConfig{Provider: "stub", VoiceID: "v1"}
	if _, err := pipeline.SynthesizeUtterance(context.Background(), "s", voice, "hello"); err != nil {
		t.Fatal(err)
	}
	if got := pipeline.LastUsage().Characters; got != 42 {
		t.Fatalf("usage = %d, want 42", got)
	}
	// A second call is a cache hit and must report no usage.
	if _, err := pipeline.SynthesizeUtterance(context.Background(), "s", voice, "hello"); err != nil {
		t.Fatal(err)
	}
	if got := pipeline.LastUsage().Characters; got != 0 {
		t.Fatalf("cache hit reported usage %d, want 0", got)
	}
}
```

Provider tests: ElevenLabs `Synthesize` sets `LastUsage().Characters` from the `character-cost` response header; Whisper sends a body that requests usage and reads `usage.seconds` (duration model) or `usage.total_tokens`; `ttsgemini` sets tokens from `UsageMetadata`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run TestPipelineReportsUsage ./pkg/media/ -v`
Expected: FAIL.

- [ ] **Step 3: Add the media capability**

In `pkg/media/capabilities.go` (or a new `pkg/media/usage.go`):

```go
// Usage is a provider's reported consumption in the media layer's own terms, so
// pkg/media never imports pkg/harness.
type Usage struct {
	Characters   int
	InputTokens  int
	OutputTokens int
	Requests     int
	Estimated    bool
}

// UsageReporter is implemented by media clients that can report what the last
// call consumed.
type UsageReporter interface {
	LastUsage() Usage
}
```

- [ ] **Step 4: Track usage in the pipeline**

Add to `TTSPipeline` a `lastUsage Usage` guarded by the existing `flightMu` (or a small mutex). Reset it to zero at the start of a request that resolves from cache; set it from the client after a successful synthesis:

```go
func (p *TTSPipeline) LastUsage() Usage {
	p.flightMu.Lock()
	defer p.flightMu.Unlock()
	return p.lastUsage
}
```

On the cache-hit path set `p.lastUsage = Usage{}`; on a miss, after `cache.Put`, if `reporter, ok := p.client.(UsageReporter); ok { p.lastUsage = reporter.LastUsage() }` else `p.lastUsage = Usage{Characters: len([]rune(spoken)), Requests: 1, Estimated: true}`. The `spoken` text is available in `SynthesizeUtteranceForce`'s caller; pass the character count into the utterance function or set it in `SynthesizeSegmentForce`.

- [ ] **Step 5: Provider parsing**

- `ttselevenlabs`: in `do`, after the response is returned, read `resp.Header.Get("character-cost")` (fallback to counting the request text) and store it; add `LastUsage() media.Usage` returning `{Characters: n}`. Keep `Metered() == true`.
- `ttsgemini`: map `UsageMetadata` to `Usage{InputTokens, OutputTokens}` and add `LastUsage()`.
- `sttwhisperhttp`: add `response_format` (`verbose_json` for `whisper-1`) so `usage` is returned; parse `usage.type == "duration"` (`seconds`) or `usage.total_tokens`; add `LastUsage()` returning the appropriate unit (characters = 0; use `Requests: 1` plus durations recorded as characters * 0 — represent duration as `Characters` only if you also price per second; simplest is `Requests: 1` and `Estimated: false`, with the duration carried in the failure-free log).
- `imagehttp`: when the JSON body contains `usage`, read `input_tokens`/`output_tokens`; otherwise `Usage{Requests: 1, Estimated: true}`. Imagen (`imagegemini`) returns no usage: `LastUsage() == Usage{Requests: 1, Estimated: true}`.

- [ ] **Step 6: Run tests and vet**

Run: `go test ./pkg/media/ ./pkg/provider/... && mise run lint`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add pkg/media pkg/provider/ttsgemini pkg/provider/ttselevenlabs pkg/provider/sttwhisperhttp pkg/provider/imagehttp pkg/provider/imagegemini
git commit -m "feat(usage): report speech, transcription, and image usage"
```

---

### Task 3: Service records usage with cost

**Files:**
- Create: `pkg/gui/usage.go`
- Modify: `pkg/gui/service.go` (`Service` struct ~line 36-63, `prepareTurn` ~line 1143-1258, `GetSegmentAudio` ~line 1671-1709)
- Modify: `pkg/engine/orchestrator.go` (`ProcessActionStream` turn start)
- Test: `pkg/gui/usage_record_test.go`

**Interfaces:**
- Consumes: `harness.UsageContext`, `storage.SaveUsage`, `pricing.CostMicros`, `media.UsageReporter`.
- Produces: `(*Service).RecordUsage(gameID string, turn int, role string, u harness.Usage)` (a `harness.UsageSink`); `(*Service).GameUsage` / `GlobalUsage` (Task 5).
- `(*engine.TurnOrchestrator).SetUsageContext(ctx *harness.UsageContext)` which calls `ctx.SetTurn(turnNum)`.

- [ ] **Step 1: Write the failing test**

```go
func TestTurnUsageIsRecordedWithCost(t *testing.T) {
	gameID, svc := turnFixture(t)
	svc.configMgr.Get().Providers.Prices = []config.PriceConfig{
		{Provider: "gemini", PerMillionInput: 1_000_000, PerMillionOutput: 2_000_000},
	}
	svc.RecordUsage(gameID, 1, "gm", harness.Usage{Provider: "gemini", Model: "m", InputTokens: 1000, OutputTokens: 500})

	store, err := svc.store(gameID)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := store.UsageByTurn(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Role != "gm" || rows[0].CostMicros != 2000 {
		t.Fatalf("usage rows = %+v, want 1 gm row costing 2000 micros", rows)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestTurnUsageIsRecordedWithCost ./pkg/gui/ -v`
Expected: FAIL — `RecordUsage` undefined.

- [ ] **Step 3: Implement the sink**

Create `pkg/gui/usage.go`:

```go
package gui

import (
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/pricing"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// RecordUsage implements harness.UsageSink: it prices a provider call and writes
// it to the campaign's ledger. It never fails a turn.
func (s *Service) RecordUsage(gameID string, turn int, role string, u harness.Usage) {
	store, err := s.store(gameID)
	if err != nil {
		return
	}
	price := pricing.Resolve(u.Provider, u.Model, s.configMgr.Get())
	rec := storage.UsageRecord{
		TurnNumber:   turn,
		Role:         role,
		Provider:     u.Provider,
		Model:        u.Model,
		InputTokens:  u.InputTokens,
		OutputTokens: u.OutputTokens,
		Characters:   u.Characters,
		Requests:     u.Requests,
		Estimated:    u.Estimated,
		CostMicros:   int64(pricing.CostMicros(u, price)),
	}
	if err := store.SaveUsage(rec); err != nil {
		trace.OrNil(s.logger).Event("usage.record_error", map[string]interface{}{"error": err.Error()})
	}
}
```

- [ ] **Step 4: Create one UsageContext per turn**

In `prepareTurn`, after `jsEngine` is built:

```go
	usageCtx := harness.NewUsageContext(s, gameID)
	router.SetUsageRecorder(usageCtx)
```

and when the extractor is built, attach the same context:

```go
	orchestrator.SetUsageContext(usageCtx)
```

In `engine.TurnOrchestrator`, add:

```go
	usageCtx *harness.UsageContext

func (o *TurnOrchestrator) SetUsageContext(ctx *harness.UsageContext) { o.usageCtx = ctx }
```

and in `ProcessActionStream`, once `turnNum` is known:

```go
	if o.usageCtx != nil {
		o.usageCtx.SetTurn(turnNum)
	}
```

Because the orchestrator and the router share the same `UsageContext`, extraction (which may run concurrently) is stamped with the same turn.

- [ ] **Step 5: Record media usage**

In `GetSegmentAudio`, after `pipeline.SynthesizeSegmentForce` returns a path:

```go
	path, err := pipeline.SynthesizeSegmentForce(ctx, turn.Segments[segmentIndex], narratorVoice, s.voiceFor(gameID), isForce)
	if err != nil {
		return "", err
	}
	if u := pipeline.LastUsage(); u.Characters != 0 || u.InputTokens != 0 || u.OutputTokens != 0 || u.Requests != 0 {
		providerKey := media.ProviderKey(s.configMgr.Get().Media.TTS)
		s.RecordUsage(gameID, turnNumber, "tts", harness.Usage{
			Provider: providerKey, Model: s.configMgr.Get().Media.TTS.Model,
			InputTokens: u.InputTokens, OutputTokens: u.OutputTokens,
			Characters: u.Characters, Requests: u.Requests, Estimated: u.Estimated,
		})
	}
	return path, nil
```

(A cache hit reports zero and records nothing.) Apply the same pattern to image generation and transcription call sites with roles `image` and `stt`, and turn `0` for previews/auditions.

- [ ] **Step 6: Run test and the package**

Run: `go test -run TestTurnUsageIsRecordedWithCost ./pkg/gui/ -v && go test ./pkg/gui/ ./pkg/engine/`
Expected: PASS (re-run once on the known flake).

- [ ] **Step 7: Commit**

```bash
git add pkg/gui/usage.go pkg/gui/service.go pkg/gui/usage_record_test.go pkg/engine/orchestrator.go
git commit -m "feat(usage): record per-turn usage with cost"
```

---

### Task 4: Rate-limit blocks and funds failures

**Files:**
- Modify: `pkg/gui/service.go` (`Service` struct, `BeginTurn`, `GetSegmentAudio`, previews)
- Modify: `pkg/gui/server.go` (turn route and previews map `ErrRateLimitedUntil` to 429)
- Test: `pkg/gui/limits_test.go`

**Interfaces:**
- Consumes: `harness.LimitRegistry`, `harness.ErrRateLimitedUntil`.
- Produces: `(*Service).Limits(ctx) *LimitsDTO` (Task 5); enforcement at turn/preview/synth boundaries.

- [ ] **Step 1: Write the failing test**

```go
func TestRateLimitBlocksTurnsForThatRoleOnly(t *testing.T) {
	gameID, svc := turnFixture(t)
	svc.limits.Block("gemini", "gm", time.Now().Add(time.Hour))

	if _, err := svc.BeginTurn(gameID); err == nil {
		t.Fatal("expected BeginTurn to be blocked")
	} else {
		var limited *harness.ErrRateLimitedUntil
		if !errors.As(err, &limited) || limited.RetryAfter() <= 0 {
			t.Fatalf("want ErrRateLimitedUntil, got %v", err)
		}
	}

	// A different role is unaffected.
	svc.configMgr.Get().Media.TTS.Type = "cli"
	if blocked, _ := svc.limits.Blocked("gemini", "tts"); blocked {
		t.Fatal("tts must not be blocked by a gm block")
	}
}

func TestChangingTheProviderClearsTheBlock(t *testing.T) {
	_, svc := turnFixture(t)
	svc.limits.Block("gemini", "gm", time.Now().Add(time.Hour))
	// Simulate switching the gm role to another provider, then re-checking.
	svc.limits.Clear("gemini", "gm")
	if _, ok := svc.limits.Blocked("gemini", "gm"); ok {
		t.Fatal("clearing must lift the block")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run 'TestRateLimit|TestChangingTheProvider' ./pkg/gui/ -v`
Expected: FAIL.

- [ ] **Step 3: Enforce at the boundaries**

Add `limits *harness.LimitRegistry` to `Service`, initialised in `NewService`. Add a helper:

```go
// providerKeyForRole resolves the provider identity a role currently uses, so a
// block follows the provider and is lifted by a configuration change.
func (s *Service) providerKeyForRole(role string) string {
	cfg := s.configMgr.Get()
	switch role {
	case "tts":
		return media.ProviderKey(cfg.Media.TTS)
	case "stt":
		return media.ProviderKey(cfg.Media.STT)
	case "image":
		return media.ProviderKey(cfg.Media.Image)
	default:
		return roleProviderID(cfg, role)
	}
}

func (s *Service) guardRole(role string) error {
	key := s.providerKeyForRole(role)
	if until, ok := s.limits.Blocked(key, role); ok {
		return &harness.ErrRateLimitedUntil{Provider: key, Role: role, Until: until}
	}
	return nil
}
```

Call `s.guardRole("gm")` at the top of `BeginTurn` (before taking the lock), and `s.guardRole("tts")` / `"stt"` / `"image"` before those calls (including previews and auditions).

When a generation fails, classify and record:

```go
func (s *Service) noteFailure(role string, err error) {
	key := s.providerKeyForRole(role)
	switch harness.ClassifyProviderError(err) {
	case harness.FailureRateLimited:
		retryAfter := retryAfterFor(err)
		if retryAfter <= 0 {
			retryAfter = 30 * time.Second
		}
		s.limits.Block(key, role, time.Now().Add(retryAfter))
	case harness.FailureInsufficientFunds:
		s.limits.RecordFundsFailure(key, role, err.Error())
	default:
		s.limits.ClearFundsFailure(key, role)
	}
}
```

`retryAfterFor` extracts `RetryAfterMS` from a `*harness.GenerationFailure` or `RetryAfter()` from `*harness.RateLimitedError`.

- [ ] **Step 4: Map to HTTP**

In `pkg/gui/server.go`, in the turn route and preview routes, translate the sentinel:

```go
	var limited *harness.ErrRateLimitedUntil
	if errors.As(err, &limited) {
		w.Header().Set("Retry-After", strconv.Itoa(int(limited.RetryAfter().Seconds())+1))
		writeJSONError(w, http.StatusTooManyRequests, limited.Error(), "rate_limited")
		return
	}
```

`BeginTurn` must return the wrapped error from the route, so the existing turn handler maps it (it already has a failure-response path).

- [ ] **Step 5: Run tests and the suite**

Run: `go test -run 'TestRateLimit|TestChangingTheProvider' ./pkg/gui/ -v && mise run test`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/gui/service.go pkg/gui/server.go pkg/gui/limits_test.go
git commit -m "feat(limits): block rate-limited provider roles and track funds failures"
```

---

### Task 5: Usage and limits API

**Files:**
- Modify: `pkg/gui/types.go` (DTOs), `pkg/gui/service.go` (`GameUsage`, `GlobalUsage`, `Limits`), `pkg/gui/server.go` (routes)
- Modify: `pkg/gui/types.go` `TurnEvent` (add `RetryAfterMS`)
- Test: `pkg/gui/usage_api_test.go`

**Interfaces:**
- Produces:

```go
type UsageRowDTO struct {
	TurnNumber   int    `json:"turn_number"`
	Role         string `json:"role"`
	Provider     string `json:"provider"`
	Model        string `json:"model,omitempty"`
	InputTokens  int    `json:"input_tokens,omitempty"`
	OutputTokens int    `json:"output_tokens,omitempty"`
	Characters   int    `json:"characters,omitempty"`
	Requests     int    `json:"requests,omitempty"`
	Estimated    bool   `json:"estimated,omitempty"`
	CostMicros   int64  `json:"cost_micros,omitempty"`
}

type UsageDTO struct {
	Rows       []UsageRowDTO     `json:"rows,omitempty"`
	ByProvider map[string]int64  `json:"by_provider,omitempty"`
	ByRole     map[string]int64  `json:"by_role,omitempty"`
	TotalCost  int64             `json:"total_cost_micros"`
	Currency   string            `json:"currency,omitempty"`
	Campaigns  []CampaignUsageDTO `json:"campaigns,omitempty"` // global drilldown
}

type CampaignUsageDTO struct {
	GameID    string `json:"game_id"`
	Name      string `json:"name,omitempty"`
	TotalCost int64  `json:"total_cost_micros"`
}

type LimitsDTO struct {
	Blocks []harness.LimitState `json:"blocks"`
}
```

- [ ] **Step 1: Write the failing test**

```go
func TestUsageEndpointReturnsTurnRows(t *testing.T) {
	gameID, svc := turnFixture(t)
	svc.RecordUsage(gameID, 1, "gm", harness.Usage{Provider: "gemini", InputTokens: 5})
	server := NewServer(svc, http.NotFoundHandler())

	req := httptest.NewRequest("GET", "/api/game/"+gameID+"/usage", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var dto UsageDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &dto); err != nil {
		t.Fatal(err)
	}
	if len(dto.Rows) != 1 || dto.Rows[0].Role != "gm" {
		t.Fatalf("rows = %+v", dto.Rows)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestUsageEndpointReturnsTurnRows ./pkg/gui/ -v`
Expected: FAIL (404).

- [ ] **Step 3: Implement the service methods and routes**

`GameUsage` reads the campaign store's rows and summary; `GlobalUsage` iterates campaigns (from the games directory), sums totals, and builds the per-campaign drilldown; `Limits` returns `s.limits.Snapshot()`. Add routes `/api/game/{id}/usage`, `/api/usage`, `/api/limits` alongside the existing `/api/game/{id}/...` handling, calling `GET`-only handlers. Set `RetryAfterMS` on `TurnEvent` from `GenerationFailure.RetryAfterMS` where turn errors are emitted.

- [ ] **Step 4: Run test and suite**

Run: `go test -run TestUsageEndpointReturnsTurnRows ./pkg/gui/ -v && mise run test`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/types.go pkg/gui/service.go pkg/gui/server.go pkg/gui/usage_api_test.go
git commit -m "feat(usage): expose usage and limit state over the API"
```

---

### Task 6: Usage tab and inline status

**Files:**
- Create: `frontend/src/components/UsagePanel.tsx`
- Modify: `frontend/src/components/SettingsStudio.tsx` (tab list ~line 108,354-399, add a `usage` tab), `frontend/src/api/client.ts`, `frontend/src/types.ts`, `frontend/src/App.tsx` (header chip + banner), `frontend/src/components/StoryTheater.tsx` (chip)

**Interfaces:**
- Consumes: `GET /api/game/{id}/usage`, `/api/usage`, `/api/limits`; `TurnEvent.retry_after_ms`.

- [ ] **Step 1: Add the API client and types**

Mirror the DTOs in `frontend/src/types.ts` and add `APIClient.getGameUsage`, `getGlobalUsage`, `getLimits` in `client.ts` (matching the existing fetch style).

- [ ] **Step 2: Build the Usage panel**

`UsagePanel.tsx`: totals for the current campaign, an "All campaigns" toggle showing `campaigns` then a per-campaign turn table, a provider breakdown, a role breakdown, the currency, and an "estimated" badge per row. A "no price configured" label where `cost_micros == 0` but usage is present.

- [ ] **Step 3: Wire the tab**

Add `'usage'` to the `activeSubTab` union and the tab button row; render `<UsagePanel />` for it. Place it after `preferences`.

- [ ] **Step 4: Inline status**

Poll `/api/limits` every 15 s (and once on mount) in `App.tsx`; render a header chip per block with a countdown from `until`. When a turn fails with `code == "rate_limited"`, show the countdown from `retry_after_ms` and disable the submit button until it expires. When `code == "insufficient_funds"`, render the persistent banner and do not disable submitting. Mirror the chip in `StoryTheater.tsx` for previews.

- [ ] **Step 5: Verify**

Run: `mise run test:frontend && mise run build`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add frontend/src
git commit -m "feat(usage): add the Usage panel and inline limit status"
```

---

### Task 7: Full verification

- [ ] **Step 1: Run the whole suite**

Run: `mise run test`
Expected: PASS (re-run `./pkg/gui/` on the known flake).

- [ ] **Step 2: Vet and build**

Run: `mise run lint && mise run build`
Expected: clean.

- [ ] **Step 3: Manual smoke**

Run `mise run dev:gui`, play one turn on a metered provider, and confirm the Usage tab shows the turn's rows and the header chip appears when a 429 is simulated.

---

## Self-Review Notes

- Spec coverage: provider parsing (§3.1.1) → Tasks 1-2; ledger writes and attribution (§3.3, §3.6) → Task 3; block enforcement and funds handling (§3.5) → Task 4; API and inline error fields (§3.7) → Task 5; UI (§3.7) → Task 6.
- Type consistency: `harness.Usage*`, `media.Usage`/`UsageReporter`, `storage.UsageRecord`, `pricing.Price`, `UsageDTO`/`LimitsDTO` are used consistently across tasks.
- Note the deliberate layer boundary: `pkg/media` reports its own `Usage` shape and never imports `pkg/harness`; the conversion happens in `pkg/gui`.
