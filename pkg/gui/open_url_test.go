package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenURLRouteRequiresDesktopWindow(t *testing.T) {
	svc := NewService(t.TempDir())
	server := NewServer(svc, nil)

	body, _ := json.Marshal(OpenURLRequestDTO{URL: "https://example.com"})
	req := httptest.NewRequest(http.MethodPost, "/api/open-url", strings.NewReader(string(body)))
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	// Without a window there is nothing to open the link, so the caller falls
	// back to a browser tab.
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 without an opener, got %d: %s", w.Code, w.Body.String())
	}
}

func TestOpenURLRouteOpensThroughTheDesktopWindow(t *testing.T) {
	svc := NewService(t.TempDir())
	server := NewServer(svc, nil)

	var opened string
	svc.SetURLOpener(func(url string) error {
		opened = url
		return nil
	})

	body, _ := json.Marshal(OpenURLRequestDTO{URL: "https://example.com/issues"})
	req := httptest.NewRequest(http.MethodPost, "/api/open-url", strings.NewReader(string(body)))
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
	if opened != "https://example.com/issues" {
		t.Errorf("opener received %q, want the requested url", opened)
	}
}

func TestOpenURLRouteRejectsNonWebSchemes(t *testing.T) {
	svc := NewService(t.TempDir())
	server := NewServer(svc, nil)

	opened := false
	svc.SetURLOpener(func(string) error {
		opened = true
		return nil
	})

	body, _ := json.Marshal(OpenURLRequestDTO{URL: "file:///etc/passwd"})
	req := httptest.NewRequest(http.MethodPost, "/api/open-url", strings.NewReader(string(body)))
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a non-web scheme, got %d: %s", w.Code, w.Body.String())
	}
	if opened {
		t.Error("opener must not run for a rejected scheme")
	}
}

func TestOpenURLRouteRejectsOtherMethods(t *testing.T) {
	svc := NewService(t.TempDir())
	server := NewServer(svc, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/open-url", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}
