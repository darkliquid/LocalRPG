package gui

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSegmentAudioRouteWithForce(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writeSegmentTurn(t, svc, gameID)
	server := NewServer(svc, http.NotFoundHandler())

	req := httptest.NewRequest("GET", "/api/game/"+gameID+"/turn/1/segment/1/audio?force=1", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() == 0 {
		t.Errorf("expected audio bytes")
	}
}

func TestPlayTurnAudioRouteWithForce(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writeSegmentTurn(t, svc, gameID)
	server := NewServer(svc, http.NotFoundHandler())

	req := httptest.NewRequest("POST", "/api/game/"+gameID+"/turn/1/play?force=1", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	// In test environments without an audio output device, playback may return 500 (ErrUnavailable).
	// If audio is available, it returns 204. Neither should be 400 or 404.
	if rec.Code == http.StatusBadRequest || rec.Code == http.StatusNotFound {
		t.Fatalf("unexpected route failure: code %d, body %s", rec.Code, rec.Body.String())
	}
}

func TestPlaySegmentAudioRouteWithForce(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writeSegmentTurn(t, svc, gameID)
	server := NewServer(svc, http.NotFoundHandler())

	req := httptest.NewRequest("POST", "/api/game/"+gameID+"/turn/1/segment/1/play?force=true", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code == http.StatusBadRequest || rec.Code == http.StatusNotFound {
		t.Fatalf("unexpected route failure: code %d, body %s", rec.Code, rec.Body.String())
	}
}
