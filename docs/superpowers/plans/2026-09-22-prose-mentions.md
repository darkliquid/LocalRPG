# Prose Mentions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make a character who is merely *described* count as in play, by recording the names a turn's prose actually contains, deterministically and without a model call.

**Architecture:** A new `harness.ResolveProseMentions` scans a turn's narration and input for the names of known entities and returns mentions of a new `prose` kind. `Timeline.RecordTurn` records them alongside the wikilink and speech mentions it already has, so every writer benefits. Retrieval's overlap count is deduplicated by entity ID, because one entity can legitimately carry several mention kinds.

**Tech Stack:** Go 1.27.1, the standard library only.

**Spec:** `docs/superpowers/specs/2026-09-22-narrative-coherence-and-trace-design.md` (section 5.4)

## Global Constraints

- Go tests use the standard library only (`testing`, `t.TempDir()`); no testify. Use `interface{}`, never `any`.
- No new dependencies and **no model call**: this pass is deterministic, which is what makes it safe to run on every turn and safe to backfill.
- Prose mentions are additive. They must never change how a turn is segmented, attributed, or read aloud.
- A mention is recorded once per entity per turn. An entity already mentioned by wikilink, speech, extraction, or as the player or location is not mentioned again.
- Rewriting `history.jsonl` is out of scope for the recording path. The backfill is a separate, explicit command, because that file is otherwise append-only.
- `go vet ./...`, `go test -count=1 ./...`, and `cd frontend && npx tsc --noEmit` must pass. Never commit the deletion of `pkg/gui/dist/.gitkeep`.

## Scope & Splitting

This plan closes the retrieval gap for turns where extraction did not run or missed someone. It is deliberately separate from the coherence plans: it changes what a turn records, which is a data concern, while the memory and repair plans change what the narrator is sent.

| Out of scope | Why |
| --- | --- |
| Re-running extraction over history | Needs a model call per turn and cannot be done offline |
| A prose scan over *entity bodies* | The question is who a turn involves, not who is described in a note |
| The summary, aliases, continuity checks | Their own plans |

---

### Task 1: Deduplicate the overlap count

**Files:**
- Modify: `pkg/harness/context.go`
- Test: `pkg/harness/context_test.go`

**Interfaces:**
- Consumes: `(*Store).ListEntitiesForTurn`, `ContextRequest` (existing)
- Produces: no signature change; the ranking behaviour is corrected

One entity can hold several `turn_entities` rows, because the table's key is `(turn_number, entity_id, mention)`. A character who is both wikilinked and extracted therefore counts twice in a ranking that counts rows. This is a bug in the retrieval added by the previous plan, and it is fixed first so the prose work lands on correct arithmetic.

- [ ] **Step 1: Write the failing test**

Append to `pkg/harness/context_test.go`:

```go
func TestRelevantHistoryCountsAnEntityOncePerTurn(t *testing.T) {
	store := newTestEntityStore(t)
	saveEntity(t, store, &entity.Entity{ID: "guard-kael", Name: "Guard Kael", Type: "character", Hash: "h1"})
	saveEntity(t, store, &entity.Entity{ID: "aldon-harbour", Name: "Aldon Harbour", Type: "location", Hash: "h2"})
	saveEntity(t, store, &entity.Entity{ID: "sera-vane", Name: "Sera Vane", Type: "character", Hash: "h3"})

	// Turn 1 names both characters, and Kael carries two mention kinds, which is
	// what the schema allows and what an extracted plus wikilinked turn produces.
	if err := store.SaveTurn(storage.TurnRecord{
		Number: 1, Timestamp: time.Now(), Mode: "Do", Narration: "Kael and Sera spoke.", Location: "oakhaven-tavern",
		Entities: []storage.TurnEntityRef{
			{EntityID: "guard-kael", Mention: "wikilink"},
			{EntityID: "guard-kael", Mention: "extracted"},
			{EntityID: "sera-vane", Mention: "speech"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	// Turn 2 names only Sera, so a per-row count would still rank turn 1 first,
	// but a deduplicated count puts them level and recency decides.
	if err := store.SaveTurn(storage.TurnRecord{
		Number: 2, Timestamp: time.Now(), Mode: "Do", Narration: "Sera waited.", Location: "oakhaven-tavern",
		Entities: []storage.TurnEntityRef{{EntityID: "sera-vane", Mention: "wikilink"}},
	}); err != nil {
		t.Fatal(err)
	}

	assembler := NewContextAssembler(store)
	result, err := assembler.Assemble(ContextRequest{
		LocationID: "aldon-harbour",
		Action:     "I wait",
		Recent: []RecentTurn{
			{Number: 3, Mode: "Do", Narration: "Kael: \"Sera was here.\""},
		},
		TurnNumber: 4,
		PlayerID:   "player",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Both turns mention two entities each once deduplicated, so the newer turn
	// must rank first.
	if !strings.Contains(result.Prompt, "RELEVANT HISTORY") {
		t.Fatalf("expected retrieval to fire:\n%s", result.Prompt)
	}
	history := result.Prompt[strings.Index(result.Prompt, "RELEVANT HISTORY"):]
	if strings.Index(history, "Turn 2") > strings.Index(history, "Turn 1") {
		t.Errorf("expected turn 2 to rank first once mentions are deduplicated:\n%s", history)
	}
}
```

