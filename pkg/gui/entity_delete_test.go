package gui

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDeleteGameEntityRemovesTheNoteAndIndexEntry(t *testing.T) {
	gameID, svc := setupTestGame(t)

	if err := svc.DeleteGameEntity(context.Background(), gameID, "captain-kaelen"); err != nil {
		t.Fatalf("DeleteGameEntity: %v", err)
	}
	if _, err := svc.GetEntity(context.Background(), gameID, "captain-kaelen"); err == nil {
		t.Fatal("expected the deleted note to be gone from the index")
	}
}

func TestDeleteGameEntityMissingIsNotExist(t *testing.T) {
	gameID, svc := setupTestGame(t)

	err := svc.DeleteGameEntity(context.Background(), gameID, "no-such-note")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("err = %v, want fs.ErrNotExist", err)
	}
}

func TestDeleteGameEntityRouteReturnsNotFoundForMissing(t *testing.T) {
	gameID, svc := setupTestGame(t)
	server := NewServer(svc, http.NotFoundHandler())

	req := httptest.NewRequest(http.MethodDelete, "/api/game/"+gameID+"/entity/no-such-note", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}