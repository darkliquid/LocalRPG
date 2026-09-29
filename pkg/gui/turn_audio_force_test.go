package gui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegenerateSegmentAudioReturnsClipURLs(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writeSegmentTurn(t, svc, gameID)
	server := NewServer(svc, http.NotFoundHandler())

	req := httptest.NewRequest("POST", "/api/game/"+gameID+"/turn/1/segment/1/audio", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "/api/audio/clip/") {
		t.Errorf("body = %s, want content-addressed clip URLs", rec.Body.String())
	}
}

func TestSegmentAudioRouteIsNotAGet(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writeSegmentTurn(t, svc, gameID)
	server := NewServer(svc, http.NotFoundHandler())

	// Playback reads content-addressed clips now; a GET of the old route is gone.
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest("GET", "/api/game/"+gameID+"/turn/1/segment/1/audio", nil))
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET segment audio = %d, want it no longer served", rec.Code)
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