Note: the window turns passed in must mention both characters for both to be query entities. The fixture's recent turn names both in speech, so the query set is `{guard-kael, sera-vane}`.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestRelevantHistoryCountsAnEntityOncePerTurn ./pkg/harness/`
Expected: FAIL — turn 1 ranks first, because Kael's two rows give it a higher count.

- [ ] **Step 3: Deduplicate the count**

In `pkg/harness/context.go`, replace the overlap loop in `relevantHistory`:

```go
		// Overlap is per candidate: how many of the query entities this turn names,
		// counted once each. One entity can hold several mention rows, and counting
		// rows would weight a wikilinked-and-extracted character twice.
		mentions, err := c.store.ListEntitiesForTurn(candidate.Number)
		if err != nil {
			continue
		}
		named := make(map[string]bool, len(mentions))
		for _, mention := range mentions {
			named[mention.EntityID] = true
		}
		overlap := 0
		for _, id := range query {
			if named[id] {
				overlap++
			}
		}
		if overlap == 0 {
			continue
		}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -count=1 ./pkg/harness/ ./pkg/engine/`
Expected: PASS, including the existing retrieval tests.

- [ ] **Step 5: Commit**

```bash
git add pkg/harness/context.go pkg/harness/context_test.go
git commit -m "fix(harness): count a turn's entities once when ranking retrieval"
```

---

### Task 2: A deterministic prose scan

**Files:**
- Modify: `pkg/entity/mention.go`
- Modify: `pkg/harness/extractor.go`
- Test: `pkg/harness/extractor_test.go`

**Interfaces:**
- Produces: `entity.MentionProse = "prose"`
- Produces: `harness.ResolveProseMentions(store *storage.Store, texts ...string) []entity.Mention`

- [ ] **Step 1: Write the failing test**

Append to `pkg/harness/extractor_test.go`:

```go
func TestResolveProseMentionsFindsNamesInProse(t *testing.T) {
	store := newTestEntityStore(t)
	if err := store.SaveEntity(&entity.Entity{ID: "guard-kael", Name: "Guard Kael", Type: "character", Body: "A warden."}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveEntity(&entity.Entity{ID: "sera-vane", Name: "Sera Vane", Type: "character", Body: "A smuggler."}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveEntity(&entity.Entity{ID: "aldon-harbour", Name: "Aldon Harbour", Type: "location", Body: "Salt."}); err != nil {
		t.Fatal(err)
	}

	mentions := ResolveProseMentions(store, "Kael waits by the water.", "I ask after Sera Vane.")

	byID := make(map[string]string, len(mentions))
	for _, mention := range mentions {
		byID[mention.ID] = mention.Kind
	}
	if byID["guard-kael"] != entity.MentionProse {
		t.Errorf("expected Kael's bare surname to resolve, got %+v", mentions)
	}
	if byID["sera-vane"] != entity.MentionProse {
		t.Errorf("expected Sera Vane's full name to resolve, got %+v", mentions)
	}
	if _, present := byID["aldon-harbour"]; present {
		t.Errorf("a location named in passing should not be a prose mention")
	}
}

func TestResolveProseMentionsIgnoresShortAndPartialNames(t *testing.T) {
	store := newTestEntityStore(t)
	if err := store.SaveEntity(&entity.Entity{ID: "the-woman", Name: "The Woman", Type: "character", Body: "Unnamed."}); err != nil {
		t.Fatal(err)
	}

	// A single generic word is not evidence of involvement, and matching it would
	// mark almost every turn as naming her.
	if mentions := ResolveProseMentions(store, "The woman said nothing at all."); len(mentions) != 0 {
		t.Errorf("expected no mention for a single generic word, got %+v", mentions)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestResolveProseMentions ./pkg/harness/`
