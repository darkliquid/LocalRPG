package gui_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/gui"
	_ "github.com/darkliquid/localrpg/pkg/provider/all"
)

func TestOfflinePresetEndpoint(t *testing.T) {
	dir := t.TempDir()
	svc := gui.NewService(dir)
	server := gui.NewServer(svc, http.NotFoundHandler())

	// Set GM to gemini (cloud)
	cfg := config.DefaultConfig()
	cfg.Agents.Roles["gm"] = config.AgentRoleConfig{Type: "builtin", BuiltinName: "gemini"}
	if _, err := svc.SaveSettings(context.Background(), *cfg); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	body := bytes.NewBufferString(`{"tts":"native-os"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/config/offline-preset", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var res gui.OfflinePresetResponseDTO
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(res.Changes) == 0 {
		t.Fatal("expected changes to be returned")
	}

	// Verify settings were persisted and now offline
	settings, err := svc.GetSettings(context.Background())
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if settings.Config.Agents.Roles["gm"].BuiltinName != "narrative-oracle" {
		t.Fatalf("gm = %+v", settings.Config.Agents.Roles["gm"])
	}
}

func TestOfflineReportEndpoint(t *testing.T) {
	dir := t.TempDir()
	svc := gui.NewService(dir)
	server := gui.NewServer(svc, http.NotFoundHandler())

	// Apply offline preset to establish clean offline configuration
	cfg := config.DefaultConfig()
	config.ApplyOfflinePreset(cfg, "native-os")
	if _, err := svc.SaveSettings(context.Background(), *cfg); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	// Clean/offline report initially
	req := httptest.NewRequest(http.MethodGet, "/api/config/offline-report", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var rep gui.OfflineReportResponseDTO
	if err := json.Unmarshal(w.Body.Bytes(), &rep); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	if !rep.Offline {
		t.Fatalf("expected initial config to be offline, got %+v", rep)
	}

	// Change GM to gemini
	cfg = config.DefaultConfig()
	cfg.Agents.Roles["gm"] = config.AgentRoleConfig{Type: "builtin", BuiltinName: "gemini"}
	if _, err := svc.SaveSettings(context.Background(), *cfg); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/config/offline-report", nil)
	w2 := httptest.NewRecorder()
	server.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w2.Code, w2.Body.String())
	}

	var rep2 gui.OfflineReportResponseDTO
	if err := json.Unmarshal(w2.Body.Bytes(), &rep2); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	if rep2.Offline || len(rep2.Issues) == 0 {
		t.Fatalf("expected issues in report, got %+v", rep2)
	}
}
