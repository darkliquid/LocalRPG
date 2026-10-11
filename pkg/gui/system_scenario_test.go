package gui

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/systemtest"
)

func TestSaveAndDeleteSystemScenario(t *testing.T) {
	svc := NewService(t.TempDir())
	setupFreeformSystem(t, svc)

	scenario := systemtest.Scenario{
		Name: "standard hit",
		Steps: []systemtest.Step{{
			Action: "check",
			Input:  "standard",
			Expect: systemtest.Expectations{Outcome: "strong"},
		}},
	}
	if err := svc.SaveSystemScenario(context.Background(), "freeform", scenario); err != nil {
		t.Fatalf("SaveSystemScenario: %v", err)
	}

	resp, err := svc.SystemScenarios(context.Background(), "freeform")
	if err != nil {
		t.Fatalf("SystemScenarios: %v", err)
	}
	if len(resp.Scenarios) != 1 || resp.Scenarios[0].Name != "standard hit" {
		t.Fatalf("scenarios = %+v", resp.Scenarios)
	}

	if err := svc.SaveSystemScenario(context.Background(), "freeform", systemtest.Scenario{Name: "empty"}); err == nil {
		t.Fatal("a scenario with no steps should be refused")
	}

	if err := svc.DeleteSystemScenario(context.Background(), "freeform", "standard hit"); err != nil {
		t.Fatalf("DeleteSystemScenario: %v", err)
	}
	if err := svc.DeleteSystemScenario(context.Background(), "freeform", "standard hit"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("second delete err = %v, want fs.ErrNotExist", err)
	}
}

func TestSystemTestsRouteSavesAndDeletes(t *testing.T) {
	svc := NewService(t.TempDir())
	setupFreeformSystem(t, svc)
	srv := NewServer(svc, http.NotFoundHandler())

	body := strings.NewReader(`{"name":"route scenario","steps":[{"action":"check","input":"standard"}]}`)
	post := httptest.NewRequest(http.MethodPost, "/api/system/tests/freeform", body)
	post.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, post)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	del := httptest.NewRequest(http.MethodDelete, "/api/system/tests/freeform?name="+url.QueryEscape("route scenario"), nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, del)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE status = %d, want 204: %s", rec.Code, rec.Body.String())
	}
}