Expected: FAIL — `undefined: ResolveProseMentions`.

- [ ] **Step 3: Write the implementation**

In `pkg/entity/mention.go`, add the kind beside the others:

```go
	MentionProse     = "prose"
```

In `pkg/harness/extractor.go`, add the scan:

```go
// proseNameMinRunes is the shortest entity name the scan will match on its own
// words. Below it, a name is too generic to be evidence: matching a character
// called "The Woman" against every scene containing a woman would mark almost
// every turn as involving her.
const proseNameMinRunes = 8

// ResolveProseMentions finds known characters the given texts name in prose, which
// is how a turn records involvement when extraction is disabled, when extraction
// misses someone, or when neither spoke nor was linked.
//
// It is deterministic on purpose: no model call, so it is cheap enough to run on
// every turn and safe to run over a campaign's whole history.
func ResolveProseMentions(store *storage.Store, texts ...string) []entity.Mention {
	if store == nil || len(texts) == 0 {
		return nil
	}

	joined := strings.ToLower(strings.Join(texts, "\n"))
	if strings.TrimSpace(joined) == "" {
		return nil
	}

	summaries, err := store.ListEntities()
	if err != nil {
		return nil
	}

	mentions := make([]entity.Mention, 0)
	for _, summary := range summaries {
		if summary.Type != "character" {
			continue
		}

		if name := strings.TrimSpace(summary.Name); len([]rune(name)) >= proseNameMinRunes {
			if containsWord(joined, strings.ToLower(name)) {
				mentions = append(mentions, entity.Mention{ID: summary.ID, Kind: entity.MentionProse})
				continue
			}
		}

		// A long name is also known by its longest single word: "Guard Kael" is
		// called Kael far more often than he is called by his title.
		if word := longestNameWord(summary.Name); word != "" && containsWord(joined, word) {
			mentions = append(mentions, entity.Mention{ID: summary.ID, Kind: entity.MentionProse})
		}
	}
	return mentions
}

// containsWord reports whether the text contains the word with boundaries, so
// "kael" does not match "kaeldrin".
func containsWord(text, word string) bool {
	index := 0
	for {
		found := strings.Index(text[index:], word)
		if found == -1 {
			return false
		}
		found += index

		beforeOK := found == 0 || !isWordRune(rune(text[found-1]))
		after := found + len(word)
		afterOK := after >= len(text) || !isWordRune(rune(text[after]))
		if beforeOK && afterOK {
			return true
		}
		index = found + len(word)
	}
}

func isWordRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '\'' || r == '-'
}

// longestNameWord returns the longest word of a name that is itself long enough to
// be distinctive.
func longestNameWord(name string) string {
	longest := ""
	for _, word := range strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return !isWordRune(r)
	}) {
		if len([]rune(word)) >= 5 && len([]rune(word)) > len([]rune(longest)) {
			longest = word
		}
	}
	return longest
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/harness/ ./pkg/entity/ && go vet ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/entity/mention.go pkg/harness/extractor.go pkg/harness/extractor_test.go
git commit -m "feat(harness): find the characters a turn names in prose"
```

---

### Task 3: Record prose mentions with the turn

**Files:**
- Modify: `pkg/engine/timeline.go`
- Test: `pkg/engine/timeline_test.go`

**Interfaces:**
- Consumes: `harness.ResolveProseMentions` (Task 2), `Timeline.RecordTurn` (existing)
- Produces: `Timeline.RecordTurn` records `prose` mentions, deduplicated against the ones it already has

