package gui

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestServiceWithWorld(t *testing.T, worldID string) *Service {
	t.Helper()
	dir := t.TempDir()
	svc := NewService(dir)
	worldDir := svc.resolver.WorldDir(worldID)
	if err := os.MkdirAll(worldDir, 0755); err != nil {
		t.Fatal(err)
	}
	worldYAML := "id: " + worldID + "\nname: " + worldID + "\ndescription: A test world\n"
	if err := os.WriteFile(filepath.Join(worldDir, "world.yaml"), []byte(worldYAML), 0644); err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestExportContentStreamsAPackage(t *testing.T) {
	svc := newTestServiceWithWorld(t, "ashen_reach")
	var buf bytes.Buffer
	m, err := svc.ExportContent(context.Background(), "world", "ashen_reach", &buf)
	if err != nil {
		t.Fatal(err)
	}
	if m.ID != "ashen_reach" || buf.Len() == 0 {
		t.Fatalf("manifest %+v, %d bytes", m, buf.Len())
	}
}

func TestExportContentEndpoint(t *testing.T) {
	svc := newTestServiceWithWorld(t, "ashen_reach")
	server := NewServer(svc, http.NotFoundHandler())

	req := httptest.NewRequest("POST", "/api/content/export", strings.NewReader(`{"type":"world","id":"ashen_reach"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "application/gzip" {
		t.Errorf("content-type = %s, want application/gzip", rec.Header().Get("Content-Type"))
	}
	disposition := rec.Header().Get("Content-Disposition")
	if !strings.Contains(disposition, "ashen_reach") || !strings.Contains(disposition, ".lrpgworld") {
		t.Errorf("unexpected Content-Disposition: %s", disposition)
	}
}

func TestExportContentEndpointSystem(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir)
	sysDir := svc.resolver.SystemDir("fantasy_d20")
	if err := os.MkdirAll(sysDir, 0755); err != nil {
		t.Fatal(err)
	}
	sysYAML := "id: fantasy_d20\nname: Fantasy D20\ndescription: Test system\n"
	if err := os.WriteFile(filepath.Join(sysDir, "system.yaml"), []byte(sysYAML), 0644); err != nil {
		t.Fatal(err)
	}

	server := NewServer(svc, http.NotFoundHandler())
	req := httptest.NewRequest("POST", "/api/content/export", strings.NewReader(`{"type":"system","id":"fantasy_d20"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	disposition := rec.Header().Get("Content-Disposition")
	if !strings.Contains(disposition, "fantasy_d20") || !strings.Contains(disposition, ".lrpgsystem") {
		t.Errorf("unexpected Content-Disposition: %s", disposition)
	}
}

func TestExportContentEndpointWithPath(t *testing.T) {
	svc := newTestServiceWithWorld(t, "ashen_reach")
	server := NewServer(svc, http.NotFoundHandler())

	targetFile := filepath.Join(t.TempDir(), "exports", "custom.lrpgworld")
	body := `{"type":"world","id":"ashen_reach","target_path":"` + targetFile + `"}`
	req := httptest.NewRequest("POST", "/api/content/export", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if fi, err := os.Stat(targetFile); err != nil || fi.Size() == 0 {
		t.Fatalf("target file was not created or empty: %v", err)
	}

	var res ExportContentResultDTO
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if res.Path != targetFile || res.ID != "ashen_reach" || res.Type != "world" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestExportContentEndpointMismatchedExtension(t *testing.T) {
	svc := newTestServiceWithWorld(t, "ashen_reach")
	server := NewServer(svc, http.NotFoundHandler())

	targetFile := filepath.Join(t.TempDir(), "exports", "custom.lrpgsystem")
	body := `{"type":"world","id":"ashen_reach","target_path":"` + targetFile + `"}`
	req := httptest.NewRequest("POST", "/api/content/export", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 Bad Request", rec.Code)
	}
}
