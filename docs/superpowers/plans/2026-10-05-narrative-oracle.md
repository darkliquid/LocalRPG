# Narrative Oracle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the offline oracle read the turn's state and compose stateful prose from a richer grammar, deterministically.

**Architecture:** `pkg/provider/oracle` parses the assembled prompt into parts (tier, stats, stakes, entities, location) and composes prose from openers, consequences, a cast line, and a place line, with the stat value selecting variant sets.

**Tech Stack:** Go standard library (`regexp`, `math/rand` seeded per call).

**Spec:** `docs/superpowers/specs/2026-10-05-narrative-oracle-design.md`
**Depends on:** SYS-1.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `interface{}`, not `any`, in Go.
- Deterministic: the same prompt yields byte-identical prose; no clock, no global RNG.
- The oracle remains a `ModelProvider`; streaming is unchanged.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The prompt parts

**Files:**
- Modify: `pkg/provider/oracle/provider.go`
- Test: `pkg/provider/oracle/parse_test.go`

**Interfaces:**
- Consumes: the assembled prompt.
- Produces: `type parts struct { Tier, Action, Stakes, Location string; Stats []statValue; Entities []string }`, `func parsePrompt(prompt string) parts`.

- [ ] **Step 1: Write the failing test**

```go
func TestParsePrompt(t *testing.T) {
	prompt := "[MECHANICS RESULT: strong]\nPlayer Action: pick the lock\n" +
		"Stakes: the alarm sounds\nLocation: Saltmarch\nNear [[Garrick]] and [[Kaelen]].\n" +
		"Player stats: Edge 3, Grit 1.\n"
	p := parsePrompt(prompt)
	if p.Tier != "strong" || p.Action != "pick the lock" || p.Location != "Saltmarch" {
		t.Fatalf("parts = %+v", p)
	}
	if len(p.Entities) != 2 || p.Entities[0] != "Garrick" {
		t.Fatalf("entities = %v", p.Entities)
	}
	if len(p.Stats) != 2 || p.Stats[0].Name != "Edge" || p.Stats[0].Value != 3 {
		t.Fatalf("stats = %+v", p.Stats)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/provider/oracle/ -run TestParsePrompt -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add the `parts` struct and `parsePrompt` with small regexes for the tier, the action, the stakes, the
location, the wikilinks, and the `Player stats:` line. Keep the existing tier parsing.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/provider/oracle/ -run TestParsePrompt -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/provider/oracle/provider.go pkg/provider/oracle/parse_test.go
git commit -m "feat(oracle): parse the turn's state from the prompt"
```

---

### Task 2: The grammar

**Files:**
- Create: `pkg/provider/oracle/grammar.go`
- Test: `pkg/provider/oracle/grammar_test.go`

**Interfaces:**
- Consumes: `parts`.
- Produces: `func opener(p parts, rng *rand.Rand) string`, `func consequence(p parts, rng *rand.Rand) string`.

- [ ] **Step 1: Write the failing tests**

```go
func TestOpenerByTier(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	if opener(parts{Tier: "strong"}, rng) == "" || opener(parts{Tier: "miss"}, rng) == "" {
		t.Fatal("each tier should have an opener")
	}
}
func TestConsequenceReferencesStakes(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	got := consequence(parts{Tier: "miss", Stakes: "the alarm sounds"}, rng)
	if !strings.Contains(got, "alarm") {
		t.Fatalf("consequence = %q", got)
	}
}
func TestHighStatSelectsConfidentVariants(t *testing.T) {
	// A high governing stat should be able to select a confident variant set.
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/provider/oracle/ -run 'TestOpener|TestConsequence|TestHighStat' -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add variant sets per tier for openers and consequences, and a stat-driven selection: a high value
biases toward confident/competent variants, a low value toward strained ones. Each function takes the
seeded RNG.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/provider/oracle/ -run 'TestOpener|TestConsequence|TestHighStat' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/provider/oracle/grammar.go pkg/provider/oracle/grammar_test.go
git commit -m "feat(oracle): add the prose grammar"
```

---

### Task 3: Cast and place lines

**Files:**
- Modify: `pkg/provider/oracle/grammar.go`
- Test: `pkg/provider/oracle/grammar_test.go` (append)

