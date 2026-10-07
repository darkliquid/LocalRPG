package main

import "testing"

func TestTestSystemCLIPassesReference(t *testing.T) {
	if code := runTestSystem(debugConfig{Reference: true, SystemID: "narrative_2d6"}); code != 0 {
		t.Fatalf("a reference system with passing scenarios should exit 0, got %d", code)
	}
}

func TestTestSystemCLIReportsUnknownReference(t *testing.T) {
	if code := runTestSystem(debugConfig{Reference: true, SystemID: "nope"}); code == 0 {
		t.Fatal("an unknown reference system should exit non-zero")
	}
}

func TestTestSystemCLIWithNoMatchingScenario(t *testing.T) {
	code := runTestSystem(debugConfig{Reference: true, SystemID: "narrative_2d6", Scenario: "___none___"})
	if code != 0 {
		t.Fatalf("no matching scenario should exit 0, got %d", code)
	}
}
