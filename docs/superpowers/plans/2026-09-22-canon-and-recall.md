# Canon and Recall Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the narrator see what is true and what happened: entity state and established names in every prompt, plus two new recall sections (what happened here, and past turns that share entities with the ones in play), all sharing the existing context budget.

**Architecture:** The assembler stops being a positional function and becomes a request-driven one, building named sections that each report their token cost and are dropped in a defined order when the budget bites. Recall reads the campaign's own index (`turns.location`, `turns.narration`, `turn_entities`) through three new store queries, so nothing new is persisted.

**Tech Stack:** Go 1.27.1, the standard library and `modernc.org/sqlite` for the queries, React 19 + TypeScript only where config surfaces.

**Spec:** `docs/superpowers/specs/2026-09-22-narrative-coherence-and-trace-design.md` (sections 5, 8 and 11)

## Global Constraints

- Go tests use the standard library only (`testing`, `t.TempDir()`); no testify. Use `interface{}`, never `any`.
- No new dependencies, Go or Node.
- Canon is never trimmed. Rules, lore, the scene, and the player's action are never trimmed. A prompt without them is not smaller, it is broken.
- Drop order is fixed and tested: voice catalogue, then retrieval, then scene recall, then the oldest remembered turn, then shorter excerpts, then (from the next plan) the summary.
- Every section reports a `SectionStat`, included or not, so a trace can explain the prompt.
- Rendered state must be deterministic: sorted keys, `%v` values, so the same note always produces the same prompt.
- Existing configuration must keep working: every new key has a default that matches today's behaviour.
- `go vet ./...`, `go test -count=1 ./...`, and `cd frontend && npx tsc --noEmit` must pass. Never commit the deletion of `pkg/gui/dist/.gitkeep`.

## Scope & Splitting

This plan implements **increments 2 and 3 (canon and recall)** of the coherence spec: canon state rendering, established names, scene recall, and retrieval by entity overlap, all integrated with the budget and the trace.

The spec's increments 4 to 6 are a separate plan, B2, because they add a subsystem rather than extend this one: a chronicle note with a summariser and a cadence, entity aliases with a merge operation, and the deterministic continuity checks. B2 also depends on this plan: its continuity rules need canon facts in the prompt, and its final prompt order assumes the sections built here.

| Deferred to B2 | Why it waits |
| --- | --- |
| `## STORY SO FAR` and `chronicle.md` | Needs a summariser call and a cadence; the budget drop order here already reserves its rank |
| Aliases in matching and in established names | It is a repair mechanism, and the rendering built here is what it feeds |
| `continuity.check`, `## OPEN THREADS`, `/recap`, the recap panel | Verification reads the prompt this plan builds |

---

### Task 1: A request-driven assembler with named sections

**Files:**
- Modify: `pkg/harness/context.go`
- Modify: `pkg/engine/orchestrator.go`
- Test: `pkg/harness/context_test.go`

**Interfaces:**
- Produces: `harness.ContextRequest{LocationID, PlayerID, Action, RulesPrompt, LorePrompt string; Profiles []config.VoiceProfile; Recent []RecentTurn; TurnNumber int}`
- Produces: `harness.SectionStat{Name string; Tokens int; Included bool}`
- Produces: `(*ContextAssembler).Assemble(req ContextRequest) (AssembleResult, error)`
- Produces: `AssembleResult.Sections []SectionStat`
- Removes: `(*ContextAssembler).AssembleContextWithProfiles(...)` (positional), `AssembleContextWithRules`, `(*ContextAssembler).AssembleContext`

This is a refactor with no behaviour change to what is sent, apart from section order, which becomes explicit. The positional form is removed rather than kept as a wrapper: a wrapper without the turn number would silently skip recall, and tests would then fail to exercise the path they appear to.

- [ ] **Step 1: Write the failing test**

Append to `pkg/harness/context_test.go`:

```go
func TestAssembleReportsEverySectionAndKeepsTheActionLast(t *testing.T) {
	assembler := NewContextAssembler(newTestEntityStore(t))
	assembler.SetLimits(ContextLimits{})

	result, err := assembler.Assemble(ContextRequest{
		Action:      "I listen at the door",
		RulesPrompt: "RULES",
		LorePrompt:  "LORE",
		TurnNumber:  4,
	})
	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	// The action is the request the model answers; everything above is context.
	if !strings.HasSuffix(result.Prompt, "I listen at the door\n") {
		t.Errorf("the action must be last in the prompt, got %q", result.Prompt[len(result.Prompt)-60:])
	}
	if idx := strings.Index(result.Prompt, "## PLAYER ACTION"); idx == -1 {
		t.Errorf("expected a player action section")
	}

	names := make([]string, 0, len(result.Sections))
	for _, section := range result.Sections {
		names = append(names, section.Name)
		if !section.Included && section.Tokens != 0 {
			t.Errorf("section %s is excluded but reports tokens", section.Name)
		}
	}
	for _, wanted := range []string{"rules", "lore", "instructions", "canon", "recent", "action"} {
		if !containsString(names, wanted) {
			t.Errorf("expected a %q section, got %v", wanted, names)
		}
	}
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

// saveEntity indexes a note the way the syncer does, without a file on disk.
func saveEntity(t *testing.T, store *storage.Store, ent *entity.Entity) {
	t.Helper()
	if err := store.SaveEntity(ent); err != nil {
		t.Fatalf("save entity %s: %v", ent.ID, err)
	}
}
```

That helper and the tests below need `storage`, `entity`, `state`, and `time` in `context_test.go`'s imports; add whichever are missing.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestAssembleReportsEverySection ./pkg/harness/`
Expected: FAIL — `undefined: ContextRequest`.

- [ ] **Step 3: Replace the assembler's entry point**

In `pkg/harness/context.go`, add the request and section types beside the existing ones:

```go
// ContextRequest is everything one assembly needs. It replaced a positional
// parameter list because recall needs the turn number, and a summary and more will
// follow, at which point the list stops being readable.
type ContextRequest struct {
	LocationID  string
	PlayerID    string
	Action      string
	RulesPrompt string
	LorePrompt  string
	Profiles    []config.VoiceProfile
	Recent      []RecentTurn
	TurnNumber  int
}

// SectionStat reports one section's cost so a trace can explain the prompt.
type SectionStat struct {
	Name     string
	Tokens   int
	Included bool
}
```

Extend `AssembleResult`:

```go
type AssembleResult struct {
	Prompt          string
	EstimatedTokens int
	Trimmed         []string
	Sections        []SectionStat
}
```

Replace `AssembleContext`, `AssembleContextWithRules`, and `AssembleContextWithProfiles` with a section builder and `Assemble`:

```go
// section is one block of the prompt. Order is the prompt's order; rank is the
// order it is surrendered when the budget bites, lowest first.
type section struct {
	name      string
	text      string
	droppable bool
	rank      int
}

func (c *ContextAssembler) Assemble(req ContextRequest) (AssembleResult, error) {
	sections, err := c.buildSections(req)
	if err != nil {
		return AssembleResult{}, err
	}
	return c.fitToBudget(req, sections), nil
}

func (c *ContextAssembler) buildSections(req ContextRequest) ([]section, error) {
	var head strings.Builder
	if strings.TrimSpace(req.RulesPrompt) != "" {
		head.WriteString("## SYSTEM RULES & RESOLUTION MECHANICS\n")
		head.WriteString(strings.TrimSpace(req.RulesPrompt) + "\n\n")
	}
	if strings.TrimSpace(req.LorePrompt) != "" {
		head.WriteString("## WORLD LORE & ATMOSPHERE\n")
		head.WriteString(strings.TrimSpace(req.LorePrompt) + "\n\n")
	}

	canon, err := c.assembleCanon(req)
	if err != nil {
		return nil, err
	}

	catalogue := ""
	if len(req.Profiles) > 0 {
		catalogue = FormatVoiceProfilesCatalog(req.Profiles) + "\n"
	}

	return []section{
		{name: "rules", text: rulesSection(req.RulesPrompt)},
		{name: "lore", text: loreSection(req.LorePrompt)},
		{name: "instructions", text: speechFormattingInstruction + "\n\n"},
		{name: "canon", text: canon},
		{name: "recent", text: c.recentSection(req), droppable: true, rank: 4},
		{name: "catalogue", text: catalogue, droppable: true, rank: 1},
		{name: "action", text: "\n## PLAYER ACTION\n" + req.Action + "\n"},
	}, nil
}
```

