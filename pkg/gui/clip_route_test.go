package gui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/media"
)

func TestClipRouteServesAStoredClip(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writeSegmentTurn(t, svc, gameID)

	clips, err := svc.GetSegmentClips(context.Background(), gameID, 1, 1)
	if err != nil || len(clips) == 0 {
		t.Fatalf("GetSegmentClips = %#v, %v", clips, err)
	}
	key := media.ClipKeyForPath(clips[0])

	server := NewServer(svc, http.NotFoundHandler())
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest("GET", "/api/audio/clip/"+key, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() == 0 {
		t.Error("expected the clip's bytes")
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "ogg") {
		t.Errorf("Content-Type = %q, want an Ogg type", ct)
	}
	if rec.Header().Get("ETag") == "" {
		t.Error("expected an ETag on the clip response")
	}
}

func TestClipRouteRejectsUnknownAndMalformedKeys(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writeSegmentTurn(t, svc, gameID)
	server := NewServer(svc, http.NotFoundHandler())

	for _, path := range []string{
		"/api/audio/clip/" + strings.Repeat("0", 64),
		"/api/audio/clip/nothex",
		"/api/audio/clip/" + strings.Repeat("a", 63),
		"/api/audio/clip/" + strings.Repeat("F", 64),
	} {
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s = %d, want 404", path, rec.Code)
		}
	}
}

func TestClipKeyIsValidated(t *testing.T) {
	accepted := strings.Repeat("0", 64)
	if !clipKeyPattern.MatchString(accepted) {
		t.Errorf("a 64 character lowercase hex key must be accepted, %q was not", accepted)
	}

	for _, key := range []string{
		"",
		"abc",
		strings.Repeat("A", 64),
		strings.Repeat("z", 64),
		strings.Repeat("0", 63),
		"../config",
		strings.Repeat("0", 64) + "/x",
	} {
		if clipKeyPattern.MatchString(key) {
			t.Errorf("%q was accepted as a clip key", key)
		}
	}
}

func TestClipPathRefusesAKeyThatIsNotOne(t *testing.T) {
	_, svc := setupTestGame(t)
	for _, key := range []string{"", "..", "../../etc/passwd", "abc/def", strings.Repeat("0", 64) + "/x"} {
		if path, ok := svc.ClipPath(key); ok {
			t.Errorf("ClipPath(%q) = %q, want a refusal", key, path)
		}
	}
}
