package gui

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func createTestPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func TestAssetEndpointsAndSummary(t *testing.T) {
	tempDir := t.TempDir()

	gameDir := filepath.Join(tempDir, "games", "test-game")
	if err := os.MkdirAll(filepath.Join(gameDir, "assets"), 0755); err != nil {
		t.Fatalf("mkdir game assets: %v", err)
	}
	worldDir := filepath.Join(tempDir, "worlds", "test-world")
	if err := os.MkdirAll(filepath.Join(worldDir, "assets"), 0755); err != nil {
		t.Fatalf("mkdir world assets: %v", err)
	}

	gameYAML := "id: test-game\nname: Test Game\nsystem: test-sys\nworld: test-world\nplayer: Hero\n"
	if err := os.WriteFile(filepath.Join(gameDir, "game.yaml"), []byte(gameYAML), 0644); err != nil {
		t.Fatalf("write game.yaml: %v", err)
	}
	worldYAML := "id: test-world\nname: Test World\ndescription: A world\ngenre: fantasy\n"
	if err := os.WriteFile(filepath.Join(worldDir, "world.yaml"), []byte(worldYAML), 0644); err != nil {
		t.Fatalf("write world.yaml: %v", err)
	}

	// Write a banner file into game assets
	pngBytes := createTestPNG(t)
	if err := os.WriteFile(filepath.Join(gameDir, "assets", "banner.png"), pngBytes, 0644); err != nil {
		t.Fatalf("write banner.png: %v", err)
	}

	svc := NewService(tempDir)
	srv := NewServer(svc, http.NotFoundHandler())

	// Test ListGames carries BannerURL
	games, err := svc.ListGames(context.Background())
	if err != nil {
		t.Fatalf("ListGames: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("expected 1 game, got %d", len(games))
	}
	if games[0].BannerURL != "/api/game/test-game/banner" {
		t.Errorf("expected BannerURL /api/game/test-game/banner, got %q", games[0].BannerURL)
	}
	if games[0].IconURL != "" {
		t.Errorf("expected empty IconURL, got %q", games[0].IconURL)
	}

	// Test GET /api/game/test-game/banner
	req := httptest.NewRequest(http.MethodGet, "/api/game/test-game/banner", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("GET banner expected 200, got %d", rr.Code)
	}
	if rr.Header().Get("Content-Type") != "image/png" {
		t.Errorf("expected Content-Type image/png, got %q", rr.Header().Get("Content-Type"))
	}

	// Test POST /api/game/test-game/icon upload
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "icon.png")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(pngBytes); err != nil {
		t.Fatalf("write part: %v", err)
	}
	writer.Close()

	uploadReq := httptest.NewRequest(http.MethodPost, "/api/game/test-game/icon", &body)
	uploadReq.Header.Set("Content-Type", writer.FormDataContentType())
	uploadRR := httptest.NewRecorder()
	srv.ServeHTTP(uploadRR, uploadReq)
	if uploadRR.Code != http.StatusOK {
		t.Fatalf("POST icon expected 200, got %d: %s", uploadRR.Code, uploadRR.Body.String())
	}

	// Verify icon file was written to disk
	if _, err := os.Stat(filepath.Join(gameDir, "assets", "icon.png")); err != nil {
		t.Errorf("expected icon.png to exist on disk: %v", err)
	}
}