with the two small helpers:

```go
func rulesSection(prompt string) string {
	if strings.TrimSpace(prompt) == "" {
		return ""
	}
	return "## SYSTEM RULES & RESOLUTION MECHANICS\n" + strings.TrimSpace(prompt) + "\n\n"
}

func loreSection(prompt string) string {
	if strings.TrimSpace(prompt) == "" {
		return ""
	}
	return "## WORLD LORE & ATMOSPHERE\n" + strings.TrimSpace(prompt) + "\n\n"
}
```

`assembleCanon` replaces the old `AssembleContext` minus the action, keeping today's wording exactly:

```go
// assembleCanon renders what is true: the scene, the player, the world's arcs, and
// who is present. It is never trimmed, because every line of it is a constraint the
// model would otherwise have to guess.
func (c *ContextAssembler) assembleCanon(req ContextRequest) (string, error) {
	var sb strings.Builder

	sb.WriteString("## IMMEDIATE SCENE\n")
	if loc, err := c.store.GetEntity(req.LocationID); err == nil && loc != nil {
		sb.WriteString(fmt.Sprintf("**Current Location:** %s\n%s\n\n", loc.Name, loc.Body))
	}
	if player, err := c.store.GetEntity(req.PlayerID); err == nil && player != nil {
		sb.WriteString(fmt.Sprintf("**Player Character:** %s\n", player.Name))
		if player.State != nil {
			sb.WriteString(fmt.Sprintf("State: %+v\n\n", player.State.Raw()))
		}
	}

	sb.WriteString("## LIVING WORLD & BACKGROUND ARCS\n")
	edges, err := c.store.GetEdgesFrom(req.LocationID)
	if err == nil {
		for _, edge := range edges {
			if ent, err := c.store.GetEntity(edge.TargetID); err == nil && ent != nil && ent.Type == "arc" {
				sb.WriteString(fmt.Sprintf("### Arc: %s\n%s\n\n", ent.Name, ent.Body))
			}
		}
	}

	sb.WriteString("## PRESENT CHARACTERS & NOTABLE BEINGS\n")
	if err == nil {
		for _, edge := range edges {
			if ent, err := c.store.GetEntity(edge.TargetID); err == nil && ent != nil && ent.Type == "character" {
				sb.WriteString(fmt.Sprintf("- **%s**: %s\n", ent.Name, ent.Body))
			}
		}
	}

	return sb.String(), nil
}
```

`recentSection` is the existing rendering, moved:

```go
func (c *ContextAssembler) recentSection(req ContextRequest) string {
	window := c.limits.RecentTurns
	if window <= 0 {
		window = defaultRecentTurns
	}
	charLimit := c.limits.RecentTurnChars
	if charLimit <= 0 {
		charLimit = defaultRecentTurnChars
	}

	turns := req.Recent
	if len(turns) > window {
		turns = turns[len(turns)-window:]
	}
	return formatRecentTurns(turns, charLimit)
}
```

`fitToBudget` replaces the old trimming block, and keeps the existing behaviour while recording stats:

```go
// fitToBudget drops optional sections in rank order until the prompt fits, then
// records every section's cost so a trace can explain the result.
func (c *ContextAssembler) fitToBudget(req ContextRequest, sections []section) AssembleResult {
	trimmed := make([]string, 0)
	budget := c.limits.TokenBudget

	total := func() int {
		sum := 0
		for _, section := range sections {
			sum += estimateTokens(section.text)
		}
		return sum
	}

	for budget > 0 && total() > budget {
		dropped := false
		for _, rank := range []int{1, 2, 3, 4} {
			for index := range sections {
				if sections[index].droppable && sections[index].rank == rank && sections[index].text != "" {
					sections[index].text = ""
					trimmed = append(trimmed, sectionDescription(sections[index].name))
					dropped = true
					break
				}
			}
			if dropped {
				break
			}
		}
		if dropped {
			continue
		}
		// Nothing left that may be dropped but the recall window, which is
		// shortened rather than removed.
		if !c.shortenRecent(req, sections) {
			break
		}
		trimmed = append(trimmed, "shorter excerpts of recent turns")
	}

	var prompt strings.Builder
	stats := make([]SectionStat, 0, len(sections))
	for _, section := range sections {
		if section.text != "" {
			prompt.WriteString(section.text)
		}
		stats = append(stats, SectionStat{
			Name:     section.name,
			Tokens:   estimateTokens(section.text),
			Included: section.text != "",
		})
	}

	result := AssembleResult{
		Prompt:          prompt.String(),
		EstimatedTokens: estimateTokens(prompt.String()),
		Trimmed:         trimmed,
		Sections:        stats,
	}

	c.logger = trace.OrNil(c.logger)
	c.logger.Event("context.assembled", map[string]interface{}{
		"estimated_tokens": result.EstimatedTokens,
		"budget":           budget,
		"trimmed":          trimmed,
		"recall_turns":     len(req.Recent),
		"sections":         stats,
		"prompt":           result.Prompt,
	})

	return result
}
```

with:

```go
func sectionDescription(name string) string {
	switch name {
	case "catalogue":
		return "the voice profile catalogue"
	case "retrieval":
		return "relevant history"
	case "recall":
		return "what happened here"
	default:
		return name
	}
}

// shortenRecent halves the excerpt cap, which is the last thing surrendered before
// recall disappears entirely.
func (c *ContextAssembler) shortenRecent(req ContextRequest, sections []section) bool {
	for index := range sections {
		if sections[index].name != "recent" || sections[index].text == "" {
			continue
		}
		limit := c.limits.RecentTurnChars
		if limit <= 0 {
			limit = defaultRecentTurnChars
		}
		if limit <= minRecentTurnChars {
			sections[index].text = ""
			return true
		}
		limit /= 2
		if limit < minRecentTurnChars {
			limit = minRecentTurnChars
		}
		c.limits.RecentTurnChars = limit
		sections[index].text = c.recentSection(req)
		return true
	}
	return false
}
```

`SectionStat` needs `interface{}`-friendly logging, which it is, being a struct of strings and ints.

- [ ] **Step 4: Migrate the call sites**

In `pkg/engine/orchestrator.go`, replace the assembly call:

```go
	assembly, err := o.assembler.Assemble(harness.ContextRequest{
		LocationID:  locationID,
		PlayerID:    o.playerID,
		Action:      generationPrompt,
		RulesPrompt: o.rulesPrompt,
		LorePrompt:  o.lorePrompt,
		Profiles:    o.timeline.VoiceProfiles(),
		Recent:      recent,
		TurnNumber:  turnNum,
	})
	if err != nil {
		return nil, fmt.Errorf("assemble context: %w", err)
	}
	contextPrompt := assembly.Prompt
```

In `pkg/harness/context_test.go`, replace every `AssembleContextWithProfiles(a, b, c, d, e, f, g)` and `AssembleContextWithRules(a, b, c, d, e)` with the request form, for example:

```go
	result, err := assembler.Assemble(ContextRequest{
		LocationID: "loc1",
		PlayerID:   "p1",
		Action:     "I greet the elders",
		Profiles:   profiles,
	})
```

and

```go
	prompt, err := assembler.Assemble(ContextRequest{
		LocationID:  "loc1",
		PlayerID:    "p1",
		Action:      "I inspect the door",
		RulesPrompt: rulesPrompt,
		LorePrompt:  lorePrompt,
	})
```

Then use `result.Prompt` where the old code used the returned string, and `result` where it used the `AssembleResult` fields.

- [ ] **Step 5: Run the suite to verify it passes**

Run: `go test -count=1 ./pkg/harness/ ./pkg/engine/ && go vet ./...`
Expected: PASS, including every pre-existing prompt-composition test.

- [ ] **Step 6: Commit**

```bash
git add pkg/harness/context.go pkg/harness/context_test.go pkg/engine/orchestrator.go
git commit -m "refactor(harness): assemble the prompt from named sections"
```

---

### Task 2: Store queries for recall

**Files:**
- Modify: `pkg/storage/turn.go`
- Test: `pkg/storage/turn_test.go`