`RecordTurn` is the single writer for a turn, so recording here means the GUI, the TUI, and any future writer all get the behaviour without each remembering to.

- [ ] **Step 1: Write the failing test**

Append to `pkg/engine/timeline_test.go`:

```go
func TestRecordTurnRecordsProseMentions(t *testing.T) {
	store := newTestStore(t)
	timeline := NewTimeline(core.NewPathResolver(t.TempDir()), store, NewHistoryLogger(filepath.Join(t.TempDir(), "history.jsonl")), "campaign-01")

	entitiesDir := timeline.EntitiesDir()
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "guard-kael", Name: "Guard Kael", Type: "character", Body: "A warden."})
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "sera-vane", Name: "Sera Vane", Type: "character", Body: "A smuggler."})
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		t.Fatal(err)
	}

	turn := Turn{
		Number:    1,
		Timestamp: time.Now(),
		Mode:      "Do",
		Input:     "I ask after Sera Vane",
		Narration: "Kael says nothing, and the water keeps moving.",
		Entities:  []entity.Mention{{ID: "guard-kael", Kind: entity.MentionWikilink}},
	}
	if err := timeline.RecordTurn(&turn, nil); err != nil {
		t.Fatalf("RecordTurn failed: %v", err)
	}

	refs, err := store.ListEntitiesForTurn(1)
	if err != nil {
		t.Fatal(err)
	}

	kinds := make(map[string][]string)
	for _, ref := range refs {
		kinds[ref.EntityID] = append(kinds[ref.EntityID], ref.Mention)
	}
	if len(kinds["sera-vane"]) != 1 || kinds["sera-vane"][0] != entity.MentionProse {
		t.Errorf("expected Sera Vane recorded from the prose, got %+v", kinds["sera-vane"])
	}
	// Kael was already linked, so the scan must not add a second row for him.
	if len(kinds["guard-kael"]) != 1 || kinds["guard-kael"][0] != entity.MentionWikilink {
		t.Errorf("expected Kael recorded once as a wikilink, got %+v", kinds["guard-kael"])
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestRecordTurnRecordsProseMentions ./pkg/engine/`
Expected: FAIL — no `prose` mention for Sera Vane.

- [ ] **Step 3: Record them in `RecordTurn`**

In `pkg/engine/timeline.go`, extend the head of `RecordTurn`:

```go
func (t *Timeline) RecordTurn(turn *Turn, extracted []harness.ExtractedEntity) error {
	// A turn's mentions are what later recall reasons about, so the names its prose
	// contains are recorded here rather than left to whichever writer remembered to
	// resolve them. Extraction records its own, so this only adds what is missing.
	for _, mention := range harness.ResolveProseMentions(t.store, turn.Narration, turn.Input) {
		if !containsMention(turn.Entities, mention.ID) {
			turn.Entities = append(turn.Entities, mention)
		}
	}

	pending, err := t.stageEntities(turn, extracted)
	if err != nil {
		return err
	}
```

`containsMention` already exists in the package and is used by `stageEntities`, so nothing new is needed for deduplication.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -count=1 ./pkg/engine/ && go vet ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/timeline.go pkg/engine/timeline_test.go
git commit -m "feat(engine): record the characters a turn names in prose"
```

---

### Task 4: Prove retrieval now fires without the extractor

**Files:**
- Test: `pkg/engine/orchestrator_recall_test.go`

**Interfaces:**
- Consumes: everything above
- Produces: no new interface; the gap is closed by test

- [ ] **Step 1: Write the failing test**

Append to `pkg/engine/orchestrator_recall_test.go`:

```go
// proseNamingProvider answers with a description rather than a wikilink or a spoken
// line, which is the case the extractor would normally cover.
type proseNamingProvider struct {
	mu        sync.Mutex
	calls     int
	onRequest func(harness.GenerateRequest)
}

func (p *proseNamingProvider) ID() string { return "prose-naming" }

func (p *proseNamingProvider) Generate(ctx context.Context, req harness.GenerateRequest) (*harness.GenerateResponse, error) {
	return &harness.GenerateResponse{Text: "The gate stays shut."}, nil
}

