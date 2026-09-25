# Continuity Verification and Open Threads Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Notice when a reply drifts from what the campaign already knows, report it on the turn rather than silently accepting it, and keep unresolved plot threads visible to both the narrator and the player.

**Architecture:** A deterministic pass over each turn's prose and segments produces findings: rules only, no model call, so it is cheap enough for every turn and predictable enough to trust. Findings are recorded on the turn and can be marked addressed in a per-campaign sidecar, because the timeline is append-only. Arcs gain a status and a derived last-advanced turn, which feeds an open-threads block in canon, an idle nudge in the UI, and the recap.

**Tech Stack:** Go 1.27.1, the standard library only, React 19 + TypeScript for the findings and threads UI.

**Spec:** `docs/superpowers/specs/2026-09-22-narrative-coherence-and-trace-design.md` (sections 7 and 8)

## Global Constraints

- Go tests use the standard library only (`testing`, `t.TempDir()`); no testify. Use `interface{}`, never `any`.
- No new dependencies, Go or Node.
- **No model call.** Every rule is deterministic; that is what makes it safe to run on every turn.
- Findings are advisory. Nothing is rewritten automatically: a correction is a `/gm` directive the player chooses to send.
- `history.jsonl` stays append-only. Addressing a finding writes to a sidecar, never edits a turn.
- Rules fire only on statements that claim canon. A capitalised noun is not evidence on its own.
- Every new key has a default matching today's behaviour, and `agents.continuity_checks: false` disables the pass.
- `go vet ./...`, `go test -count=1 ./...`, and `cd frontend && npx tsc --noEmit` must pass. Never commit the deletion of `pkg/gui/dist/.gitkeep`.

## Scope & Splitting

This is the last coherence plan, covering **increments 6**. It depends on the canon and recall plan (for `RenderState`, the section model, and retrieval) and the memory plan (for the chronicle and the recap it extends). The canon repair plan is independent of it.

---

### Task 1: Arcs have a status and an advance

**Files:**
- Create: `pkg/engine/threads.go`
- Test: `pkg/engine/threads_test.go`

**Interfaces:**
- Produces: `engine.ArcStatuses = []string{"open", "complicated", "resolved"}`
- Produces: `engine.ArcStatus(ent *entity.Entity) string`, returning `open` for anything outside the set
- Produces: `engine.Thread{ID, Name, Status string; LastAdvanced, Idle int}`
- Produces: `engine.OpenThreads(store *storage.Store, latestTurn int) ([]Thread, error)`

- [x] **Step 1: Write the failing test**

Create `pkg/engine/threads_test.go`:

```go
package engine

import (
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/state"
	"github.com/darkliquid/localrpg/pkg/storage"
)

func TestArcStatusFallsBackToOpen(t *testing.T) {
	if got := ArcStatus(nil); got != "open" {
		t.Errorf("ArcStatus(nil) = %q, want open", got)
	}

	unset := &entity.Entity{ID: "a", Name: "A", Type: "arc"}
	if got := ArcStatus(unset); got != "open" {
		t.Errorf("ArcStatus(unset) = %q, want open", got)
	}

	// A typo degrades rather than breaking the section that counts unresolved arcs.
	typo := &entity.Entity{Type: "arc", State: state.NewState(map[string]interface{}{"status": "compleated"})}
	if got := ArcStatus(typo); got != "open" {
		t.Errorf("ArcStatus(typo) = %q, want open", got)
	}

	resolved := &entity.Entity{Type: "arc", State: state.NewState(map[string]interface{}{"status": "resolved"})}
	if got := ArcStatus(resolved); got != "resolved" {
		t.Errorf("ArcStatus(resolved) = %q", got)
	}
}

func TestOpenThreadsReportsIdleUnresolvedArcs(t *testing.T) {
	store := newTestStore(t)
	if err := store.SaveEntity(&entity.Entity{ID: "the-miasma", Name: "The Creeping Miasma", Type: "arc", Body: "A creeping fog."}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveEntity(&entity.Entity{
		ID: "the-siege", Name: "The Iron Siege", Type: "arc", Body: "The siege grinds on.",
		State: state.NewState(map[string]interface{}{"status": "resolved"}),
	}); err != nil {
		t.Fatal(err)
	}

	// The miasma was last touched at turn 2, and the campaign is at turn 9.
	if err := store.SaveTurn(storage.TurnRecord{
		Number: 2, Timestamp: time.Now(), Mode: "Do", Narration: "The fog rose.",
		Entities: []storage.TurnEntityRef{{EntityID: "the-miasma", Mention: "wikilink"}},
	}); err != nil {
		t.Fatal(err)
	}

	threads, err := OpenThreads(store, 9)
	if err != nil {
		t.Fatalf("OpenThreads failed: %v", err)
	}
	if len(threads) != 1 {
		t.Fatalf("expected one open thread, got %+v", threads)
	}
	if threads[0].ID != "the-miasma" {
		t.Errorf("thread = %q, want the-miasma", threads[0].ID)
	}
	if threads[0].LastAdvanced != 2 || threads[0].Idle != 7 {
		t.Errorf("LastAdvanced = %d, Idle = %d; want 2 and 7", threads[0].LastAdvanced, threads[0].Idle)
	}
}

func TestOpenThreadsPutsTheStalestFirst(t *testing.T) {
	store := newTestStore(t)
	for _, id := range []string{"fresh", "stale"} {
		if err := store.SaveEntity(&entity.Entity{ID: id, Name: id, Type: "arc", Body: "An arc."}); err != nil {
			t.Fatal(err)
		}
	}
	for number, id := range map[int]string{1: "stale", 8: "fresh"} {
		if err := store.SaveTurn(storage.TurnRecord{
			Number: number, Timestamp: time.Now(), Mode: "Do", Narration: "Something.",
			Entities: []storage.TurnEntityRef{{EntityID: id, Mention: "wikilink"}},
		}); err != nil {
			t.Fatal(err)
		}
	}

	threads, err := OpenThreads(store, 9)
	if err != nil {
		t.Fatal(err)
	}
	if len(threads) != 2 {
		t.Fatalf("expected two threads, got %+v", threads)
	}
	// A thread nobody has touched for eight turns is the one worth surfacing.
	if threads[0].ID != "stale" {
		t.Errorf("expected the staler thread first, got %+v", threads)
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

Run: `go test -run 'TestArcStatus|TestOpenThreads' ./pkg/engine/`
Expected: FAIL — `undefined: ArcStatus`.

- [x] **Step 3: Write the implementation**

Create `pkg/engine/threads.go`:

```go
package engine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// ArcStatuses is the closed set an arc's status may take. Anything else is treated
// as open, so a typo degrades rather than hiding a live thread.
var ArcStatuses = []string{"open", "complicated", "resolved"}

