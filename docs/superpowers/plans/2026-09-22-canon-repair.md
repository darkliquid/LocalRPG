# Canon Repair Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a player tell the system that two names are one being, so a renamed character stops becoming a second entity and stops losing their history, their voice, and their place in recall.

**Architecture:** Entities gain an `aliases` list. Matching and speaker resolution consult it, the prompt lists it, and merging folds one note into another as an explicit player action. Nothing happens automatically: deciding two names are one being is a judgement the engine cannot make, but it can carry out the decision once made.

**Tech Stack:** Go 1.27.1, the standard library and `modernc.org/sqlite`, React 19 + TypeScript for the Codex action.

**Spec:** `docs/superpowers/specs/2026-09-22-narrative-coherence-and-trace-design.md` (section 6)

## Global Constraints

- Go tests use the standard library only (`testing`, `t.TempDir()`); no testify. Use `interface{}`, never `any`.
- No new dependencies, Go or Node.
- No migration path. The project is pre-release, so existing notes without `aliases` behave exactly as they do now.
- Aliases are matched in addition to the existing signals, never instead of them: an entity found by ID or name is found first.
- Merging is explicit and destructive. It must be reachable only from a confirmation, and it must never be triggered by a model.
- A merge rewrites inbound links so no note points at an entity that no longer exists.
- `go vet ./...`, `go test -count=1 ./...`, and `cd frontend && npx tsc --noEmit` must pass. Never commit the deletion of `pkg/gui/dist/.gitkeep`.

## Scope & Splitting

