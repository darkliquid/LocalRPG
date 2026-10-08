package sysgen

import (
	"context"
	"fmt"
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
	}}
	s, err := Generate(context.Background(), g, Brief{Description: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Mechanics == nil || len(s.Mechanics.Checks.Profiles) != 1 {
		t.Fatalf("mechanics = %+v", s.Mechanics)
	}
}
