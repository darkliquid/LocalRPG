package worldgen

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type stubGen struct{ calls int }

func (s *stubGen) GenerateJSON(_ context.Context, prompt, schema string) ([]byte, error) {
	s.calls++
	return []byte(`{}`), nil
}

func TestGenerateRunsEveryStep(t *testing.T) {
	g := &stubGen{}
	var steps []Step
	_, err := Generate(context.Background(), g, Brief{Premise: "a drowned kingdom"}, func(s Step) {
		steps = append(steps, s)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 4 {
		t.Fatalf("steps = %d, want 4", len(steps))
	}
	if steps[0].Name != StepOutline || steps[3].Name != StepLink {
		t.Fatalf("steps = %+v", steps)
	}
	if g.calls != 4 {
		t.Fatalf("generator calls = %d, want 4", g.calls)
	}
}

func TestGenerateRepairsMalformedStep(t *testing.T) {
	g := &jsonGen{responses: []string{
		"```json\n{\"name\":\"Ashen Reach\",\"premise\":\"a dying frontier\",\n```",
	}}
	draft, err := Generate(context.Background(), g, Brief{Premise: "x"}, nil)
	if err != nil {
		t.Fatalf("a fenced reply should be repaired: %v", err)
	}
	if draft.World.Name != "Ashen Reach" {
		t.Fatalf("name = %q", draft.World.Name)
	}
}

func TestGenerateKeepsEarlierStepsOnFailure(t *testing.T) {
	g := &jsonGen{
		responses: []string{
			`{"name":"Ashen Reach","premise":"a dying frontier"}`,
			`{"locations":[{"name":"Saltmarch"}],"factions":[{"name":"The Tidewatch"}]}`,
		},
		failAt: 3,
	}
	var steps []Step
	draft, err := Generate(context.Background(), g, Brief{Premise: "x"}, func(s Step) {
		steps = append(steps, s)
	})
	if err == nil {
		t.Fatal("a failing step must be reported")
	}
	if len(draft.Entities) != 2 {
		t.Fatalf("earlier steps should survive: entities = %d", len(draft.Entities))
	}
	last := steps[len(steps)-1]
	if last.Name != StepCharacters || last.Status != StatusError {
		t.Fatalf("last step = %+v", last)
	}
}

func TestGenerateCancelledAborts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Generate(ctx, &stubGen{}, Brief{Premise: "x"}, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestGenerateWithoutAGeneratorErrors(t *testing.T) {
	if _, err := Generate(context.Background(), nil, Brief{Premise: "x"}, nil); err == nil {
		t.Fatal("a nil generator must error")
	}
}

func TestBudgetGeneratorRefusesPastTheCap(t *testing.T) {
	budget := &BudgetGenerator{Inner: &stubGen{}, Max: 2}
	for i := 0; i < 2; i++ {
		if _, err := budget.GenerateJSON(context.Background(), "", ""); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	_, err := budget.GenerateJSON(context.Background(), "", "")
	if !errors.Is(err, ErrCallBudgetExceeded) {
		t.Fatalf("err = %v, want ErrCallBudgetExceeded", err)
	}
	if !strings.Contains(err.Error(), "max_calls") {
		t.Fatalf("the cap should be named: %v", err)
	}
}
