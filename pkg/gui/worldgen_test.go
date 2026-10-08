package gui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/trace"
	"github.com/darkliquid/localrpg/pkg/worldgen"
)

// sequencedProvider returns scripted replies in order, so a test can drive the
// multi-step pipeline. Once the script is exhausted it answers with an empty
// object, which every step tolerates.
type sequencedProvider struct {
	id        string
	usage     *harness.Usage
	responses []string

	mu         sync.Mutex
	calls      int
	lastPrompt string
}

func (p *sequencedProvider) ID() string { return p.id }

func (p *sequencedProvider) Generate(_ context.Context, req harness.GenerateRequest) (*harness.GenerateResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	p.lastPrompt = req.Prompt
	text := "{}"
	if p.calls <= len(p.responses) {
		text = p.responses[p.calls-1]
	}
	return &harness.GenerateResponse{Text: text, Usage: p.usage}, nil
}

func (p *sequencedProvider) Stream(context.Context, harness.GenerateRequest, chan<- harness.StreamChunk) error {
	return nil
}

func (p *sequencedProvider) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

// worldGenService returns a service whose generation runs against a scripted
// provider. The gm role is made non-placeholder so the pipeline resolves it
// rather than falling back to the deterministic oracle.
func worldGenService(t *testing.T, provider *sequencedProvider) *Service {
	t.Helper()
	svc := NewService(t.TempDir())

	cfg := svc.configMgr.Get()
	gm := cfg.Agents.Roles[config.RoleGM]
	gm.Type = "http"
	gm.Command = ""
	gm.BuiltinName = ""
	gm.Endpoint = "http://127.0.0.1:9/v1"
	cfg.Agents.Roles[config.RoleGM] = gm

	original := textRouterFactory
	textRouterFactory = func(*config.Config, trace.Logger) (*harness.Router, error) {
		router := harness.NewRouter()
		router.RegisterProvider(provider)
		router.AssignRole(config.RoleGM, provider.id)
		return router, nil
	}
	t.Cleanup(func() { textRouterFactory = original })
	return svc
}

// collectEvents returns an emitter that records every event.
func collectEvents(events *[]TurnEvent) func(TurnEvent) error {
	return func(event TurnEvent) error {
		*events = append(*events, event)
		return nil
	}
}

const fullWorldScript = `{"name":"Ashen Reach","genre":"dark fantasy","premise":"a dying frontier"}`