func (p *proseNamingProvider) Stream(ctx context.Context, req harness.GenerateRequest, out chan<- harness.StreamChunk) error {
	defer close(out)

	if p.onRequest != nil {
		p.onRequest(req)
	}

	p.mu.Lock()
	p.calls++
	call := p.calls
	p.mu.Unlock()

	// No wikilink and no speech: the name is only ever narrated. The extractor is
	// off in this fixture, so only the prose scan can see it.
	text := "Garrick keeps his own counsel."
	if call == 1 {
		text = "Garrick mentioned the oil was low."
	}
	out <- harness.StreamChunk{Text: text, Done: true}
	return nil
}

func TestRetrievalWorksWithoutTheExtractor(t *testing.T) {
	var prompts []string
	provider := &proseNamingProvider{
		onRequest: func(req harness.GenerateRequest) {
			prompts = append(prompts, req.Prompt)
		},
	}

	orchestrator, timeline, store := streamingOrchestrator(t, provider)
	// No extractor is set, which is the gap this plan closes.

	entitiesDir := timeline.EntitiesDir()
	writeTestEntityNote(t, entitiesDir, &entity.Entity{
		ID: "garrick", Name: "Garrick", Type: "character", Body: "A fence with a long memory.",
	})
	writeTestEntityNote(t, entitiesDir, &entity.Entity{
		ID: "aldon-harbour", Name: "Aldon Harbour", Type: "location", Body: "Salt air and gulls.",
	})
	writeTestEntityNote(t, entitiesDir, &entity.Entity{
		ID: "alden-tavern", Name: "Alden Tavern", Type: "location", Body: "Warm and low.",
	})
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		t.Fatal(err)
	}
	movePlayerTo(t, entitiesDir, store, "aldon-harbour")

	if _, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I look around", nil); err != nil {
		t.Fatalf("turn 1 failed: %v", err)
	}

	movePlayerTo(t, entitiesDir, store, "alden-tavern")
	for i := 0; i < 8; i++ {
		if _, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I wait", nil); err != nil {
			t.Fatalf("turn %d failed: %v", i+2, err)
		}
	}

	last := prompts[len(prompts)-1]
	if !strings.Contains(last, "RELEVANT HISTORY") {
		t.Fatalf("expected retrieval to fire from prose mentions alone:\n%s", last)
	}
	if !strings.Contains(last, "the oil was low") {
		t.Errorf("expected the harbour turn retrieved:\n%s", last)
	}
}
```

- [ ] **Step 2: Run the test to verify it passes**

Run: `go test -count=1 -run TestRetrievalWorksWithoutTheExtractor -v ./pkg/engine/`
Expected: PASS.

- [ ] **Step 3: Run the full gate**

Run: `go test -count=1 ./... && go vet ./...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add pkg/engine/orchestrator_recall_test.go
git commit -m "test(engine): prove retrieval works without an extractor"
```

---

### Task 5: Backfill a campaign's history (explicit, opt-in)

**Files:**
- Create: `pkg/engine/backfill.go`
- Test: `pkg/engine/backfill_test.go`
- Modify: `cmd/localrpg/play.go` (no; this is a command, see below)
- Modify: `cmd/localrpg/main.go` and a new `cmd/localrpg/reindex.go`

**Interfaces:**
- Produces: `engine.BackfillProseMentions(paths *core.PathResolver, store *storage.Store, gameID string) (int, error)` returning how many turns gained a mention
- Produces: the CLI command `localrpg reindex <game-id>`

This is the one part that touches `history.jsonl`, so it is a command a person runs rather than something a turn does. It is safe because the scan is deterministic: running it twice produces the same mentions.

- [ ] **Step 1: Write the failing test**

Create `pkg/engine/backfill_test.go`:

```go
package engine

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/storage"
)