This plan implements **increment 5**. Increment 6 (the continuity checks, open threads, and the recap's threads) is a separate plan: it verifies what the model wrote, which is a different job from repairing what it got wrong, and nothing in it depends on merging.

---

### Task 1: Entities carry aliases

**Files:**
- Modify: `pkg/entity/entity.go`
- Modify: `pkg/storage/store.go`
- Test: `pkg/entity/entity_test.go`, `pkg/storage/store_test.go`

**Interfaces:**
- Produces: `entity.Entity.Aliases []string`, `entity.EntityFrontmatter.Aliases []string`
- Produces: `storage.EntitySummary.Aliases []string`

`ListEntities` already selects `frontmatter_json`, so the index gains aliases without a SQL change: the summary parses them from the same blob it already reads for location and tags.

- [ ] **Step 1: Write the failing test**

Append to `pkg/entity/entity_test.go`:

```go
func TestAliasesRoundTripThroughMarkdown(t *testing.T) {
	original := &Entity{
		ID:      "guard-kael",
		Name:    "Guard Kael",
		Type:    "character",
		Body:    "A warden of the ember.",
		Aliases: []string{"The Ember Warden", "Kael"},
	}

	data, err := original.SerializeMarkdown()
	if err != nil {
		t.Fatalf("SerializeMarkdown failed: %v", err)
	}

	parsed, err := ParseMarkdownEntity(data)
	if err != nil {
		t.Fatalf("ParseMarkdownEntity failed: %v", err)
	}
	if len(parsed.Aliases) != 2 {
		t.Fatalf("Aliases = %v, want two", parsed.Aliases)
	}
	if parsed.Aliases[0] != "The Ember Warden" || parsed.Aliases[1] != "Kael" {
		t.Errorf("aliases did not round-trip: %v", parsed.Aliases)
	}
}

func TestAnEntityWithoutAliasesHasNone(t *testing.T) {
	parsed, err := ParseMarkdownEntity([]byte("---\nid: sera\nname: Sera\ntype: character\n---\nBody.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Aliases) != 0 {
		t.Errorf("expected no aliases, got %v", parsed.Aliases)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestAliases ./pkg/entity/`
Expected: FAIL — `unknown field Aliases`.

- [ ] **Step 3: Add the field**

In `pkg/entity/entity.go`, add to `Entity`:

```go
	// Aliases are other names the same being is known by. They exist because a
	// model will rename a character, and the alternative to recording both names is
	// a second entity losing the first one's history.
	Aliases []string
```

and to `EntityFrontmatter`:

```go
	Aliases    []string               `yaml:"aliases,omitempty" json:"aliases,omitempty"`
```

In `SerializeMarkdown`, set `Aliases: e.Aliases` on the frontmatter; in `ParseMarkdownEntity`, set `Aliases: frontmatter.Aliases` on the entity. Follow the surrounding pattern for the other optional fields.

- [ ] **Step 4: Carry them into the index summary**

In `pkg/storage/store.go`, extend `EntitySummary` and the existing decode of `frontmatter_json` in `ListEntities`:

```go
type EntitySummary struct {
	ID       string
	Name     string
	Type     string
	Location string
	Tags     []string
	Aliases  []string
}
```

```go
		var meta struct {
			Location string   `json:"location"`
			Tags     []string `json:"tags"`
			Aliases  []string `json:"aliases"`
		}
		if err := json.Unmarshal([]byte(fmJSON), &meta); err == nil {
			summary.Location = meta.Location
			summary.Tags = meta.Tags
			summary.Aliases = meta.Aliases
		}
```

Append to `pkg/storage/store_test.go`:

```go
func TestListEntitiesCarriesAliases(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.SaveEntity(&entity.Entity{
		ID: "guard-kael", Name: "Guard Kael", Type: "character", Body: "A warden.",
		Aliases: []string{"The Ember Warden"}, Hash: "h1",
	}); err != nil {
		t.Fatal(err)
	}

	summaries, err := store.ListEntities()
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected one summary, got %d", len(summaries))
	}
	if len(summaries[0].Aliases) != 1 || summaries[0].Aliases[0] != "The Ember Warden" {
		t.Errorf("Aliases = %v, want the note's alias", summaries[0].Aliases)
	}
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/entity/ ./pkg/storage/ && go vet ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/entity/entity.go pkg/entity/entity_test.go pkg/storage/store.go pkg/storage/store_test.go
git commit -m "feat(entity): let a note record the other names a being is known by"
```

---

### Task 2: Matching and speech resolution honour aliases

**Files:**
- Modify: `pkg/harness/extractor.go`
- Modify: `pkg/harness/context.go`
- Test: `pkg/harness/extractor_test.go`, `pkg/harness/context_test.go`

**Interfaces:**
- Consumes: `entity.Aliases`, `storage.EntitySummary.Aliases` (Task 1)
- Produces: no signature change; three lookups gain a signal

- [ ] **Step 1: Write the failing test**

Append to `pkg/harness/extractor_test.go`:

```go
func TestResolveSpeakerIDAndMatchingKnowAliases(t *testing.T) {
	store := newTestEntityStore(t)
	if err := store.SaveEntity(&entity.Entity{
		ID: "guard-kael", Name: "Guard Kael", Type: "character", Body: "A warden.",
		Aliases: []string{"The Ember Warden"},
	}); err != nil {
		t.Fatal(err)
	}

	if got := ResolveSpeakerID(store, "The Ember Warden"); got != "guard-kael" {
		t.Errorf("ResolveSpeakerID(alias) = %q, want guard-kael", got)
	}

	matched := MatchExistingEntity(store, &ExtractedEntity{Name: "The Ember Warden", Type: "character"})
	if matched == nil || matched.ID != "guard-kael" {
		t.Errorf("expected the alias to match the existing entity, got %+v", matched)
	}

	// An alias does not invent an entity: an unknown name still matches nothing.
	if MatchExistingEntity(store, &ExtractedEntity{Name: "Someone Else", Type: "character"}) != nil {
		t.Errorf("expected an unrelated name not to match")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestResolveSpeakerIDAndMatchingKnowAliases ./pkg/harness/`
Expected: FAIL — an alias does not resolve.

- [ ] **Step 3: Consult aliases**

In `pkg/harness/extractor.go`, add a shared helper and use it in both lookups:

```go
// aliasMatches reports whether a candidate names an entity through one of its
// aliases. Aliases are matched after IDs and names, never before: a note's own name
// is always the stronger signal.
func aliasMatches(candidate string, aliases []string) bool {
	slug := entity.Slugify(entity.WikilinkTarget(candidate))
	if slug == "" {
		return false
	}
	for _, alias := range aliases {
		if entity.Slugify(alias) == slug {
			return true
		}
	}
	return false
}
```

In `ResolveSpeakerID`, extend the summary scan:

```go
	for _, summary := range summaries {
		if entity.Slugify(summary.Name) == slug || aliasMatches(cleaned, summary.Aliases) {
			return summary.ID, nil
		}
	}
```

In `MatchExistingEntity`, extend the first summary loop:

```go
	for _, summary := range summaries {
		if summary.ID == raw.ID || summary.ID == nameKey || entity.Slugify(summary.Name) == nameKey {
			// … existing body …
		}
		if aliasMatches(raw.Name, summary.Aliases) {
			if ent, err := store.GetEntity(summary.ID); err == nil && ent != nil {
				return ent
			}
		}
	}
```

In `pkg/harness/context.go`, `establishedNames` lists aliases beside the name, so the model is told both are one being:

```go
		seen[id] = true
		if len(ent.Aliases) > 0 {
			names = append(names, fmt.Sprintf("%s (also known as %s)", ent.Name, strings.Join(ent.Aliases, ", ")))
			return
		}
		names = append(names, ent.Name)
```

Append to `pkg/harness/context_test.go`:

```go
func TestEstablishedNamesListAliases(t *testing.T) {
	store := newTestEntityStore(t)
	saveEntity(t, store, &entity.Entity{
		ID: "guard-kael", Name: "Guard Kael", Type: "character", Hash: "h1",
		Aliases: []string{"The Ember Warden"},
	})

	assembler := NewContextAssembler(store)
	result, err := assembler.Assemble(ContextRequest{Action: "I wait"})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result.Prompt, "also known as") || !strings.Contains(result.Prompt, "The Ember Warden") {
		t.Errorf("expected the alias in the established names:\n%s", result.Prompt)
	}
}
```

That test needs the alias-bearing entity to be in play; `establishedNames` only covers entities reachable from the scene or the recall window. The fixture's `assembleCanon` reads the location's edges, so add the character to the location's wikilinks, or pass a recent turn mentioning them:

```go
	assembler := NewContextAssembler(store)
	result, err := assembler.Assemble(ContextRequest{
		Action:     "I wait",
		TurnNumber: 2,
		Recent:     []RecentTurn{{Number: 1, Mode: "Do", Narration: "Kael waited."}},
	})
```

with a store whose turn 1 mentions `guard-kael`, as the earlier established-names test does.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/harness/ && go vet ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/harness/extractor.go pkg/harness/context.go pkg/harness/extractor_test.go pkg/harness/context_test.go
git commit -m "feat(harness): resolve and name a being by their aliases too"
```

---

### Task 3: Merging two notes into one

**Files:**
- Modify: `pkg/storage/store.go`
- Modify: `pkg/gui/service.go`
- Test: `pkg/gui/service_test.go`

**Interfaces:**
- Produces: `(*Store).DeleteEntity(id string) error`
- Produces: `(*Service).MergeEntities(ctx context.Context, gameID, sourceID, targetID string) (*EntityDTO, error)`

- [ ] **Step 1: Write the failing test**

Append to `pkg/gui/service_test.go`:

```go
func TestMergeFoldsANoteIntoAnother(t *testing.T) {
	gameID, svc := setupTestGame(t)

	// A duplicate: the same being under the name the model invented for him.
	if err := svc.SaveEntity(context.Background(), gameID, "the-ember-warden",
		"---\nid: the-ember-warden\nname: The Ember Warden\ntype: character\ntags: [warden]\naliases: [Kael]\n---\nStands vigil by the brazier.\n"); err != nil {
		t.Fatal(err)
	}

	// Another note links to the duplicate, so the merge has a link to rewrite.
	captain := "---\nid: captain-kaelen\nname: Captain Kaelen\ntype: npc\n---\nReports to [[the-ember-warden]] each dawn.\n"
	if err := svc.SaveEntity(context.Background(), gameID, "captain-kaelen", captain); err != nil {
		t.Fatal(err)
	}

	merged, err := svc.MergeEntities(context.Background(), gameID, "the-ember-warden", "captain-kaelen")
	if err != nil {
		t.Fatalf("MergeEntities failed: %v", err)
	}
	if merged.ID != "captain-kaelen" {
		t.Errorf("merged into %q, want captain-kaelen", merged.ID)
	}
	if !strings.Contains(merged.Markdown, "Stands vigil by the brazier.") {
		t.Errorf("expected the source body folded in:\n%s", merged.Markdown)
	}
	if !strings.Contains(merged.Markdown, "Kael") {
		t.Errorf("expected the source aliases folded in:\n%s", merged.Markdown)
	}

	// The source note is gone, from disk and from the index.
	if _, err := svc.GetEntity(context.Background(), gameID, "the-ember-warden"); err == nil {
		t.Errorf("expected the source note to be removed")
	}

	// Inbound links now point at the survivor rather than at a note that is gone.
	note, err := svc.GetEntity(context.Background(), gameID, "captain-kaelen")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(note.Markdown, "[[the-ember-warden]]") {
		t.Errorf("expected the inbound link rewritten:\n%s", note.Markdown)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestMergeFoldsANoteIntoAnother ./pkg/gui/`
Expected: FAIL — `svc.MergeEntities undefined`.

- [ ] **Step 3: Add the index delete**

In `pkg/storage/store.go`:

```go
// DeleteEntity removes an entity and its edges from the index. The note on disk is
// the caller's business: the index is derived, and a merge deletes both.
func (s *Store) DeleteEntity(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM edges WHERE source_id = ? OR target_id = ?`, id, id); err != nil {
		return fmt.Errorf("clear edges: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM entities WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete entity: %w", err)
	}
	return tx.Commit()
}
```

- [ ] **Step 4: Implement the merge**

In `pkg/gui/service.go`:

```go
// MergeEntities folds one note into another: the survivor keeps its identity and
// gains the source's prose, tags, aliases, and turn history, every note that linked
// to the source is rewritten to point at the survivor, and the source is removed.
//
// It is deliberately explicit. Deciding that two names are one being is a judgement
// the engine cannot make, but it can carry the decision out once a player makes it.
func (s *Service) MergeEntities(ctx context.Context, gameID, sourceID, targetID string) (*EntityDTO, error) {
	if sourceID == targetID {
		return nil, fmt.Errorf("cannot merge %q into itself", sourceID)
	}

	gameDir := s.resolver.GameDir(gameID)
	sourceData, err := os.ReadFile(filepath.Join(gameDir, "entities", sourceID+".md"))
	if err != nil {
		return nil, fmt.Errorf("read source %q: %w", sourceID, err)
	}
	targetPath := filepath.Join(gameDir, "entities", targetID+".md")
	targetData, err := os.ReadFile(targetPath)
	if err != nil {
		return nil, fmt.Errorf("read target %q: %w", targetID, err)
	}

	source, err := entity.ParseMarkdownEntity(sourceData)
	if err != nil {
		return nil, fmt.Errorf("parse source %q: %w", sourceID, err)
	}
	target, err := entity.ParseMarkdownEntity(targetData)
	if err != nil {
		return nil, fmt.Errorf("parse target %q: %w", targetID, err)
	}

	// The survivor keeps its name and gains what the source knew.
	if body := strings.TrimSpace(source.Body); body != "" {
		target.Body = strings.TrimSpace(target.Body) + "\n\n" + body
	}
	target.Aliases = appendUnique(target.Aliases, source.Name)
	target.Aliases = appendUnique(target.Aliases, source.Aliases...)
	target.Tags = appendUnique(target.Tags, source.Tags...)
	for _, number := range source.History {
		target.History = appendTurnNumber(target.History, number)
	}

	merged, err := target.SerializeMarkdown()
	if err != nil {
		return nil, fmt.Errorf("serialize merged note: %w", err)
	}

	// Write the survivor first: a failure after this point leaves both notes rather
	// than losing the source's content.
	if err := os.WriteFile(targetPath, merged, 0644); err != nil {
		return nil, fmt.Errorf("write merged note: %w", err)
	}

	if err := s.rewriteInboundLinks(gameDir, sourceID, targetID); err != nil {
		return nil, err
	}

	if err := os.Remove(filepath.Join(gameDir, "entities", sourceID+".md")); err != nil {
		return nil, fmt.Errorf("remove source note: %w", err)
	}

	store, err := s.store(gameID)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	if err := store.DeleteEntity(sourceID); err != nil {
		return nil, fmt.Errorf("remove source from the index: %w", err)
	}

	syncer := storage.NewSyncer(store)
	for _, id := range []string{targetID, sourceID} {
		_ = syncer.SyncFile(filepath.Join(gameDir, "entities", id+".md"))
	}

	return s.GetEntity(ctx, gameID, targetID)
}

// rewriteInboundLinks points every note that linked to the source at the survivor,
// so no note is left pointing at an entity that no longer exists.
func (s *Service) rewriteInboundLinks(gameDir, sourceID, targetID string) error {
	entitiesDir := filepath.Join(gameDir, "entities")
	entries, err := os.ReadDir(entitiesDir)
	if err != nil {
		return fmt.Errorf("read entities dir: %w", err)
	}

	pattern := regexp.MustCompile(`\[\[\s*` + regexp.QuoteMeta(sourceID) + `(\s*\|[^\]]*)?\]\]`)
	replacement := "[[" + targetID + "$1]]"

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".md") || strings.TrimSuffix(entry.Name(), ".md") == sourceID {
			continue
		}

		path := filepath.Join(entitiesDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if !pattern.Match(data) {
			continue
		}

		updated := pattern.ReplaceAll(data, []byte(replacement))
		if err := os.WriteFile(path, updated, 0644); err != nil {
			return fmt.Errorf("rewrite links in %s: %w", entry.Name(), err)
		}
	}
	return nil
}

// appendUnique adds values that are not already present, preserving order.
func appendUnique(existing []string, values ...string) []string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}

		found := false
		for _, candidate := range existing {
			if strings.EqualFold(candidate, trimmed) {
				found = true
				break
			}
		}
		if !found {
			existing = append(existing, trimmed)
		}
	}
	return existing
}
```

`appendTurnNumber` lives in `pkg/engine` and is unexported, so the service cannot call it. Replace that loop with an inlined union:

```go
	for _, number := range source.History {
		already := false
		for _, known := range target.History {
			if known == number {
				already = true
				break
			}
		}
		if !already {
			target.History = append(target.History, number)
		}
	}
```

Since `Service` is in a different package from `entity`, `appendUnique` here is a local helper; if `entity` grows one later, use that instead.

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test -count=1 ./pkg/gui/ && go vet ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/storage/store.go pkg/gui/service.go pkg/gui/service_test.go
git commit -m "feat(gui): fold one note into another when a player says they are one"
```

---

### Task 4: The merge endpoint

**Files:**
- Modify: `pkg/gui/server.go`
- Modify: `pkg/gui/types.go`
- Test: `pkg/gui/server_test.go`

**Interfaces:**
- Produces: `POST /api/game/{id}/entity/{source}/merge` with body `{"into": "<id>"}`, answering with the merged `EntityDTO`

- [ ] **Step 1: Write the failing test**

Append to `pkg/gui/server_test.go`:

```go
func TestMergeRouteFoldsOneNoteIntoAnother(t *testing.T) {
	gameID, svc := setupTestGame(t)
	server := NewServer(svc, http.NotFoundHandler())

	if err := svc.SaveEntity(context.Background(), gameID, "the-ember-warden",
		"---\nid: the-ember-warden\nname: The Ember Warden\ntype: character\n---\nStands vigil.\n"); err != nil {
		t.Fatal(err)
	}

	body := `{"into":"captain-kaelen"}`
	req := httptest.NewRequest(http.MethodPost, "/api/game/"+gameID+"/entity/the-ember-warden/merge", strings.NewReader(body))
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("merge: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var merged EntityDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &merged); err != nil {
		t.Fatalf("decode merged entity: %v", err)
	}
	if merged.ID != "captain-kaelen" {
		t.Errorf("merged into %q, want captain-kaelen", merged.ID)
	}

	// A merge with no target is a malformed request, not a silent no-op.
	req = httptest.NewRequest(http.MethodPost, "/api/game/"+gameID+"/entity/captain-kaelen/merge", strings.NewReader(`{}`))
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for a missing target, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestMergeRoute ./pkg/gui/`
Expected: FAIL — 404, because nothing serves `merge`.

- [ ] **Step 3: Add the route**

In `pkg/gui/types.go`:

```go
// MergeEntityRequestDTO names the note that should survive a merge.
type MergeEntityRequestDTO struct {
	Into string `json:"into"`
}
```

In `pkg/gui/server.go`, inside the `entity` case, before the plain entity read:

```go
		// POST /api/game/{id}/entity/{source}/merge folds one note into another.
		if r.Method == http.MethodPost && len(parts) >= 4 && parts[3] == "merge" {
			var req MergeEntityRequestDTO
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnBody)).Decode(&req); err != nil {
				http.Error(w, "invalid request body", http.StatusBadRequest)
				return
			}
			if strings.TrimSpace(req.Into) == "" {
				http.Error(w, "a merge needs a note to merge into", http.StatusBadRequest)
				return
			}

			merged, err := s.service.MergeEntities(r.Context(), gameID, parts[2], req.Into)
			if err != nil {
				writeGameError(w, err)
				return
			}
			writeJSON(w, merged)
			return
		}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -count=1 ./pkg/gui/ && go vet ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/server.go pkg/gui/types.go pkg/gui/server_test.go
git commit -m "feat(gui): expose the merge as an endpoint"
```

---

### Task 5: The Codex offers the merge

**Files:**
- Modify: `frontend/src/api/client.ts`
- Modify: `frontend/src/components/CodexDrawer.tsx`
- Modify: `frontend/src/App.tsx`
- Test: `cd frontend && npx tsc --noEmit`

**Interfaces:**
- Produces: `APIClient.mergeEntity(sourceID, intoID)`
- Produces: a "Merge into…" control in the Codex note editor, confirmed before it runs

- [ ] **Step 1: Add the client method**

In `frontend/src/api/client.ts`:

```ts
  async mergeEntity(sourceID: string, intoID: string): Promise<EntityNote> {
    const res = await fetch(`/api/game/${this.gameID}/entity/${sourceID}/merge`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ into: intoID }),
    });
    if (!res.ok) throw new Error(`mergeEntity: ${res.statusText}`);
    return res.json();
  }