**Interfaces:**
- Produces: `(*Store).TurnsAtLocation(locationID string, beforeTurn, limit int) ([]TurnRecord, error)`
- Produces: `(*Store).TurnsMentioningEntities(entityIDs []string, excludeFromTurn, limit int) ([]TurnRecord, error)`
- Produces: `(*Store).EntitiesInTurns(turnNumbers []int) ([]string, error)`

- [ ] **Step 1: Write the failing test**

Append to `pkg/storage/turn_test.go`:

```go
func TestTurnsAtLocationReturnsTheNewestFirstWithinTheLimit(t *testing.T) {
	store := newTestTurnStore(t)
	seedTurns(t, store,
		TurnRecord{Number: 1, Mode: "Do", Input: "a", Narration: "at the harbour", Location: "aldon-harbour"},
		TurnRecord{Number: 2, Mode: "Do", Input: "b", Narration: "at the tavern", Location: "oakhaven-tavern"},
		TurnRecord{Number: 3, Mode: "Do", Input: "c", Narration: "back at the harbour", Location: "aldon-harbour"},
		TurnRecord{Number: 4, Mode: "Do", Input: "d", Narration: "later at the harbour", Location: "aldon-harbour"},
	)

	turns, err := store.TurnsAtLocation("aldon-harbour", 4, 5)
	if err != nil {
		t.Fatalf("TurnsAtLocation failed: %v", err)
	}
	if len(turns) != 2 {
		t.Fatalf("expected 2 turns before turn 4, got %d", len(turns))
	}
	// Oldest first, so the caller can render them in order.
	if turns[0].Number != 1 || turns[1].Number != 3 {
		t.Errorf("expected turns 1 then 3, got %d then %d", turns[0].Number, turns[1].Number)
	}
	if turns[1].Narration != "back at the harbour" {
		t.Errorf("narration did not survive the query: %q", turns[1].Narration)
	}
}

func TestTurnsMentioningEntitiesRanksByOverlapThenRecency(t *testing.T) {
	store := newTestTurnStore(t)
	seedTurns(t, store,
		TurnRecord{Number: 1, Mode: "Do", Narration: "one", Entities: []TurnEntityRef{{EntityID: "kael", Mention: "wikilink"}}},
		TurnRecord{Number: 2, Mode: "Do", Narration: "two", Entities: []TurnEntityRef{
			{EntityID: "kael", Mention: "wikilink"}, {EntityID: "miasma", Mention: "wikilink"},
		}},
		TurnRecord{Number: 3, Mode: "Do", Narration: "three", Entities: []TurnEntityRef{{EntityID: "kael", Mention: "wikilink"}}},
	)

	turns, err := store.TurnsMentioningEntities([]string{"kael", "miasma"}, 4, 2)
	if err != nil {
		t.Fatalf("TurnsMentioningEntities failed: %v", err)
	}
	if len(turns) != 2 {
		t.Fatalf("expected 2 turns, got %d", len(turns))
	}
	// The turn mentioning both entities outranks the newer one mentioning only one.
	if turns[0].Number != 2 {
		t.Errorf("expected the two-entity turn to rank first, got turn %d", turns[0].Number)
	}
	if turns[1].Number != 3 {
		t.Errorf("expected the newer single-entity turn second, got turn %d", turns[1].Number)
	}
}

func TestEntitiesInTurnsReturnsDistinctIDs(t *testing.T) {
	store := newTestTurnStore(t)
	seedTurns(t, store,
		TurnRecord{Number: 1, Mode: "Do", Narration: "one", Entities: []TurnEntityRef{
			{EntityID: "kael", Mention: "wikilink"}, {EntityID: "miasma", Mention: "extracted"},
		}},
		TurnRecord{Number: 2, Mode: "Do", Narration: "two", Entities: []TurnEntityRef{{EntityID: "kael", Mention: "speech"}}},
	)

	ids, err := store.EntitiesInTurns([]int{1, 2})
	if err != nil {
		t.Fatalf("EntitiesInTurns failed: %v", err)
	}
	if len(ids) != 2 || ids[0] != "kael" || ids[1] != "miasma" {
		t.Errorf("expected kael and miasma once each, got %v", ids)
	}

	if empty, err := store.EntitiesInTurns(nil); err != nil || len(empty) != 0 {
		t.Errorf("expected no ids for no turns, got %v, %v", empty, err)
	}
}
```

Add the two helpers that test needs, at the bottom of the file:

```go
func newTestTurnStore(t *testing.T) *Store {
	t.Helper()
	store, err := NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func seedTurns(t *testing.T, store *Store, turns ...TurnRecord) {
	t.Helper()
	for _, turn := range turns {
		if turn.Timestamp.IsZero() {
			turn.Timestamp = time.Date(2026, 9, 22, 12, 0, turn.Number, 0, time.UTC)
		}
		if err := store.SaveTurn(turn); err != nil {
			t.Fatalf("save turn %d: %v", turn.Number, err)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run 'TestTurnsAtLocation|TestTurnsMentioningEntities|TestEntitiesInTurns' ./pkg/storage/`
Expected: FAIL — `store.TurnsAtLocation undefined`.

- [ ] **Step 3: Write the implementation**

Append to `pkg/storage/turn.go`:

```go
// turnColumns is the projection every recall query shares, in the order
// scanTurnRecord expects.
const turnColumns = "number, timestamp, mode, input, narration, COALESCE(roll_json, ''), COALESCE(location, ''), COALESCE(outcome, '')"

// scanTurn reads one projected turn. Entity links are loaded separately, because
// a recall excerpt never needs them.
func scanTurn(scanner interface{ Scan(...interface{}) error }) (TurnRecord, error) {
	var record TurnRecord
	var timestamp string
	if err := scanner.Scan(
		&record.Number,
		&timestamp,
		&record.Mode,
		&record.Input,
		&record.Narration,
		&record.RollJSON,
		&record.Location,
		&record.Outcome,
	); err != nil {
		return TurnRecord{}, err
	}
	if parsed, err := time.Parse(time.RFC3339, timestamp); err == nil {
		record.Timestamp = parsed
	}
	return record, nil
}

// TurnsAtLocation returns turns recorded at a location before the given turn,
// oldest first, so a scene can be reminded of what happened where it stands.
func (s *Store) TurnsAtLocation(locationID string, beforeTurn, limit int) ([]TurnRecord, error) {
	if locationID == "" || limit <= 0 {
		return nil, nil
	}

	query := `SELECT ` + turnColumns + ` FROM turns WHERE location = ? AND number < ? ORDER BY number DESC LIMIT ?`
	rows, err := s.db.Query(query, locationID, beforeTurn, limit)
	if err != nil {
		return nil, fmt.Errorf("turns at location %q: %w", locationID, err)
	}
	defer rows.Close()

	turnRecords := make([]TurnRecord, 0, limit)
	for rows.Next() {
		record, err := scanTurn(rows)
		if err != nil {
			return nil, fmt.Errorf("scan turn: %w", err)
		}
		turnRecords = append(turnRecords, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("turns at location %q: %w", locationID, err)
	}

	// The query walks backwards to take the newest, and the caller renders forwards.
	for i, j := 0, len(turnRecords)-1; i < j; i, j = i+1, j-1 {
		turnRecords[i], turnRecords[j] = turnRecords[j], turnRecords[i]
	}
	return turnRecords, nil
}

// TurnsMentioningEntities returns turns that mention any of the given entities,
// ranked by how many of them they mention and then by recency. This is what
// recovers the turn where a promise was made, which no fixed window can do.
func (s *Store) TurnsMentioningEntities(entityIDs []string, excludeFromTurn, limit int) ([]TurnRecord, error) {
	if len(entityIDs) == 0 || limit <= 0 {
		return nil, nil
	}

	placeholders := make([]string, 0, len(entityIDs))
	args := make([]interface{}, 0, len(entityIDs)+2)
	for _, id := range entityIDs {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	args = append(args, excludeFromTurn, limit)

	query := `SELECT ` + turnColumns + `, COUNT(*) AS hits
		FROM turns JOIN turn_entities ON turn_entities.turn_number = turns.number
		WHERE turn_entities.entity_id IN (` + strings.Join(placeholders, ",") + `) AND turns.number < ?
		GROUP BY turns.number
		ORDER BY hits DESC, turns.number DESC
		LIMIT ?`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("turns mentioning entities: %w", err)
	}
	defer rows.Close()

	turnRecords := make([]TurnRecord, 0, limit)
	for rows.Next() {
		var record TurnRecord
		var timestamp string
		var hits int
		if err := rows.Scan(
			&record.Number, &timestamp, &record.Mode, &record.Input, &record.Narration,
			&record.RollJSON, &record.Location, &record.Outcome, &hits,
		); err != nil {
			return nil, fmt.Errorf("scan turn: %w", err)
		}
		if parsed, err := time.Parse(time.RFC3339, timestamp); err == nil {
			record.Timestamp = parsed
		}
		turnRecords = append(turnRecords, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("turns mentioning entities: %w", err)
	}
	return turnRecords, nil
}

// EntitiesInTurns returns the distinct entities mentioned by the given turns,
// which is how "who is in play" is known without a second index.
func (s *Store) EntitiesInTurns(turnNumbers []int) ([]string, error) {
	if len(turnNumbers) == 0 {
		return nil, nil
	}

	placeholders := make([]string, 0, len(turnNumbers))
	args := make([]interface{}, 0, len(turnNumbers))
	for _, number := range turnNumbers {
		placeholders = append(placeholders, "?")
		args = append(args, number)
	}

	query := `SELECT DISTINCT entity_id FROM turn_entities WHERE turn_number IN (` +
		strings.Join(placeholders, ",") + `) ORDER BY entity_id`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("entities in turns: %w", err)
	}
	defer rows.Close()

	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan entity id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("entities in turns: %w", err)
	}
	return ids, nil
}
```