// Thread is one unresolved arc as the prompt and the UI see it.
type Thread struct {
	ID           string
	Name         string
	Status       string
	LastAdvanced int
	Idle         int
}

// ArcStatus reads an arc's status, treating anything outside the set as open.
func ArcStatus(ent *entity.Entity) string {
	if ent == nil || ent.State == nil {
		return "open"
	}

	raw, ok := ent.State.Get("status")
	if !ok {
		return "open"
	}
	status, ok := raw.(string)
	if !ok {
		return "open"
	}

	normalised := strings.ToLower(strings.TrimSpace(status))
	for _, allowed := range ArcStatuses {
		if normalised == allowed {
			return allowed
		}
	}
	return "open"
}

// OpenThreads lists the arcs that are still unresolved, stalest first, each with
// how long since a turn last touched it. It reads the index rather than the notes,
// because "when was this last advanced" is a question about turns.
func OpenThreads(store *storage.Store, latestTurn int) ([]Thread, error) {
	if store == nil {
		return nil, nil
	}

	summaries, err := store.ListEntities()
	if err != nil {
		return nil, fmt.Errorf("list entities: %w", err)
	}

	threads := make([]Thread, 0)
	for _, summary := range summaries {
		if summary.Type != "arc" {
			continue
		}

		ent, err := store.GetEntity(summary.ID)
		if err != nil || ent == nil {
			continue
		}
		if status := ArcStatus(ent); status == "resolved" {
			continue
		}

		lastAdvanced := 0
		if numbers, err := store.ListTurnsForEntity(summary.ID); err == nil && len(numbers) > 0 {
			lastAdvanced = numbers[0]
			for _, number := range numbers {
				if number > lastAdvanced {
					lastAdvanced = number
				}
			}
		}

		threads = append(threads, Thread{
			ID:           summary.ID,
			Name:         summary.Name,
			Status:       ArcStatus(ent),
			LastAdvanced: lastAdvanced,
			Idle:         latestTurn - lastAdvanced,
		})
	}

	// Stalest first: the thread nobody has touched is the one most likely to be
	// forgotten, and the one worth a nudge.
	sort.SliceStable(threads, func(i, j int) bool {
		if threads[i].Idle == threads[j].Idle {
			return threads[i].ID < threads[j].ID
		}
		return threads[i].Idle > threads[j].Idle
	})
	return threads, nil
}
```

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/engine/ && go vet ./...`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/engine/threads.go pkg/engine/threads_test.go
git commit -m "feat(engine): give arcs a status and a last-advanced turn"
```

---

### Task 2: Open threads in canon

**Files:**
- Modify: `pkg/config/types.go`
- Modify: `pkg/harness/context.go`
- Modify: `pkg/engine/orchestrator.go`
- Test: `pkg/harness/context_test.go`

**Interfaces:**
- Consumes: `engine.OpenThreads` (Task 1)
- Produces: `ContextRequest.Threads []string`
- Produces: `config.AgentsConfig.ThreadIdleTurns`, `.ThreadsMax`; accessors `ThreadIdleTurns() int` (10) and `ThreadsMax() int` (8)

- [x] **Step 1: Write the failing test**

Append to `pkg/harness/context_test.go`:

```go
func TestOpenThreadsAreAlwaysInThePrompt(t *testing.T) {
	assembler := NewContextAssembler(newTestEntityStore(t))

	result, err := assembler.Assemble(ContextRequest{
		Action: "I ask about the fog",
		// Canon is never trimmed, so an idle thread reaches the narrator even under
		// a budget that is far too small for the rest.
		Threads: []string{"The Creeping Miasma (open, last advanced 7 turns ago)"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result.Prompt, "OPEN THREADS") || !strings.Contains(result.Prompt, "Creeping Miasma") {
		t.Fatalf("expected the thread in canon:\n%s", result.Prompt)
	}

	for _, section := range result.Sections {
		if section.Name == "canon" && !section.Included {
			t.Errorf("canon was trimmed, which must never happen")
		}
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

Run: `go test -run TestOpenThreadsAreAlwaysInThePrompt ./pkg/harness/`
Expected: FAIL — no thread in the prompt.

- [x] **Step 3: Add the config**

In `pkg/config/types.go`, extend `AgentsConfig` and add the accessors:

```go
	// ThreadIdleTurns is when the UI nudges about a thread. It is presentation only:
	// the prompt always lists unresolved threads, however long they have been quiet.
	ThreadIdleTurns int `yaml:"thread_idle_turns" json:"thread_idle_turns"`
	// ThreadsMax caps the open-threads block, most stale first.
	ThreadsMax int `yaml:"threads_max" json:"threads_max"`
```

```go
// ThreadIdleTurns is when the UI nudges about an unresolved thread.
func (c *Config) ThreadIdleTurns() int {
	if c.Agents.ThreadIdleTurns <= 0 {
		return 10
	}
	return c.Agents.ThreadIdleTurns
}

// ThreadsMax caps how many open threads the prompt carries.
func (c *Config) ThreadsMax() int {
	if c.Agents.ThreadsMax <= 0 {
		return 8
	}
	return c.Agents.ThreadsMax
}
```

- [x] **Step 4: Add the section**

In `pkg/harness/context.go`, add to `ContextRequest`:

```go
	// Threads are the unresolved arcs, already rendered with their idle counts. They
	// are canon, so they are never trimmed: a thread goes quiet precisely when the
	// narrator should be prompted to return to it.
	Threads []string
```

render them inside `assembleCanon`, after the present characters:

```go
	if len(req.Threads) > 0 {
		sb.WriteString("\n## OPEN THREADS\n")
		for _, thread := range req.Threads {
			sb.WriteString("- " + thread + "\n")
		}
	}
```

In `pkg/engine/orchestrator.go`, build them at assembly time and pass them in:

```go
	// Open threads are canon, so they are built whether or not anything in the scene
	// touches them: a thread the party has walked away from is the one most likely to
	// be forgotten.
	threads := make([]string, 0)
	if open, err := OpenThreads(o.store, turnNum); err == nil {
		for index, thread := range open {
			if index == o.threadsMax {
				break
			}
			threads = append(threads, fmt.Sprintf("%s (%s, last advanced %d turns ago)", thread.Name, thread.Status, thread.Idle))
		}
	}
```

with `threadsMax int` on the orchestrator and:

```go
// SetThreadsMax caps how many open threads the prompt carries.
func (o *TurnOrchestrator) SetThreadsMax(max int) {
	o.threadsMax = max
}
```

and default it in `NewTurnOrchestrator` beside the other defaults, or apply `defaultThreadsMax = 8` in the loop when unset. Use:

```go
			if index == o.threadsCap() {
				break
			}
```

```go
// threadsCap is the configured cap, or a sane one when nothing set it.
func (o *TurnOrchestrator) threadsCap() int {
	if o.threadsMax <= 0 {
		return defaultThreadsMax
	}
	return o.threadsMax
}
```

Add `const defaultThreadsMax = 8` beside the other engine constants, and pass `Threads: threads` in the `harness.ContextRequest`.

- [x] **Step 5: Wire the service**

In `pkg/gui/service.go`, `prepareTurn` sets it from config:

```go
	orchestrator.SetThreadsMax(cfg.ThreadsMax())
```

- [x] **Step 6: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/harness/ ./pkg/engine/ ./pkg/config/ && go vet ./...`
Expected: PASS.

- [x] **Step 7: Commit**

```bash
git add pkg/config/ pkg/harness/context.go pkg/harness/context_test.go pkg/engine/ pkg/gui/service.go
git commit -m "feat(harness): keep unresolved threads in front of the narrator"
```

---

### Task 3: The continuity rules

**Files:**
- Create: `pkg/engine/continuity.go`
- Test: `pkg/engine/continuity_test.go`

**Interfaces:**
- Produces: `engine.ContinuityFinding{Rule, Note string}`
- Produces: `engine.CheckContinuity(store *storage.Store, turn *Turn, locationID, playerID string) []ContinuityFinding`
- Produces: rule names `unknown-entity`, `location-drift`, `unresolved-speaker`, `reintroduction`, `state-contradiction`

- [x] **Step 1: Write the failing test**

Create `pkg/engine/continuity_test.go`. One test per rule, plus the false positives that must stay silent:

```go
package engine

import (
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/state"
)

func continuityStore(t *testing.T) *storage.Store {
	t.Helper()

	store := newTestStore(t)
	mustSave := func(ent *entity.Entity) {
		t.Helper()
		if err := store.SaveEntity(ent); err != nil {
			t.Fatal(err)
		}
	}

	mustSave(&entity.Entity{ID: "guard-kael", Name: "Guard Kael", Type: "character", Body: "A warden."})
	mustSave(&entity.Entity{ID: "aldon-harbour", Name: "Aldon Harbour", Type: "location", Body: "Salt."})
	mustSave(&entity.Entity{ID: "oakhaven-tavern", Name: "Oakhaven Tavern", Type: "location", Body: "Ale."})
	mustSave(&entity.Entity{
		ID: "the-bastion", Name: "The Ashen Bastion", Type: "location", Body: "A sanctuary.",
		State: state.NewState(map[string]interface{}{"brazier_lit": true}),
	})
	return store
}

func ruleNames(findings []ContinuityFinding) []string {
	names := make([]string, 0, len(findings))
	for _, finding := range findings {
		names = append(names, finding.Rule)
	}
	return names
}

func hasRule(findings []ContinuityFinding, rule string) bool {
	for _, finding := range findings {
		if finding.Rule == rule {
			return true
		}
	}
	return false
}

func TestContinuityFlagsANameIntroducedWithoutANote(t *testing.T) {
	store := continuityStore(t)

	turn := &Turn{Number: 1, Narration: "A figure steps forward.\nThe Pale Sister: \"You are late.\""}
	findings := CheckContinuity(store, turn, "aldon-harbour", "player")

	if !hasRule(findings, "unknown-entity") {
		t.Errorf("expected an unknown entity finding, got %v", ruleNames(findings))
	}

	// A capitalised phrase that claims nothing is not evidence.
	quiet := &Turn{Number: 1, Narration: "The rain fell on the harbour and the gulls cried."}
	if hasRule(CheckContinuity(store, quiet, "aldon-harbour", "player"), "unknown-entity") {
		t.Errorf("plain prose must not be flagged")
	}
}

func TestContinuityFlagsALocationThePartyIsNotAt(t *testing.T) {
	store := continuityStore(t)

	turn := &Turn{Number: 1, Narration: "She remembers Oakhaven Tavern fondly."}
	findings := CheckContinuity(store, turn, "aldon-harbour", "player")

	if !hasRule(findings, "location-drift") {
		t.Errorf("expected a location drift finding, got %v", ruleNames(findings))
	}

	// Naming where they are is not drift.
	here := &Turn{Number: 1, Narration: "Aldon Harbour is quiet tonight."}
	if hasRule(CheckContinuity(store, here, "aldon-harbour", "player"), "location-drift") {
		t.Errorf("naming the current location must not be flagged")
	}
}

func TestContinuityFlagsSpeechThatResolvedToNobody(t *testing.T) {
	store := continuityStore(t)

	turn := &Turn{Number: 1, Narration: "Someone whispers.\nA Nameless Voice: \"Behind you.\""}
	findings := CheckContinuity(store, turn, "aldon-harbour", "player")

	if !hasRule(findings, "unresolved-speaker") {
		t.Errorf("expected an unresolved speaker finding, got %v", ruleNames(findings))
	}

	// A line that resolves is not a finding.
	known := &Turn{Number: 1, Narration: "Guard Kael: \"Behind you.\""}
	if hasRule(CheckContinuity(store, known, "aldon-harbour", "player"), "unresolved-speaker") {
		t.Errorf("a resolved speaker must not be flagged")
	}
}

func TestContinuityFlagsReintroducingSomeoneKnown(t *testing.T) {
	store := continuityStore(t)

	turn := &Turn{Number: 2, Narration: "A stranger named Guard Kael waits by the water."}
	findings := CheckContinuity(store, turn, "aldon-harbour", "player")

	if !hasRule(findings, "reintroduction") {
		t.Errorf("expected a reintroduction finding, got %v", ruleNames(findings))
	}
}

func TestContinuityFlagsAContradictedState(t *testing.T) {
	store := continuityStore(t)

	// The note says the brazier is lit, and the prose says it is out.
	turn := &Turn{Number: 2, Narration: "The brazier was dark and the bastion felt cold.", Location: "the-bastion"}
	findings := CheckContinuity(store, turn, "the-bastion", "player")

	if !hasRule(findings, "state-contradiction") {
		t.Errorf("expected a state contradiction finding, got %v", ruleNames(findings))
	}

	// Consistent prose is silent.
	lit := &Turn{Number: 2, Narration: "The brazier burned steadily, keeping the mist back.", Location: "the-bastion"}
	if hasRule(CheckContinuity(store, lit, "the-bastion", "player"), "state-contradiction") {
		t.Errorf("consistent prose must not be flagged")
	}
}

func TestContinuityFindingsSayWhatTheySaw(t *testing.T) {
	store := continuityStore(t)

	turn := &Turn{Number: 1, Narration: "She remembers Oakhaven Tavern fondly."}
	findings := CheckContinuity(store, turn, "aldon-harbour", "player")

	for _, finding := range findings {
		if strings.TrimSpace(finding.Note) == "" {
			t.Errorf("finding %q carries no explanation", finding.Rule)
		}
	}
}
```

Add `"github.com/darkliquid/localrpg/pkg/storage"` to that file's imports.

- [x] **Step 2: Run the test to verify it fails**

Run: `go test -run TestContinuity ./pkg/engine/`
Expected: FAIL — `undefined: CheckContinuity`.

- [x] **Step 3: Write the rules**

Create `pkg/engine/continuity.go`:

```go
package engine

import (
	"fmt"
	"strings"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// ContinuityFinding is one thing a turn's prose does that canon disagrees with. It
// is advisory: the engine reports, and the player decides.
type ContinuityFinding struct {
	Rule string
	Note string
}

// Rule names, as they appear in the trace and in the sidecar that records which
// findings a player has dealt with.
const (
	RuleUnknownEntity      = "unknown-entity"
	RuleLocationDrift      = "location-drift"
	RuleUnresolvedSpeaker  = "unresolved-speaker"
	RuleReintroduction     = "reintroduction"
	RuleStateContradiction = "state-contradiction"
)

// discoveryCues are the phrasings that introduce someone as if for the first time.
var discoveryCues = []string{"a stranger", "an unfamiliar", "a figure", "a newcomer", "introduces himself", "introduces herself", "you do not recognise"}

// contradictionCues describe a state being false, for the boolean keys a note
// tracks. They are deliberately few: this rule is the most likely to be wrong, and
// a short list keeps it from crying wolf.
var contradictionCues = []string{"cold", "dark", "unlit", "doused", "out", "dead", "extinguished", "guttered"}

// CheckContinuity compares a turn against what the campaign knows. It is
// deterministic and model-free, which is what makes it safe to run every turn.
func CheckContinuity(store *storage.Store, turn *Turn, locationID, playerID string) []ContinuityFinding {
	if store == nil || turn == nil {
		return nil
	}

	findings := make([]ContinuityFinding, 0)
	findings = append(findings, checkUnknownEntities(store, turn)...)
	findings = append(findings, checkLocationDrift(store, turn, locationID)...)
	findings = append(findings, checkUnresolvedSpeakers(store, turn)...)
	findings = append(findings, checkReintroductions(store, turn)...)
	findings = append(findings, checkStateContradictions(store, turn, locationID)...)
	return findings
}

// checkUnknownEntities flags a name that claims canon and has no note. It fires on a
// speaker attribution or a naming construction, never on a capitalised phrase alone:
// scenery is invented legitimately, and flagging it would make the rule noise.
func checkUnknownEntities(store *storage.Store, turn *Turn) []ContinuityFinding {
	findings := make([]ContinuityFinding, 0)

	for _, line := range strings.Split(turn.Narration, "\n") {
		candidate := speakingName(line)
		if candidate == "" {
			continue
		}
		if resolvesToEntity(store, candidate) {
			continue
		}
		findings = append(findings, ContinuityFinding{
			Rule: RuleUnknownEntity,
			Note: fmt.Sprintf("%q speaks but has no note", candidate),
		})
	}

	for _, phrase := range namedPhrases(turn.Narration) {
		if resolvesToEntity(store, phrase) {
			continue
		}
		findings = append(findings, ContinuityFinding{
			Rule: RuleUnknownEntity,
			Note: fmt.Sprintf("%q is named but has no note", phrase),
		})
	}
	return findings
}

// speakingName returns the speaker of a line shaped like `Name: "…"`, or "".
func speakingName(line string) string {
	trimmed := strings.TrimSpace(line)
	colon := strings.Index(trimmed, ":")
	if colon <= 0 {
		return ""
	}

	rest := strings.TrimSpace(trimmed[colon+1:])
	if !strings.HasPrefix(rest, "\"") && !strings.HasPrefix(rest, "“") {
		return ""
	}

	return strings.TrimSpace(entity.WikilinkTarget(strings.Trim(trimmed[:colon], "*_ ")))
}

// namedPhrases returns the names a line claims through a naming construction.
func namedPhrases(text string) []string {
	phrases := make([]string, 0)
	lowered := strings.ToLower(text)

	for _, cue := range []string{"named ", "called ", "known as "} {
		index := 0
		for {
			found := strings.Index(lowered[index:], cue)
			if found == -1 {
				break
			}
			start := index + found + len(cue)

			phrase := ""
			for _, word := range strings.Fields(text[start:]) {
				if word == "" || !isCapitalised(word) {
					break
				}
				phrase = strings.TrimSpace(phrase + " " + strings.Trim(word, ".,;:\"'"))
			}
			if phrase != "" {
				phrases = append(phrases, phrase)
			}
			index = start
		}
	}
	return phrases
}

func isCapitalised(word string) bool {
	runes := []rune(strings.Trim(word, ".,;:\"'"))
	if len(runes) == 0 {
		return false
	}
	return runes[0] >= 'A' && runes[0] <= 'Z'
}

// resolvesToEntity reports whether a name belongs to something the campaign knows,
// by name or by alias.
func resolvesToEntity(store *storage.Store, name string) bool {
	return harness.ResolveSpeakerID(store, name) != ""
}

// checkLocationDrift flags prose that names a known location other than the one the
// party is standing in, because that is the prose teleporting them.
func checkLocationDrift(store *storage.Store, turn *Turn, locationID string) []ContinuityFinding {
	findings := make([]ContinuityFinding, 0)

	summaries, err := store.ListEntities()
	if err != nil {
		return findings
	}

	for _, summary := range summaries {
		if summary.Type != "location" || summary.ID == locationID {
			continue
		}
		if !strings.Contains(turn.Narration, summary.Name) {
			continue
		}
		findings = append(findings, ContinuityFinding{
			Rule: RuleLocationDrift,
			Note: fmt.Sprintf("%q is named while the party is elsewhere", summary.Name),
		})
	}
	return findings
}

// checkUnresolvedSpeakers flags a line shaped like speech whose speaker is unknown.
// The parser leaves such a line in the narration, so this is the only place the
// loss is visible.
func checkUnresolvedSpeakers(store *storage.Store, turn *Turn) []ContinuityFinding {
	findings := make([]ContinuityFinding, 0)

	for _, line := range strings.Split(turn.Narration, "\n") {
		candidate := speakingName(line)
		if candidate == "" || resolvesToEntity(store, candidate) {
			continue
		}
		findings = append(findings, ContinuityFinding{
			Rule: RuleUnresolvedSpeaker,
			Note: fmt.Sprintf("%q speaks but is not a known character", candidate),
		})
	}
	return findings
}

// checkReintroductions flags a known character introduced as if new, which is how a
// model quietly forgets the party has met them.
func checkReintroductions(store *storage.Store, turn *Turn) []ContinuityFinding {
	findings := make([]ContinuityFinding, 0)
	lowered := strings.ToLower(turn.Narration)

	for _, cue := range discoveryCues {
		if !strings.Contains(lowered, cue) {
			continue
		}

		summaries, err := store.ListEntities()
		if err != nil {
			return findings
		}
		for _, summary := range summaries {
			if summary.Type != "character" || summary.Name == "" {
				continue
			}
			if strings.Contains(turn.Narration, summary.Name) {
				findings = append(findings, ContinuityFinding{
					Rule: RuleReintroduction,
					Note: fmt.Sprintf("%q is introduced as new but is already known", summary.Name),
				})
			}
		}
		break
	}
	return findings
}

// checkStateContradictions flags prose that describes a tracked boolean state as
// false. It is scoped to the scene and to those present, because a state mentioned
// elsewhere is not the narrator contradicting anything.
func checkStateContradictions(store *storage.Store, turn *Turn, locationID string) []ContinuityFinding {
	findings := make([]ContinuityFinding, 0)

	ids := []string{locationID, turn.Location}
	if edges, err := store.GetEdgesFrom(locationID); err == nil {
		for _, edge := range edges {
			ids = append(ids, edge.TargetID)
		}
	}

	lowered := strings.ToLower(turn.Narration)
	seen := make(map[string]bool, len(ids))

	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true

		ent, err := store.GetEntity(id)
		if err != nil || ent == nil || ent.State == nil {
			continue
		}

		for key, value := range ent.State.Raw() {
			lit, ok := value.(bool)
			if !ok || !lit {
				continue
			}
			if !strings.Contains(lowered, strings.ToLower(contradictionSubject(key))) {
				continue
			}
			for _, cue := range contradictionCues {
				if strings.Contains(lowered, cue) {
					findings = append(findings, ContinuityFinding{
						Rule: RuleStateContradiction,
						Note: fmt.Sprintf("%q is described as %s while %s says it is true", contradictionSubject(key), cue, ent.Name),
					})
					break
				}
			}
		}
	}
	return findings
}

// contradictionSubject turns a state key into the word prose would use: brazier_lit
// is described by talking about the brazier.
func contradictionSubject(key string) string {
	if index := strings.IndexAny(key, "_-"); index > 0 {
		return key[:index]
	}
	return key
}
```

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/engine/ && go vet ./...`
Expected: PASS. If `unknown-entity` fires twice for one line, keep both: the two checks answer different questions, and the note distinguishes them.

- [x] **Step 5: Commit**

```bash
git add pkg/engine/continuity.go pkg/engine/continuity_test.go
git commit -m "feat(engine): notice when a reply drifts from what the campaign knows"
```

---

### Task 4: Findings on the turn

**Files:**
- Modify: `pkg/engine/history.go`
- Modify: `pkg/engine/orchestrator.go`
- Modify: `pkg/config/types.go`
- Modify: `pkg/gui/types.go`, `pkg/gui/service.go`
- Modify: `frontend/src/types.ts`, `frontend/src/components/ChronicleView.tsx`
- Test: `pkg/engine/orchestrator_test.go`, `pkg/gui/service_test.go`

**Interfaces:**
- Produces: `Turn.ContinuityNotes []string`, `TurnDTO.ContinuityNotes []string`
- Produces: `(*TurnOrchestrator).SetContinuityChecks(bool)`, `config.AgentsConfig.ContinuityChecks *bool` with `ContinuityChecks() bool` defaulting to true

- [x] **Step 1: Write the failing test**

Append to `pkg/engine/orchestrator_test.go`:

```go
func TestATurnRecordsWhatItsProseContradicts(t *testing.T) {
	provider := &scriptedStreamProvider{chunks: []string{"She remembers Oakhaven Tavern fondly."}}
	orchestrator, timeline, store := streamingOrchestrator(t, provider)
	orchestrator.SetContinuityChecks(true)
	orchestrator.SetChronicler(nil)

	if err := store.SaveEntity(&entity.Entity{ID: "oakhaven-tavern", Name: "Oakhaven Tavern", Type: "location", Body: "Ale."}); err != nil {
		t.Fatal(err)
	}

	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I look around", nil)
	if err != nil {
		t.Fatalf("turn failed: %v", err)
	}
	if len(turn.ContinuityNotes) == 0 {
		t.Fatalf("expected a continuity note about the named location")
	}

	// The note is persisted with the turn, so a reader sees it later.
	turns, err := timeline.history.LoadHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 || len(turns[0].ContinuityNotes) == 0 {
		t.Errorf("expected the note recorded, got %+v", turns)
	}
}

func TestContinuityChecksCanBeSwitchedOff(t *testing.T) {
	provider := &scriptedStreamProvider{chunks: []string{"She remembers Oakhaven Tavern fondly."}}
	orchestrator, _, store := streamingOrchestrator(t, provider)
	orchestrator.SetContinuityChecks(false)

	if err := store.SaveEntity(&entity.Entity{ID: "oakhaven-tavern", Name: "Oakhaven Tavern", Type: "location", Body: "Ale."}); err != nil {
		t.Fatal(err)
	}

	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I look around", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(turn.ContinuityNotes) != 0 {
		t.Errorf("expected no notes when the checks are off, got %v", turn.ContinuityNotes)
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

Run: `go test -run 'TestATurnRecordsWhatItsProseContradicts|TestContinuityChecksCanBeSwitchedOff' ./pkg/engine/`
Expected: FAIL — `orchestrator.SetContinuityChecks undefined`.

- [x] **Step 3: Record them**

In `pkg/engine/history.go`, add to `Turn`:

```go
	// ContinuityNotes record prose that contradicts what the campaign knows. They are
	// advisory: nothing is rewritten, and a correction is the player's to send.
	ContinuityNotes []string `json:"continuity_notes,omitempty"`
```

In `pkg/engine/orchestrator.go`:

```go
	continuityChecks *bool
```

```go
// SetContinuityChecks turns the deterministic continuity pass on or off. A nil value
// is the default, which is on.
func (o *TurnOrchestrator) SetContinuityChecks(enabled bool) {
	o.continuityChecks = &enabled
}

// continuityEnabled reports whether the pass should run.
func (o *TurnOrchestrator) continuityEnabled() bool {
	return o.continuityChecks == nil || *o.continuityChecks
}
```

and after the segments are built, before the reconcile:

```go
	if o.continuityEnabled() {
		for _, finding := range CheckContinuity(o.store, &turn, locationID, o.playerID) {
			turn.ContinuityNotes = append(turn.ContinuityNotes, finding.Note)
		}
	}
```

In `pkg/config/types.go`, add to `AgentsConfig` and an accessor that distinguishes "unset" from "off":

```go
	// ContinuityChecks runs the deterministic drift pass. A pointer distinguishes
	// "not configured" from "switched off", because the default is on.
	ContinuityChecks *bool `yaml:"continuity_checks" json:"continuity_checks,omitempty"`
```

```go
// ContinuityChecks runs the deterministic drift pass unless it is switched off.
func (c *Config) ContinuityChecks() bool {
	return c.Agents.ContinuityChecks == nil || *c.Agents.ContinuityChecks
}
```

In `pkg/gui/types.go`, add `ContinuityNotes []string json:"continuity_notes,omitempty"` to `TurnDTO`, and map it in `segmentDTOs`' caller, `turnDTO`:

```go
		ContinuityNotes: turn.ContinuityNotes,
```

In `pkg/gui/service.go`, `prepareTurn` applies the config:

```go
	orchestrator.SetContinuityChecks(cfg.ContinuityChecks())
```

- [x] **Step 4: Show them**

In `frontend/src/types.ts`, add `continuity_notes?: string[];` to `Turn`.

In `frontend/src/components/ChronicleView.tsx`, under the turn's segments, beside the existing context-trim note:

```tsx
            {turn.continuity_notes && turn.continuity_notes.length > 0 && (
              <div className="text-xs font-mono text-amber-400/90 pt-1 space-y-1">
                {turn.continuity_notes.map((note, index) => (
                  <div key={index} className="flex items-start gap-2">
                    <span>{note}</span>
                    <button
                      onClick={() => onCorrect?.(note)}
                      className="shrink-0 px-1.5 py-0.5 rounded border border-amber-500/40 hover:bg-amber-600/20 cursor-pointer transition-colors"
                      title="Send this as a correction to the GM"
                    >
                      Correct
                    </button>
                  </div>
                ))}
              </div>
            )}
```

with `onCorrect?: (note: string) => void` added to the props, and wired in `App.tsx`:

```tsx
  const handleCorrect = (note: string) => {
    void handleActionSubmit('GM', `/gm ${note}`);
  };
```

```tsx
                onCorrect={handleCorrect}
```

- [x] **Step 5: Run the full gate**

Run: `go test -count=1 ./... && go vet ./... && (cd frontend && npx tsc --noEmit)`
Expected: PASS.

- [x] **Step 6: Commit**

```bash
git add pkg/engine/ pkg/config/ pkg/gui/ frontend/src/
git commit -m "feat(engine): report a turn's continuity findings on the turn"
```

---

### Task 5: Addressing a finding

**Files:**
- Create: `pkg/gui/findings.go`
- Modify: `pkg/gui/service.go`, `pkg/gui/server.go`, `pkg/gui/types.go`
- Modify: `frontend/src/api/client.ts`, `frontend/src/types.ts`, `frontend/src/App.tsx`, `frontend/src/components/ChronicleView.tsx`
- Test: `pkg/gui/service_test.go`, `pkg/gui/server_test.go`

**Interfaces:**
- Produces: `games/<id>/findings.json` holding `{"addressed":[{"turn":3,"rule":"unknown-entity"}]}`
- Produces: `(*Service).Findings(gameID string) (FindingsDTO, error)`, `(*Service).AddressFinding(gameID string, turn int, rule string) error`
- Produces: `GET`/`POST /api/game/{id}/findings`

- [x] **Step 1: Write the failing test**

Append to `pkg/gui/service_test.go`:

```go
func TestAddressingAFindingSurvivesAReload(t *testing.T) {
	gameID, svc := setupTestGame(t)

	before, err := svc.Findings(gameID)
	if err != nil {
		t.Fatalf("Findings failed: %v", err)
	}
	if len(before.Addressed) != 0 {
		t.Fatalf("expected nothing addressed, got %+v", before)
	}

	if err := svc.AddressFinding(gameID, 3, "unknown-entity"); err != nil {
		t.Fatalf("AddressFinding failed: %v", err)
	}
	// Addressing the same finding twice is not an error and not a duplicate.
	if err := svc.AddressFinding(gameID, 3, "unknown-entity"); err != nil {
		t.Fatal(err)
	}

	after, err := svc.Findings(gameID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Addressed) != 1 {
		t.Fatalf("expected one addressed finding, got %+v", after.Addressed)
	}
	if after.Addressed[0].Turn != 3 || after.Addressed[0].Rule != "unknown-entity" {
		t.Errorf("unexpected record: %+v", after.Addressed[0])
	}

	// The sidecar is how this survives, because the timeline is append-only.
	path := filepath.Join(svc.GetResolver().GameDir(gameID), "findings.json")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected a sidecar on disk: %v", err)
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

Run: `go test -run TestAddressingAFinding ./pkg/gui/`
Expected: FAIL — `svc.Findings undefined`.

- [x] **Step 3: Write the sidecar**

Create `pkg/gui/findings.go`:

```go
package gui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// findingsFile is where addressed continuity findings are recorded. It is a sidecar
// rather than an edit to history.jsonl, because that log is append-only and a
// dismissal is a player's decision about a turn, not a change to it.
const findingsFile = "findings.json"

// AddressedFinding names one finding a player has dealt with.
type AddressedFinding struct {
	Turn int    `json:"turn"`
	Rule string `json:"rule"`
}

// FindingsDTO is the whole sidecar as a client reads it.
type FindingsDTO struct {
	Addressed []AddressedFinding `json:"addressed"`
}

func (s *Service) findingsPath(gameID string) string {
	return filepath.Join(s.resolver.GameDir(gameID), findingsFile)
}

// Findings returns what has been addressed for a campaign.
func (s *Service) Findings(gameID string) (FindingsDTO, error) {
	result := FindingsDTO{Addressed: []AddressedFinding{}}

	data, err := os.ReadFile(s.findingsPath(gameID))
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return result, fmt.Errorf("read findings: %w", err)
	}

	if err := json.Unmarshal(data, &result); err != nil {
		return result, fmt.Errorf("parse findings: %w", err)
	}
	if result.Addressed == nil {
		result.Addressed = []AddressedFinding{}
	}
	return result, nil
}

// AddressFinding records that a player has dealt with one finding. Recording it twice
// is a no-op rather than an error, because a client may retry.
func (s *Service) AddressFinding(gameID string, turn int, rule string) error {
	current, err := s.Findings(gameID)
	if err != nil {
		return err
	}

	for _, existing := range current.Addressed {
		if existing.Turn == turn && existing.Rule == rule {
			return nil
		}
	}

	current.Addressed = append(current.Addressed, AddressedFinding{Turn: turn, Rule: rule})

	data, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return fmt.Errorf("encode findings: %w", err)
	}
	if err := os.WriteFile(s.findingsPath(gameID), data, 0644); err != nil {
		return fmt.Errorf("write findings: %w", err)
	}
	return nil
}
```

- [x] **Step 4: Add the routes**

In `pkg/gui/server.go`, in the game switch:

```go
	case "findings":
		switch r.Method {
		case http.MethodGet:
			found, err := s.service.Findings(gameID)
			if err != nil {
				writeGameError(w, err)
				return
			}
			writeJSON(w, found)

		case http.MethodPost:
			var req AddressedFinding
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnBody)).Decode(&req); err != nil {
				http.Error(w, "invalid request body", http.StatusBadRequest)
				return
			}
			if err := s.service.AddressFinding(gameID, req.Turn, req.Rule); err != nil {
				writeGameError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)

		default:
			http.NotFound(w, r)
		}
```

- [x] **Step 5: Show addressed findings in the client**

In `frontend/src/types.ts`:

```ts
export interface AddressedFinding {
  turn: number;
  rule: string;
}
```

In `frontend/src/api/client.ts`:

```ts
  async getFindings(): Promise<{ addressed: AddressedFinding[] }> {
    const res = await fetch(`/api/game/${this.gameID}/findings`);
    if (!res.ok) throw new Error(`getFindings: ${res.statusText}`);
    return res.json();
  }

  async addressFinding(turn: number, rule: string): Promise<void> {
    const res = await fetch(`/api/game/${this.gameID}/findings`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ turn, rule }),
    });
    if (!res.ok) throw new Error(`addressFinding: ${res.statusText}`);
  }
```

`TurnDTO` does not yet carry the rule name, so a finding is addressed as a whole turn's notes rather than per rule. Keep it simple and honest: address by turn, with the rule recorded as `continuity` for the pass as a whole:

```ts
  const handleAddress = (turnNumber: number) => {
    client?.addressFinding(turnNumber, 'continuity').catch(console.error);
    setAddressed((prev) => new Set(prev).add(turnNumber));
  };
```

`ChronicleView` gains `addressedTurns?: Set<number>` and renders the findings block dimmed, with the Correct button replaced by "Addressed", when the turn is in that set.

- [x] **Step 6: Run the full gate**

Run: `go test -count=1 ./... && go vet ./... && (cd frontend && npx tsc --noEmit)`
Expected: PASS.

- [x] **Step 7: Commit**

```bash
git add pkg/gui/ frontend/src/
git commit -m "feat(gui): let a player mark a continuity finding as dealt with"
```

---

### Task 6: Threads in the recap and an idle nudge

**Files:**
- Modify: `pkg/gui/service.go`, `pkg/gui/types.go`
- Modify: `frontend/src/types.ts`, `frontend/src/components/LivingWorldDrawer.tsx`, `frontend/src/App.tsx`
- Test: `pkg/gui/service_test.go`

**Interfaces:**
- Consumes: `engine.OpenThreads` (Task 1), `RecapDTO` (memory plan)
- Produces: `RecapDTO.Threads []ThreadDTO` and `ThreadDTO{ID, Name, Status string; LastAdvanced, Idle int}`

- [x] **Step 1: Write the failing test**

Append to `pkg/gui/service_test.go`:

```go
func TestRecapCarriesTheOpenThreads(t *testing.T) {
	gameID, svc := setupTestGame(t)

	if err := svc.SaveEntity(context.Background(), gameID, "the-miasma",
		"---\nid: the-miasma\nname: The Creeping Miasma\ntype: arc\n---\nA creeping fog.\n"); err != nil {
		t.Fatal(err)
	}

	recap, err := svc.GetRecap(context.Background(), gameID)
	if err != nil {
		t.Fatalf("GetRecap failed: %v", err)
	}
	if len(recap.Threads) != 1 {
		t.Fatalf("expected one open thread, got %+v", recap.Threads)
	}
	if recap.Threads[0].ID != "the-miasma" || recap.Threads[0].Status != "open" {
		t.Errorf("unexpected thread: %+v", recap.Threads[0])
	}
}

func TestResolvedThreadsAreNotOpen(t *testing.T) {
	gameID, svc := setupTestGame(t)

	if err := svc.SaveEntity(context.Background(), gameID, "the-siege",
		"---\nid: the-siege\nname: The Iron Siege\ntype: arc\nstate:\n  status: resolved\n---\nThe siege broke.\n"); err != nil {
		t.Fatal(err)
	}

	recap, err := svc.GetRecap(context.Background(), gameID)
	if err != nil {
		t.Fatal(err)
	}
	if len(recap.Threads) != 0 {
		t.Errorf("expected no open threads, got %+v", recap.Threads)
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

Run: `go test -run 'TestRecapCarriesTheOpenThreads|TestResolvedThreadsAreNotOpen' ./pkg/gui/`
Expected: FAIL — `recap.Threads undefined`.

- [x] **Step 3: Add them to the DTO**

In `pkg/gui/types.go`:

```go
// ThreadDTO is one unresolved arc as the client sees it.
type ThreadDTO struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	LastAdvanced int    `json:"last_advanced"`
	Idle         int    `json:"idle"`
}
```

and `Threads []ThreadDTO \`json:"threads,omitempty"\`` on `RecapDTO`.

In `pkg/gui/service.go`, `GetRecap` fills them:

```go
	latest := 0
	historyPath := filepath.Join(s.resolver.GameDir(gameID), "history.jsonl")
	if turns, err := engine.NewHistoryLogger(historyPath).LoadHistory(); err == nil && len(turns) > 0 {
		latest = turns[len(turns)-1].Number
	}

	threads := make([]ThreadDTO, 0)
	if open, err := engine.OpenThreads(s.storeOrNil(gameID), latest); err == nil {
		for _, thread := range open {
			threads = append(threads, ThreadDTO{
				ID:           thread.ID,
				Name:         thread.Name,
				Status:       thread.Status,
				LastAdvanced: thread.LastAdvanced,
				Idle:         thread.Idle,
			})
		}
	}

	return &RecapDTO{
		Summary:     chronicle.Summary,
		ThroughTurn: chronicle.ThroughTurn,
		Enabled:     s.configMgr.Get().SummaryEvery() > 0,
		Threads:     threads,
	}, nil
```

- [x] **Step 4: Show them, and nudge on the stale ones**

In `frontend/src/types.ts`:

```ts
export interface Thread {
  id: string;
  name: string;
  status: string;
  last_advanced: number;
  idle: number;
}
```

and `threads?: Thread[];` on `Recap`.

In `frontend/src/components/LivingWorldDrawer.tsx`, below the Story So Far block:

```tsx
      {recap?.threads && recap.threads.length > 0 && (
        <div className="space-y-2">
          <h3 className="text-sm font-cinzel text-amber-400 font-bold uppercase tracking-wider">Open Threads</h3>
          {recap.threads.map((thread) => {
            // A quiet thread is the one most likely to be forgotten, so it is the one
            // the player is nudged about. The narrator sees them all either way.
            const stale = thread.idle >= idleTurns;
            return (
              <div
                key={thread.id}
                className={`text-xs rounded-xl border p-2.5 space-y-1 ${
                  stale ? 'bg-amber-950/30 border-amber-500/40' : 'bg-black/30 border-white/5'
                }`}
              >
                <div className="flex items-center justify-between">
                  <span className="text-stone-200">{thread.name}</span>
                  <span className="font-mono text-[11px] text-stone-400">{thread.status}</span>
                </div>
                <div className="text-[11px] font-mono text-stone-500">
                  {thread.last_advanced > 0
                    ? `Last advanced at turn ${thread.last_advanced} (${thread.idle} turns ago)`
                    : 'Not advanced yet'}
                  {stale && ' - worth returning to'}
                </div>
              </div>
            );
          })}
        </div>
      )}
```

with `idleTurns: number` added to the props, passed from `App.tsx` as `config?.agents.thread_idle_turns ?? 10`, and `recap.threads.length` included in the empty-state condition so the panel does not claim there is nothing when only the summary is absent.

- [x] **Step 5: Run the full gate**

Run: `go test -count=1 ./... && go vet ./... && (cd frontend && npx tsc --noEmit && npm run build)`
Expected: PASS. Restore `pkg/gui/dist/.gitkeep` afterwards and do not stage its deletion.

- [x] **Step 6: Commit**

```bash
git add pkg/gui/ frontend/src/
git commit -m "feat(gui): show open threads and nudge on the ones gone quiet"
```

---

## Self-Review

**Spec coverage** (coherence spec sections 7 and 8, increment 6):

| Requirement | Task |
| --- | --- |
| Arc status as a closed set, unknown treated as open | 1 |
| `last_advanced` derived from mentions | 1 |
| Open threads always in the prompt, stalest first | 2 |
| `thread_idle_turns` is presentation only, never changes the prompt | 2, 6 |
| Five deterministic rules: unknown entity, location drift, unresolved speaker, reintroduction, state contradiction | 3 |
| Rules scoped to statements that claim canon | 3 |
| Findings reported on the turn, persisted, and actionable | 4 |
| `history.jsonl` stays append-only; addressed findings live in a sidecar | 5 |
| Recap carries open threads | 6 |
| Idle nudge in the UI only | 6 |

**Placeholder scan:** no "TBD", no "similar to Task N". Task 3 gives every rule's cues as named slices, so the tuning points are visible rather than buried in regexes.

**Type consistency:** `Thread`, `ContinuityFinding`, `CheckContinuity`, the five rule names, `AddressedFinding`, `FindingsDTO`, and `ThreadDTO` are each defined once and used with the same signatures. `ContinuityNotes` is a `[]string` on both `Turn` and `TurnDTO`, matching the existing `ContextNotes`.

**Two honest gaps, stated rather than hidden:**

1. **`state-contradiction` is the weakest rule** and it knows it: it fires only on boolean state that is true, keyed against a short cue list, in the scene. A richer check would compare the prose against every tracked value and would be wrong more often. If it proves noisy in use, the fix is to shorten `contradictionCues` or drop the rule, not to widen it.
2. **Findings are addressed per turn, not per rule**, because the turn DTO carries the notes as prose rather than as structured findings. The sidecar records `rule: "continuity"` for the turn as a whole. Making it per rule means carrying `{rule, note}` pairs through the DTO, which is a small change if the granularity is wanted.
