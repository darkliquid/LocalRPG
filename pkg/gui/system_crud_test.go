package gui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func newSystemTestService(t *testing.T) *Service {
	t.Helper()
	return NewService(t.TempDir())
}

func TestSystemDeleteReturnsNoContent(t *testing.T) {
	svc := newSystemTestService(t)
	created, err := svc.SaveSystem(context.Background(), CreateSystemRequestDTO{
		ID:   "custom-rules",
		Name: "Custom Rules",
	})
	if err != nil {
		t.Fatalf("SaveSystem: %v", err)
	}
	server := NewServer(svc, http.NotFoundHandler())

	req := httptest.NewRequest(http.MethodDelete, "/api/system/"+created.ID, nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204: %s", rec.Code, rec.Body.String())
	}

	// Verify system is gone
	getReq := httptest.NewRequest(http.MethodGet, "/api/system/"+created.ID, nil)
	getRec := httptest.NewRecorder()
	server.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusNotFound {
		t.Fatalf("get after delete status = %d, want 404", getRec.Code)
	}
}

func TestSystemDeleteMissingReturnsNotFound(t *testing.T) {
	svc := newSystemTestService(t)
	server := NewServer(svc, http.NotFoundHandler())

	req := httptest.NewRequest(http.MethodDelete, "/api/system/nonexistent", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("delete missing status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

func TestSystemDeleteInUseByCampaignRefusesWithoutForce(t *testing.T) {
	svc := newSystemTestService(t)
	created, err := svc.SaveSystem(context.Background(), CreateSystemRequestDTO{
		ID:   "custom-rules",
		Name: "Custom Rules",
	})
	if err != nil {
		t.Fatalf("SaveSystem: %v", err)
	}

	// Create a game referencing this system
	gameDir := filepath.Join(svc.resolver.GamesDir(), "campaign-1")
	if err := os.MkdirAll(gameDir, 0755); err != nil {
		t.Fatal(err)
	}
	manifestYAML := "id: campaign-1\nname: Campaign One\nworld: world-1\nsystem: " + created.ID + "\n"
	if err := os.WriteFile(filepath.Join(gameDir, "game.yaml"), []byte(manifestYAML), 0644); err != nil {
		t.Fatal(err)
	}

	server := NewServer(svc, http.NotFoundHandler())

	// Without force -> 409 Conflict
	req := httptest.NewRequest(http.MethodDelete, "/api/system/"+created.ID, nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("delete in-use status = %d, want 409: %s", rec.Code, rec.Body.String())
	}

	// With force=true -> 204 No Content
	forceReq := httptest.NewRequest(http.MethodDelete, "/api/system/"+created.ID+"?force=true", nil)
	forceRec := httptest.NewRecorder()
	server.ServeHTTP(forceRec, forceReq)

	if forceRec.Code != http.StatusNoContent {
		t.Fatalf("force delete status = %d, want 204: %s", forceRec.Code, forceRec.Body.String())
	}
}

func TestSystemDeleteInUseByWorldRefusesWithoutForce(t *testing.T) {
	svc := newSystemTestService(t)
	created, err := svc.SaveSystem(context.Background(), CreateSystemRequestDTO{
		ID:   "custom-rules",
		Name: "Custom Rules",
	})
	if err != nil {
		t.Fatalf("SaveSystem: %v", err)
	}

	// Create a world referencing this system
	_, err = svc.CreateWorld(context.Background(), CreateWorldRequestDTO{
		ID:            "world-1",
		Name:          "World One",
		DefaultSystem: created.ID,
	})
	if err != nil {
		t.Fatalf("CreateWorld: %v", err)
	}

	server := NewServer(svc, http.NotFoundHandler())

	// Without force -> 409 Conflict
	req := httptest.NewRequest(http.MethodDelete, "/api/system/"+created.ID, nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("delete in-use by world status = %d, want 409: %s", rec.Code, rec.Body.String())
	}

	// With force=true -> 204 No Content
	forceReq := httptest.NewRequest(http.MethodDelete, "/api/system/"+created.ID+"?force=true", nil)
	forceRec := httptest.NewRecorder()
	server.ServeHTTP(forceRec, forceReq)

	if forceRec.Code != http.StatusNoContent {
		t.Fatalf("force delete status = %d, want 204: %s", forceRec.Code, forceRec.Body.String())
	}
}