```

- [ ] **Step 2: Add the control**

In `frontend/src/components/CodexDrawer.tsx`, add to the props:

```tsx
  entities?: EntitySummary[];
  onMerge?: (sourceID: string, intoID: string) => void;
```

and in the note header, beside Save, a control that only appears when there is something to merge into:

```tsx
            <div className="flex items-center gap-2">
              {onMerge && (entities?.length ?? 0) > 1 && (
                <select
                  onChange={(e) => {
                    if (e.target.value && entity) {
                      onMerge(entity.id, e.target.value);
                      e.target.value = '';
                    }
                  }}
                  className="bg-stone-900 border border-stone-700 rounded-lg pl-2.5 pr-8 py-1.5 text-xs font-mono text-stone-300 focus:outline-none cursor-pointer"
                  defaultValue=""
                >
                  <option value="" disabled>Merge into...</option>
                  {(entities ?? [])
                    .filter((candidate) => candidate.id !== entity.id)
                    .map((candidate) => (
                      <option key={candidate.id} value={candidate.id}>
                        {candidate.name}
                      </option>
                    ))}
                </select>
              )}
              <button
                onClick={() => onSave(entity.id, markdown)}
                className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-amber-600 hover:bg-amber-500 text-stone-950 font-cinzel font-bold text-xs shadow-md transition-all cursor-pointer"
              >
                <Save className="w-3.5 h-3.5" />
                <span>Save</span>
              </button>
            </div>