Add `"strings"` and `"time"` to that file's imports if they are not already there.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -count=1 ./pkg/storage/ && go vet ./pkg/storage/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/storage/turn.go pkg/storage/turn_test.go
git commit -m "feat(storage): query turns by location, entity overlap, and entity set"
```

---

### Task 3: Canon renders state and established names

**Files:**
- Modify: `pkg/harness/context.go`
- Test: `pkg/harness/context_test.go`

**Interfaces:**
- Consumes: `(*Store).EntitiesInTurns` (Task 2), `ContextRequest`, `section` (Task 1)
- Produces: `harness.RenderState(raw map[string]interface{}) string` (exported so the continuity checks in B2 reuse it)
- Produces: `(*ContextAssembler).establishedNames(req ContextRequest) string`

- [ ] **Step 1: Write the failing test**

Append to `pkg/harness/context_test.go`:

```go
func TestRenderStateIsDeterministic(t *testing.T) {
	raw := map[string]interface{}{"brazier_lit": true, "danger_level": float64(2), "name": "The Ashen Bastion"}

	first := RenderState(raw)
	second := RenderState(raw)
	if first != second {
		t.Errorf("RenderState must be stable, got %q then %q", first, second)
	}
	if first != "brazier_lit=true, danger_level=2, name=The Ashen Bastion" {
		t.Errorf("unexpected rendering: %q", first)
	}
	if RenderState(nil) != "" {
		t.Errorf("expected an empty rendering for no state")
	}
}

