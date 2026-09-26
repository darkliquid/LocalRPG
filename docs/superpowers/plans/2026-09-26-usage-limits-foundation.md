# Usage, Limits & Pricing Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the shared types the usage/limits feature needs: a rate-limit and insufficient-funds taxonomy, a provider+role block registry, a pricing calculator, a per-campaign usage ledger, and the harness usage plumbing.

**Architecture:** Five additive, independently testable units across `pkg/harness`, `pkg/provider`, `pkg/pricing` (new), and `pkg/storage`. No provider or GUI behaviour changes yet; the integration plan consumes these.

**Tech Stack:** Go 1.27 (stdlib `testing`), SQLite via `modernc.org/sqlite`. No new dependencies.

**Spec:** `docs/superpowers/specs/2026-09-26-usage-cost-and-provider-limits-design.md`

## Global Constraints

- Go tests use only `testing` and `t.TempDir()`; no testify, no mocks.
- Use `interface{}`, never `any`; wrap errors with `fmt.Errorf("...: %w", err)`; `go vet ./...` clean.
- No new dependencies.
- Money is integer micros (1e-6 currency units); never float.
- Conventional Commits with a scope; subject under 72 chars.
- Verification: `mise run test`, `mise run lint`, `mise run build`.
- Known pre-existing `pkg/gui` TempDir-cleanup flake (`TestRegenerateCharacterPortraitEndpoint`) — re-run before treating a failure as real.

---

### Task 1: Failure taxonomy and typed provider errors

**Files:**
- Modify: `pkg/harness/failure.go` (codes ~line 16-22, `GenerationFailure` ~line 29-45, `ClassifyProviderError` ~line 66-82)
- Create: `pkg/provider/errors.go`
- Test: `pkg/harness/failure_limits_test.go`, `pkg/provider/errors_test.go`

**Interfaces:**
- Produces: `FailureRateLimited`, `FailureInsufficientFunds`; `GenerationFailure.RetryAfterMS int64`; `provider.RateLimitedError`, `provider.InsufficientFundsError`.

- [ ] **Step 1: Write the failing tests**

Create `pkg/provider/errors_test.go`:

```go
package provider

import (
	"errors"
	"testing"
	"time"
)

func TestRateLimitedErrorCarriesRetryAfter(t *testing.T) {
	err := &RateLimitedError{RetryAfter: 30 * time.Second, Message: "slow down"}
	var target *RateLimitedError
	if !errors.As(err, &target) {
		t.Fatal("RateLimitedError does not satisfy errors.As")
	}
	if target.RetryAfter != 30*time.Second {
		t.Fatalf("RetryAfter = %v", target.RetryAfter)
	}
}

func TestInsufficientFundsErrorMessage(t *testing.T) {
	err := &InsufficientFundsError{Message: "no credits"}
	if err.Error() != "no credits" {
		t.Fatalf("Error() = %q", err.Error())
	}
}
```

Create `pkg/harness/failure_limits_test.go`:

```go
package harness

import (
	"errors"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/provider"
)

func TestClassifyRateLimited(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want FailureCode
	}{
		{"typed with retry-after", &provider.RateLimitedError{RetryAfter: 12 * time.Second}, FailureRateLimited},
		{"typed without retry-after", &provider.RateLimitedError{}, FailureRateLimited},
		{"marker 429", errors.New("server returned 429 Too Many Requests"), FailureRateLimited},
		{"marker resource exhausted", errors.New("RESOURCE_EXHAUSTED"), FailureRateLimited},
		{"funds typed", &provider.InsufficientFundsError{Message: "insufficient_credits"}, FailureInsufficientFunds},
		{"funds marker", errors.New("insufficient_quota"), FailureInsufficientFunds},
	}
	for _, tc := range cases {
		if got := ClassifyProviderError(tc.err); got != tc.want {
			t.Errorf("%s: ClassifyProviderError = %q, want %q", tc.name, got, tc.want)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run 'TestClassifyRateLimited|TestRateLimited|TestInsufficientFunds' ./pkg/harness/ ./pkg/provider/ -v`
Expected: FAIL — undefined types and codes.

- [ ] **Step 3: Add the typed errors**

Create `pkg/provider/errors.go`:

```go
package provider

import (
	"fmt"
	"time"
)

// RateLimitedError is a provider refusal that should unblock after a delay. A
// zero RetryAfter means the provider advertised no window and the caller
// chooses a default.
type RateLimitedError struct {
	RetryAfter time.Duration
	Message    string
}

func (e *RateLimitedError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "provider rate limit reached"
}

// InsufficientFundsError is a provider refusal caused by the account having no
// credit. It is never retried automatically.
type InsufficientFundsError struct{ Message string }

func (e *InsufficientFundsError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "provider rejected the request: insufficient funds"
}

// RateLimitedf and InsufficientFundsf are small constructors so providers read
// cleanly.
func RateLimitedf(retryAfter time.Duration, format string, args ...interface{}) error {
	return &RateLimitedError{RetryAfter: retryAfter, Message: fmt.Sprintf(format, args...)}
}

func InsufficientFundsf(format string, args ...interface{}) error {
	return &InsufficientFundsError{Message: fmt.Sprintf(format, args...)}
}
```

- [ ] **Step 4: Add the codes and classification**

In `pkg/harness/failure.go`, add to the constants block:

```go
	FailureRateLimited       FailureCode = "rate_limited"
	FailureInsufficientFunds FailureCode = "insufficient_funds"
```

Add to `GenerationFailure`:

```go
	// RetryAfterMS is the provider's advertised backoff, 0 when none was given.
	RetryAfterMS int64 `json:"retry_after_ms,omitempty"`
```

Extend `ClassifyProviderError` (before the context-window markers):

```go
	var rateLimited *provider.RateLimitedError
	if errors.As(err, &rateLimited) {
		return FailureRateLimited
	}
	var funds *provider.InsufficientFundsError
	if errors.As(err, &funds) {
		return FailureInsufficientFunds
	}
	lower := strings.ToLower(err.Error())
	for _, marker := range []string{"insufficient_quota", "insufficient_credits", "payment_required", "out of credits", "not enough credits"} {
		if strings.Contains(lower, marker) {
			return FailureInsufficientFunds
		}
	}
	for _, marker := range []string{"429", "too many requests", "rate limit", "rate_limit", "resource_exhausted"} {
		if strings.Contains(lower, marker) {
			return FailureRateLimited
		}
	}
```

`pkg/harness` must import `pkg/provider` here; confirm there is no cycle (`pkg/provider` imports `pkg/harness`, so this would be a cycle). **Resolve by moving the typed errors into `pkg/harness`** instead: define `RateLimitedError` and `InsufficientFundsError` in `pkg/harness/failure.go`, and have `pkg/provider` alias them (`type RateLimitedError = harness.RateLimitedError`). Put the constructors in harness; provider providers return the harness types through the alias. Update the test imports accordingly (`pkg/harness` test only; the `pkg/provider` test uses the alias).

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test -run 'TestClassifyRateLimited|TestRateLimited|TestInsufficientFunds' ./pkg/harness/ ./pkg/provider/ -v && go build ./...`
Expected: PASS, no import cycle.

- [ ] **Step 6: Commit**

```bash
git add pkg/harness/failure.go pkg/harness/failure_limits_test.go pkg/provider/errors.go pkg/provider/errors_test.go
git commit -m "feat(limits): classify rate limits and insufficient funds"
```

---

### Task 2: Provider + role block registry

**Files:**
- Create: `pkg/harness/limits.go`
- Test: `pkg/harness/limits_test.go`

**Interfaces:**
- Produces:

```go
type LimitKey struct{ Provider, Role string }
type LimitRegistry struct{ /* … */ }
func NewLimitRegistry() *LimitRegistry
func (r *LimitRegistry) Block(providerKey, role string, until time.Time)
func (r *LimitRegistry) Blocked(providerKey, role string) (time.Time, bool)
func (r *LimitRegistry) Clear(providerKey, role string)
func (r *LimitRegistry) Snapshot() []LimitState
type LimitState struct{ Provider, Role string `json:"provider","role"`; Until time.Time; FundsFailure string }
type ErrRateLimitedUntil struct{ Provider, Role string; Until time.Time }
func (e *ErrRateLimitedUntil) Error() string
```

- [ ] **Step 1: Write the failing tests**

Create `pkg/harness/limits_test.go`:

```go
package harness

import (
	"errors"
	"testing"
	"time"
)

func TestRegistryBlocksAndExpiresOnRead(t *testing.T) {
	r := NewLimitRegistry()
	r.Block("gemini", "gm", time.Now().Add(50*time.Millisecond))
	if _, ok := r.Blocked("gemini", "gm"); !ok {
		t.Fatal("expected a block")
	}
	if _, ok := r.Blocked("gemini", "tts"); ok {
		t.Fatal("a different role must be unaffected")
	}
	time.Sleep(80 * time.Millisecond)
	if _, ok := r.Blocked("gemini", "gm"); ok {
		t.Fatal("block should have expired")
	}
}