```

The existing header's Save button is replaced by this block.

- [ ] **Step 3: Confirm and call it from App**

In `frontend/src/App.tsx`:

```tsx
  const handleMergeEntity = async (sourceID: string, intoID: string) => {
    if (!client || !activeGameID) return;

    const sourceName = entities.find((candidate) => candidate.id === sourceID)?.name ?? sourceID;
    const targetName = entities.find((candidate) => candidate.id === intoID)?.name ?? intoID;
    const confirmed = window.confirm(
      `Merge "${sourceName}" into "${targetName}"? Its prose, tags, aliases, and turn history move across, and the note is removed.`
    );
    if (!confirmed) return;

    try {
      const merged = await client.mergeEntity(sourceID, intoID);
      setSelectedEntity(merged);
      refreshCorpus();
    } catch (err) {
      console.error('merge failed:', err);
    }
  };
```

and pass it to the drawer:

```tsx
                onMerge={handleMergeEntity}
```

- [ ] **Step 4: Typecheck and build**

Run: `cd frontend && npx tsc --noEmit && npm run build`
Expected: no type errors, a successful build. Restore `pkg/gui/dist/.gitkeep` afterwards and do not stage its deletion.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/
git commit -m "feat(frontend): merge one codex note into another"
```

---

## Self-Review