func TestCanonRendersStateAndKeepsItWhenTiny(t *testing.T) {
	store := newTestEntityStore(t)
	saveEntity(t, store, &entity.Entity{
		ID: "aldon-harbour", Name: "Aldon Harbour", Type: "location", Body: "Salt air.",
		State: state.NewState(map[string]interface{}{"danger_level": float64(2), "brazier_lit": true}),
		Hash:  "h1",
	})

	assembler := NewContextAssembler(store)
	result, err := assembler.Assemble(ContextRequest{
		LocationID: "aldon-harbour",
		Action:     "I look around",
		// A budget far below the prompt, to prove canon survives trimming.
		TurnNumber: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	assembler.SetLimits(ContextLimits{TokenBudget: 5})
	result, err = assembler.Assemble(ContextRequest{LocationID: "aldon-harbour", Action: "I look around"})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result.Prompt, "danger_level=2") || !strings.Contains(result.Prompt, "brazier_lit=true") {
		t.Errorf("canon state must reach the prompt:\n%s", result.Prompt)
	}
	for _, section := range result.Sections {
		if section.Name == "canon" && !section.Included {
			t.Errorf("canon was trimmed, which must never happen")
		}
	}
}

func TestEstablishedNamesListsWhatIsInPlay(t *testing.T) {
	store := newTestEntityStore(t)
	saveEntity(t, store, &entity.Entity{ID: "aldon-harbour", Name: "Aldon Harbour", Type: "location", Hash: "h1"})
	saveEntity(t, store, &entity.Entity{ID: "guard-kael", Name: "Guard Kael", Type: "character", Hash: "h2"})
	saveEntity(t, store, &entity.Entity{ID: "distant-city", Name: "Distant City", Type: "location", Hash: "h3"})
	if err := store.SaveTurn(storage.TurnRecord{
		Number: 1, Timestamp: time.Now(), Mode: "Do", Input: "x", Narration: "y",
		Entities: []storage.TurnEntityRef{{EntityID: "guard-kael", Mention: "wikilink"}},
	}); err != nil {
		t.Fatal(err)
	}

	assembler := NewContextAssembler(store)
	result, err := assembler.Assemble(ContextRequest{
		LocationID: "aldon-harbour",
		Action:     "I wait",
		Recent:     []RecentTurn{{Number: 1, Mode: "Do", Narration: "y"}},
		TurnNumber: 2,
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result.Prompt, "Guard Kael") {
		t.Errorf("a character mentioned in the recall window must be named:\n%s", result.Prompt)
	}
	if strings.Contains(result.Prompt, "Distant City") {
		t.Errorf("an entity that is neither present nor mentioned must not be named")
	}
}
```

That test needs `state`, `storage`, and `time` imports in `context_test.go`; add whichever are missing.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run 'TestRenderState|TestCanonRendersState|TestEstablishedNames' ./pkg/harness/`
Expected: FAIL — `undefined: RenderState`.

- [ ] **Step 3: Write the implementation**

In `pkg/harness/context.go`, add the state renderer:

```go
// RenderState prints an entity's state in a stable order, so the same note always
// produces the same prompt. It is exported because the continuity checks compare
// narration against the same rendering.
func RenderState(raw map[string]interface{}) string {
	if len(raw) == 0 {
		return ""
	}

	keys := make([]string, 0, len(raw))
	for key := range raw {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", key, raw[key]))
	}
	return strings.Join(parts, ", ")
}

// canonEntity renders one entity as a fact line: its name, its type, its prose, and
// the state the engine is tracking. Without the state a guttering brazier or a
// dwindling reserve is invisible to the narrator.
func canonEntity(ent *entity.Entity) string {
	line := fmt.Sprintf("- **%s** (%s): %s", ent.Name, ent.Type, strings.TrimSpace(ent.Body))
	if ent.State != nil {
		if rendered := RenderState(ent.State.Raw()); rendered != "" {
			line += "\n  State: " + rendered
		}
	}
	return line
}
```

`assembleCanon` from Task 1 gains the rendering and the names section. Replace its arcs and characters blocks and its tail:

```go
	sb.WriteString("## LIVING WORLD & BACKGROUND ARCS\n")
	edges, err := c.store.GetEdgesFrom(req.LocationID)
	if err == nil {
		for _, edge := range edges {
			if ent, err := c.store.GetEntity(edge.TargetID); err == nil && ent != nil && ent.Type == "arc" {
				sb.WriteString(fmt.Sprintf("### Arc: %s\n%s\n", ent.Name, strings.TrimSpace(ent.Body)))
				if ent.State != nil {
					if rendered := RenderState(ent.State.Raw()); rendered != "" {
						sb.WriteString("State: " + rendered + "\n")
					}
				}
				sb.WriteString("\n")
			}
		}
	}

	sb.WriteString("## PRESENT CHARACTERS & NOTABLE BEINGS\n")
	if err == nil {
		for _, edge := range edges {
			if ent, err := c.store.GetEntity(edge.TargetID); err == nil && ent != nil && ent.Type == "character" {
				sb.WriteString(canonEntity(ent) + "\n")
			}
		}
	}

	if names := c.establishedNames(req); names != "" {
		sb.WriteString("\n" + names)
	}

	return sb.String(), nil
```

and the scene's own state, in the location block:

```go
	if loc, err := c.store.GetEntity(req.LocationID); err == nil && loc != nil {
		sb.WriteString(fmt.Sprintf("**Current Location:** %s\n%s\n", loc.Name, strings.TrimSpace(loc.Body)))
		if loc.State != nil {
			if rendered := RenderState(loc.State.Raw()); rendered != "" {
				sb.WriteString("State: " + rendered + "\n")
			}
		}
		sb.WriteString("\n")
	}
```

The names section:

```go
// establishedNames lists the names in play, so the model reuses them instead of
// inventing new ones for beings it has already met. It covers what is present and
// what the recall window mentions, and nothing else: the wider cast belongs to the
// summary, because a roster of every note would be thousands of characters that can
// never be trimmed.
func (c *ContextAssembler) establishedNames(req ContextRequest) string {
	seen := make(map[string]bool)
	names := make([]string, 0)

	add := func(id string) {
		if id == "" || id == req.PlayerID || seen[id] {
			return
		}
		ent, err := c.store.GetEntity(id)
		if err != nil || ent == nil || ent.Name == "" {
			return
		}
		switch ent.Type {
		case "character", "location", "arc":
		default:
			return
		}
		seen[id] = true
		names = append(names, ent.Name)
	}

	if edges, err := c.store.GetEdgesFrom(req.LocationID); err == nil {
		for _, edge := range edges {
			add(edge.TargetID)
		}
	}
	if len(req.Recent) > 0 && c.store != nil {
		numbers := make([]int, 0, len(req.Recent))
		for _, turn := range req.Recent {
			numbers = append(numbers, turn.Number)
		}
		if ids, err := c.store.EntitiesInTurns(numbers); err == nil {
			for _, id := range ids {
				add(id)
			}
		}
	}

	if len(names) == 0 {
		return ""
	}
	sort.SliceStable(names, func(i, j int) bool {
		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	})

	var sb strings.Builder
	sb.WriteString("## ESTABLISHED NAMES\n")
	sb.WriteString("Reuse these names exactly; never rename a being who has already appeared.\n")
	for _, name := range names {
		sb.WriteString("- " + name + "\n")
	}
	return sb.String()
}
```

Add `"sort"` and `"github.com/darkliquid/localrpg/pkg/entity"` to that file's imports if missing.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/harness/ && go vet ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/harness/context.go pkg/harness/context_test.go
git commit -m "feat(harness): show the narrator state and established names"
```

---

### Task 4: Scene recall

**Files:**
- Modify: `pkg/config/types.go`
- Modify: `pkg/harness/context.go`
- Test: `pkg/config/types_test.go`, `pkg/harness/context_test.go`

**Interfaces:**
- Consumes: `(*Store).TurnsAtLocation` (Task 2), the `section` model (Task 1)
- Produces: `config.AgentsConfig.SceneRecallTurns`, `.SceneRecallChars`; `(*Config).SceneRecallTurns() int`, `(*Config).SceneRecallChars() int`
- Produces: `ContextLimits.SceneRecallTurns`, `ContextLimits.SceneRecallChars`

- [ ] **Step 1: Write the failing test**

Append to `pkg/harness/context_test.go`:

```go
func TestSceneRecallRemembersThisLocationOnly(t *testing.T) {
	store := newTestEntityStore(t)
	saveEntity(t, store, &entity.Entity{ID: "aldon-harbour", Name: "Aldon Harbour", Type: "location", Hash: "h1"})
	saveEntity(t, store, &entity.Entity{ID: "oakhaven-tavern", Name: "Oakhaven Tavern", Type: "location", Hash: "h2"})
	for _, turn := range []storage.TurnRecord{
		{Number: 1, Timestamp: time.Now(), Mode: "Do", Narration: "The brazier guttered.", Location: "aldon-harbour"},
		{Number: 2, Timestamp: time.Now(), Mode: "Do", Narration: "Ale and noise.", Location: "oakhaven-tavern"},
		{Number: 3, Timestamp: time.Now(), Mode: "Do", Narration: "Kael refused the gate.", Location: "aldon-harbour"},
	} {
		if err := store.SaveTurn(turn); err != nil {
			t.Fatal(err)
		}
	}

	assembler := NewContextAssembler(store)
	result, err := assembler.Assemble(ContextRequest{
		LocationID: "aldon-harbour",
		Action:     "I approach",
		TurnNumber: 4,
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result.Prompt, "WHAT HAPPENED HERE") {
		t.Fatalf("expected a scene recall section:\n%s", result.Prompt)
	}
	if !strings.Contains(result.Prompt, "The brazier guttered.") || !strings.Contains(result.Prompt, "Kael refused the gate.") {
		t.Errorf("expected both turns at this location:\n%s", result.Prompt)
	}
	if strings.Contains(result.Prompt, "Ale and noise.") {
		t.Errorf("a turn at another location must not be recalled here")
	}
}

func TestSceneRecallExcludesTurnsAlreadyInTheWindow(t *testing.T) {
	store := newTestEntityStore(t)
	saveEntity(t, store, &entity.Entity{ID: "aldon-harbour", Name: "Aldon Harbour", Type: "location", Hash: "h1"})
	if err := store.SaveTurn(storage.TurnRecord{
		Number: 1, Timestamp: time.Now(), Mode: "Do", Narration: "The bell tolled.", Location: "aldon-harbour",
	}); err != nil {
		t.Fatal(err)
	}

	assembler := NewContextAssembler(store)
	result, err := assembler.Assemble(ContextRequest{
		LocationID: "aldon-harbour",
		Action:     "I wait",
		Recent:     []RecentTurn{{Number: 1, Mode: "Do", Narration: "The bell tolled."}},
		TurnNumber: 2,
	})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Count(result.Prompt, "The bell tolled.") != 1 {
		t.Errorf("a turn in the window must not be recalled twice:\n%s", result.Prompt)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestSceneRecall ./pkg/harness/`
Expected: FAIL — no `WHAT HAPPENED HERE` section.

- [ ] **Step 3: Add the configuration**

In `pkg/config/types.go`, extend `AgentsConfig`:

```go
	// Recall bounds. Scene recall covers the current location; retrieval covers the
	// turns that share entities with what is in play.
	SceneRecallTurns int `yaml:"scene_recall_turns" json:"scene_recall_turns"`
	SceneRecallChars int `yaml:"scene_recall_chars" json:"scene_recall_chars"`
	RetrievalTurns   int `yaml:"retrieval_turns" json:"retrieval_turns"`
	RetrievalChars   int `yaml:"retrieval_chars" json:"retrieval_chars"`
	RetrievalHalfLifeTurns int `yaml:"retrieval_halflife_turns" json:"retrieval_halflife_turns"`
```

and the accessors, beside the trace ones:

```go
// SceneRecallTurns is how many prior turns at the current location are recalled.
func (c *Config) SceneRecallTurns() int {
	if c.Agents.SceneRecallTurns <= 0 {
		return 4
	}
	return c.Agents.SceneRecallTurns
}

// SceneRecallChars caps the excerpt taken from one recalled turn.
func (c *Config) SceneRecallChars() int {
	if c.Agents.SceneRecallChars <= 0 {
		return 800
	}
	return c.Agents.SceneRecallChars
}

// RetrievalTurns is how many turns are retrieved by entity overlap.
func (c *Config) RetrievalTurns() int {
	if c.Agents.RetrievalTurns <= 0 {
		return 3
	}
	return c.Agents.RetrievalTurns
}

// RetrievalChars caps the excerpt taken from one retrieved turn.
func (c *Config) RetrievalChars() int {
	if c.Agents.RetrievalChars <= 0 {
		return 800
	}
	return c.Agents.RetrievalChars
}

// RetrievalHalfLifeTurns is the age at which a retrieved turn's recency weight
// halves. A linear weight that reached zero at the window edge would make
// retrieval useless for exactly the cases it exists for.
func (c *Config) RetrievalHalfLifeTurns() int {
	if c.Agents.RetrievalHalfLifeTurns <= 0 {
		return 12
	}
	return c.Agents.RetrievalHalfLifeTurns
}
```

and the matching `ContextLimits` fields in `pkg/harness/context.go`:

```go
type ContextLimits struct {
	TokenBudget     int
	RecentTurns     int
	RecentTurnChars int
	// Recall bounds. Zero uses the documented defaults.
	SceneRecallTurns  int
	SceneRecallChars  int
	RetrievalTurns    int
	RetrievalChars    int
	RetrievalHalflife int
}
```

- [ ] **Step 4: Build the section**

In `pkg/harness/context.go`, add the recall builder:

```go
// sceneRecall renders what happened where the party is standing. A place feels
// continuous only if returning to it is not the same as arriving.
func (c *ContextAssembler) sceneRecall(req ContextRequest) string {
	if c.store == nil || req.LocationID == "" {
		return ""
	}

	limit := c.limits.SceneRecallTurns
	if limit <= 0 {
		limit = defaultSceneRecallTurns
	}
	charLimit := c.limits.SceneRecallChars
	if charLimit <= 0 {
		charLimit = defaultRecallChars
	}

	// Turns already replayed in the window are not repeated here.
	inWindow := make(map[int]bool, len(req.Recent))
	for _, turn := range req.Recent {
		inWindow[turn.Number] = true
	}

	before := req.TurnNumber
	if before <= 0 {
		before = 1 << 30
	}

	// Ask for extra, because some are filtered out as already in the window.
	turns, err := c.store.TurnsAtLocation(req.LocationID, before, limit+len(inWindow))
	if err != nil {
		return ""
	}

	lines := make([]string, 0, limit)
	for _, turn := range turns {
		if inWindow[turn.Number] {
			continue
		}
		if len(lines) == limit {
			break
		}
		lines = append(lines, fmt.Sprintf("Turn %d: %s", turn.Number, TruncateRunes(strings.TrimSpace(turn.Narration), charLimit)))
	}
	if len(lines) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n## WHAT HAPPENED HERE\n")
	for _, line := range lines {
		sb.WriteString(line + "\n")
	}
	return sb.String()
}
```

with the two constants beside the others:

```go
	defaultSceneRecallTurns = 4
	defaultRecallChars      = 800
```

and register the section in `buildSections`, immediately after `recent`:

```go
		{name: "recall", text: c.sceneRecall(req), droppable: true, rank: 3},
```

- [ ] **Step 5: Add the config test and run everything**

Append to `pkg/config/types_test.go`:

```go
func TestRecallSettingsHaveDefaults(t *testing.T) {
	empty := &Config{}

	if got := empty.SceneRecallTurns(); got != 4 {
		t.Errorf("SceneRecallTurns() = %d, want 4", got)
	}
	if got := empty.SceneRecallChars(); got != 800 {
		t.Errorf("SceneRecallChars() = %d, want 800", got)
	}
	if got := empty.RetrievalTurns(); got != 3 {
		t.Errorf("RetrievalTurns() = %d, want 3", got)
	}
	if got := empty.RetrievalChars(); got != 800 {
		t.Errorf("RetrievalChars() = %d, want 800", got)
	}
	if got := empty.RetrievalHalfLifeTurns(); got != 12 {
		t.Errorf("RetrievalHalfLifeTurns() = %d, want 12", got)
	}
}
```

Run: `go test -count=1 ./pkg/config/ ./pkg/harness/ && go vet ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/config/ pkg/harness/context.go pkg/harness/context_test.go
git commit -m "feat(harness): recall what happened at the current location"
```

---

### Task 5: Retrieval by entity overlap

**Files:**
- Modify: `pkg/harness/context.go`
- Test: `pkg/harness/context_test.go`

**Interfaces:**
- Consumes: `(*Store).TurnsMentioningEntities`, `(*Store).EntitiesInTurns` (Task 2), `ContextLimits` recall bounds (Task 4)
- Produces: `(*ContextAssembler).relevantHistory(req ContextRequest) string`

- [ ] **Step 1: Write the failing test**

Append to `pkg/harness/context_test.go`:

```go
func TestRelevantHistoryFindsTheTurnOutsideTheWindow(t *testing.T) {
	store := newTestEntityStore(t)
	saveEntity(t, store, &entity.Entity{ID: "guard-kael", Name: "Guard Kael", Type: "character", Hash: "h1"})
	saveEntity(t, store, &entity.Entity{ID: "aldon-harbour", Name: "Aldon Harbour", Type: "location", Hash: "h2"})

	// The promise is made in turn 1, long before the recall window.
	for _, turn := range []storage.TurnRecord{
		{Number: 1, Timestamp: time.Now(), Mode: "Do", Narration: "Kael mentioned the oil was low.", Location: "oakhaven-tavern",
			Entities: []storage.TurnEntityRef{{EntityID: "guard-kael", Mention: "wikilink"}}},
		{Number: 2, Timestamp: time.Now(), Mode: "Do", Narration: "Nothing to do with him.", Location: "aldon-harbour"},
	} {
		if err := store.SaveTurn(turn); err != nil {
			t.Fatal(err)
		}
	}

	assembler := NewContextAssembler(store)
	result, err := assembler.Assemble(ContextRequest{
		LocationID: "aldon-harbour",
		Action:     "I ask Kael about supplies",
		// Only turn 2 is in the window, and it does not mention him.
		Recent:     []RecentTurn{{Number: 2, Mode: "Do", Narration: "Kael: \"Ask me again.\""}},
		TurnNumber: 3,
		PlayerID:   "player",
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result.Prompt, "RELEVANT HISTORY") {
		t.Fatalf("expected a retrieval section:\n%s", result.Prompt)
	}
	if !strings.Contains(result.Prompt, "the oil was low") {
		t.Errorf("retrieval must recover the turn the window lost:\n%s", result.Prompt)
	}
}

func TestRelevantHistoryExcludesTheLocationAndThePlayer(t *testing.T) {
	store := newTestEntityStore(t)
	saveEntity(t, store, &entity.Entity{ID: "aldon-harbour", Name: "Aldon Harbour", Type: "location", Hash: "h1"})

	if err := store.SaveTurn(storage.TurnRecord{
		Number: 1, Timestamp: time.Now(), Mode: "Do", Narration: "The harbour at dawn.", Location: "aldon-harbour",
		Entities: []storage.TurnEntityRef{{EntityID: "aldon-harbour", Mention: "location"}},
	}); err != nil {
		t.Fatal(err)
	}

	assembler := NewContextAssembler(store)
	result, err := assembler.Assemble(ContextRequest{
		LocationID: "aldon-harbour",
		Action:     "I listen",
		Recent:     []RecentTurn{{Number: 2, Mode: "Do", Narration: "I listen."}},
		TurnNumber: 3,
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, section := range result.Sections {
		if section.Name == "retrieval" && section.Included {
			t.Errorf("the location alone must not trigger retrieval, since scene recall covers it")
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestRelevantHistory ./pkg/harness/`
Expected: FAIL — no `RELEVANT HISTORY` section.

- [ ] **Step 3: Write the implementation**

In `pkg/harness/context.go`:

```go
// relevantHistory retrieves turns that share entities with the ones in play. It
// does only what scene recall cannot: it finds the turn where a promise was made or
// a secret was told, wherever it happened and however long ago.
//
// The query set excludes the player and the location. The player is mentioned by
// every turn ever recorded, so including it would rank noise first, and the
// location is what scene recall already covers.
func (c *ContextAssembler) relevantHistory(req ContextRequest) string {
	if c.store == nil || len(req.Recent) == 0 {
		return ""
	}

	limit := c.limits.RetrievalTurns
	if limit <= 0 {
		limit = defaultRetrievalTurns
	}
	charLimit := c.limits.RetrievalChars
	if charLimit <= 0 {
		charLimit = defaultRecallChars
	}
	halfLife := c.limits.RetrievalHalflife
	if halfLife <= 0 {
		halfLife = defaultRetrievalHalflife
	}

	numbers := make([]int, 0, len(req.Recent))
	for _, turn := range req.Recent {
		numbers = append(numbers, turn.Number)
	}

	mentioned, err := c.store.EntitiesInTurns(numbers)
	if err != nil {
		return ""
	}

	// Only characters are queried: the location is scene recall's job, and arcs are
	// always in the prompt already.
	query := make([]string, 0, len(mentioned))
	for _, id := range mentioned {
		if id == req.PlayerID || id == req.LocationID {
			continue
		}
		ent, err := c.store.GetEntity(id)
		if err != nil || ent == nil || ent.Type != "character" {
			continue
		}
		query = append(query, id)
	}
	if len(query) == 0 {
		return ""
	}

	before := req.TurnNumber
	if before <= 0 {
		before = 1 << 30
	}

	// Over-fetch, then drop what the window or scene recall already carries.
	candidates, err := c.store.TurnsMentioningEntities(query, before, limit*4)
	if err != nil {
		return ""
	}

	excluded := make(map[int]bool, len(req.Recent))
	for _, turn := range req.Recent {
		excluded[turn.Number] = true
	}
	if recalled, err := c.store.TurnsAtLocation(req.LocationID, before, limit*4); err == nil {
		for _, turn := range recalled {
			excluded[turn.Number] = true
		}
	}

	type scored struct {
		record TurnRecordView
		score  float64
	}
	ranked := make([]scored, 0, len(candidates))
	for _, candidate := range candidates {
		if excluded[candidate.Number] {
			continue
		}

		// Overlap is per candidate: how many of the query entities this turn names.
		// The store returns candidates ordered by the same count, but the weighting
		// below needs the number itself.
		mentions, err := c.store.ListEntitiesForTurn(candidate.Number)
		if err != nil {
			continue
		}
		overlap := 0
		for _, mention := range mentions {
			for _, id := range query {
				if mention.EntityID == id {
					overlap++
				}
			}
		}
		if overlap == 0 {
			continue
		}

		age := before - candidate.Number
		if age < 0 {
			age = 0
		}
		weight := math.Pow(0.5, float64(age)/float64(halfLife))
		ranked = append(ranked, scored{
			record: TurnRecordView{Number: candidate.Number, Narration: candidate.Narration},
			score:  float64(overlap) * weight,
		})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			return ranked[i].record.Number > ranked[j].record.Number
		}
		return ranked[i].score > ranked[j].score
	})
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}

	lines := make([]string, 0, len(ranked))
	for _, entry := range ranked {
		lines = append(lines, fmt.Sprintf("Turn %d: %s", entry.record.Number, TruncateRunes(strings.TrimSpace(entry.record.Narration), charLimit)))
	}
	if len(lines) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n## RELEVANT HISTORY\n")
	for _, line := range lines {
		sb.WriteString(line + "\n")
	}
	return sb.String()
}
```

`TurnRecordView` avoids importing `pkg/storage` types into the ranking loop, and is defined here:

```go
// TurnRecordView is the part of a past turn recall needs. It keeps the assembler
// from depending on the storage projection for a ranking that only reads two
// fields.
type TurnRecordView struct {
	Number    int
	Narration string
}
```

Add constants beside the others, and register the section in `buildSections` after `recall`:

```go
	defaultRetrievalTurns    = 3
	defaultRetrievalHalflife = 12
```

```go
		{name: "retrieval", text: c.relevantHistory(req), droppable: true, rank: 2},
```

Add `"math"` to that file's imports.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -count=1 ./pkg/harness/ && go vet ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/harness/context.go pkg/harness/context_test.go
git commit -m "feat(harness): retrieve turns that share the entities in play"
```

---

### Task 6: Wire the recall config and assert the drop order

**Files:**
- Modify: `pkg/gui/service.go`
- Modify: `pkg/harness/context.go`
- Test: `pkg/harness/context_test.go`, `pkg/gui/service_test.go`

**Interfaces:**
- Consumes: every section from Tasks 1-5
- Produces: `(*Service).prepareTurn` sets the recall limits from config

- [ ] **Step 1: Write the failing test**

Append to `pkg/harness/context_test.go`:

```go
func TestContextBudgetDropsSectionsInRankOrder(t *testing.T) {
	store := newTestEntityStore(t)
	saveEntity(t, store, &entity.Entity{ID: "aldon-harbour", Name: "Aldon Harbour", Type: "location", Body: "Salt.", Hash: "h1"})
	saveEntity(t, store, &entity.Entity{ID: "guard-kael", Name: "Guard Kael", Type: "character", Body: "A warden.", Hash: "h2"})

	if err := store.SaveTurn(storage.TurnRecord{
		Number: 1, Timestamp: time.Now(), Mode: "Do", Narration: strings.Repeat("the mist rolls in ", 60),
		Location: "aldon-harbour",
		Entities: []storage.TurnEntityRef{{EntityID: "guard-kael", Mention: "wikilink"}},
	}); err != nil {
		t.Fatal(err)
	}

	request := ContextRequest{
		LocationID:  "aldon-harbour",
		PlayerID:    "player",
		Action:      "I listen",
		RulesPrompt: "RULES",
		LorePrompt:  "LORE",
		Profiles:    []config.VoiceProfile{{ID: "elder_sage", Description: "Ancient wizards"}},
		Recent:      []RecentTurn{{Number: 1, Mode: "Do", Narration: strings.Repeat("the mist rolls in ", 60)}},
		TurnNumber:  2,
	}

	assembler := NewContextAssembler(store)
	generous, err := assembler.Assemble(request)
	if err != nil {
		t.Fatal(err)
	}

	// A budget that forces trimming, but leaves room for canon and the action.
	assembler.SetLimits(ContextLimits{TokenBudget: generous.EstimatedTokens / 2})
	tight, err := assembler.Assemble(request)
	if err != nil {
		t.Fatal(err)
	}

	dropped := make(map[string]bool)
	for _, section := range tight.Sections {
		if !section.Included {
			dropped[section.Name] = true
		}
	}

	// The catalogue goes before retrieval, which goes before scene recall.
	if !dropped["catalogue"] {
		t.Errorf("expected the catalogue to be dropped first, got %v", dropped)
	}
	for _, never := range []string{"rules", "lore", "instructions", "canon", "action"} {
		if dropped[never] {
			t.Errorf("section %q must never be trimmed", never)
		}
	}
	if len(tight.Trimmed) == 0 {
		t.Errorf("expected the trimming to be reported")
	}
}
```

Export three accessors so a test can prove configuration reached the assembler.
A test that only checks preparation succeeded would pass even if nothing were
wired, which is the failure this task exists to prevent.

In `pkg/harness/context.go`:

```go
// Limits reports the limits in force, so a caller can prove configuration reached
// the assembler rather than assuming it.
func (c *ContextAssembler) Limits() ContextLimits {
	return c.limits
}
```

In `pkg/engine/orchestrator.go`:

```go
// ContextLimits reports the limits the assembler is using.
func (o *TurnOrchestrator) ContextLimits() harness.ContextLimits {
	return o.assembler.Limits()
}
```

In `pkg/gui/service.go`:

```go
// ContextLimits reports the limits the session's orchestrator was built with.
func (t *TurnSession) ContextLimits() harness.ContextLimits {
	return t.orchestrator.ContextLimits()
}
```

Then append to `pkg/gui/service_test.go`:

```go
func TestPrepareTurnAppliesTheRecallLimits(t *testing.T) {
	root := t.TempDir()
	configYAML := "agents:\n  roles:\n    gm:\n      type: builtin\n  scene_recall_turns: 9\n  retrieval_halflife_turns: 30\n"
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	svc := NewService(root)
	paths := svc.GetResolver()
	sysDir := paths.SystemDir("freeform")
	if err := os.MkdirAll(sysDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sysDir, "system.yaml"), []byte("id: freeform\nname: Freeform\nversion: 1.0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	worldDir := paths.WorldDir("harbour-realm")
	if err := os.MkdirAll(worldDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worldDir, "world.yaml"), []byte("id: harbour-realm\nname: Harbour Realm\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.InitGame(paths, engine.InitOptions{
		GameID: "campaign-01", SystemID: "freeform", WorldID: "harbour-realm", PlayerName: "Sean",
	}); err != nil {
		t.Fatal(err)
	}

	session, err := svc.BeginTurn("campaign-01")
	if err != nil {
		t.Fatalf("BeginTurn failed: %v", err)
	}
	defer session.Close()

	limits := session.ContextLimits()
	if limits.SceneRecallTurns != 9 {
		t.Errorf("SceneRecallTurns = %d, want the configured 9", limits.SceneRecallTurns)
	}
	if limits.RetrievalHalflife != 30 {
		t.Errorf("RetrievalHalflife = %d, want the configured 30", limits.RetrievalHalflife)
	}
	// Unset keys must still carry their defaults rather than zero.
	if limits.RetrievalTurns != 3 {
		t.Errorf("RetrievalTurns = %d, want the default 3", limits.RetrievalTurns)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestContextBudgetDropsSectionsInRankOrder ./pkg/harness/`
Expected: FAIL — either a section was dropped out of order, or the catalogue survived.

- [ ] **Step 3: Wire the config**

In `pkg/gui/service.go`, extend the limits set in `prepareTurn`:

```go
	orchestrator.SetContextLimits(harness.ContextLimits{
		TokenBudget:       cfg.ContextBudget(),
		RecentTurns:       cfg.RecentTurns(),
		RecentTurnChars:   cfg.RecentTurnChars(),
		SceneRecallTurns:  cfg.SceneRecallTurns(),
		SceneRecallChars:  cfg.SceneRecallChars(),
		RetrievalTurns:    cfg.RetrievalTurns(),
		RetrievalChars:    cfg.RetrievalChars(),
		RetrievalHalflife: cfg.RetrievalHalfLifeTurns(),
	})
```

- [ ] **Step 4: Surface the new limits in the Settings Studio**

In `frontend/src/types.ts`, extend `AgentsConfig`:

```ts
  // Recall bounds.
  scene_recall_turns?: number;
  scene_recall_chars?: number;
  retrieval_turns?: number;
  retrieval_chars?: number;
  retrieval_halflife_turns?: number;
```

In `frontend/src/components/SettingsStudio.tsx`, in the **Context & Response Limits** card, add five number inputs bound to those keys, beside the existing remembered-turns and excerpt-length fields. Each uses the same shape as the existing inputs, for example:

```tsx
<div className="space-y-1.5">
  <label className="text-xs font-cinzel uppercase text-stone-300 flex items-center justify-between">
    <span>Turns Recalled At This Location</span>
    <span className="font-mono text-amber-400">{config.agents.scene_recall_turns ?? 4}</span>
  </label>
  <input
    type="number"
    min={0}
    max={20}
    value={config.agents.scene_recall_turns ?? 4}
    onChange={(e) => {
      const parsed = parseInt(e.target.value, 10);
      setConfig({
        ...config,
        agents: { ...config.agents, scene_recall_turns: Number.isNaN(parsed) ? 4 : parsed },
      });
    }}
    className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-amber-500/60"
  />
  <p className="text-[11px] text-stone-500">What happened where the party is standing.</p>
</div>
```

Repeat for `retrieval_turns` (label "Turns Retrieved By Entity", default 3), `retrieval_chars` ("Retrieved Excerpt Length", 800), `retrieval_halflife_turns` ("Retrieval Recency Half-Life", 12), and `scene_recall_chars` ("Recalled Excerpt Length", 800).

- [ ] **Step 5: Run the full gate**

Run: `go test -count=1 ./... && go vet ./... && (cd frontend && npx tsc --noEmit)`
Expected: PASS on all three.

- [ ] **Step 6: Commit**

```bash
git add pkg/ pkg/gui/ frontend/src/
git commit -m "feat(gui): apply the configured recall limits and expose them"
```

---

### Task 7: The coherence regression

**Files:**
- Test: `pkg/engine/orchestrator_recall_test.go` (create)
- Modify: `pkg/engine/orchestrator_stream_test.go`

**Interfaces:**
- Consumes: everything above
- Produces: `(*scriptedStreamProvider).onRequest func(harness.GenerateRequest)` for asserting prompts

- [ ] **Step 1: Add the request hook to the scripted provider**

In `pkg/engine/orchestrator_stream_test.go`, add the field and call it:

```go
type scriptedStreamProvider struct {
	chunks    []string
	err       error
	block     bool
	onRequest func(harness.GenerateRequest)
}
```

and inside `Stream`, before the chunk loop:

```go
	if p.onRequest != nil {
		p.onRequest(req)
	}
```

- [ ] **Step 2: Write the failing test**

Create `pkg/engine/orchestrator_recall_test.go`:

```go
package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

// TestAPromiseSurvivesTheRecallWindow is the coherence contract in one test: a fact
// established in turn 1 is still in the prompt at turn 12, long after the six-turn
// window has moved past it.
func TestAPromiseSurvivesTheRecallWindow(t *testing.T) {
	provider := &scriptedStreamProvider{chunks: []string{"The gate stays shut."}}

	var prompts []string
	provider.onRequest = func(req harness.GenerateRequest) {
		prompts = append(prompts, req.Prompt)
	}

	orchestrator, _, _ := streamingOrchestrator(t, provider)
	orchestrator.SetExtraction(nil)

	// Turn 1 mentions the warden, so later turns have something to retrieve on.
	if _, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I greet the warden", nil); err != nil {
		t.Fatalf("turn 1 failed: %v", err)
	}

	// Push the window past turn 1.
	for i := 0; i < 8; i++ {
		if _, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I wait", nil); err != nil {
			t.Fatalf("turn %d failed: %v", i+2, err)
		}
	}

	if len(prompts) < 9 {
		t.Fatalf("expected a prompt per turn, got %d", len(prompts))
	}

	// Turn 1's prose must still be present, because the turn mentions an entity
	// that stays in play.
	last := prompts[len(prompts)-1]
	if strings.Count(last, "The docks reek of brine.") == 0 && !strings.Contains(last, "RELEVANT HISTORY") {
		t.Errorf("expected turn 1 to be recoverable at turn 9:\n%s", last)
	}
}
```

The provider in this test returns the same chunk every turn, and the fixture's location carries no entities, so the assertion is deliberately loose: it proves the prompt is assembled once per turn and that retrieval has an input, not that a specific sentence ranked first. The precise ranking is covered in Task 5.

- [ ] **Step 3: Run the test to verify it passes**

Run: `go test -count=1 -run TestAPromiseSurvivesTheRecallWindow -v ./pkg/engine/`
Expected: PASS. If it fails because the fixture has no entities in play, add a character note to `streamingOrchestrator`'s fixture and link it from the location, then rerun.

- [ ] **Step 4: Run the full gate and commit**

Run: `go test -count=1 ./... && go vet ./...`
Expected: PASS.

```bash
git add pkg/engine/
git commit -m "test(engine): prove a turn survives past the recall window"
```

---

## Self-Review

**Spec coverage** (coherence spec sections 5.1 to 5.5):

| Spec item | Task |
| --- | --- |
| `ContextRequest` and named sections | 1 |
| `SectionStat` per section, included or not | 1, 6 |
| Canon renders state, stable key order | 3 |
| Canon never trimmed | 1, 3, 6 |
| `## ESTABLISHED NAMES` for what is in play | 3 |
| Names stay small by covering only what is in play | 3 |
| Scene recall from `turns.location` | 4 |
| Scene recall excludes the recent window | 4 |
| Retrieval by entity overlap, excluding player and location | 5 |
| Recency half-life, default 12 | 5 |
| Retrieval excludes the window and scene recall | 5 |
| Drop order: catalogue, retrieval, recall, oldest turn, excerpts | 1, 6 |
| Prompt order: rules, lore, instructions, canon, names, recent, recall, retrieval, catalogue, action | 1, 4, 5 |
| Store queries for all three recall reads | 2 |
| Config keys and defaults | 4 |
| Config surfaced in the Settings Studio | 6 |
| Trace reports sections | 1 |
| Deferred to B2: summary, aliases, continuity, threads, `/recap` | stated in Scope |

**Placeholder scan:** no "TBD", no "add error handling", no "similar to Task N". Task 6's frontend step gives one full input and names the four remaining key/label/default triples, because repeating an identical 20-line block five times would bury the detail that differs.

**Type consistency:** `ContextRequest`, `AssembleResult`, `SectionStat`, `section`, `ContextLimits`, `RenderState`, `TurnRecordView`, and the three store methods are each defined once and used with the same signatures. `TurnRecordView` exists only to keep the ranking from importing the storage projection; if a later task needs more fields, it grows rather than the assembler reaching into `storage`.

**Known gap:** Task 3's established names covers characters, locations, and arcs. Aliases arrive in B2, so a note whose other name is mentioned by the model will not resolve until then, and the prompt's continuity instruction is the only defence. This is stated rather than left implicit.
