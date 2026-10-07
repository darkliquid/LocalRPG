package gui

import (
	"bytes"
	"context"
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
	if !strings.Contains(disposition, "ashen_reach") || !strings.Contains(disposition, ".lrpgpack") {
		t.Errorf("unexpected Content-Disposition: %s", disposition)
	}
}
