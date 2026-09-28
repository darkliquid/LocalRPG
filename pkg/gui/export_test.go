package gui

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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

	if _, err := svc.StartExport(context.Background(), ExportRequestDTO{GameID: "g", Format: "pdf"}); !errors.Is(err, ErrExportFormat) {
		t.Fatalf("StartExport = %v, want ErrExportFormat", err)
	}
}

func TestStartExportVideoRequiresFFmpeg(t *testing.T) {
	t.Setenv("PATH", "")
	svc := NewService(t.TempDir())

	if _, err := svc.StartExport(context.Background(), ExportRequestDTO{GameID: "g", Format: "video"}); !errors.Is(err, ErrExportNoFFmpeg) {
		t.Fatalf("StartExport = %v, want ErrExportNoFFmpeg", err)
	}
}

func TestExportOutputPathIsUnderCampaignExports(t *testing.T) {
	svc := NewService(t.TempDir())

	path := svc.exportOutputPath("campaign-01", "web")
	want := svc.GetResolver().ExportsDir("campaign-01")
	if !strings.HasPrefix(path, want) {
		t.Fatalf("output path %q is not under %q", path, want)
	}
	if strings.Contains(path, "dist") {
		t.Fatalf("output path must not use the SPA dist directory: %q", path)
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
}

func TestStartExportRouteRejectsBadFormat(t *testing.T) {
	svc := NewService(t.TempDir())
	server := NewServer(svc, http.NotFoundHandler())

	body := strings.NewReader(`{"game_id":"g","format":"pdf"}`)
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

	body := strings.NewReader(`{"game_id":"campaign-01","format":"web"}`)
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
