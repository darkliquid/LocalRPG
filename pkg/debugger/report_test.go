package debugger_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/debugger"
)

func TestGenerateReport(t *testing.T) {
	tmpDir := t.TempDir()

	report := debugger.TestReport{
		ScenarioName: "Fault Recovery",
		StartTime:    time.Now().Add(-5 * time.Second),
		EndTime:      time.Now(),
		DurationMs:   5000,
		Passed:       true,
		Actions: []debugger.ActionRecord{
			{
				ID:         "act-1",
				ActionType: "navigate",
				Status:     "passed",
				DurationMs: 120,
			},
		},
		TotalActions: 1,
	}

	outPath := filepath.Join(tmpDir, "report.html")
	if err := debugger.ExportHTMLReport(report, outPath); err != nil {
		t.Fatalf("ExportHTMLReport failed: %v", err)
	}

	content, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	if !strings.Contains(string(content), "Fault Recovery") {
		t.Errorf("Expected report to contain 'Fault Recovery'")
	}
	if !strings.Contains(string(content), "act-1") {
		t.Errorf("Expected report to contain action 'act-1'")
	}
}
