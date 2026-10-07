package gui

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/content"
)

func createWorldPackageBytes(t *testing.T, id, name string, hasScript bool) []byte {
	t.Helper()
	dir := t.TempDir()
	worldYAML := "id: " + id + "\nname: " + name + "\ndescription: A test world\n"
	if err := os.WriteFile(filepath.Join(dir, "world.yaml"), []byte(worldYAML), 0644); err != nil {
		t.Fatal(err)
	}
	if hasScript {
		if err := os.WriteFile(filepath.Join(dir, "mechanics.js"), []byte("// test script"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	var buf bytes.Buffer
	if _, err := content.Pack(dir, "world", content.ManifestMeta{}, &buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestImportContentInstallsANewWorld(t *testing.T) {
	svc := NewService(t.TempDir())
	pkgBytes := createWorldPackageBytes(t, "new_world", "New World", true)

	res, err := svc.ImportContent(context.Background(), bytes.NewReader(pkgBytes), "refuse")
	if err != nil {
		t.Fatalf("ImportContent: %v", err)
	}

	if res.ID != "new_world" {
		t.Errorf("res.ID = %q, want new_world", res.ID)
	}
	if !res.HasScript {
		t.Errorf("res.HasScript = false, want true")
	}
	if res.Action != "installed" {
		t.Errorf("res.Action = %q, want installed", res.Action)
	}

	destPath := filepath.Join(svc.resolver.WorldDir("new_world"), "world.yaml")
	if _, err := os.Stat(destPath); err != nil {
		t.Fatalf("world.yaml not installed at %s: %v", destPath, err)
	}
}

func TestImportContentConflictModes(t *testing.T) {
	svc := newTestServiceWithWorld(t, "conflict_world")
	pkgBytes := createWorldPackageBytes(t, "conflict_world", "Conflicted World", false)

	// Refuse mode
	_, err := svc.ImportContent(context.Background(), bytes.NewReader(pkgBytes), "refuse")
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected already exists error, got %v", err)
	}

	// Rename mode
	renameRes, err := svc.ImportContent(context.Background(), bytes.NewReader(pkgBytes), "rename")
	if err != nil {
		t.Fatalf("ImportContent rename: %v", err)
	}
	if renameRes.ID == "conflict_world" {
		t.Fatalf("expected renamed ID, got %q", renameRes.ID)
	}
	if renameRes.Action != "renamed" {
		t.Fatalf("action = %q, want renamed", renameRes.Action)
	}

	// Overwrite mode
	overwriteRes, err := svc.ImportContent(context.Background(), bytes.NewReader(pkgBytes), "overwrite")
	if err != nil {
		t.Fatalf("ImportContent overwrite: %v", err)
	}
	if overwriteRes.ID != "conflict_world" {
		t.Fatalf("overwriteRes.ID = %q, want conflict_world", overwriteRes.ID)
	}
	if overwriteRes.Action != "overwritten" {
		t.Fatalf("action = %q, want overwritten", overwriteRes.Action)
	}
}

func TestImportContentRejectsTampered(t *testing.T) {
	svc := NewService(t.TempDir())
	badBytes := []byte("this is not a valid gzip tar package")

	_, err := svc.ImportContent(context.Background(), bytes.NewReader(badBytes), "refuse")
	if err == nil {
		t.Fatal("expected error importing invalid package, got nil")
	}
}

func TestImportContentEndpoint(t *testing.T) {
	svc := NewService(t.TempDir())
	server := NewServer(svc, http.NotFoundHandler())

	pkgBytes := createWorldPackageBytes(t, "endpoint_world", "Endpoint World", false)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", "endpoint_world-1.0.0.lrpgpack")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(pkgBytes); err != nil {
		t.Fatal(err)
	}
	if err := mw.WriteField("on_conflict", "refuse"); err != nil {
		t.Fatal(err)
	}
	_ = mw.Close()

	req := httptest.NewRequest("POST", "/api/content/import", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "endpoint_world") {
		t.Fatalf("body did not contain endpoint_world: %s", rec.Body.String())
	}
}
