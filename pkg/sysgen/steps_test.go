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
		`{"resolution":"ladder","advancement":false}`,
		`{"stats":[{"id":"sanity"}],"checks":{"notation":"2d6","outcome":["strong","weak","miss"],
		  "profiles":{"pbta":{"ladder":[{"min":10,"outcome":"strong"},{"min":7,"outcome":"weak"},{"min":0,"outcome":"miss"}]}}}}`,
		`{"hooks":[]}`,
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
	g := &jsonGen{responses: []string{`{}`, `{"stats":[]}`, `{"hooks":[]}`, `{"rules":"Roll 2d6."}`}}
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
}

func TestHooksStepAssemblesScript(t *testing.T) {
	g := &jsonGen{responses: []string{
		`{}`,
		`{"stats":[]}`,
		`{"hooks":[{"event":"action","name":"heal","code":"state.hp += 5;"}]}`,
		`{"rules":"Roll 2d6."}`,
	}}
	s, err := Generate(context.Background(), g, Brief{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s.Script, "heal") {
		t.Fatalf("expected script to contain heal hook, got: %q", s.Script)
	}
}

func TestSchemaStepRejectsInvalidProfiles(t *testing.T) {
	g := &jsonGen{responses: []string{
		`{"resolution":"ladder"}`,
		`{"checks":{"profiles":{"broken":{}}}}`, // No dc, ladder, or pool
	}}
	_, err := Generate(context.Background(), g, Brief{Description: "x"})
	if err == nil {
		t.Fatal("expected validation error for invalid profile, got nil")
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


