package gui

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProtectCrossOrigin(t *testing.T) {
	handler := ProtectCrossOrigin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	cases := []struct {
		name       string
		method     string
		origin     string
		fetchSite  string
		wantStatus int
	}{
		{
			name:       "same-origin write allowed",
			method:     http.MethodPost,
			origin:     "http://localhost:8080",
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "native webview origin allowed",
			method:     http.MethodPost,
			origin:     "wails://wails",
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "local tooling without browser headers allowed",
			method:     http.MethodPut,
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "cross-site write rejected by fetch metadata",
			method:     http.MethodPost,
			origin:     "http://evil.example",
			fetchSite:  "cross-site",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "cross-site write rejected by origin mismatch",
			method:     http.MethodPost,
			origin:     "http://evil.example",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "cross-site read allowed",
			method:     http.MethodGet,
			origin:     "http://evil.example",
			fetchSite:  "cross-site",
			wantStatus: http.StatusNoContent,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "http://localhost:8080/api/games", nil)
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.fetchSite != "" {
				req.Header.Set("Sec-Fetch-Site", tc.fetchSite)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("expected status %d, got %d", tc.wantStatus, rec.Code)
			}
		})
	}
}

func TestServerDoesNotEmitWildcardCORS(t *testing.T) {
	gameID, svc := setupTestGame(t)
	server := ProtectCrossOrigin(NewServer(svc, http.NotFoundHandler()))

	req := httptest.NewRequest(http.MethodGet, "/api/game/"+gameID+"/state", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("expected no wildcard CORS header, got %q", got)
	}
}
