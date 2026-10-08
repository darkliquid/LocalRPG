package gui

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// sysGenService returns a service whose generation runs against a scripted provider.
func sysGenService(t *testing.T, provider *sequencedProvider) *Service {
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

func TestGenerateSystemWritesNothing(t *testing.T) {
	provider := &sequencedProvider{id: "gen", responses: []string{
		`{"resolution":"d20","stats":["vigor","wit"],"skills":["brawl","lore"],"health":"points","advancement":true}`,
		`{"stats":[{"id":"vigor","label":"Vigor"},{"id":"wit","label":"Wit"}],"skills":[{"id":"brawl","label":"Brawl","stat":"vigor"},{"id":"lore","label":"Lore","stat":"wit"}],"health":{"type":"points","max":10,"current":10},"checks":{"notation":"1d20","outcome":["failure","success"],"profiles":{"check":{"notation":"1d20","dc":10}}}}`,
		`{"hooks":[]}`,
		`{"rules":"# Rules\n\nRoll 1d20."}`,
	}}
	svc := sysGenService(t, provider)

	var events []TurnEvent
	draft, err := svc.GenerateSystem(context.Background(), SystemGenerateRequestDTO{
		Name:        "Iron & Steam",
		Description: "A steampunk d20 tabletop system",
	}, collectEvents(&events))
	if err != nil {
		t.Fatalf("GenerateSystem failed: %v", err)
	}
	if draft == nil {
		t.Fatal("expected draft to be non-nil")
	}

	// Verify events include steps and draft
	stepNames := make(map[string]bool)
	hasDraftEvent := false
	for _, ev := range events {
		if ev.Type == WorldEventStep && ev.Step != nil {
			stepNames[ev.Step.Name] = true
		}
		if ev.Type == WorldEventDraft && ev.SystemDraft != nil {
			hasDraftEvent = true
		}
	}
	for _, expectedStep := range []string{"shape", "schema", "hooks", "rules", "verify"} {
		if !stepNames[expectedStep] {
			t.Errorf("expected step %q in events, got steps: %v", expectedStep, stepNames)
		}
	}
	if !hasDraftEvent {
		t.Error("expected draft event in streamed events")
	}

	// Verify nothing was written to systems/<id>/
	sysDir := svc.resolver.SystemDir(draft.ID)
	if _, err := os.Stat(filepath.Join(sysDir, "system.yaml")); err == nil {
		t.Fatalf("generating a system should NOT write system.yaml to %s", sysDir)
	}

	// Verify draft is stored in .drafts/
	draftFile := filepath.Join(svc.systemDraftsDir(), draft.ID+".yaml")
	if _, err := os.Stat(draftFile); err != nil {
		t.Fatalf("draft should be saved in %s: %v", draftFile, err)
	}
}

func TestGenerateSystemReportsVerification(t *testing.T) {
	provider := &sequencedProvider{id: "gen", responses: []string{
		`{"resolution":"d20","stats":["str"],"skills":[],"health":"points","advancement":false}`,
		`{"stats":[{"id":"str","label":"Strength"}],"checks":{"notation":"1d20","outcome":["failure","success"],"profiles":{"check":{"notation":"1d20","dc":10}}}}`,
		`{"hooks":[]}`,
		`{"rules":"# Rules"}`,
	}}
	svc := sysGenService(t, provider)

	var events []TurnEvent
	draft, err := svc.GenerateSystem(context.Background(), SystemGenerateRequestDTO{
		Name:        "Simple",
		Description: "Simple system",
	}, collectEvents(&events))
	if err != nil {
		t.Fatalf("GenerateSystem failed: %v", err)
	}

	if !draft.Verify.OK {
		t.Errorf("expected draft.Verify.OK to be true, got false (failures: %v)", draft.Verify.Failures)
	}
}

func TestCommitAndDiscardSystemDraft(t *testing.T) {
	provider := &sequencedProvider{id: "gen", responses: []string{
		`{"resolution":"d20","stats":["str"],"skills":[],"health":"points","advancement":false}`,
		`{"stats":[{"id":"str","label":"Strength"}],"checks":{"notation":"1d20","outcome":["failure","success"],"profiles":{"check":{"notation":"1d20","dc":10}}}}`,
		`{"hooks":[]}`,
		`{"rules":"# Rules\n\nRoll dice."}`,
	}}
	svc := sysGenService(t, provider)

	var events []TurnEvent
	draft, err := svc.GenerateSystem(context.Background(), SystemGenerateRequestDTO{
		Name:        "Commit Test",
		Description: "Testing commit",
	}, collectEvents(&events))
	if err != nil {
		t.Fatal(err)
	}

	// Verify it can be listed and retrieved
	ids, err := svc.ListSystemDraftIDs(context.Background())
	if err != nil || len(ids) != 1 || ids[0] != draft.ID {
		t.Fatalf("ListSystemDraftIDs = %v, err = %v", ids, err)
	}
	loaded, err := svc.GetSystemDraft(context.Background(), draft.ID)
	if err != nil || loaded.Name != "Commit Test" {
		t.Fatalf("GetSystemDraft = %+v, err = %v", loaded, err)
	}

	// Commit draft
	detail, err := svc.CommitSystemDraft(context.Background(), SystemDraftCommitRequestDTO{
		DraftID: draft.ID,
	})
	if err != nil {
		t.Fatalf("CommitSystemDraft failed: %v", err)
	}
	if detail.ID != draft.ID {
		t.Errorf("committed detail ID = %q, want %q", detail.ID, draft.ID)
	}

	// Verify systems/<id>/system.yaml exists now
	sysDir := svc.resolver.SystemDir(draft.ID)
	if _, err := os.Stat(filepath.Join(sysDir, "system.yaml")); err != nil {
		t.Fatalf("system.yaml should exist in %s after commit: %v", sysDir, err)
	}

	// Verify draft is deleted after commit
	if _, err := svc.GetSystemDraft(context.Background(), draft.ID); err == nil {
		t.Fatal("draft should be deleted after commit")
	}
}
