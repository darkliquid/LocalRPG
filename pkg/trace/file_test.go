package trace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileSinkWritesOneJSONObjectPerLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace", "trace.jsonl")
	sink, err := NewFileSink(path, FileOptions{Level: LevelSummary, MaxBytes: 1 << 20, MaxFiles: 3})
	if err != nil {
		t.Fatalf("NewFileSink failed: %v", err)
	}
	defer func() { _ = sink.Close() }()

	sink.Event("turn.begin", map[string]interface{}{"game": "test-campaign", "number": 2})
	sink.Event("provider.response", map[string]interface{}{"finish_reason": "stop"})

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}

	lines := splitLines(string(data))
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), data)
	}

	var first map[string]interface{}
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("line is not JSON: %v", err)
	}
	if first["event"] != "turn.begin" {
		t.Errorf("event = %v, want turn.begin", first["event"])
	}
	if first["level"] != "summary" {
		t.Errorf("level = %v, want summary", first["level"])
	}
	if _, ok := first["ts"]; !ok {
		t.Errorf("every line must carry its own timestamp: %v", first)
	}
	if first["number"] != float64(2) {
		t.Errorf("fields must survive: %v", first)
	}
}

func TestFileSinkStampsTheCampaign(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	sink, err := NewFileSink(path, FileOptions{Level: LevelSummary, MaxBytes: 1 << 20, MaxFiles: 3})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sink.Close() }()

	sink.SetGame("test-campaign")
	sink.Event("turn.begin", map[string]interface{}{"number": 1})

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"game":"test-campaign"`) {
		t.Errorf("expected the campaign stamped on the line: %s", data)
	}
}

func TestFileSinkIsOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	sink, err := NewFileSink(path, FileOptions{Level: LevelFull, MaxBytes: 1 << 20, MaxFiles: 3})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sink.Close() }()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("trace mode = %o, want 600; it can contain prompts", perm)
	}
}

func TestFileSinkRotatesAndKeepsTheNewestFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trace.jsonl")
	// A tiny ceiling and a rotate check of 1 make rotation happen on every write.
	sink, err := NewFileSink(path, FileOptions{Level: LevelSummary, MaxBytes: 120, MaxFiles: 2, RotateCheck: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sink.Close() }()

	for i := 0; i < 12; i++ {
		sink.Event("turn.begin", map[string]interface{}{"number": i, "padding": "0123456789012345678901234567890123456789"})
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected the live file to exist: %v", err)
	}
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Errorf("expected one rotated file: %v", err)
	}
	// MaxFiles counts rotated files, so .1 and .2 are kept and .3 is dropped.
	if _, err := os.Stat(path + ".2"); err != nil {
		t.Errorf("expected two rotated files with MaxFiles=2: %v", err)
	}
	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Errorf("expected older rotations to be dropped, stat err = %v", err)
	}

	// The live file must be fresh after rotation, not a continuation.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(splitLines(string(data))) > 4 {
		t.Errorf("live file looks unrotated: %q", data)
	}
}

func TestFileSinkCountsFailuresInsteadOfFailingTheTurn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	sink, err := NewFileSink(path, FileOptions{Level: LevelSummary, MaxBytes: 1 << 20, MaxFiles: 3})
	if err != nil {
		t.Fatal(err)
	}
	if err := sink.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// A write after close must not panic and must be counted.
	sink.Event("turn.begin", map[string]interface{}{"number": 1})
	if sink.Failures() == 0 {
		t.Errorf("expected the failure to be counted")
	}
}

func TestFileSinkRefusesAnUnusablePath(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := NewFileSink(filepath.Join(blocker, "trace.jsonl"), FileOptions{Level: LevelFull}); err == nil {
		t.Errorf("expected a file where a directory is required to fail")
	}
}

func TestMultiSendsToEveryLogger(t *testing.T) {
	first := NewMemory(LevelFull)
	second := NewMemory(LevelFull)

	multi := Multi(first, second)
	if !multi.Enabled(LevelFull) {
		t.Errorf("expected multi to be enabled when any sink is")
	}
	multi.SetGame("test-campaign")
	multi.Event("turn.begin", map[string]interface{}{"number": 1})

	if len(first.Names()) != 1 || len(second.Names()) != 1 {
		t.Errorf("expected both sinks to record, got %v and %v", first.Names(), second.Names())
	}
	event, _ := first.Find("turn.begin")
	if event.Fields["game"] != "test-campaign" {
		t.Errorf("expected the campaign forwarded to every sink, got %+v", event.Fields)
	}
}

func splitLines(data string) []string {
	lines := make([]string, 0)
	for _, line := range strings.Split(strings.TrimRight(data, "\n"), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
