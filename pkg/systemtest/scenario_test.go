package systemtest

import "testing"

func TestLoadScenario(t *testing.T) {
	in := []byte("name: a test\nseed: 7\nsteps:\n  - action: do\n    expect:\n      outcome: weak\n")
	s, err := LoadScenario(in)
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "a test" || s.Seed != 7 || len(s.Steps) != 1 || s.Steps[0].Expect.Outcome != "weak" {
		t.Fatalf("scenario = %+v", s)
	}
}

func TestLoadScenarioRejectsEmpty(t *testing.T) {
	if _, err := LoadScenario([]byte("name: empty\n")); err == nil {
		t.Fatal("a scenario with no steps should be rejected")
	}
}
