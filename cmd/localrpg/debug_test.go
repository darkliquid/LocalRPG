package main

import (
	"testing"
)

func TestParseDebugFlags(t *testing.T) {
	cmd, args, err := parseDebugArgs([]string{"test-run", "--scenario", "scenarios/smoke.yaml", "--headless=true"})
	if err != nil {
		t.Fatalf("parseDebugArgs failed: %v", err)
	}
	if cmd != "test-run" {
		t.Errorf("Expected command 'test-run', got %s", cmd)
	}
	if args.Scenario != "scenarios/smoke.yaml" || !args.Headless {
		t.Errorf("Unexpected args parsed: %+v", args)
	}
}