func TestBackfillAddsProseMentionsToExistingTurns(t *testing.T) {
	tempDir := t.TempDir()
	store := newTestStore(t)
	timeline := NewTimeline(core.NewPathResolver(tempDir), store, NewHistoryLogger(filepath.Join(tempDir, "history.jsonl")), "campaign-01")

	entitiesDir := timeline.EntitiesDir()
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "garrick", Name: "Garrick", Type: "character", Body: "A fence."})
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		t.Fatal(err)
	}

	// A turn recorded before the scan existed: its prose names Garrick, but its
	// record does not say so.
	record := `{"number":1,"timestamp":"2026-09-22T10:00:00Z","mode":"Do","input":"I look around","narration":"Garrick said nothing.","entities":[{"id":"player","mention":"player"}]}` + "\n"
	path := filepath.Join(tempDir, "history.jsonl")
	if err := os.WriteFile(path, []byte(record), 0644); err != nil {
		t.Fatal(err)
	}
	if err := timeline.EnsureIndexed(); err != nil {
		t.Fatal(err)
	}

	changed, err := BackfillProseMentions(core.NewPathResolver(tempDir), store, "campaign-01")
	if err != nil {
		t.Fatalf("BackfillProseMentions failed: %v", err)
	}
	if changed != 1 {
		t.Errorf("expected one turn updated, got %d", changed)
	}

	refs, err := store.ListEntitiesForTurn(1)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ref := range refs {
		if ref.EntityID == "garrick" && ref.Mention == entity.MentionProse {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a prose mention for Garrick, got %+v", refs)
	}

	// Running it again must change nothing: the scan is deterministic.
	again, err := BackfillProseMentions(core.NewPathResolver(tempDir), store, "campaign-01")
	if err != nil {
		t.Fatal(err)
	}
	if again != 0 {
		t.Errorf("expected a second run to be a no-op, got %d", again)
	}
}

