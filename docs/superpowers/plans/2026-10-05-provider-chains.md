# Provider Chains Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a role or purpose declare an ordered provider chain and a selection rule, tried in order as a fallback chain.

**Architecture:** A chain plus a rule per role/purpose; the router and a media chain order the list by the rule (`first`, `cheapest`, `local-first`, `by-tag`) and try it in order, recording attempts.

**Tech Stack:** Go standard library.

**Spec:** `docs/superpowers/specs/2026-10-05-provider-chains-design.md`
**Depends on:** MP-1, MP-2, LF-3.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `any`, not `interface{}`, in Go.
- No chain is exactly today's behaviour.
- The ordering is a permutation of the chain.
- An unknown member or rule is a validation error.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The chain configuration

**Files:**
- Modify: `pkg/config/types.go` (`AgentRoleConfig`, `MediaConfig`), `pkg/config/manager.go`
- Test: `pkg/config/types_test.go` (append)

**Interfaces:**
- Consumes: nothing.
- Produces: `AgentRoleConfig.Chain/Select/Tag`, `MediaConfig.PurposeChains` (or a `PurposeConfig`), and accessors.

- [ ] **Step 1: Write the failing test**

```go
func TestChainRoundTrips(t *testing.T) {
	in := []byte("agents:\n  roles:\n    gm:\n      chain: [good, cheap]\n      select: cheapest\n")
	var cfg Config
	if err := yaml.Unmarshal(in, &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Agents.Roles["gm"].Chain) != 2 || cfg.Agents.Roles["gm"].Select != "cheapest" {
		t.Fatalf("role = %+v", cfg.Agents.Roles["gm"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/config/ -run TestChainRoundTrips -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add `Chain []string`, `Select string`, `Tag string` to `AgentRoleConfig`, and a chain per purpose on
the media config; default `Select` to `first` via an accessor.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/config/ -run TestChainRoundTrips -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/config
git commit -m "feat(config): declare a provider chain"
```

---

### Task 2: The ordering rules

**Files:**
- Create: `pkg/harness/chain.go`
- Test: `pkg/harness/chain_test.go`

**Interfaces:**
- Consumes: the pricing ledger, LF-3's tiers and features.
- Produces: `func OrderChain(ids []string, rule, tag string, price func(string) (int64, bool), tier func(string) (provider.Tier, []provider.Feature)) []string`.

- [ ] **Step 1: Write the failing tests**

```go
func TestOrderChainFirst(t *testing.T) {
	if got := OrderChain([]string{"a", "b"}, "first", "", nil, nil); got[0] != "a" {
		t.Fatalf("first = %v", got)
	}
}
func TestOrderChainCheapest(t *testing.T) {
	price := func(id string) (int64, bool) { return map[string]int64{"a": 10, "b": 1}[id], true }
	if got := OrderChain([]string{"a", "b"}, "cheapest", "", price, nil); got[0] != "b" {
		t.Fatalf("cheapest = %v", got)
	}
}
func TestOrderChainLocalFirst(t *testing.T) {
	tier := func(id string) (provider.Tier, []provider.Feature) {
		return map[string]provider.Tier{"cloud": provider.TierCloud, "local": provider.TierOfflineBasic}[id], nil
	}
	if got := OrderChain([]string{"cloud", "local"}, "local-first", "", nil, tier); got[0] != "local" {
		t.Fatalf("local-first = %v", got)
	}
}
func TestOrderChainIsAPermutation(t *testing.T) { /* any rule yields the same members */ }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/harness/ -run TestOrderChain -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Implement the four rules as stable sorts with the given accessors; `first` keeps the order; an
unknown rule defaults to `first`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/harness/ -run TestOrderChain -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/harness/chain.go pkg/harness/chain_test.go
git commit -m "feat(harness): order a provider chain"
```

---

### Task 3: The router uses the chain

**Files:**
- Modify: `pkg/harness/router.go`
- Test: `pkg/harness/router_test.go` (append)