**Spec coverage** (coherence spec section 6):

| Requirement | Task |
| --- | --- |
| `aliases` frontmatter, honoured by matching | 1, 2 |
| Aliases consulted by speaker resolution, so speech attributions resolve | 2 |
| Aliases listed in the prompt so the model knows both are one being | 2 |
| Merge as an explicit player action, never automatic | 3, 4, 5 |
| Merge unions prose, tags, aliases, and history; target wins on conflict | 3 |
| Inbound links rewritten so nothing points at a removed note | 3 |
| Reachable only behind a confirmation | 5 |
| Increment 6 (continuity, threads, recap threads) | separate plan, stated in Scope |

**Placeholder scan:** no "TBD", no "similar to Task N". Task 3 gives the history union inline because the engine's helper is unexported and the service is a different package.

**Type consistency:** `entity.Aliases`, `EntitySummary.Aliases`, `aliasMatches`, `(*Store).DeleteEntity`, `(*Service).MergeEntities`, and `MergeEntityRequestDTO` are each defined once and used with the same signatures.

**Known gap, stated rather than hidden:** aliases only help once someone records them. Nothing infers that "The Ember Warden" is "Guard Kael"; the merge is how a player says so, and the prompt's continuity instruction is what keeps the problem rare in the first place. A future improvement would be suggesting a merge when two entities share a location and a role, but that is a suggestion engine with its own false positives, and it is not in scope here.