func TestBackfillLeavesATurnItCannotParse(t *testing.T) {
	tempDir := t.TempDir()
	store := newTestStore(t)
	path := filepath.Join(tempDir, "history.jsonl")
	if err := os.WriteFile(path, []byte("{not json}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	changed, err := BackfillProseMentions(core.NewPathResolver(tempDir), store, "campaign-01")
	if err != nil {
		t.Fatalf("a malformed line must not fail the backfill: %v", err)
	}
	if changed != 0 {
		t.Errorf("expected nothing changed, got %d", changed)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{not json}\n" {
		t.Errorf("expected the log left intact, got %q", data)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestBackfill ./pkg/engine/`
Expected: FAIL — `undefined: BackfillProseMentions`.

- [ ] **Step 3: Write the implementation**

Create `pkg/engine/backfill.go`:

```go
package engine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// BackfillProseMentions adds the prose mentions a campaign's recorded turns are
// missing, so recall can see characters that were only ever described in turns
// recorded before the scan existed.
//
// It rewrites history.jsonl, which is otherwise append-only, so it runs only when a
// person asks for it and leaves a copy at history.jsonl.bak. It is safe to repeat:
// the scan is deterministic and a mention already present is left alone.
func BackfillProseMentions(paths *core.PathResolver, store *storage.Store, gameID string) (int, error) {
	historyPath := filepath.Join(paths.GameDir(gameID), "history.jsonl")

	logger := NewHistoryLogger(historyPath)
	turns, err := logger.LoadHistory()
	if err != nil {
		return 0, fmt.Errorf("load history: %w", err)
	}

	changed := 0
	for i := range turns {
		turn := &turns[i]
		added := false

		for _, mention := range harness.ResolveProseMentions(store, turn.Narration, turn.Input) {
			if containsMention(turn.Entities, mention.ID) {
				continue
			}
			turn.Entities = append(turn.Entities, mention)
			added = true
		}

		if added {
			changed++
		}
	}

	if changed == 0 {
		return 0, nil
	}

	// A copy first: this file is the canonical timeline, and a mistake here is not
	// recoverable from anywhere else.
	if data, err := os.ReadFile(historyPath); err == nil {
		if err := os.WriteFile(historyPath+".bak", data, 0644); err != nil {
			return 0, fmt.Errorf("back up history: %w", err)
		}
	}

	if err := logger.RewriteHistory(turns); err != nil {
		return 0, fmt.Errorf("rewrite history: %w", err)
	}
	if err := NewTimeline(paths, store, logger, gameID).EnsureIndexed(); err != nil {
		return 0, fmt.Errorf("reindex turns: %w", err)
	}
	return changed, nil
}
```

- [ ] **Step 4: Add the history rewrite**

`HistoryLogger` currently only appends. Add a rewrite to `pkg/engine/history.go`:

```go
// RewriteHistory replaces the log with the given turns. It exists for the prose
// mention backfill, which is the one operation that must revise recorded turns;
// everything else appends. The caller is responsible for taking a copy first.
func (h *HistoryLogger) RewriteHistory(turns []Turn) error {
	var buffer bytes.Buffer
	for _, turn := range turns {
		encoded, err := json.Marshal(turn)
		if err != nil {
			return fmt.Errorf("encode turn %d: %w", turn.Number, err)
		}
		buffer.Write(encoded)
		buffer.WriteByte('\n')
	}
	if err := os.WriteFile(h.path, buffer.Bytes(), 0644); err != nil {
		return fmt.Errorf("write history: %w", err)
	}
	return nil
}
```

Add `"bytes"` to that file's imports if it is missing.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/engine/ && go vet ./...`
Expected: PASS.

- [ ] **Step 6: Add the command**

Create `cmd/localrpg/reindex.go`:

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/engine"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// handleReindexCommand repairs a campaign's recorded mentions so recall can see
// characters its turns only ever described. It is explicit because it rewrites the
// canonical timeline, and it leaves a copy beside it.
func handleReindexCommand(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: localrpg reindex <game-id>")
		os.Exit(1)
	}

	cfgMgr := config.NewConfigManager()
	cfg, _ := cfgMgr.Load()

	gameID := args[0]
	paths := core.NewCustomPathResolver(cfg.Paths.Systems, cfg.Paths.Worlds, cfg.Paths.Games, cfg.Paths.Cache)
	if _, err := os.Stat(filepath.Join(paths.GameDir(gameID), "game.yaml")); err != nil {
		fmt.Fprintf(os.Stderr, "Error: no such campaign %q\n", gameID)
		os.Exit(1)
	}

	store, err := storage.OpenGameStore(paths, gameID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening game database: %v\n", err)
		os.Exit(1)
	}

	changed, err := engine.BackfillProseMentions(paths, store, gameID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if changed == 0 {
		fmt.Println("Nothing to update: every turn already records the names its prose contains.")
		return
	}
	fmt.Printf("Updated %d turn(s). A copy of the previous log is at %s.bak\n",
		changed, filepath.Join(paths.GameDir(gameID), "history.jsonl"))
}
```

and dispatch it from `cmd/localrpg/main.go`, beside the other commands:

```go
	case "reindex":
		handleReindexCommand(args[1:])
```

- [ ] **Step 7: Run the gate and commit**

Run: `go build ./... && go test -count=1 ./cmd/... ./pkg/engine/ && go vet ./...`
Expected: PASS.

```bash
git add pkg/engine/backfill.go pkg/engine/backfill_test.go pkg/engine/history.go cmd/localrpg/reindex.go cmd/localrpg/main.go
git commit -m "feat(engine): add a reindex command to backfill prose mentions"
```

---

## Self-Review

**Spec coverage** (coherence spec section 5.4, retrieval):

| Requirement | Task |
| --- | --- |
| A character named in prose counts as in play | 2, 3, 4 |
| Retrieval works when the extractor is disabled or misses someone | 4 |
| Overlap ranking counts an entity once per turn | 1 |
| No model call, so it is cheap and repeatable | 2, 5 |
| Existing campaigns can benefit | 5 |

**Placeholder scan:** no "TBD", no "similar to Task N". Task 5 names its two failure modes and tests both.

**Type consistency:** `entity.MentionProse`, `harness.ResolveProseMentions`, `engine.BackfillProseMentions`, and `HistoryLogger.RewriteHistory` are each defined once and used with the same signatures.

**Correction of record:** this plan exists because an earlier claim of mine was wrong. `Timeline.stageEntities` already records extraction entities as `extracted` mentions, so the gap was never "extractor entities are unlinked". It is that turns recorded without a working extractor, or where the extractor misses someone, have nothing recorded. The prose scan closes that, and it also covers the described-but-never-linked case that extraction was being relied on to catch.

**Open question for the reviewer:** Task 5 rewrites `history.jsonl`, which the canonical-db spec calls append-only. The alternative is to leave existing campaigns un-backfilled and accept that recall improves only for turns recorded from now on. The plan takes the rewrite because the scan is deterministic and the copy makes it recoverable, but this is a genuine trade and worth a decision before Task 5 runs.
