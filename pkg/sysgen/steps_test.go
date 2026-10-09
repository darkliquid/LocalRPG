package sysgen

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

type jsonGen struct {
	responses []string
	idx       int
	prompts   []string
}

func (j *jsonGen) GenerateJSON(ctx context.Context, prompt, schema string) ([]byte, error) {
	j.prompts = append(j.prompts, prompt)
	if j.idx >= len(j.responses) {
		return nil, fmt.Errorf("no more responses (call %d)", j.idx)
	}
	resp := j.responses[j.idx]
	j.idx++
	return []byte(resp), nil
}

func TestSchemaStepParsesMechanics(t *testing.T) {
	g := &jsonGen{responses: []string{
		`{"resolution":"ladder","health":"none","advancement":"none"}`,
		`{"stats":[{"id":"might"}],"notation":"2d6","ladder":[{"min":10,"outcome":"strong"},{"min":7,"outcome":"weak"},{"min":0,"outcome":"miss"}]}`,
		`{"rules":"2d6 rules"}`,
	}}
	s, err := Generate(context.Background(), g, Brief{Description: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Mechanics == nil || len(s.Mechanics.Checks.Profiles) != 1 {
		t.Fatalf("mechanics = %+v", s.Mechanics)
	}
}

func TestHooksStepIsSkippedWhenUnneeded(t *testing.T) {
	g := &jsonGen{responses: []string{
		`{"resolution":"dc","health":"none","advancement":"none"}`,
		`{"stats":[],"dc":10,"notation":"1d20"}`,
		`{"rules":"Roll 1d20."}`,
	}}
	s, err := Generate(context.Background(), g, Brief{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(s.Script) != "" {
		t.Fatalf("expected no script, got %q", s.Script)
	}
	if s.RulesPrompt == "" {
		t.Fatal("rules should be generated")
	}
	if g.idx != 3 {
		t.Fatalf("expected 3 model calls when no hatch is requested, got %d", g.idx)
	}
}

func TestHooksStepAssemblesScript(t *testing.T) {
	g := &jsonGen{responses: []string{
		`{"resolution":"dc","health":"none","advancement":"none"}`,
		`{"stats":[],"dc":10,"notation":"1d20"}`,
		`{"hooks":[{"event":"action","name":"heal","code":"state.hp += 5;"}]}`,
		`{"rules":"Roll 1d20."}`,
	}}
	s, err := Generate(context.Background(), g, Brief{Description: "a spend economy"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s.Script, "heal") {
		t.Fatalf("expected script to contain heal hook, got: %q", s.Script)
	}
	if len(s.Notes) == 0 {
		t.Fatal("a generated script should be recorded as a note")
	}
}

func TestFillStepRejectsAnUnbuildableTemplate(t *testing.T) {
	g := &jsonGen{responses: []string{
		`{"resolution":"ladder","health":"none","advancement":"none"}`,
		`{"stats":[]}`, // no ladder: the template cannot build
	}}
	_, err := Generate(context.Background(), g, Brief{Description: "x"})
	if err == nil {
		t.Fatal("expected a build error for a ladder template with no ladder")
	}
}

func TestChooseTemplateMapsADescription(t *testing.T) {
	g := &jsonGen{responses: []string{`{"resolution":"ladder","health":"single","advancement":"none","reason":"PbtA"}`}}
	tpl, choice, err := chooseTemplate(context.Background(), g, Brief{Description: "PbtA with three stats"})
	if err != nil {
		t.Fatal(err)
	}
	if tpl.Resolution != "ladder" || choice.Reason == "" {
		t.Fatalf("template %+v choice %+v", tpl, choice)
	}
}

func TestChooseTemplateRejectsAnUnknownResolution(t *testing.T) {
	g := &jsonGen{responses: []string{`{"resolution":"tarot"}`}}
	if _, _, err := chooseTemplate(context.Background(), g, Brief{}); err == nil {
		t.Fatal("an unknown resolution should not match a template")
	}
}

func TestFillParamsProducesABuiltSpec(t *testing.T) {
	g := &jsonGen{responses: []string{`{"stats":[{"id":"might"}],"ladder":[{"min":7,"outcome":"weak"}],"health_stat":"might"}`}}
	tpl := Template{Resolution: "ladder", Health: "single"}
	params, err := fillParams(context.Background(), g, tpl, Brief{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tpl.Build(params); err != nil {
		t.Fatal(err)
	}
}

func TestRequestedHatches(t *testing.T) {
	if got := requestedHatches(Brief{Description: "a spend economy"}); len(got) == 0 {
		t.Fatal("a spend economy should request the resource_spend hatch")
	}
	if got := requestedHatches(Brief{Description: "plain d20"}); len(got) != 0 {
		t.Fatal("a plain d20 should request no hatch")
	}
	if got := requestedHatches(Brief{Description: "characters die at zero"}); len(got) != 1 || got[0] != "custom_on_health_zero" {
		t.Fatalf("hatches = %+v", got)
	}
}

func TestEstimateCallsCountsAHatch(t *testing.T) {
	if got := EstimateCalls(Brief{Description: "plain d20"}); got != 3 {
		t.Fatalf("calls = %d, want 3", got)
	}
	if got := EstimateCalls(Brief{Description: "a spend economy"}); got != 4 {
		t.Fatalf("calls = %d, want 4", got)
	}
}

func TestAssembleScript(t *testing.T) {
	hooks := []hookEntry{
		{Raw: "// raw js comment"},
		{Event: "action", Name: "strike", Code: "return { outcome: 'hit' };"},
		{Event: "turn_begin", Code: "state.buff = false;"},
		{Event: "turn_end", Code: "state.tick = true;"},
		{Event: "world_tick", Code: "state.world = true;"},
		{Event: "check", Name: "custom", Code: "return { outcome: 'pass' };"},
		{Event: "health_zero", Code: "return 'dead';"},
		{Event: "unknown", Code: "console.log('fallback');"},
	}
	script := assembleScript(hooks)
	expectedSubstrings := []string{
		"// raw js comment",
		`onAction("strike", function(ctx) {`,
		`onTurnBegin(function(ctx) {`,
		`onTurnEnd(function(ctx) {`,
		`onWorldTick(function(ctx) {`,
		`onCheck("custom", function(req) {`,
		`onHealthZero(function(effect) {`,
		`console.log('fallback');`,
	}
	for _, substr := range expectedSubstrings {
		if !strings.Contains(script, substr) {
			t.Errorf("expected script to contain %q, got:\n%s", substr, script)
		}
	}
}
