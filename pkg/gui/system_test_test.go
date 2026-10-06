package gui

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/systemtest"
)

func TestSystemTestEndpoint(t *testing.T) {
	_, svc := setupTestGame(t)
	resp, err := svc.TestSystem(context.Background(), SystemTestRequestDTO{
		System: SystemTestSystemDTO{ID: "t", Script: `onAction("do", () => ({ outcome: "weak" }))`},
		Scenarios: []systemtest.Scenario{
			{Name: "ok", Steps: []systemtest.Step{{Action: "do", Expect: systemtest.Expectations{Outcome: "weak"}}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Failures) != 0 {
		t.Fatalf("failures = %+v", resp.Failures)
	}
}

func TestSystemTestEndpointReportsFailures(t *testing.T) {
	_, svc := setupTestGame(t)
	resp, err := svc.TestSystem(context.Background(), SystemTestRequestDTO{
		System: SystemTestSystemDTO{ID: "t", Script: `onAction("do", () => ({ outcome: "weak" }))`},
		Scenarios: []systemtest.Scenario{
			{Name: "bad", Steps: []systemtest.Step{{Action: "do", Expect: systemtest.Expectations{Outcome: "strong"}}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Failures) == 0 {
		t.Fatal("expected a failure")
	}
}

func TestSystemScenariosReadsTheTestsDirectory(t *testing.T) {
	_, svc := setupTestGame(t)
	got, err := svc.SystemScenarios(context.Background(), "test_sys")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Scenarios) != 0 {
		t.Fatalf("a system with no tests dir should have no scenarios, got %+v", got.Scenarios)
	}
}
