package gui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleGenerateAssetPreview(t *testing.T) {
	tempDir := t.TempDir()

	service := NewService(tempDir)
	cfg := service.configMgr.Get()
	cfg.Media.Image.Type = "builtin"
	cfg.Media.Image.BuiltinName = "procedural-art"
	if err := service.configMgr.Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	server := NewServer(service, http.NotFoundHandler())

	body, _ := json.Marshal(GenerateAssetPreviewRequestDTO{
		Kind:        "banner",
		Name:        "Test Realm",
		Description: "A misty land",
		ArtStyle:    "Dark Fantasy",
		Genre:       "Gothic",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/generate-asset-preview", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}
	contentType := w.Header().Get("Content-Type")
	if contentType == "" {
		t.Errorf("expected Content-Type header on image response")
	}
	if w.Body.Len() == 0 {
		t.Errorf("expected non-empty image body")
	}
}
