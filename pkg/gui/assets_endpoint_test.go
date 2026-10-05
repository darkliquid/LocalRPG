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
	"strings"
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
	if !strings.HasPrefix(games[0].BannerURL, "/api/game/test-game/banner?v=") {
		t.Errorf("expected a versioned campaign banner URL, got %q", games[0].BannerURL)
	}
	if games[0].BannerSource != "campaign" {
		t.Errorf("BannerSource = %q, want campaign", games[0].BannerSource)
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

func TestGameAssetFallsBackToTheWorld(t *testing.T) {
	tempDir := t.TempDir()

	gameDir := filepath.Join(tempDir, "games", "test-game")
	if err := os.MkdirAll(gameDir, 0755); err != nil {
		t.Fatalf("mkdir game: %v", err)
	}
	worldAssets := filepath.Join(tempDir, "worlds", "test-world", "assets")
	if err := os.MkdirAll(worldAssets, 0755); err != nil {
		t.Fatalf("mkdir world assets: %v", err)
	}

	gameYAML := "id: test-game\nname: Test Game\nsystem: test-sys\nworld: test-world\nplayer: Hero\n"
	if err := os.WriteFile(filepath.Join(gameDir, "game.yaml"), []byte(gameYAML), 0644); err != nil {
		t.Fatalf("write game.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "worlds", "test-world", "world.yaml"), []byte("id: test-world\nname: Test World\n"), 0644); err != nil {
		t.Fatalf("write world.yaml: %v", err)
	}

	pngBytes := createTestPNG(t)
	for _, name := range []string{"banner.png", "icon.png"} {
		if err := os.WriteFile(filepath.Join(worldAssets, name), pngBytes, 0644); err != nil {
			t.Fatalf("write world %s: %v", name, err)
		}
	}

	svc := NewService(tempDir)
	srv := NewServer(svc, http.NotFoundHandler())

	games, err := svc.ListGames(context.Background())
	if err != nil {
		t.Fatalf("ListGames: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("expected 1 game, got %d", len(games))
	}
	if !strings.HasPrefix(games[0].BannerURL, "/api/game/test-game/banner?v=") {
		t.Errorf("BannerURL = %q, want a versioned campaign route", games[0].BannerURL)
	}
	if !strings.HasPrefix(games[0].IconURL, "/api/game/test-game/icon?v=") {
		t.Errorf("IconURL = %q, want a versioned campaign route", games[0].IconURL)
	}
	if games[0].BannerSource != "world" || games[0].IconSource != "world" {
		t.Errorf("sources = %q/%q, want world/world", games[0].BannerSource, games[0].IconSource)
	}
	worldIconURL := games[0].IconURL

	req := httptest.NewRequest(http.MethodGet, "/api/game/test-game/icon", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET icon expected 200 from the world fallback, got %d", rr.Code)
	}
	if rr.Header().Get("Content-Type") != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", rr.Header().Get("Content-Type"))
	}
	if !bytes.Equal(rr.Body.Bytes(), pngBytes) {
		t.Errorf("served icon does not match the world's bytes")
	}

	// A campaign icon of its own wins over the world's, and the URL changes so a
	// browser cannot keep showing the image it cached at the old URL.
	campaignBytes := append([]byte{}, pngBytes...)
	campaignBytes = append(campaignBytes, []byte("campaign")...)
	if err := os.MkdirAll(filepath.Join(gameDir, "assets"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gameDir, "assets", "icon.png"), campaignBytes, 0644); err != nil {
		t.Fatal(err)
	}
	games, err = svc.ListGames(context.Background())
	if err != nil {
		t.Fatalf("ListGames: %v", err)
	}
	if games[0].IconSource != "campaign" {
		t.Errorf("IconSource = %q, want campaign after its own icon exists", games[0].IconSource)
	}
	if games[0].IconURL == worldIconURL {
		t.Errorf("IconURL = %q, want it to change when the served file changes", games[0].IconURL)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/game/test-game/icon", nil)
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET icon expected 200, got %d", rr.Code)
	}
	if !bytes.Equal(rr.Body.Bytes(), campaignBytes) {
		t.Errorf("served icon does not match the campaign's own bytes")
	}

	// Clearing the campaign's icon restores the world's, at the world's URL.
	delReq := httptest.NewRequest(http.MethodDelete, "/api/game/test-game/icon", nil)
	delRR := httptest.NewRecorder()
	srv.ServeHTTP(delRR, delReq)
	if delRR.Code != http.StatusNoContent {
		t.Fatalf("DELETE icon expected 204, got %d: %s", delRR.Code, delRR.Body.String())
	}
	games, err = svc.ListGames(context.Background())
	if err != nil {
		t.Fatalf("ListGames: %v", err)
	}
	if games[0].IconSource != "world" {
		t.Errorf("IconSource = %q, want world after clearing", games[0].IconSource)
	}
	if games[0].IconURL != worldIconURL {
		t.Errorf("IconURL = %q, want the world's URL restored (%q)", games[0].IconURL, worldIconURL)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/game/test-game/icon", nil)
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET icon expected 200 after clearing, got %d", rr.Code)
	}
	if !bytes.Equal(rr.Body.Bytes(), pngBytes) {
		t.Errorf("served icon does not match the world's bytes after clearing")
	}
}
