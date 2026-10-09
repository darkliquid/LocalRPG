package sysgen

import (
	"context"
	"strings"
	"testing"
)

type stubGen struct{}

func (stubGen) GenerateJSON(_ context.Context, prompt, schema string) ([]byte, error) {
	switch {
	case strings.Contains(schema, `"success_on"`):
		return []byte(`{"stats":[{"id":"sanity"}],"dc":12,"notation":"1d20","health_stat":"sanity"}`), nil
	case strings.Contains(schema, `"resolution"`):
		return []byte(`{"resolution":"dc","health":"single","advancement":"none","reason":"d20"}`), nil
	case strings.Contains(schema, `"hooks"`):
		return []byte(`{"hooks":[]}`), nil
	case strings.Contains(schema, `"rules"`):
		return []byte(`{"rules":"# Rules"}`), nil
	}
	return []byte(`{}`), nil
}

func TestGenerateReturnsASystem(t *testing.T) {
	s, err := Generate(context.Background(), stubGen{}, Brief{Name: "Gritty", Description: "gritty d20"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Name == "" || s.Mechanics == nil {
		t.Fatalf("system = %+v", s)
	}
	if len(s.Mechanics.Checks.Profiles) != 1 {
		t.Fatalf("expected one built profile, got %+v", s.Mechanics.Checks.Profiles)
	}
}

func TestGeneratedSystemWithNoHooksHasEmptyScript(t *testing.T) {
	s, err := Generate(context.Background(), stubGen{}, Brief{Name: "Clean System", Description: "no hooks"})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if s.Script != "" {
		t.Errorf("expected s.Script to be empty string, got %q", s.Script)
	}
	if len(s.Notes) != 0 {
		t.Errorf("expected no notes for a covered description, got %v", s.Notes)
	}
}

func TestGenerateUsesTemplates(t *testing.T) {
	g := &jsonGen{responses: []string{
		`{"resolution":"ladder","health":"none","advancement":"none"}`,
		`{"stats":[{"id":"might"}],"ladder":[{"min":7,"outcome":"weak"}]}`,
		`{"rules":"Roll 2d6."}`,
	}}
	s, err := Generate(context.Background(), g, Brief{Description: "PbtA"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Mechanics == nil || len(s.Mechanics.Checks.Profiles) != 1 {
		t.Fatalf("mechanics = %+v", s.Mechanics)
	}
	if _, ok := s.Mechanics.Checks.Profiles["ladder_none"]; !ok {
		t.Fatalf("expected the ladder_none template profile, got %+v", s.Mechanics.Checks.Profiles)
	}
}

func TestGenerateRecordsGapsAsNotes(t *testing.T) {
	g := &jsonGen{responses: []string{
		`{"resolution":"dc","health":"none","advancement":"none","gaps":"a bespoke initiative clock is not covered"}`,
		`{"stats":[],"dc":10,"notation":"1d20"}`,
		`{"rules":"Roll 1d20."}`,
	}}
	s, err := Generate(context.Background(), g, Brief{Description: "d20 with a bespoke initiative clock"})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Notes) != 1 || !strings.Contains(s.Notes[0], "initiative clock") {
		t.Fatalf("notes = %v", s.Notes)
	}
}