func TestGenerateWorldStreamsStepsAndDraft(t *testing.T) {
	provider := &sequencedProvider{id: "gen", responses: []string{
		fullWorldScript,
		`{"locations":[{"name":"Saltmarch"}],"factions":[{"name":"The Tidewatch"}]}`,
		`{"characters":[{"name":"Maren Vale"}]}`,
		`{"entities":[{"id":"saltmarch","body":"A port watched by [[Maren Vale]]."}]}`,
	}}
	svc := worldGenService(t, provider)

	var events []TurnEvent
	draft, err := svc.GenerateWorld(context.Background(), WorldGenerateRequestDTO{
		Premise: "a drowned kingdom",
		Counts:  CountsDTO{Locations: 1, Factions: 1, Characters: 1},
	}, collectEvents(&events))
	if err != nil {
		t.Fatal(err)
	}
	if draft == nil || draft.Name != "Ashen Reach" {
		t.Fatalf("draft = %+v", draft)
	}
	if len(draft.Entities) != 3 {
		t.Fatalf("entities = %+v", draft.Entities)
	}
	if !containsStrings(draft.Entities[0].Body, "[[Maren Vale]]") {
		t.Fatalf("body = %q", draft.Entities[0].Body)
	}

	steps, drafts := 0, 0
	for _, event := range events {
		switch event.Type {
		case WorldEventStep:
			steps++
		case WorldEventDraft:
			drafts++
		}
	}
	if steps != 4 || drafts != 1 {
		t.Fatalf("events = %+v (steps %d drafts %d)", events, steps, drafts)
	}

	// The draft persists for review, and no live world is written.
	if _, err := os.Stat(filepath.Join(svc.worldDraftsDir(), "ashen-reach.yaml")); err != nil {
		t.Fatalf("draft not persisted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(svc.resolver.WorldsDir(), "ashen-reach")); !os.IsNotExist(err) {
		t.Fatal("generation must not write a live world")
	}
}

func TestGenerateWorldDryRunMakesNoCall(t *testing.T) {
	provider := &sequencedProvider{id: "gen"}
	svc := worldGenService(t, provider)

	var events []TurnEvent
	draft, err := svc.GenerateWorld(context.Background(), WorldGenerateRequestDTO{
		Premise: "x", DryRun: true,
	}, collectEvents(&events))
	if err != nil {
		t.Fatal(err)
	}
	if draft != nil {
		t.Fatalf("a dry run must not produce a draft: %+v", draft)
	}
	if provider.count() != 0 {
		t.Fatalf("a dry run made %d call(s)", provider.count())
	}
	if len(events) != 1 || events[0].Type != WorldEventEstimate || events[0].Estimate == nil {
		t.Fatalf("events = %+v", events)
	}
	if events[0].Estimate.Calls != 4 {
		t.Fatalf("estimate = %+v", events[0].Estimate)
	}
}

func TestGenerateWorldCapAborts(t *testing.T) {
	provider := &sequencedProvider{id: "gen", responses: []string{fullWorldScript}}
	svc := worldGenService(t, provider)
	svc.configMgr.Get().Generation.MaxCalls = 2

	var events []TurnEvent
	draft, err := svc.GenerateWorld(context.Background(), WorldGenerateRequestDTO{Premise: "x"}, collectEvents(&events))
	if err == nil || !strings.Contains(err.Error(), "max_calls") {
		t.Fatalf("err = %v, want the cap named", err)
	}
	if draft != nil {
		t.Fatalf("a capped generation must be discarded: %+v", draft)
	}
	if _, statErr := os.Stat(filepath.Join(svc.worldDraftsDir(), "ashen-reach.yaml")); !os.IsNotExist(statErr) {
		t.Fatal("a capped generation must not persist a draft")
	}
}

func TestGenerateWorldRecordsUsage(t *testing.T) {
	usage := &harness.Usage{Provider: "llm:gemini", Model: "gemini-3.8-flash", InputTokens: 120, OutputTokens: 60}
	provider := &sequencedProvider{id: "gen", usage: usage, responses: []string{fullWorldScript}}
	svc := worldGenService(t, provider)

	if _, err := svc.GenerateWorld(context.Background(), WorldGenerateRequestDTO{Premise: "x"}, func(TurnEvent) error { return nil }); err != nil {
		t.Fatal(err)
	}

	ledger, err := svc.usageLedger()
	if err != nil {
		t.Fatal(err)
	}
	rows, err := ledger.UsageByGame(UsageScopeGlobal)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, row := range rows {
		if row.Role == generatorRole {
			found = true
			if row.InputTokens == 0 && row.OutputTokens == 0 {
				t.Fatalf("the generation's tokens were not recorded: %+v", row)
			}
		}
	}
	if !found {
		t.Fatalf("no %s usage row: %+v", generatorRole, rows)
	}
}

func TestGenerateWorldFallsBackToTheOracle(t *testing.T) {
	// A default service's gm is the shipped echo command, which cannot generate.
	svc := NewService(t.TempDir())
	var events []TurnEvent
	draft, err := svc.GenerateWorld(context.Background(), WorldGenerateRequestDTO{
		Premise: "a drowned kingdom",
	}, collectEvents(&events))
	if err != nil {
		t.Fatal(err)
	}
	if draft == nil || !draft.Oracle {
		t.Fatalf("draft = %+v, want the oracle fallback", draft)
	}
	if draft.Name == "" || len(draft.Entities) == 0 {
		t.Fatalf("the oracle should still produce a world: %+v", draft)
	}
}

func TestGenerateWorldCancelledEmitsNoDraft(t *testing.T) {
	provider := &sequencedProvider{id: "gen", responses: []string{fullWorldScript}}
	svc := worldGenService(t, provider)

	ctx, cancel := context.WithCancel(context.Background())
	var events []TurnEvent
	_, err := svc.GenerateWorld(ctx, WorldGenerateRequestDTO{Premise: "x"}, func(event TurnEvent) error {
		events = append(events, event)
		// The client goes away after the first step.
		cancel()
		return errors.New("client disconnected")
	})
	if err == nil {
		t.Fatal("a cancelled generation should report the emit failure")
	}
	if _, statErr := os.Stat(filepath.Join(svc.worldDraftsDir(), "ashen-reach.yaml")); !os.IsNotExist(statErr) {
		t.Fatal("a cancelled generation must not persist a draft")
	}
}

func TestPreviewWorldEntitiesWritesNothing(t *testing.T) {
	provider := &sequencedProvider{id: "gen", responses: []string{
		`{"entities":[{"name":"The Tidewatch","type":"faction"}]}`,
	}}
	svc := worldGenService(t, provider)
	world := mustCreateWorld(t, svc, "Ember Peak")

	batch, err := svc.PreviewWorldEntities(context.Background(), world.ID, WorldEntityBatchRequestDTO{
		Instruction: "add a faction", Kinds: []string{"faction"}, Count: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Entities) != 1 || batch.Entities[0].ID != "the-tidewatch" {
		t.Fatalf("batch = %+v", batch)
	}
	if entities, err := os.ReadDir(filepath.Join(svc.resolver.WorldDir(world.ID), "entities")); err == nil && len(entities) != 0 {
		t.Fatalf("a preview must write nothing: %+v", entities)
	}
}

func TestEntityAcceptWritesAndIndexes(t *testing.T) {
	provider := &sequencedProvider{id: "gen", responses: []string{
		`{"entities":[{"name":"The Tidewatch","type":"faction"}]}`,
	}}
	svc := worldGenService(t, provider)
	world := mustCreateWorld(t, svc, "Ember Peak")

	batch, err := svc.PreviewWorldEntities(context.Background(), world.ID, WorldEntityBatchRequestDTO{Instruction: "x", Count: 1})
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.AcceptWorldEntities(context.Background(), world.ID, WorldEntityAcceptRequestDTO{Entities: batch.Entities})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Written) != 1 || result.Written[0] != "the-tidewatch" {
		t.Fatalf("result = %+v", result)
	}

	detail, err := svc.GetWorld(context.Background(), world.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Entities) != 1 || detail.Entities[0].ID != "the-tidewatch" {
		t.Fatalf("world entities = %+v", detail.Entities)
	}

	// The note parses, so it is a real entity, not a stray file.
	note, err := os.ReadFile(filepath.Join(svc.resolver.WorldDir(world.ID), "entities", "the-tidewatch.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !containsStrings(string(note), "type: faction", "The Tidewatch") {
		t.Fatalf("note = %q", note)
	}
}

func TestEntityAcceptRefusesAClashAndRenamesOnRequest(t *testing.T) {
	provider := &sequencedProvider{id: "gen", responses: []string{
		`{"entities":[{"name":"The Tidewatch","type":"faction"}]}`,
	}}
	svc := worldGenService(t, provider)
	world := mustCreateWorld(t, svc, "Ember Peak")

	batch, err := svc.PreviewWorldEntities(context.Background(), world.ID, WorldEntityBatchRequestDTO{Instruction: "x", Count: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AcceptWorldEntities(context.Background(), world.ID, WorldEntityAcceptRequestDTO{Entities: batch.Entities}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AcceptWorldEntities(context.Background(), world.ID, WorldEntityAcceptRequestDTO{Entities: batch.Entities}); !errors.Is(err, ErrWorldEntityExists) {
		t.Fatalf("err = %v, want ErrWorldEntityExists", err)
	}

	renamed, err := svc.AcceptWorldEntities(context.Background(), world.ID, WorldEntityAcceptRequestDTO{
		Entities: batch.Entities, Rename: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(renamed.Renamed) != 1 || renamed.Renamed[0] != "the-tidewatch-2" {
		t.Fatalf("renamed = %+v", renamed)
	}
}

func TestAcceptWorldEntitiesRefusesAnEmptyBatch(t *testing.T) {
	svc := NewService(t.TempDir())
	world := mustCreateWorld(t, svc, "Ember Peak")
	if _, err := svc.AcceptWorldEntities(context.Background(), world.ID, WorldEntityAcceptRequestDTO{}); err == nil {
		t.Fatal("an empty batch must be refused")
	}
}

func TestPreviewWorldEntitiesFromAFolder(t *testing.T) {
	provider := &sequencedProvider{id: "gen", responses: []string{
		`{"entities":[{"name":"Saltmarch","type":"location","description":"A port watched by [[The Tidewatch]]."}]}`,
	}}
	svc := worldGenService(t, provider)
	world := mustCreateWorld(t, svc, "Ember Peak")

	// An existing entity the extraction should link to rather than repeat.
	if _, err := svc.AcceptWorldEntities(context.Background(), world.ID, WorldEntityAcceptRequestDTO{
		Entities: []WorldDraftEntityDTO{
			{ID: "the-tidewatch", Name: "The Tidewatch", Type: "faction", Body: "A crew."},
		},
	}); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("# Saltmarch\n\nA port.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	batch, err := svc.PreviewWorldEntities(context.Background(), world.ID, WorldEntityBatchRequestDTO{
		Source: &WorldSourceDTO{Kind: "folder", Path: dir},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Entities) != 1 {
		t.Fatalf("batch = %+v", batch)
	}
	if batch.Entities[0].Source != "a.md" {
		t.Fatalf("source = %q, want a.md", batch.Entities[0].Source)
	}
	if !containsStrings(batch.Entities[0].Body, "[[The Tidewatch]]") {
		t.Fatalf("a link to the world's own entity should survive: %q", batch.Entities[0].Body)
	}

	// The extraction is told which world it is joining.
	if !containsStrings(provider.lastPrompt, "Ember Peak", "The Tidewatch", "Saltmarch") {
		t.Fatalf("prompt = %q", provider.lastPrompt)
	}

	// The preview writes nothing beyond the entity that was already accepted.
	detail, err := svc.GetWorld(context.Background(), world.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Entities) != 1 {
		t.Fatalf("a preview must write nothing: %+v", detail.Entities)
	}
}

func TestPreviewWorldEntitiesFromAFolderNeedsAReadableSource(t *testing.T) {
	svc := NewService(t.TempDir())
	world := mustCreateWorld(t, svc, "Ember Peak")

	if _, err := svc.PreviewWorldEntities(context.Background(), world.ID, WorldEntityBatchRequestDTO{
		Source: &WorldSourceDTO{Kind: "folder", Path: t.TempDir()},
	}); err == nil {
		t.Fatal("an empty folder must error")
	}
}

func TestEnhanceWorldWritesNothing(t *testing.T) {
	provider := &sequencedProvider{id: "gen", responses: []string{
		`{"proposals":[{"kind":"lore","title":"History","body":"Long ago.","reason":"fills a gap"}]}`,
	}}
	svc := worldGenService(t, provider)
	world := mustCreateWorld(t, svc, "Ember Peak")

	before := worldDirSnapshot(t, svc, world.ID)
	resp, err := svc.EnhanceWorld(context.Background(), world.ID, WorldEnhanceRequestDTO{Instruction: "deepen it"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Proposals) != 1 || resp.Proposals[0].Kind != worldgen.KindLore {
		t.Fatalf("proposals = %+v", resp.Proposals)
	}
	if after := worldDirSnapshot(t, svc, world.ID); after != before {
		t.Fatal("enhance must write nothing")
	}
}

func TestEnhanceApplyWritesOnlyAccepted(t *testing.T) {
	svc := NewService(t.TempDir())
	world := mustCreateWorld(t, svc, "Ember Peak")

	result, err := svc.ApplyWorldEnhancements(context.Background(), world.ID, WorldEnhanceApplyRequestDTO{
		Proposals: []WorldEnhancementDTO{
			{Kind: worldgen.KindLore, Title: "History", Body: "Long ago."},
			{Kind: worldgen.KindHook, Title: "The debt", Body: "A creditor arrives."},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Written) != 1 {
		t.Fatalf("result = %+v", result)
	}

	lore, err := os.ReadFile(filepath.Join(svc.resolver.WorldDir(world.ID), "prompts", "lore.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !containsStrings(string(lore), "## History", "Long ago.", "## Hooks", "The debt") {
		t.Fatalf("lore = %q", lore)
	}
}

func TestApplyWorldEnhancementsWithNoProposalsIsANoOp(t *testing.T) {
	svc := NewService(t.TempDir())
	world := mustCreateWorld(t, svc, "Ember Peak")
	before := worldDirSnapshot(t, svc, world.ID)

	result, err := svc.ApplyWorldEnhancements(context.Background(), world.ID, WorldEnhanceApplyRequestDTO{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Written) != 0 {
		t.Fatalf("result = %+v", result)
	}
	if after := worldDirSnapshot(t, svc, world.ID); after != before {
		t.Fatal("applying no proposals must leave every file unchanged")
	}
}

func TestCommitDraftWritesOnlyAccepted(t *testing.T) {
	svc := NewService(t.TempDir())
	saveDraftForTest(t, svc, worldgen.Draft{
		ID:    "ashen-reach",
		World: coreWorld("Ashen Reach"),
		Lore:  "# Lore\n\nA dying frontier.\n",
		Entities: []worldgen.DraftEntity{
			{ID: "saltmarch", Name: "Saltmarch", Type: "location", Body: "A port."},
			{ID: "rejected", Name: "Rejected", Type: "concept", Body: "Should not be written."},
		},
	})

	world, err := svc.CommitDraft(context.Background(), DraftCommitRequestDTO{
		DraftID: "ashen-reach",
		Entities: []WorldDraftEntityDTO{
			{ID: "saltmarch", Name: "Saltmarch", Type: "location", Body: "A port."},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if world.ID != "ashen-reach" || world.Name != "Ashen Reach" {
		t.Fatalf("world = %+v", world)
	}

	detail, err := svc.GetWorld(context.Background(), world.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Entities) != 1 || detail.Entities[0].ID != "saltmarch" {
		t.Fatalf("entities = %+v", detail.Entities)
	}
	if _, err := os.Stat(filepath.Join(svc.worldDraftsDir(), "ashen-reach.yaml")); !os.IsNotExist(err) {
		t.Fatal("committing should delete the draft")
	}
}

func TestCommitDraftRefusesAnEmptySelection(t *testing.T) {
	svc := NewService(t.TempDir())
	saveDraftForTest(t, svc, worldgen.Draft{ID: "empty", World: coreWorld("Empty")})

	_, err := svc.CommitDraft(context.Background(), DraftCommitRequestDTO{
		DraftID:  "empty",
		Sections: []WorldDraftSectionDTO{},
		Entities: []WorldDraftEntityDTO{},
	})
	if !errors.Is(err, ErrEmptyDraftSelection) {
		t.Fatalf("err = %v, want ErrEmptyDraftSelection", err)
	}
}

func TestCommitDraftRefusesAnExistingWorldID(t *testing.T) {
	svc := NewService(t.TempDir())
	mustCreateWorld(t, svc, "Ember Peak")
	saveDraftForTest(t, svc, worldgen.Draft{
		ID:       "ember-peak",
		World:    coreWorld("Ember Peak"),
		Entities: []worldgen.DraftEntity{{ID: "saltmarch", Name: "Saltmarch", Type: "location"}},
	})

	_, err := svc.CommitDraft(context.Background(), DraftCommitRequestDTO{DraftID: "ember-peak"})
	if !errors.Is(err, ErrWorldExists) {
		t.Fatalf("err = %v, want ErrWorldExists", err)
	}
}

func TestCommitDraftIsAtomic(t *testing.T) {
	svc := NewService(t.TempDir())
	saveDraftForTest(t, svc, worldgen.Draft{
		ID:       "bad-ids",
		World:    coreWorld("Bad Ids"),
		Entities: []worldgen.DraftEntity{{ID: "../escape", Name: "Escape", Type: "concept"}},
	})

	if _, err := svc.CommitDraft(context.Background(), DraftCommitRequestDTO{DraftID: "bad-ids"}); err == nil {
		t.Fatal("an invalid entity id must fail the commit")
	}
	if _, err := os.Stat(filepath.Join(svc.resolver.WorldsDir(), "bad-ids")); !os.IsNotExist(err) {
		t.Fatal("a failed commit must leave no world directory")
	}
	entries, err := os.ReadDir(svc.resolver.WorldsDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".commit-") {
			t.Fatalf("a failed commit left a staging directory: %s", entry.Name())
		}
	}
}

func TestCommitIntoExistingWorldAppends(t *testing.T) {
	svc := NewService(t.TempDir())
	world := mustCreateWorld(t, svc, "Ember Peak")
	lorePath := filepath.Join(svc.resolver.WorldDir(world.ID), "prompts", "lore.md")
	if err := os.MkdirAll(filepath.Dir(lorePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lorePath, []byte("# Lore\n\nExisting.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	saveDraftForTest(t, svc, worldgen.Draft{
		ID:       "merge",
		World:    coreWorld("Merged"),
		Sections: []worldgen.DraftSection{{Title: "History", Body: "Added."}},
		Entities: []worldgen.DraftEntity{{ID: "saltmarch", Name: "Saltmarch", Type: "location", Body: "A port."}},
	})

	summary, err := svc.CommitDraft(context.Background(), DraftCommitRequestDTO{
		DraftID:       "merge",
		TargetWorldID: world.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.ID != world.ID {
		t.Fatalf("summary = %+v", summary)
	}

	lore, err := os.ReadFile(lorePath)
	if err != nil {
		t.Fatal(err)
	}
	if !containsStrings(string(lore), "Existing.", "## History", "Added.") {
		t.Fatalf("lore = %q", lore)
	}
	detail, err := svc.GetWorld(context.Background(), world.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Entities) != 1 {
		t.Fatalf("entities = %+v", detail.Entities)
	}
}

func TestCommitIntoExistingWorldRefusesAClash(t *testing.T) {
	svc := NewService(t.TempDir())
	world := mustCreateWorld(t, svc, "Ember Peak")
	if _, err := svc.AcceptWorldEntities(context.Background(), world.ID, WorldEntityAcceptRequestDTO{
		Entities: []WorldDraftEntityDTO{{ID: "saltmarch", Name: "Saltmarch", Type: "location", Body: "A port."}},
	}); err != nil {
		t.Fatal(err)
	}

	saveDraftForTest(t, svc, worldgen.Draft{
		ID:       "merge",
		World:    coreWorld("Merged"),
		Entities: []worldgen.DraftEntity{{ID: "saltmarch", Name: "Saltmarch", Type: "location", Body: "A different port."}},
	})

	_, err := svc.CommitDraft(context.Background(), DraftCommitRequestDTO{DraftID: "merge", TargetWorldID: world.ID})
	if !errors.Is(err, ErrWorldEntityExists) {
		t.Fatalf("err = %v, want ErrWorldEntityExists", err)
	}
}

func TestDraftReloadAndDiscard(t *testing.T) {
	svc := NewService(t.TempDir())
	saveDraftForTest(t, svc, worldgen.Draft{
		ID:       "ashen-reach",
		World:    coreWorld("Ashen Reach"),
		Lore:     "# Lore\n\nA.\n\n## History\n\nB.\n",
		Entities: []worldgen.DraftEntity{{ID: "saltmarch", Name: "Saltmarch", Type: "location"}},
	})

	draft, err := svc.GetDraft(context.Background(), "ashen-reach")
	if err != nil {
		t.Fatal(err)
	}
	if draft.Name != "Ashen Reach" || len(draft.Entities) != 1 {
		t.Fatalf("draft = %+v", draft)
	}
	if len(draft.Sections) != 2 {
		t.Fatalf("sections = %+v", draft.Sections)
	}

	ids, err := svc.ListDraftIDs(context.Background())
	if err != nil || len(ids) != 1 || ids[0] != "ashen-reach" {
		t.Fatalf("ids = %+v err %v", ids, err)
	}

	if err := svc.DiscardDraft(context.Background(), "ashen-reach"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetDraft(context.Background(), "ashen-reach"); err == nil {
		t.Fatal("a discarded draft should not load")
	}
}

func TestIngestWorldFromAFolder(t *testing.T) {
	provider := &sequencedProvider{id: "gen", responses: []string{
		`{"name":"Saltmarch Reach","description":"A wet frontier.","lore":"# Lore\n\nWet.\n",
		  "entities":[{"name":"Saltmarch","type":"location","description":"A port."}]}`,
	}}
	svc := worldGenService(t, provider)

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("# Saltmarch\n\nA port.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var events []TurnEvent
	draft, err := svc.GenerateWorld(context.Background(), WorldGenerateRequestDTO{
		Premise: "keep it nautical",
		Source:  &WorldSourceDTO{Kind: "folder", Path: dir},
	}, collectEvents(&events))
	if err != nil {
		t.Fatal(err)
	}
	if draft == nil || draft.Name != "Saltmarch Reach" || len(draft.Entities) != 1 {
		t.Fatalf("draft = %+v", draft)
	}
	if draft.Entities[0].Source != "a.md" {
		t.Fatalf("source = %q", draft.Entities[0].Source)
	}
	if _, err := os.Stat(filepath.Join(svc.resolver.WorldsDir(), "saltmarch-reach")); !os.IsNotExist(err) {
		t.Fatal("ingestion must not write a live world")
	}
}

func TestIngestRefusesAnEmptySource(t *testing.T) {
	svc := NewService(t.TempDir())
	if _, err := svc.GenerateWorld(context.Background(), WorldGenerateRequestDTO{
		Source: &WorldSourceDTO{Kind: "folder", Path: t.TempDir()},
	}, func(TurnEvent) error { return nil }); err == nil {
		t.Fatal("an empty folder must error")
	}
}

func TestResolveGeneratorRole(t *testing.T) {
	cases := []struct {
		name string
		cfg  *config.Config
		want string
	}{
		{"nil config", nil, ""},
		{"shipped echo default", config.DefaultConfig(), ""},
		{"explicit generator role", generatorConfig("generator", "http"), "generator"},
		{"generator inherits a real gm", generatorConfig("generator", "inherit"), "gm"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveGeneratorRole(tc.cfg); got != tc.want {
				t.Fatalf("role = %q, want %q", got, tc.want)
			}
		})
	}
}

// generatorConfig builds a config whose gm is a real provider, and whose
// generator role is either a real provider or an inheritance of gm.
func generatorConfig(kind, generatorType string) *config.Config {
	cfg := config.DefaultConfig()
	gm := cfg.Agents.Roles[config.RoleGM]
	gm.Type = "http"
	gm.Command = ""
	gm.Endpoint = "http://127.0.0.1:9/v1"
	cfg.Agents.Roles[config.RoleGM] = gm

	generator := config.AgentRoleConfig{Type: generatorType}
	if generatorType == "inherit" {
		generator.InheritFrom = config.RoleGM
	} else {
		generator.Endpoint = "http://127.0.0.1:9/v1"
	}
	cfg.Agents.Roles[kind] = generator
	return cfg
}

func mustCreateWorld(t *testing.T, svc *Service, name string) *WorldDetailDTO {
	t.Helper()
	world, err := svc.CreateWorld(context.Background(), CreateWorldRequestDTO{Name: name})
	if err != nil {
		t.Fatalf("CreateWorld: %v", err)
	}
	return world
}

func saveDraftForTest(t *testing.T, svc *Service, draft worldgen.Draft) {
	t.Helper()
	if err := worldgen.SaveDraft(svc.worldDraftsDir(), draft); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}
}

func coreWorld(name string) core.WorldManifest {
	return core.WorldManifest{ID: slugify(name), Name: name}
}

// worldDirSnapshot renders every file under a world directory, so a test can
// assert that an operation changed nothing.
func worldDirSnapshot(t *testing.T, svc *Service, worldID string) string {
	t.Helper()
	root := svc.resolver.WorldDir(worldID)
	var b strings.Builder
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		b.WriteString(rel)
		if !info.IsDir() {
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			b.WriteString(":" + string(data))
		}
		b.WriteString("\n")
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", root, err)
	}
	return b.String()
}

func containsStrings(haystack string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(haystack, needle) {
			return false
		}
	}
	return true
}

func TestSourceOverTheChunkLimitIsRefusedBeforeAnyCall(t *testing.T) {
	provider := &sequencedProvider{id: "gen", responses: []string{`{"entities":[{"name":"A","type":"faction"}]}`}}
	svc := worldGenService(t, provider)
	world := mustCreateWorld(t, svc, "Ember Peak")
	svc.configMgr.Get().Generation.MaxChunks = 1

	dir := t.TempDir()
	for _, name := range []string{"a.md", "b.md", "c.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("# "+name+"\n\nSome lore.\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	_, err := svc.PreviewWorldEntities(context.Background(), world.ID, WorldEntityBatchRequestDTO{
		Source: &WorldSourceDTO{Kind: "folder", Path: dir},
	})
	if !errors.Is(err, ErrSourceTooLarge) {
		t.Fatalf("err = %v, want ErrSourceTooLarge", err)
	}
	if !containsStrings(err.Error(), "max_chunks", "3 chunks") {
		t.Fatalf("the refusal should name the setting and the size: %v", err)
	}
	if provider.count() != 0 {
		t.Fatalf("a refused source made %d call(s)", provider.count())
	}
}

func TestIngestionIsBoundedByChunksNotTheCallCap(t *testing.T) {
	// One call per batch of four chunks, so 12 chunks are three calls. A call cap
	// of two must not stop an ingestion the chunk budget allows.
	provider := &sequencedProvider{id: "gen"}
	svc := worldGenService(t, provider)
	world := mustCreateWorld(t, svc, "Ember Peak")
	svc.configMgr.Get().Generation.MaxCalls = 2

	dir := t.TempDir()
	for i := 0; i < 12; i++ {
		name := filepath.Join(dir, fmt.Sprintf("note-%02d.md", i))
		if err := os.WriteFile(name, []byte("# Note\n\nSome lore.\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	batch, err := svc.PreviewWorldEntities(context.Background(), world.ID, WorldEntityBatchRequestDTO{
		Source: &WorldSourceDTO{Kind: "folder", Path: dir},
	})
	if err != nil {
		t.Fatalf("an ingestion within the chunk budget should not be capped by max_calls: %v", err)
	}
	if batch == nil {
		t.Fatal("expected a batch")
	}
	if provider.count() != 3 {
		t.Fatalf("calls = %d, want 3", provider.count())
	}
}

func TestGenerationLimitCodeNamesTheFailingSetting(t *testing.T) {
	if got := generationLimitCode(worldgen.ErrCallBudgetExceeded); got != ErrorCodeGenerationLimit {
		t.Fatalf("call budget code = %q", got)
	}
	if got := generationLimitCode(fmt.Errorf("wrapped: %w", ErrSourceTooLarge)); got != ErrorCodeGenerationLimit {
		t.Fatalf("source size code = %q", got)
	}
	if got := generationLimitCode(errors.New("something else")); got != "" {
		t.Fatalf("an unrelated failure got code %q", got)
	}
}
