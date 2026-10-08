package gui

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newWorldTestService(t *testing.T) *Service {
	t.Helper()
	return NewService(t.TempDir())
}

func TestCreateWorldRefusesDuplicate(t *testing.T) {
	service := newWorldTestService(t)

	first, err := service.CreateWorld(context.Background(), CreateWorldRequestDTO{Name: "Ember Peak"})
	if err != nil {
		t.Fatalf("CreateWorld: %v", err)
	}
	if first.ID == "" {
		t.Fatal("CreateWorld returned an empty id")
	}

	if _, err := service.CreateWorld(context.Background(), CreateWorldRequestDTO{ID: first.ID, Name: "Ember Peak"}); !errors.Is(err, ErrWorldExists) {
		t.Fatalf("second CreateWorld err = %v, want ErrWorldExists", err)
	}
}

func TestUpdateWorldRequiresExisting(t *testing.T) {
	service := newWorldTestService(t)
	if _, err := service.UpdateWorld(context.Background(), CreateWorldRequestDTO{ID: "missing", Name: "Missing"}); !errors.Is(err, ErrWorldNotFound) {
		t.Fatalf("UpdateWorld err = %v, want ErrWorldNotFound", err)
	}

	created, err := service.CreateWorld(context.Background(), CreateWorldRequestDTO{Name: "Ember Peak"})
	if err != nil {
		t.Fatalf("CreateWorld: %v", err)
	}
	updated, err := service.UpdateWorld(context.Background(), CreateWorldRequestDTO{ID: created.ID, Name: "Ember Peak", Description: "changed"})
	if err != nil {
		t.Fatalf("UpdateWorld: %v", err)
	}
	if updated.Description != "changed" {
		t.Fatalf("description = %q, want changed", updated.Description)
	}
}

func TestCreateWorldDerivedSlugCollision(t *testing.T) {
	service := newWorldTestService(t)
	if _, err := service.CreateWorld(context.Background(), CreateWorldRequestDTO{Name: "Ember Peak"}); err != nil {
		t.Fatalf("first CreateWorld: %v", err)
	}
	// The same name derives the same slug, which must not overwrite.
	if _, err := service.CreateWorld(context.Background(), CreateWorldRequestDTO{Name: "Ember Peak"}); !errors.Is(err, ErrWorldExists) {
		t.Fatalf("derived-slug collision err = %v, want ErrWorldExists", err)
	}
}

func TestWorldUpdateMissingIsNotFound(t *testing.T) {
	svc := newWorldTestService(t)
	server := NewServer(svc, http.NotFoundHandler())

	req := httptest.NewRequest(http.MethodPut, "/api/world/missing", strings.NewReader(`{"name":"Missing"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("update missing status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

func TestWorldUpdateReturnsOK(t *testing.T) {
	svc := newWorldTestService(t)
	created, err := svc.CreateWorld(context.Background(), CreateWorldRequestDTO{Name: "Ember Peak"})
	if err != nil {
		t.Fatalf("CreateWorld: %v", err)
	}
	server := NewServer(svc, http.NotFoundHandler())

	req := httptest.NewRequest(http.MethodPut, "/api/world/"+created.ID, strings.NewReader(`{"name":"Ember Peak","description":"changed"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
}

func TestWorldCreateDuplicateIsConflict(t *testing.T) {
	svc := newWorldTestService(t)
	server := NewServer(svc, http.NotFoundHandler())
	body := `{"name":"Ember Peak"}`

	post := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/worlds", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		return rec
	}

	if rec := post(); rec.Code != http.StatusCreated {
		t.Fatalf("first create status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	rec := post()
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate create status = %d, want 409: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"error"`) {
		t.Fatalf("duplicate create body = %q, want a JSON error envelope", rec.Body.String())
	}
}

func TestWorldDeleteReturnsNoContent(t *testing.T) {
	svc := newWorldTestService(t)
	created, err := svc.CreateWorld(context.Background(), CreateWorldRequestDTO{Name: "Ember Peak"})
	if err != nil {
		t.Fatalf("CreateWorld: %v", err)
	}
	server := NewServer(svc, http.NotFoundHandler())

	req := httptest.NewRequest(http.MethodDelete, "/api/world/"+created.ID, nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204: %s", rec.Code, rec.Body.String())
	}

	// Verify world is gone
	getReq := httptest.NewRequest(http.MethodGet, "/api/world/"+created.ID, nil)
	getRec := httptest.NewRecorder()
	server.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusNotFound {
		t.Fatalf("get after delete status = %d, want 404", getRec.Code)
	}
}

func TestWorldDeleteMissingReturnsNotFound(t *testing.T) {
	svc := newWorldTestService(t)
	server := NewServer(svc, http.NotFoundHandler())

	req := httptest.NewRequest(http.MethodDelete, "/api/world/nonexistent", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("delete missing status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

func TestWorldDeleteInUseRefusesWithoutForce(t *testing.T) {
	svc := newWorldTestService(t)
	created, err := svc.CreateWorld(context.Background(), CreateWorldRequestDTO{Name: "Ember Peak"})
	if err != nil {
		t.Fatalf("CreateWorld: %v", err)
	}

	// Create a game referencing this world
	gameDir := filepath.Join(svc.resolver.GamesDir(), "campaign-1")
	if err := os.MkdirAll(gameDir, 0755); err != nil {
		t.Fatal(err)
	}
	manifestYAML := "id: campaign-1\nname: Campaign One\nworld: " + created.ID + "\nsystem: sys-1\n"
	if err := os.WriteFile(filepath.Join(gameDir, "game.yaml"), []byte(manifestYAML), 0644); err != nil {
		t.Fatal(err)
	}

	server := NewServer(svc, http.NotFoundHandler())

	// Without force -> 409 Conflict
	req := httptest.NewRequest(http.MethodDelete, "/api/world/"+created.ID, nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("delete in-use status = %d, want 409: %s", rec.Code, rec.Body.String())
	}

	// With force=true -> 204 No Content
	forceReq := httptest.NewRequest(http.MethodDelete, "/api/world/"+created.ID+"?force=true", nil)
	forceRec := httptest.NewRecorder()
	server.ServeHTTP(forceRec, forceReq)

	if forceRec.Code != http.StatusNoContent {
		t.Fatalf("force delete status = %d, want 204: %s", forceRec.Code, forceRec.Body.String())
	}
}
