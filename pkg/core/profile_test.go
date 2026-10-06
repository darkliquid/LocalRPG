package core

import "testing"

func TestValidateProfiles(t *testing.T) {
	ok := CheckConventions{Profiles: map[string]ResolutionProfile{
		"pbta":   {Ladder: []LadderStep{{Min: 10, Outcome: "strong"}, {Min: 7, Outcome: "weak"}}},
		"d20":    {Notation: "1d20", DC: 15},
		"pool":   {SuccessOn: ">=8", Outcomes: []SuccessOutcome{{Min: 1, Max: -1, Outcome: "strong"}}},
		"blades": {Position: []string{"risky"}, Effect: []string{"standard"}, Ladder: []LadderStep{{Min: 7, Outcome: "weak"}}},
	}}
	if p := ok.Validate(); len(p) != 0 {
		t.Fatalf("valid profiles reported %v", p)
	}
	bad := CheckConventions{Profiles: map[string]ResolutionProfile{
		"empty-ladder": {DC: 0, Ladder: nil},
		"bad-pool":     {SuccessOn: ">=8"},
	}}
	if len(bad.Validate()) == 0 {
		t.Fatal("bad profiles should be rejected")
	}
}
