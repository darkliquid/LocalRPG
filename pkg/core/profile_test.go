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

func TestProfileOpposedFields(t *testing.T) {
	p := ResolutionProfile{Opposed: "might", Ties: TieOpponent}
	if p.Opposed != "might" || p.Ties != TieOpponent {
		t.Fatalf("profile = %+v", p)
	}
}

func TestValidateRejectsAnUnknownTieRule(t *testing.T) {
	bad := CheckConventions{Profiles: map[string]ResolutionProfile{
		"grapple": {DC: 10, Ties: "coin flip"},
	}}
	problems := bad.Validate()
	if len(problems) == 0 {
		t.Fatal("an unknown tie rule should be rejected")
	}
}

func TestValidateAcceptsTheTieRules(t *testing.T) {
	ok := CheckConventions{Profiles: map[string]ResolutionProfile{
		"a": {DC: 10},
		"b": {DC: 10, Ties: TieActor},
		"c": {DC: 10, Ties: TieOpponent, Opposed: "might"},
	}}
	if problems := ok.Validate(); len(problems) != 0 {
		t.Fatalf("valid tie rules were rejected: %v", problems)
	}
}