**Interfaces:**
- Consumes: `OrderChain` (Task 2).
- Produces: `Router.SetChain`, and `GenerateForRole`/`StreamForRole` trying the ordered chain.

- [ ] **Step 1: Write the failing tests**

```go
func TestRouterTriesTheChainInOrder(t *testing.T) {
	// The first fails; the second is used; an attempt is recorded.
}
func TestRouterNoChainIsUnchanged(t *testing.T) { /* the existing primary/fallback */ }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/harness/ -run 'TestRouterTriesTheChain|TestRouterNoChain' -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add `SetChain`; in `GenerateForRole`, build the ordered list and try each, recording an `Attempt` per
failure and returning the first success; keep the existing path when there is no chain.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/harness/ -run 'TestRouterTriesTheChain|TestRouterNoChain' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/harness/router.go pkg/harness/router_test.go
git commit -m "feat(harness): try a provider chain in order"
```

---

### Task 4: The media chain

**Files:**
- Create: `pkg/media/chain.go`
- Test: `pkg/media/chain_test.go`

**Interfaces:**
- Consumes: `OrderChain` (Task 2), MP-1's named configs, MP-3's purposes.
- Produces: `func SelectChain(names []string, rule, tag string, cfg config.MediaConfig) []string`.

- [ ] **Step 1: Write the failing test**

```go
func TestSelectChainOrdersMedia(t *testing.T) {
	cfg := config.MediaConfig{
		TTS:          config.TTSConfig{BuiltinName: "elevenlabs"},
		TTSProviders: map[string]config.TTSConfig{"npc": {BuiltinName: "sherpa-onnx"}},
	}
	got := SelectChain([]string{"npc", "default"}, "local-first", "", cfg)
	if got[0] != "npc" {
		t.Fatalf("order = %v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/media/ -run TestSelectChain -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Resolve each name to its config, order by the rule using the descriptors and the ledger, and return
the ordered names; the registry (MP-1) then builds them in order.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/media/ -run TestSelectChain -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/media
git commit -m "feat(media): order a media provider chain"
```

---

### Task 5: Tracing, validation, and wiring

**Files:**
- Modify: `pkg/harness/factory.go`, `pkg/config/types.go` (`Validate`), `pkg/gui/service.go`
- Test: `pkg/config/types_test.go` (append), `pkg/gui/provider_chain_test.go`

**Interfaces:**
- Consumes: Tasks 1-4.
- Produces: `RouterFromConfig` setting chains; validation; a `router.select` trace.

- [ ] **Step 1: Write the failing tests**

```go
func TestValidateRejectsBadChain(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Agents.Roles["gm"] = AgentRoleConfig{Type: "http", Endpoint: "http://x", Chain: []string{"gone"}, Select: "cheapest"}
	if len(cfg.Validate()) == 0 {
		t.Fatal("an unknown chain member should be rejected")
	}
}
func TestChainSelectionIsTraced(t *testing.T) { /* a router.select event names the rule and choice */ }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/config/ -run TestValidateRejectsBadChain -v` and `go test ./pkg/gui/ -run TestChainSelectionIsTraced -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Set each role's chain in `RouterFromConfig`, validate chain members and rules, and trace the
selection.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/config/ -run TestValidateRejectsBadChain -v` and `go test ./pkg/gui/ -run TestChainSelectionIsTraced -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/config pkg/gui pkg/harness
git commit -m "feat: validate and trace provider chains"
```

---

### Task 6: Verification

- [ ] **Step 1: Regression guard**

Add a test that a role with no chain behaves exactly as today.

- [ ] **Step 2: Property test**

Add a test that every rule's output is a permutation of the input.

- [ ] **Step 3: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 4: Confirm the acceptance criteria**

- Each rule orders the chain as documented.
- The router and media try the chain in order, recording attempts.
- An empty chain is the existing behaviour.
- An unknown member or rule is a validation error.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "test: guard the no-chain path"
```
