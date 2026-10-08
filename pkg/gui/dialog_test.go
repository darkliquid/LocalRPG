package gui

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestChooseDirectoryWithoutPicker(t *testing.T) {
	svc := NewService(t.TempDir())

	if _, err := svc.ChooseDirectory(context.Background(), ChooseDirectoryRequestDTO{}); !errors.Is(err, ErrNoNativeDialog) {
		t.Fatalf("ChooseDirectory = %v, want ErrNoNativeDialog", err)
	}
	if svc.ExportCapabilities().NativeDialog {
		t.Fatal("a headless service must not report a native dialog")
	}
}

func TestChooseDirectoryUsesThePickerAndItsTitle(t *testing.T) {
	svc := NewService(t.TempDir())
	want := t.TempDir()
	var seenTitle, seenDefault string
	svc.SetDirectoryPicker(func(title, defaultDir string) (string, error) {
		seenTitle = title
		seenDefault = defaultDir
		return want, nil
	})

	got, err := svc.ChooseDirectory(context.Background(), ChooseDirectoryRequestDTO{
		Title:      "Choose a source folder",
		DefaultDir: "/tmp/notes",
	})
	if err != nil {
		t.Fatalf("ChooseDirectory: %v", err)
	}
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
	if seenTitle != "Choose a source folder" || seenDefault != "/tmp/notes" {
		t.Fatalf("picker got title %q default %q", seenTitle, seenDefault)
	}

	if !svc.ExportCapabilities().NativeDialog {
		t.Fatal("expected capabilities to report a native dialog")
	}
}

func TestChooseDirectoryOffersADefaultTitleAndFolder(t *testing.T) {
	svc := NewService(t.TempDir())
	var seenTitle, seenDefault string
	svc.SetDirectoryPicker(func(title, defaultDir string) (string, error) {
		seenTitle = title
		seenDefault = defaultDir
		return "", nil
	})

	if _, err := svc.ChooseDirectory(context.Background(), ChooseDirectoryRequestDTO{}); err != nil {
		t.Fatalf("ChooseDirectory: %v", err)
	}
	if seenTitle == "" {
		t.Fatal("expected a default title")
	}
	if seenDefault == "" {
		t.Fatal("expected a default directory to be offered")
	}
}

func TestChooseDirectoryRouteWithoutPicker(t *testing.T) {
	svc := NewService(t.TempDir())
	server := NewServer(svc, http.NotFoundHandler())

	req := httptest.NewRequest(http.MethodPost, "/api/dialog/directory", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", rec.Code)
	}
}

func TestChooseDirectoryRouteReturnsThePickedPath(t *testing.T) {
	svc := NewService(t.TempDir())
	svc.SetDirectoryPicker(func(_, _ string) (string, error) { return "/tmp/picked", nil })
	server := NewServer(svc, http.NotFoundHandler())

	body := strings.NewReader(`{"title":"Choose a source folder"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/dialog/directory", body)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "/tmp/picked") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestChooseDirectoryRouteRejectsAnUnknownAction(t *testing.T) {
	svc := NewService(t.TempDir())
	server := NewServer(svc, http.NotFoundHandler())

	req := httptest.NewRequest(http.MethodPost, "/api/dialog/nonsense", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