func TestRegistryClearLiftsABlock(t *testing.T) {
	r := NewLimitRegistry()
	r.Block("gemini", "gm", time.Now().Add(time.Hour))
	r.Clear("gemini", "gm")
	if _, ok := r.Blocked("gemini", "gm"); ok {
		t.Fatal("Clear must lift the block")
	}
}

func TestRegistryRecordsFundsFailureWithoutBlocking(t *testing.T) {
	r := NewLimitRegistry()
	r.RecordFundsFailure("elevenlabs", "tts", "insufficient_credits")
	if _, ok := r.Blocked("elevenlabs", "tts"); ok {
		t.Fatal("insufficient funds must not block")
	}
	states := r.Snapshot()
	if len(states) != 1 || states[0].FundsFailure == "" {
		t.Fatalf("funds failure not recorded: %+v", states)
	}
	r.ClearFundsFailure("elevenlabs", "tts")
	if len(r.Snapshot()) != 0 {
		t.Fatal("funds failure should clear")
	}
}

func TestErrRateLimitedUntilCarriesDeadline(t *testing.T) {
	until := time.Now().Add(time.Minute)
	err := &ErrRateLimitedUntil{Provider: "gemini", Role: "gm", Until: until}
	var target *ErrRateLimitedUntil
	if !errors.As(err, &target) || target.Until != until {
		t.Fatalf("unexpected error: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run TestRegistry ./pkg/harness/ -v`
Expected: FAIL — undefined `NewLimitRegistry`.

- [ ] **Step 3: Implement the registry**

Create `pkg/harness/limits.go`:

```go
package harness

import (
	"fmt"
	"sync"
	"time"
)

type LimitKey struct{ Provider, Role string }

// LimitState is one provider+role the UI should show: a backoff deadline, a
// funds failure, or both.
type LimitState struct {
	Provider     string    `json:"provider"`
	Role         string    `json:"role"`
	Until        time.Time `json:"until,omitempty"`
	FundsFailure string    `json:"funds_failure,omitempty"`
}

// LimitRegistry holds in-memory rate-limit blocks and funding failures. Blocks
// expire lazily on read; funds failures clear only on a later success.
type LimitRegistry struct {
	mu     sync.Mutex
	blocks map[LimitKey]time.Time
	funds  map[LimitKey]string
}

func NewLimitRegistry() *LimitRegistry {
	return &LimitRegistry{blocks: map[LimitKey]time.Time{}, funds: map[LimitKey]string{}}
}

func (r *LimitRegistry) Block(providerKey, role string, until time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := LimitKey{providerKey, role}
	if existing, ok := r.blocks[key]; ok && existing.After(until) {
		return
	}
	r.blocks[key] = until
}

func (r *LimitRegistry) Blocked(providerKey, role string) (time.Time, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := LimitKey{providerKey, role}
	until, ok := r.blocks[key]
	if !ok {
		return time.Time{}, false
	}
	if time.Now().After(until) {
		delete(r.blocks, key)
		return time.Time{}, false
	}
	return until, true
}

func (r *LimitRegistry) Clear(providerKey, role string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.blocks, LimitKey{providerKey, role})
}

func (r *LimitRegistry) RecordFundsFailure(providerKey, role, message string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.funds[LimitKey{providerKey, role}] = message
}

func (r *LimitRegistry) ClearFundsFailure(providerKey, role string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.funds, LimitKey{providerKey, role})
}

// Snapshot reports every live block and funds failure, expiring stale blocks.
func (r *LimitRegistry) Snapshot() []LimitState {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	out := make([]LimitState, 0, len(r.blocks)+len(r.funds))
	for key, until := range r.blocks {
		if now.After(until) {
			delete(r.blocks, key)
			continue
		}
		out = append(out, LimitState{Provider: key.Provider, Role: key.Role, Until: until})
	}
	for key, message := range r.funds {
		out = append(out, LimitState{Provider: key.Provider, Role: key.Role, FundsFailure: message})
	}
	return out
}

// ErrRateLimitedUntil is returned when work is refused because a provider+role
// is backed off. The API maps it to 429 with a Retry-After.
type ErrRateLimitedUntil struct {
	Provider string
	Role     string
	Until    time.Time
}

func (e *ErrRateLimitedUntil) Error() string {
	return fmt.Sprintf("provider %q (%s) is rate limited until %s", e.Provider, e.Role, e.Until.Format(time.RFC3339))
}

func (e *ErrRateLimitedUntil) RetryAfter() time.Duration {
	if d := time.Until(e.Until); d > 0 {
		return d
	}
	return 0
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -run TestRegistry ./pkg/harness/ -v && go test ./pkg/harness/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/harness/limits.go pkg/harness/limits_test.go
git commit -m "feat(limits): add a provider+role block registry"
```

---

### Task 3: Pricing

**Files:**
- Create: `pkg/pricing/pricing.go`
- Modify: `pkg/config/types.go` (`ProvidersConfig` ~line 232-238)
- Test: `pkg/pricing/pricing_test.go`, `pkg/config/types_test.go` (append)

**Interfaces:**
- Produces: `pricing.Micros`, `pricing.Price`, `pricing.CostMicros(harness.Usage, Price) Micros`, `pricing.Resolve(providerKey, model string, cfg *config.Config) Price`; `config.PriceConfig` and `config.ProvidersConfig.Prices []PriceConfig`, `ProvidersConfig.Currency string`.

- [ ] **Step 1: Write the failing tests**

Create `pkg/pricing/pricing_test.go`:

```go
package pricing

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestCostFromTokens(t *testing.T) {
	p := Price{PerMillionInput: 1_000_000, PerMillionOutput: 2_000_000} // 1 and 2 per 1M
	u := harness.Usage{InputTokens: 1_000_000, OutputTokens: 500_000}
	if got := CostMicros(u, p); got != 2_000_000 {
		t.Fatalf("CostMicros = %d, want 2000000 micros (1 + 1)", got)
	}
}

func TestCostFromCharactersAndRequests(t *testing.T) {
	p := Price{PerCharacter: 30, PerRequest: 1000}
	u := harness.Usage{Characters: 100, Requests: 1}
	if got := CostMicros(u, p); got != 4000 {
		t.Fatalf("CostMicros = %d, want 4000 micros", got)
	}
}

func TestResolvePrefersModelThenProviderThenZero(t *testing.T) {
	cfg := &config.Config{Providers: config.ProvidersConfig{Prices: []config.PriceConfig{
		{Provider: "builtin:gemini", PerMillionInput: 1},
		{Provider: "builtin:gemini", Model: "gemini-2.5-pro", PerMillionInput: 2},
	}}}
	if got := Resolve("builtin:gemini", "gemini-2.5-pro", cfg); got.PerMillionInput != 2 {
		t.Fatalf("model override not used: %+v", got)
	}
	if got := Resolve("builtin:gemini", "other", cfg); got.PerMillionInput != 1 {
		t.Fatalf("provider fallback not used: %+v", got)
	}
	if got := Resolve("unknown", "", cfg); got != (Price{}) {
		t.Fatalf("unknown provider = %+v, want zero", got)
	}
}
```

Append to `pkg/config/types_test.go`:

```go
func TestPriceConfigRoundTrips(t *testing.T) {
	cfg := &Config{Providers: ProvidersConfig{Currency: "USD", Prices: []PriceConfig{{
		Provider: "builtin:elevenlabs", PerCharacter: 30,
	}}}}
	raw, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var back Config
	if err := yaml.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if len(back.Providers.Prices) != 1 || back.Providers.Prices[0].PerCharacter != 30 || back.Providers.Currency != "USD" {
		t.Fatalf("round trip lost prices: %+v", back.Providers)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run 'TestCost|TestResolve|TestPriceConfig' ./pkg/pricing/ ./pkg/config/ -v`
Expected: FAIL — `pkg/pricing` undefined, `config.PriceConfig` undefined.

- [ ] **Step 3: Add config fields**

In `pkg/config/types.go`:

```go
type ProvidersConfig struct {
	Gemini   GeminiProviderConfig `yaml:"gemini,omitempty" json:"gemini,omitempty"`
	// Currency is the display currency for cost figures. Prices are expressed in
	// this currency; no conversion is performed.
	Currency string `yaml:"currency,omitempty" json:"currency,omitempty"`
	// Prices override the built-in price table, matched by provider then model.
	Prices []PriceConfig `yaml:"prices,omitempty" json:"prices,omitempty"`
}

// PriceConfig is one provider's price. A zero model matches every model of the
// provider. Values are in micros (1e-6 currency units).
type PriceConfig struct {
	Provider         string `yaml:"provider" json:"provider"`
	Model            string `yaml:"model,omitempty" json:"model,omitempty"`
	PerMillionInput  int64  `yaml:"per_million_input,omitempty" json:"per_million_input,omitempty"`
	PerMillionOutput int64  `yaml:"per_million_output,omitempty" json:"per_million_output,omitempty"`
	PerCharacter     int64  `yaml:"per_character,omitempty" json:"per_character,omitempty"`
	PerRequest       int64  `yaml:"per_request,omitempty" json:"per_request,omitempty"`
}
```

- [ ] **Step 4: Implement pricing**

Create `pkg/pricing/pricing.go`:

```go
package pricing

import (
	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/harness"
)

// Micros is a currency amount in millionths, so costs are exact integers.
type Micros int64

// Price is a provider's rate card. All fields are in micros.
type Price struct {
	PerMillionInput  Micros
	PerMillionOutput Micros
	PerCharacter     Micros
	PerRequest       Micros
}

// BuiltinPrices are the known rates for providers LocalRPG ships presets for.
// They are deliberately conservative defaults; a config price always wins.
var BuiltinPrices = []config.PriceConfig{
	{Provider: "gemini", PerMillionInput: 125_000, PerMillionOutput: 500_000},
	{Provider: "openaichat", PerMillionInput: 150_000, PerMillionOutput: 600_000},
	{Provider: "elevenlabs", PerCharacter: 0},
}

// CostMicros returns the cost of one usage record under a price. A zero price
// yields zero, never a guess.
func CostMicros(u harness.Usage, p Price) Micros {
	cost := Micros(u.InputTokens) * p.PerMillionInput / 1_000_000
	cost += Micros(u.OutputTokens) * p.PerMillionOutput / 1_000_000
	cost += Micros(u.Characters) * p.PerCharacter
	cost += Micros(u.Requests) * p.PerRequest
	return cost
}

// Resolve finds the price for a provider and model: an exact provider+model
// config entry, then a provider entry, then a built-in, then zero.
func Resolve(providerKey, model string, cfg *config.Config) Price {
	if cfg != nil {
		if p, ok := lookup(cfg.Providers.Prices, providerKey, model); ok {
			return p
		}
	}
	if p, ok := lookup(BuiltinPrices, providerKey, model); ok {
		return p
	}
	return Price{}
}

func lookup(entries []config.PriceConfig, providerKey, model string) (Price, bool) {
	var providerWide *config.PriceConfig
	for i := range entries {
		entry := entries[i]
		if entry.Provider != providerKey {
			continue
		}
		if entry.Model == model && model != "" {
			return toPrice(entry), true
		}
		if entry.Model == "" && providerWide == nil {
			providerWide = &entries[i]
		}
	}
	if providerWide != nil {
		return toPrice(*providerWide), true
	}
	return Price{}, false
}

func toPrice(c config.PriceConfig) Price {
	return Price{
		PerMillionInput:  Micros(c.PerMillionInput),
		PerMillionOutput: Micros(c.PerMillionOutput),
		PerCharacter:     Micros(c.PerCharacter),
		PerRequest:       Micros(c.PerRequest),
	}
}
```

Note: the built-in `Provider` values must match the keys the recorder produces (`media.ProviderKey` for media, router provider IDs for LLMs). The integration plan reconciles these; keep the table small here.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test -run 'TestCost|TestResolve|TestPriceConfig' ./pkg/pricing/ ./pkg/config/ -v && go test ./pkg/pricing/ ./pkg/config/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/pricing pkg/config/types.go pkg/config/types_test.go
git commit -m "feat(pricing): add a price table and cost calculation"
```

---

### Task 4: Usage ledger

**Files:**
- Modify: `pkg/storage/migrate.go` (migrations list ~line 14-24)
- Create: `pkg/storage/usage.go`
- Test: `pkg/storage/usage_test.go`

**Interfaces:**
- Produces: `storage.UsageRecord`, `storage.UsageSummary`; `(*Store).SaveUsage`, `UsageByTurn`, `UsageSummary`, `UsageTotal`.

```go
type UsageRecord struct {
	TurnNumber   int
	Role         string
	Provider     string
	Model        string
	InputTokens  int
	OutputTokens int
	Characters   int
	Requests     int
	Estimated    bool
	CostMicros   int64
	CreatedAt    time.Time
}

type UsageSummary struct {
	TotalCostMicros int64
	ByProvider      map[string]int64
	ByRole          map[string]int64
	Rows            int
}
```

- [ ] **Step 1: Write the failing test**

Create `pkg/storage/usage_test.go`:

```go
package storage

import (
	"path/filepath"
	"testing"
)

func TestUsageRoundTripAndSummary(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	rows := []UsageRecord{
		{TurnNumber: 1, Role: "gm", Provider: "gemini", Model: "gemini-2.5-pro", InputTokens: 100, OutputTokens: 50, CostMicros: 40},
		{TurnNumber: 1, Role: "tts", Provider: "elevenlabs", Characters: 200, CostMicros: 6},
		{TurnNumber: 2, Role: "gm", Provider: "gemini", Model: "gemini-2.5-pro", InputTokens: 80, OutputTokens: 40, Estimated: true, CostMicros: 30},
	}
	for _, row := range rows {
		if err := store.SaveUsage(row); err != nil {
			t.Fatalf("SaveUsage: %v", err)
		}
	}

	turnOne, err := store.UsageByTurn(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(turnOne) != 2 {
		t.Fatalf("UsageByTurn(1) = %d rows, want 2", len(turnOne))
	}

	summary, err := store.UsageSummary()
	if err != nil {
		t.Fatal(err)
	}
	if summary.Rows != 3 || summary.TotalCostMicros != 76 {
		t.Fatalf("summary = %+v, want 3 rows and 76 micros", summary)
	}
	if summary.ByProvider["gemini"] != 70 || summary.ByRole["tts"] != 6 {
		t.Fatalf("breakdown wrong: %+v", summary)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestUsageRoundTripAndSummary ./pkg/storage/ -v`
Expected: FAIL — `SaveUsage` undefined.

- [ ] **Step 3: Add the migration**

In `pkg/storage/migrate.go`, add to the `migrations` slice:

```go
	{version: 8, apply: addUsageTable},
```

with:

```go
// addUsageTable records what each provider call cost, per campaign, so spend can
// be broken down by provider, role, and turn without re-deriving it.
func addUsageTable(db *sql.DB) error {
	const ddl = `
	CREATE TABLE IF NOT EXISTS usage_records (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		turn_number INTEGER NOT NULL DEFAULT 0,
		role TEXT NOT NULL,
		provider TEXT NOT NULL,
		model TEXT NOT NULL DEFAULT '',
		input_tokens INTEGER NOT NULL DEFAULT 0,
		output_tokens INTEGER NOT NULL DEFAULT 0,
		characters INTEGER NOT NULL DEFAULT 0,
		requests INTEGER NOT NULL DEFAULT 0,
		estimated INTEGER NOT NULL DEFAULT 0,
		cost_micros INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_usage_turn ON usage_records(turn_number);
	CREATE INDEX IF NOT EXISTS idx_usage_provider ON usage_records(provider, role);`
	_, err := db.Exec(ddl)
	return err
}
```

- [ ] **Step 4: Implement the store methods**

Create `pkg/storage/usage.go`:

```go
package storage

import "time"

// UsageRecord is one provider call's consumption and cost.
type UsageRecord struct {
	TurnNumber   int       `json:"turn_number"`
	Role         string    `json:"role"`
	Provider     string    `json:"provider"`
	Model        string    `json:"model,omitempty"`
	InputTokens  int       `json:"input_tokens,omitempty"`
	OutputTokens int       `json:"output_tokens,omitempty"`
	Characters   int       `json:"characters,omitempty"`
	Requests     int       `json:"requests,omitempty"`
	Estimated    bool      `json:"estimated,omitempty"`
	CostMicros   int64     `json:"cost_micros,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// UsageSummary is what a spend view needs without shipping every row.
type UsageSummary struct {
	TotalCostMicros int64            `json:"total_cost_micros"`
	ByProvider      map[string]int64 `json:"by_provider"`
	ByRole          map[string]int64 `json:"by_role"`
	Rows            int              `json:"rows"`
}

func (s *Store) SaveUsage(rec UsageRecord) error {
	const query = `
	INSERT INTO usage_records (turn_number, role, provider, model, input_tokens, output_tokens, characters, requests, estimated, cost_micros)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	estimated := 0
	if rec.Estimated {
		estimated = 1
	}
	_, err := s.db.Exec(query, rec.TurnNumber, rec.Role, rec.Provider, rec.Model,
		rec.InputTokens, rec.OutputTokens, rec.Characters, rec.Requests, estimated, rec.CostMicros)
	return err
}

func (s *Store) UsageByTurn(turn int) ([]UsageRecord, error) {
	const query = `
	SELECT turn_number, role, provider, model, input_tokens, output_tokens, characters, requests, estimated, cost_micros
	FROM usage_records WHERE turn_number = ? ORDER BY id`
	rows, err := s.db.Query(query, turn)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanUsage(rows)
}

func (s *Store) UsageSummary() (UsageSummary, error) {
	summary := UsageSummary{ByProvider: map[string]int64{}, ByRole: map[string]int64{}}
	rows, err := s.db.Query(`SELECT provider, role, cost_micros FROM usage_records`)
	if err != nil {
		return summary, err
	}
	defer rows.Close()
	for rows.Next() {
		var provider, role string
		var cost int64
		if err := rows.Scan(&provider, &role, &cost); err != nil {
			return summary, err
		}
		summary.Rows++
		summary.TotalCostMicros += cost
		summary.ByProvider[provider] += cost
		summary.ByRole[role] += cost
	}
	return summary, rows.Err()
}

// UsageTotal is convenience for the global view.
func (s *Store) UsageTotal() (int64, error) {
	var total int64
	err := s.db.QueryRow(`SELECT COALESCE(SUM(cost_micros), 0) FROM usage_records`).Scan(&total)
	return total, err
}
```

Add the shared scanner:

```go
func scanUsage(rows *sql.Rows) ([]UsageRecord, error) {
	out := make([]UsageRecord, 0)
	for rows.Next() {
		var rec UsageRecord
		var estimated int
		if err := rows.Scan(&rec.TurnNumber, &rec.Role, &rec.Provider, &rec.Model,
			&rec.InputTokens, &rec.OutputTokens, &rec.Characters, &rec.Requests, &estimated, &rec.CostMicros); err != nil {
			return nil, err
		}
		rec.Estimated = estimated != 0
		out = append(out, rec)
	}
	return out, rows.Err()
}
```

(`usage.go` needs `database/sql` imported for the scanner signature.)

- [ ] **Step 5: Run test and the package**

Run: `go test -run TestUsageRoundTripAndSummary ./pkg/storage/ -v && go test ./pkg/storage/`
Expected: PASS, including the existing migration tests.

- [ ] **Step 6: Commit**

```bash
git add pkg/storage/migrate.go pkg/storage/usage.go pkg/storage/usage_test.go
git commit -m "feat(storage): record per-call usage and cost"
```

---

### Task 5: Harness usage plumbing

**Files:**
- Modify: `pkg/harness/types.go` (`StreamChunk` ~line 8-15, `GenerateResponse` ~line 101-105)
- Modify: `pkg/harness/router.go` (struct ~line 10-16, generate/stream)
- Modify: `pkg/harness/extractor.go` (struct ~line 40-47, `Extract` ~line 336)
- Create: `pkg/harness/usage.go`
- Test: `pkg/harness/usage_test.go`

**Interfaces:**
- Produces:

```go
type Usage struct{ Provider, Model string; InputTokens, OutputTokens, Characters, Requests int; Estimated bool }
type UsageRecorder interface{ RecordUsage(role string, u Usage) }
type UsageSink interface{ RecordUsage(gameID string, turn int, role string, u Usage) }
type UsageContext struct{ /* … */ }
func NewUsageContext(sink UsageSink, gameID string) *UsageContext
func (c *UsageContext) SetTurn(turn int)
func (c *UsageContext) RecordUsage(role string, u Usage) // implements UsageRecorder
func (r *Router) SetUsageRecorder(rec UsageRecorder)
func (e *Extractor) SetUsageRecorder(rec UsageRecorder)
```

- [ ] **Step 1: Write the failing test**

Create `pkg/harness/usage_test.go`:

```go
package harness

import (
	"sync"
	"testing"
)

type captureSink struct {
	mu   sync.Mutex
	rows []struct {
		game string
		turn int
		role string
		u    Usage
	}
}

func (c *captureSink) RecordUsage(gameID string, turn int, role string, u Usage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rows = append(c.rows, struct {
		game string
		turn int
		role string
		u    Usage
	}{gameID, turn, role, u})
}

func TestUsageContextStampsTheTurn(t *testing.T) {
	sink := &captureSink{}
	ctx := NewUsageContext(sink, "campaign-01")
	ctx.SetTurn(3)
	ctx.RecordUsage("gm", Usage{Provider: "gemini", InputTokens: 10})
	ctx.RecordUsage("tts", Usage{Provider: "elevenlabs", Characters: 20})

	if len(sink.rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(sink.rows))
	}
	if sink.rows[0].turn != 3 || sink.rows[0].game != "campaign-01" || sink.rows[0].role != "gm" {
		t.Fatalf("first row = %+v", sink.rows[0])
	}
	if sink.rows[1].u.Characters != 20 {
		t.Fatalf("second row = %+v", sink.rows[1])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestUsageContextStampsTheTurn ./pkg/harness/ -v`
Expected: FAIL — undefined `NewUsageContext`.

- [ ] **Step 3: Add the usage types**

Create `pkg/harness/usage.go`:

```go
package harness

import "sync"

// Usage is one provider call's consumption. Estimated marks a value derived from
// request shape rather than reported by the provider.
type Usage struct {
	Provider     string `json:"provider,omitempty"`
	Model        string `json:"model,omitempty"`
	InputTokens  int    `json:"input_tokens,omitempty"`
	OutputTokens int    `json:"output_tokens,omitempty"`
	Characters   int    `json:"characters,omitempty"`
	Requests     int    `json:"requests,omitempty"`
	Estimated    bool   `json:"estimated,omitempty"`
}

// UsageRecorder is the sink a Router or Extractor reports to. It knows the role
// because the caller does.
type UsageRecorder interface {
	RecordUsage(role string, u Usage)
}

// UsageSink is where a UsageContext ultimately writes: a campaign database,
// stamped with the turn.
type UsageSink interface {
	RecordUsage(gameID string, turn int, role string, u Usage)
}

// UsageContext stamps every record with the campaign and the current turn. One
// is created per turn and shared by the Router and Extractor, so a concurrent
// extraction is attributed to the same turn.
type UsageContext struct {
	sink   UsageSink
	gameID string

	mu   sync.Mutex
	turn int
}

func NewUsageContext(sink UsageSink, gameID string) *UsageContext {
	return &UsageContext{sink: sink, gameID: gameID}
}

func (c *UsageContext) SetTurn(turn int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.turn = turn
}

func (c *UsageContext) RecordUsage(role string, u Usage) {
	if c == nil || c.sink == nil {
		return
	}
	c.mu.Lock()
	turn := c.turn
	gameID := c.gameID
	c.mu.Unlock()
	c.sink.RecordUsage(gameID, turn, role, u)
}
```

- [ ] **Step 4: Add usage fields and recording**

In `pkg/harness/types.go`:

```go
type StreamChunk struct {
	Text         string
	ToolCalls    []ToolCall
	Done         bool
	FinishReason string
	Error        error
	// Usage is set on the final chunk by a provider that reports it.
	Usage *Usage
}

type GenerateResponse struct {
	Text         string `json:"text"`
	CachedTokens int    `json:"cached_tokens,omitempty"`
	SessionID    string `json:"session_id,omitempty"`
	// Usage is set by a provider that reports token usage.
	Usage *Usage
}
```

In `pkg/harness/router.go`, add a recorder field and setter, and record after a successful call:

```go
	recorder UsageRecorder
```

```go
// SetUsageRecorder installs the sink LLM usage is reported to.
func (r *Router) SetUsageRecorder(rec UsageRecorder) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recorder = rec
}
```

In `GenerateForRole`, after `if code == "" {` and before `return res, nil`:

```go
		r.recordUsage(role, res.Usage)
		return res, nil
```

and in the fallback success branch. Add:

```go
func (r *Router) recordUsage(role string, u *Usage) {
	if u == nil {
		return
	}
	r.mu.RLock()
	rec := r.recorder
	r.mu.RUnlock()
	if rec != nil {
		rec.RecordUsage(role, *u)
	}
}
```

For `StreamForRole`, record the usage of the final chunk: in `forwardStream`, when a chunk with `Usage != nil` is received, call `r.recordUsage(role, chunk.Usage)`. Inspect `forwardStream` (`pkg/harness/router.go:185`) and add the call there; if it lacks the role, pass it in.

In `pkg/harness/extractor.go`, add:

```go
	recorder UsageRecorder
```

```go
// SetUsageRecorder installs the sink extraction usage is reported to.
func (e *Extractor) SetUsageRecorder(rec UsageRecorder) { e.recorder = rec }
```

and after the `e.model.Generate` call (~line 336):

```go
	if res != nil && res.Usage != nil && e.recorder != nil {
		e.recorder.RecordUsage("extractor", *res.Usage)
	}
```

- [ ] **Step 5: Run tests and vet**

Run: `go test ./pkg/harness/ && mise run lint`
Expected: PASS, vet clean.

- [ ] **Step 6: Commit**

```bash
git add pkg/harness/usage.go pkg/harness/usage_test.go pkg/harness/types.go pkg/harness/router.go pkg/harness/extractor.go
git commit -m "feat(harness): carry provider usage to a recorder"
```

---

### Task 6: Full verification

- [ ] **Step 1: Run the whole suite**

Run: `mise run test`
Expected: PASS (re-run `./pkg/gui/` once if the known flake appears).

- [ ] **Step 2: Vet and build**

Run: `mise run lint && mise run build`
Expected: clean.

---

## Self-Review Notes

- Spec coverage: taxonomy → Task 1; block registry → Task 2; pricing → Task 3; ledger → Task 4; usage plumbing → Task 5. The integration plan covers provider parsing (§3.1.1), media reporting, service/API wiring (§3.6, §3.7 API), block enforcement (§3.5), and the UI (§3.7).
- Import-cycle risk called out explicitly in Task 1 with the resolution (define the typed errors in `pkg/harness`, alias from `pkg/provider`).
- Type consistency: `harness.Usage`, `harness.UsageRecorder`, `harness.UsageSink`, `harness.UsageContext`, `storage.UsageRecord`, `pricing.Price`/`Micros`, `harness.LimitRegistry` are referenced consistently.
