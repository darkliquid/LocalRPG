package gui

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
)

func TestUncachedBeatsRoute(t *testing.T) {
	gameID, svc := setupTestGame(t)
	svc.newTTSClient = func(config.TTSConfig) (media.TTSClient, error) {
		return &inspectingClient{}, nil
	}
	server := NewServer(svc, AssetHandler())

	req := httptest.NewRequest("GET", "/api/game/"+gameID+"/tts/uncached", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "uncached") {
		t.Errorf("body = %s", rec.Body.String())
	}
}