**Interfaces:**
- Consumes: `parts`.
- Produces: `func castLine(p parts, rng *rand.Rand) string`, `func placeLine(p parts, rng *rand.Rand) string`.

- [ ] **Step 1: Write the failing tests**

```go
func TestCastLineNamesAnEntity(t *testing.T) {
	got := castLine(parts{Entities: []string{"Garrick"}}, rand.New(rand.NewSource(1)))
	if !strings.Contains(got, "Garrick") {
		t.Fatalf("cast line = %q", got)
	}
}
func TestPlaceLineNamesTheLocation(t *testing.T) {
	got := placeLine(parts{Location: "Saltmarch"}, rand.New(rand.NewSource(1)))
	if !strings.Contains(got, "Saltmarch") {
		t.Fatalf("place line = %q", got)
	}
}
func TestEmptyPartsProduceNoLines(t *testing.T) {
	if castLine(parts{}, rand.New(rand.NewSource(1))) != "" || placeLine(parts{}, rand.New(rand.NewSource(1))) != "" {
		t.Fatal("empty parts should yield no lines")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/provider/oracle/ -run 'TestCastLine|TestPlaceLine|TestEmptyParts' -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Return an empty string when there is no entity or location; otherwise a sentence naming a
deterministically chosen entity and the location.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/provider/oracle/ -run 'TestCastLine|TestPlaceLine|TestEmptyParts' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/provider/oracle/grammar.go pkg/provider/oracle/grammar_test.go
git commit -m "feat(oracle): add cast and place lines"
```

---

### Task 4: The composer

**Files:**
- Modify: `pkg/provider/oracle/provider.go`
- Test: `pkg/provider/oracle/provider_test.go` (append)

**Interfaces:**
- Consumes: `parsePrompt`, the grammar.
- Produces: `craftProse` composing the four parts.

- [ ] **Step 1: Write the failing tests**

```go
func TestCraftProseComposesParts(t *testing.T) {
	prompt := "[MECHANICS RESULT: miss]\nPlayer Action: x\nLocation: Saltmarch\nNear [[Garrick]].\n"
	got := craftProse(prompt)
	if !strings.Contains(got, "Saltmarch") || !strings.Contains(got, "Garrick") {
		t.Fatalf("prose = %q", got)
	}
}
func TestCraftProseIsDeterministic(t *testing.T) {
	prompt := "[MECHANICS RESULT: weak]\nPlayer Action: y\n"
	if craftProse(prompt) != craftProse(prompt) {
		t.Fatal("the same prompt should yield the same prose")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/provider/oracle/ -run TestCraftProse -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Compose opener, consequence, cast line, and place line (omitting empty ones) into a paragraph, seeded
from the prompt. Keep the existing provider streaming around it.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/provider/oracle/ -run TestCraftProse -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/provider/oracle/provider.go pkg/provider/oracle/provider_test.go
git commit -m "feat(oracle): compose stateful prose"
```

---

### Task 5: The fixture and verification

**Files:**
- Test: `pkg/provider/oracle/fixture_test.go`
- Modify: `pkg/harness/context_test.go` (share the fixture prompt)

**Interfaces:**
- Consumes: a real assembled prompt.
- Produces: a shared fixture so a context-section rename fails the oracle test.

- [ ] **Step 1: Write the fixture test**

```go
func TestOracleParsesARealAssembledPrompt(t *testing.T) {
	p := parsePrompt(realAssembledPromptFixture)
	if p.Tier == "" || p.Action == "" {
		t.Fatalf("parts = %+v", p)
	}
}
```

Capture a real prompt from the assembler into `realAssembledPromptFixture` (a const in the test).

- [ ] **Step 2: Run it**

Run: `go test ./pkg/provider/oracle/ -run TestOracleParsesAReal -v`
Expected: PASS.

- [ ] **Step 3: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 4: Confirm the acceptance criteria**

- The oracle reads tier, stats, stakes, entities, and location.
- The prose composes all four parts and omits empty ones.
- It is deterministic.
- A prompt with no result is neutral.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "test: pin the oracle against a real assembled prompt"
```
