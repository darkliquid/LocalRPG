package gui

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportManagerSerialisesPerCampaign(t *testing.T) {
	m := newExportManager()

	if _, err := m.begin("campaign-01", "web"); err != nil {
		t.Fatalf("first begin: %v", err)
	}
	if _, err := m.begin("campaign-01", "video"); !errors.Is(err, ErrExportInFlight) {
		t.Fatalf("second begin = %v, want ErrExportInFlight", err)
	}
	if _, err := m.begin("campaign-02", "web"); err != nil {
		t.Fatalf("begin for another campaign: %v", err)
	}

	m.finish("campaign-01")
	if _, err := m.begin("campaign-01", "web"); err != nil {
		t.Fatalf("begin after finish: %v", err)
	}
}

func TestExportManagerPublishesToSubscribers(t *testing.T) {
	m := newExportManager()
	ch := m.subscribe()
	defer m.unsubscribe(ch)

	m.publish(ExportEvent{Phase: "compile", Done: 1, Total: 3})

	select {
	case event := <-ch:
		if event.Phase != "compile" || event.Done != 1 || event.Total != 3 {
			t.Fatalf("unexpected event: %+v", event)
		}
	default:
		t.Fatal("expected an event")
	}
}

func TestStartExportRejectsBadFormat(t *testing.T) {
	svc := NewService(t.TempDir())

	if _, err := svc.StartExport(context.Background(), ExportRequestDTO{GameID: "g", Format: "pdf", OutDir: t.TempDir()}); !errors.Is(err, ErrExportFormat) {
		t.Fatalf("StartExport = %v, want ErrExportFormat", err)
	}
}

func TestStartExportRequiresDestination(t *testing.T) {
	svc := NewService(t.TempDir())

	if _, err := svc.StartExport(context.Background(), ExportRequestDTO{GameID: "g", Format: "web"}); !errors.Is(err, ErrExportDirRequired) {
		t.Fatalf("StartExport = %v, want ErrExportDirRequired", err)
	}
}

func TestStartExportVideoRequiresFFmpeg(t *testing.T) {
	t.Setenv("PATH", "")
	svc := NewService(t.TempDir())

	if _, err := svc.StartExport(context.Background(), ExportRequestDTO{GameID: "g", Format: "video", OutDir: t.TempDir()}); !errors.Is(err, ErrExportNoFFmpeg) {
		t.Fatalf("StartExport = %v, want ErrExportNoFFmpeg", err)
	}
}

func TestChooseExportDirectoryWithoutPicker(t *testing.T) {
	svc := NewService(t.TempDir())

	if _, err := svc.ChooseExportDirectory(context.Background()); !errors.Is(err, ErrNoNativeDialog) {
		t.Fatalf("ChooseExportDirectory = %v, want ErrNoNativeDialog", err)
	}
}

func TestChooseExportDirectoryUsesThePicker(t *testing.T) {
	svc := NewService(t.TempDir())
	want := t.TempDir()
	var seenDefault string
	svc.SetDirectoryPicker(func(defaultDir string) (string, error) {
		seenDefault = defaultDir
		return want, nil
	})

	got, err := svc.ChooseExportDirectory(context.Background())
	if err != nil {
		t.Fatalf("ChooseExportDirectory: %v", err)
	}
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
	if seenDefault == "" {
		t.Fatalf("expected a default directory to be offered")
	}

	if !svc.ExportCapabilities().NativeDialog {
		t.Fatalf("expected capabilities to report a native dialog")
	}
}

func TestStartExportWritesIntoTheChosenDirectory(t *testing.T) {
	svc := NewService(t.TempDir())
	outDir := t.TempDir()

	job, err := svc.StartExport(context.Background(), ExportRequestDTO{
		GameID: "campaign-01", Format: "web", OutDir: outDir,
	})
	if err != nil {
		t.Fatalf("StartExport: %v", err)
	}
	if job.OutputPath == "" || !strings.HasPrefix(job.OutputPath, outDir) {
		t.Fatalf("output path %q is not under the chosen directory %q", job.OutputPath, outDir)
	}
}

func TestUniquePathAvoidsOverwrite(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "campaign-01-web")
	if err := os.MkdirAll(existing, 0755); err != nil {
		t.Fatal(err)
	}

	got := uniquePath(existing)
	if got == existing {
		t.Fatalf("uniquePath returned the existing path unchanged")
	}
	if !strings.HasPrefix(got, existing) {
		t.Fatalf("uniquePath = %q, want a variant of %q", got, existing)
	}
}

func TestExportCapabilitiesRoute(t *testing.T) {
	svc := NewService(t.TempDir())
	server := NewServer(svc, http.NotFoundHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/export/capabilities", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var caps ExportCapabilitiesDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &caps); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if caps.DefaultDir == "" {
		t.Errorf("expected a default export directory")
	}
	if caps.NativeDialog {
		t.Errorf("a headless service must not report a native dialog")
	}
}

func TestChooseDirectoryRouteWithoutPicker(t *testing.T) {
	svc := NewService(t.TempDir())
	server := NewServer(svc, http.NotFoundHandler())

	req := httptest.NewRequest(http.MethodPost, "/api/export/choose-directory", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", rec.Code)
	}
}

func TestStartExportRouteRejectsBadFormat(t *testing.T) {
	svc := NewService(t.TempDir())
	server := NewServer(svc, http.NotFoundHandler())

	body := strings.NewReader(`{"game_id":"g","format":"pdf","out_dir":"/tmp"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/export", body)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestStartExportRouteRejectsMissingDestination(t *testing.T) {
	svc := NewService(t.TempDir())
	server := NewServer(svc, http.NotFoundHandler())

	body := strings.NewReader(`{"game_id":"g","format":"web"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/export", body)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestStartExportRouteAcceptsAJob(t *testing.T) {
	svc := NewService(t.TempDir())
	t.Cleanup(svc.Close)
	server := NewServer(svc, http.NotFoundHandler())

	outDir := t.TempDir()
	body := strings.NewReader(`{"game_id":"campaign-01","format":"web","out_dir":"` + outDir + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/export", body)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rec.Code)
	}
	var job ExportJobDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &job); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !job.Running || job.OutputPath == "" {
		t.Fatalf("unexpected job: %+v", job)
	}
}

func TestCancelExportRoute(t *testing.T) {
	svc := NewService(t.TempDir())
	server := NewServer(svc, http.NotFoundHandler())

	req := httptest.NewRequest(http.MethodDelete, "/api/export/campaign-01", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rec.Code)
	}
}
