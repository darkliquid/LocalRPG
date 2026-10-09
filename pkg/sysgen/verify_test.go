package sysgen

import (
	"context"
	"testing"
)

func TestVerificationCatchesABrokenScript(t *testing.T) {
	g := &jsonGen{responses: []string{
		`{"resolution":"dc","health":"none","advancement":"none"}`,
		`{"stats":[{"id":"might"}],"dc":10,"notation":"1d20"}`,
		`{"hooks":[{"raw":"onAction(\"do\", ("}]}`,
		`{"rules":"x"}`,
	}}
	s, err := Generate(context.Background(), g, Brief{Description: "a spend economy"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Verify.OK {
		t.Fatal("a broken script should fail verification")
	}
	if len(s.Verify.Failures) == 0 {
		t.Fatal("expected failure details in s.Verify.Failures")
	}
}

func TestVerificationPassesAGoodSystem(t *testing.T) {
	g := &jsonGen{responses: []string{
		`{"resolution":"ladder","health":"none","advancement":"none"}`,
		`{"stats":[{"id":"might"}],"notation":"2d6","ladder":[{"min":10,"outcome":"strong"},{"min":7,"outcome":"weak"},{"min":0,"outcome":"miss"}]}`,
		`{"rules":"Roll 2d6."}`,
	}}
	s, err := Generate(context.Background(), g, Brief{})
	if err != nil {
		t.Fatal(err)
	}
	if !s.Verify.OK {
		t.Fatalf("expected verification to pass, got failures: %v", s.Verify.Failures)
	}
}
