# Trace and Debug View Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make a turn reconstructable after the fact: record the assembled prompt, the exact provider request, the raw generation next to its parsed segments, the extraction pass, and the media calls, into a single local JSONL file that a Debug view can read.

**Architecture:** A new `pkg/trace` package defines one `Logger` interface with three levels. Types that emit events gain a `SetLogger`, defaulting to a no-op, so nothing branches on nil and no existing constructor changes signature. Logger-aware variants of the harness factories are added rather than changing the existing ones, so no test needs updating. The file sink is a single appended `trace.jsonl` in the cache directory, rotated by size.

**Tech Stack:** Go 1.27.1, the standard library only (`encoding/json`, `bufio`, `os`, `sync`, `time`), React 19 + TypeScript for the Debug view, `mise` tasks.

**Spec:** `docs/superpowers/specs/2026-09-22-narrative-coherence-and-trace-design.md` (section 9 and section 11)

## Global Constraints

- Go tests use the standard library only (`testing`, `t.TempDir()`); no testify. Use `interface{}`, never `any`.
- No new dependencies, Go or Node.
- Tracing is **off by default** and writes nothing when off.
- A trace failure is never fatal: the sink swallows its own errors and counts them. A turn must complete with a broken trace file.
- Secrets are never written at any level. The deny-list is applied inside the sink, not by callers, so a careless call site cannot leak an `api_key`.
- Single appended file, `0600`, marked `//go:build`-free and portable: no `syscall`, no `chown`.
- The prompt is recorded **once**, on `context.assembled`. Provider events carry its hash and length, never a second copy.
- Events must work with a nil logger everywhere: `trace.OrNil(nil)` returns `trace.Nop()`.
- `go vet ./...`, `go test -count=1 ./...`, and `cd frontend && npx tsc --noEmit` must pass. Never commit the deletion of `pkg/gui/dist/.gitkeep`.

## Scope & Splitting

This plan implements **increment 1 (trace)** of the coherence spec. The event catalogue in that spec lists events owned by later increments, and they are **not** in this plan:

| Deferred | Belongs to |
| --- | --- |
| `continuity.check` | coherence increment 6 |
| `summary.regenerate` | coherence increment 4 |
| `tool.call`, `tool.result`, `tool.round`, `provider.tools` | agentic spec |
| `context.assembled.sections[]` | coherence increment 2, which adds `SectionStat` |

One catalogue refinement is made here: `extraction.result` loses `matched[]`/`created[]`, which need the store, and the orchestrator emits a new `extraction.reconcile` with them. The spec's catalogue gains that row in Task 6.

---

### Task 1: The trace level and logger interface

**Files:**
- Create: `pkg/trace/trace.go`
- Test: `pkg/trace/trace_test.go`

**Interfaces:**
- Produces: `trace.Level` (`LevelOff`, `LevelSummary`, `LevelFull`), `trace.ParseLevel(string) (Level, error)`, `(Level).String() string`
- Produces: `trace.Logger` (`Enabled(Level) bool`, `Event(name string, fields map[string]interface{})`), `trace.Nop() Logger`, `trace.OrNil(Logger) Logger`
- Produces: `trace.Memory` (`NewMemory(Level)`, `Event`, `Events()`, `Names()`, `Find(string) (Event, bool)`) and `trace.Event`

- [ ] **Step 1: Write the failing test**

Create `pkg/trace/trace_test.go`:

```go
package trace

import "testing"

func TestParseLevelAcceptsTheDocumentedNames(t *testing.T) {
	cases := map[string]Level{
		"":        LevelOff,
		"off":     LevelOff,
		"none":    LevelOff,
		"summary": LevelSummary,
		"full":    LevelFull,
		"debug":   LevelFull,
	}

	for input, want := range cases {
		got, err := ParseLevel(input)
		if err != nil {
			t.Errorf("ParseLevel(%q) failed: %v", input, err)
			continue
		}
		if got != want {
			t.Errorf("ParseLevel(%q) = %v, want %v", input, got, want)
		}
	}

	if _, err := ParseLevel("loud"); err == nil {
		t.Errorf("expected an unknown level to fail")
	}
}

func TestLevelsAreOrderedAndPrintable(t *testing.T) {
	if LevelOff >= LevelSummary || LevelSummary >= LevelFull {
		t.Fatalf("levels must be ordered off < summary < full")
	}
	for level, want := range map[Level]string{LevelOff: "off", LevelSummary: "summary", LevelFull: "full"} {
		if got := level.String(); got != want {
			t.Errorf("Level(%d).String() = %q, want %q", level, got, want)
		}
	}
}

func TestNopRecordsNothingAndNeverEnabled(t *testing.T) {
	if Nop().Enabled(LevelSummary) {
		t.Errorf("Nop must not be enabled at any level")
	}
}

func TestMemoryStampsTheCampaign(t *testing.T) {
	memory := NewMemory(LevelSummary)
	memory.SetGame("test-campaign")
	memory.Event("turn.begin", map[string]interface{}{"number": 1})

	event, ok := memory.Find("turn.begin")
	if !ok {
		t.Fatal("expected the event")
	}
	if event.Fields["game"] != "test-campaign" {
		t.Errorf("expected the campaign stamped on the event, got %+v", event.Fields)
	}
	if event.Fields["number"] != 1 {
		t.Errorf("stamping must not lose the caller's fields, got %+v", event.Fields)
	}
}

func TestMemoryOnlyRecordsEnabledEvents(t *testing.T) {
	memory := NewMemory(LevelSummary)

	if !memory.Enabled(LevelSummary) {
		t.Errorf("expected summary to be enabled at summary level")
	}
	if memory.Enabled(LevelFull) {
		t.Errorf("expected full to be disabled at summary level")
	}

	memory.Event("turn.begin", map[string]interface{}{"number": 1})
	memory.Event("context.assembled", map[string]interface{}{"tokens": 2610})

	if got := memory.Names(); len(got) != 2 || got[0] != "turn.begin" {
		t.Fatalf("Names() = %v, want the two events in order", got)
	}

	event, ok := memory.Find("context.assembled")
	if !ok {
		t.Fatalf("expected to find the assembled event")
	}
	if event.Fields["tokens"] != 2610 {
		t.Errorf("fields were not preserved: %+v", event.Fields)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run 'TestParseLevel|TestLevels|TestNop|TestMemory' ./pkg/trace/`
Expected: FAIL — `no Go files in .../pkg/trace`, or undefined symbols.

- [ ] **Step 3: Write the implementation**

Create `pkg/trace/trace.go`:

```go
// Package trace records what the system actually did: the prompt it assembled,
// the request it sent, what came back, and how the reply was parsed. It exists
// because every diagnosis of a bad turn previously meant inferring from the
// timeline, and every remedy was therefore a guess.
package trace

import (
	"fmt"
	"strings"
	"sync"
)

// Level is how much detail a trace records.
type Level int

const (
	// LevelOff writes nothing. It is the default, so normal play costs nothing.
	LevelOff Level = iota
	// LevelSummary records decisions, sizes, timings, and errors, but no payloads.
	LevelSummary
	// LevelFull additionally records prompts, generated text, and raw wire lines.
	LevelFull
)

func (l Level) String() string {
	switch l {
	case LevelSummary:
		return "summary"
	case LevelFull:
		return "full"
	default:
		return "off"
	}
}

// ParseLevel reads a configured level. An empty value means off, so configuration
// written before tracing existed behaves as it did.
func ParseLevel(value string) (Level, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "off", "none", "false":
		return LevelOff, nil
	case "summary":
		return LevelSummary, nil
	case "full", "true", "debug":
		return LevelFull, nil
	default:
		return LevelOff, fmt.Errorf("unknown trace level %q", value)
	}
}

// Logger records one structured event. Implementations must never block a turn
// and must never fail it: a sink that cannot write counts the failure instead.
type Logger interface {
	// Enabled reports whether events at level would be recorded.
	Enabled(level Level) bool
	// Event records a named event. Fields are flat values, nested maps, or lists.
	Event(name string, fields map[string]interface{})
	// SetGame stamps later events with a campaign, so one appended trace can be
	// filtered back down to a campaign without a second file.
	SetGame(gameID string)
}

type nopLogger struct{}

// Nop is the logger used when tracing is off, so no caller branches on nil.
func Nop() Logger { return nopLogger{} }

func (nopLogger) Enabled(Level) bool                   { return false }
func (nopLogger) Event(string, map[string]interface{}) {}
func (nopLogger) SetGame(string)                       {}

// OrNil makes a possibly-nil logger safe to use.
func OrNil(logger Logger) Logger {
	if logger == nil {
		return Nop()
	}
	return logger
}

// Event is one recorded event, as it was handed to the sink.
type Event struct {
	Name   string
	Fields map[string]interface{}
}

// Memory keeps events in memory. It is what tests assert against, so tracing is
// testable without touching a filesystem.
type Memory struct {
	mu     sync.Mutex
	level  Level
	game   string
	events []Event
}

func NewMemory(level Level) *Memory {
	return &Memory{level: level}
}

func (m *Memory) Enabled(level Level) bool {
	return m.level != LevelOff && level <= m.level
}

func (m *Memory) Event(name string, fields map[string]interface{}) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	stamped := fields
	if m.game != "" {
		stamped = make(map[string]interface{}, len(fields)+1)
		for key, value := range fields {
			stamped[key] = value
		}
		stamped["game"] = m.game
	}
	m.events = append(m.events, Event{Name: name, Fields: stamped})
}

// SetGame stamps later events with a campaign.
func (m *Memory) SetGame(gameID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.game = gameID
}

func (m *Memory) Events() []Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Event(nil), m.events...)
}

func (m *Memory) Names() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	names := make([]string, 0, len(m.events))
	for _, event := range m.events {
		names = append(names, event.Name)
	}
	return names
}

func (m *Memory) Find(name string) (Event, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, event := range m.events {
		if event.Name == name {
			return event, true
		}
	}
	return Event{}, false
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -count=1 ./pkg/trace/ && gofmt -l pkg/trace/`
Expected: PASS and no formatting output.

- [ ] **Step 5: Commit**

```bash
git add pkg/trace/trace.go pkg/trace/trace_test.go
git commit -m "feat(trace): add one logger interface with three levels"
```

---

### Task 2: Redaction and payload capping

**Files:**
- Create: `pkg/trace/sanitize.go`
- Test: `pkg/trace/sanitize_test.go`

**Interfaces:**
- Consumes: `trace.Level` (Task 1)
- Produces: `trace.Sanitize(fields map[string]interface{}, level Level, payloadChars int) map[string]interface{}`

- [ ] **Step 1: Write the failing test**

Create `pkg/trace/sanitize_test.go`:

```go
package trace

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSanitizeRedactsSecretsAtEveryLevel(t *testing.T) {
	fields := map[string]interface{}{
		"role":     "gm",
		"api_key":  "sk-live-1234567890",
		"endpoint": "http://localhost:8880",
	}

	for _, level := range []Level{LevelSummary, LevelFull} {
		clean := Sanitize(fields, level, 20000)
		if clean["api_key"] != "[redacted]" {
			t.Errorf("level %v: api_key = %v, want [redacted]", level, clean["api_key"])
		}
		if clean["role"] != "gm" {
			t.Errorf("level %v: unexpected field loss: %+v", level, clean)
		}
	}
}

func TestSanitizeRedactsNestedSecrets(t *testing.T) {
	fields := map[string]interface{}{
		"config": map[string]interface{}{
			"provider": map[string]interface{}{"authorization": "Bearer abc", "model": "kokoro"},
		},
		"attempts": []interface{}{map[string]interface{}{"token": "abc"}},
	}

	clean := Sanitize(fields, LevelFull, 20000)
	encoded, err := json.Marshal(clean)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"Bearer abc", `"abc"`} {
		if strings.Contains(string(encoded), secret) {
			t.Errorf("secret %q survived sanitising: %s", secret, encoded)
		}
	}
	if !strings.Contains(string(encoded), "kokoro") {
		t.Errorf("sanitising removed a harmless field: %s", encoded)
	}
}

func TestSanitizeOmitsPayloadsAtSummaryAndKeepsThemAtFull(t *testing.T) {
	fields := map[string]interface{}{"prompt": "the whole question", "prompt_chars": 19}

	summary := Sanitize(fields, LevelSummary, 20000)
	if _, present := summary["prompt"]; present {
		t.Errorf("summary level must not carry a payload")
	}
	if summary["prompt_chars"] != 19 {
		t.Errorf("summary level must keep the size, got %v", summary["prompt_chars"])
	}

	full := Sanitize(fields, LevelFull, 20000)
	if full["prompt"] != "the whole question" {
		t.Errorf("full level must keep the payload, got %v", full["prompt"])
	}
}

func TestSanitizeTruncatesLongStringsWithAMarker(t *testing.T) {
	fields := map[string]interface{}{"prompt": strings.Repeat("a", 50)}

	clean := Sanitize(fields, LevelFull, 10)
	text, ok := clean["prompt"].(string)
	if !ok {
		t.Fatalf("prompt is not a string: %T", clean["prompt"])
	}
	if !strings.HasPrefix(text, strings.Repeat("a", 10)) {
		t.Errorf("truncation did not keep the head: %q", text)
	}
	if !strings.Contains(text, "truncated") || !strings.Contains(text, "50") {
		t.Errorf("truncation marker must say what was cut: %q", text)
	}
}

func TestSanitizeKeepsCacheKeys(t *testing.T) {
	// A cache key is a content hash. Redacting anything named "key" would hide
	// exactly the value needed to find the cached clip.
	fields := map[string]interface{}{"cache_key": "06e763d1"}

	clean := Sanitize(fields, LevelFull, 20000)
	if clean["cache_key"] != "06e763d1" {
		t.Errorf("cache_key = %v, want it preserved", clean["cache_key"])
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestSanitize ./pkg/trace/`
Expected: FAIL — `undefined: Sanitize`.

- [ ] **Step 3: Write the implementation**

Create `pkg/trace/sanitize.go`:

```go
package trace

import (
	"fmt"
	"strings"
)

// payloadFields hold the substance of an event: the things whose content is the
// reason to look. They are recorded at full only, so summary stays a genuine
// "what happened and how fast" level.
var payloadFields = map[string]bool{
	"prompt":           true,
	"untrimmed_prompt": true,
	"narration":        true,
	"text":             true,
	"line":             true,
	"result":           true,
	"request":          true,
	"response":         true,
	"system":           true,
	"content":          true,
}

// secretFields are replaced at every level. The list is exact rather than a
// substring match: "cache_key" is a content hash and hiding it would remove the
// one value needed to find a cached clip.
var secretFields = map[string]bool{
	"api_key":       true,
	"apikey":        true,
	"authorization": true,
	"token":         true,
	"secret":        true,
	"password":      true,
}

// Sanitize prepares fields for writing: secrets are replaced, payloads are
// dropped below full, and every string is capped. It is applied by the sink so a
// careless call site cannot leak a key.
func Sanitize(fields map[string]interface{}, level Level, payloadChars int) map[string]interface{} {
	if payloadChars <= 0 {
		payloadChars = 20000
	}
	return sanitizeMap(fields, level, payloadChars)
}

func sanitizeMap(fields map[string]interface{}, level Level, payloadChars int) map[string]interface{} {
	clean := make(map[string]interface{}, len(fields))
	for key, value := range fields {
		lower := strings.ToLower(key)

		if secretFields[lower] {
			clean[key] = "[redacted]"
			continue
		}
		if payloadFields[lower] && level < LevelFull {
			continue
		}

		clean[key] = sanitizeValue(value, level, payloadChars)
	}
	return clean
}

func sanitizeValue(value interface{}, level Level, payloadChars int) interface{} {
	switch typed := value.(type) {
	case string:
		return truncate(typed, payloadChars)
	case map[string]interface{}:
		return sanitizeMap(typed, level, payloadChars)
	case []interface{}:
		items := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			items = append(items, sanitizeValue(item, level, payloadChars))
		}
		return items
	default:
		return value
	}
}

func truncate(value string, payloadChars int) string {
	runes := []rune(value)
	if len(runes) <= payloadChars {
		return value
	}
	return fmt.Sprintf("%s... (truncated, %d chars)", string(runes[:payloadChars]), len(runes))
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -count=1 ./pkg/trace/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/trace/sanitize.go pkg/trace/sanitize_test.go
git commit -m "feat(trace): redact secrets and gate payloads by level"
```

---

### Task 3: The JSONL file sink

**Files:**
- Create: `pkg/trace/file.go`
- Test: `pkg/trace/file_test.go`

**Interfaces:**
- Consumes: `trace.Level`, `trace.Logger` (Task 1), `trace.Sanitize` (Task 2)
- Produces: `trace.FileOptions{Level, MaxBytes, MaxFiles, RotateCheck, PayloadChars}`
- Produces: `trace.NewFileSink(path string, opts FileOptions) (*FileSink, error)`, `(*FileSink).Event`, `(*FileSink).Enabled`, `(*FileSink).Close() error`, `(*FileSink).Failures() int`, `(*FileSink).Path() string`
- Produces: `trace.Multi(loggers ...Logger) Logger`

- [ ] **Step 1: Write the failing test**

Create `pkg/trace/file_test.go`:

```go
package trace

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	if _, err := os.Stat(path + ".2"); !os.IsNotExist(err) {
		t.Errorf("expected no third file with MaxFiles=2, stat err = %v", err)
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
	multi.Event("turn.begin", map[string]interface{}{"number": 1})

	if len(first.Names()) != 1 || len(second.Names()) != 1 {
		t.Errorf("expected both sinks to record, got %v and %v", first.Names(), second.Names())
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
```

Add `"strings"` to that file's imports.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestFileSink ./pkg/trace/`
Expected: FAIL — `undefined: NewFileSink`.

- [ ] **Step 3: Write the implementation**

Create `pkg/trace/file.go`:

```go
package trace

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileOptions configures the JSONL sink.
type FileOptions struct {
	Level        Level
	MaxBytes     int64
	MaxFiles     int
	RotateCheck  int
	PayloadChars int
}

// FileSink appends one JSON object per line to a single file. It is the canonical
// trace: a per-session file would make `tail -f` a hunt for timestamps, and an
// appended file has no natural age, so size is the only honest bound.
type FileSink struct {
	mu       sync.Mutex
	path     string
	opts     FileOptions
	game     string
	file     *os.File
	writer   *bufio.Writer
	written  int64
	since    int
	failures int
}

// SetGame stamps later events with a campaign, so one appended file can still be
// filtered per campaign.
func (s *FileSink) SetGame(gameID string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.game = gameID
}

func NewFileSink(path string, opts FileOptions) (*FileSink, error) {
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = 268435456
	}
	if opts.MaxFiles <= 0 {
		opts.MaxFiles = 3
	}
	if opts.RotateCheck <= 0 {
		opts.RotateCheck = 200
	}
	if opts.PayloadChars <= 0 {
		opts.PayloadChars = 20000
	}

	sink := &FileSink{path: path, opts: opts}
	if err := sink.open(); err != nil {
		return nil, err
	}
	return sink, nil
}

func (s *FileSink) Path() string { return s.path }

func (s *FileSink) Enabled(level Level) bool {
	return s != nil && s.opts.Level != LevelOff && level <= s.opts.Level
}

// Failures reports how many events could not be written. Tracing never fails a
// turn, so a count is the only signal available.
func (s *FileSink) Failures() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.failures
}

func (s *FileSink) Event(name string, fields map[string]interface{}) {
	if s == nil || s.opts.Level == LevelOff {
		return
	}

	line := map[string]interface{}{
		"ts":    time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		"event": name,
		"level": s.opts.Level.String(),
	}
	if s.game != "" {
		line["game"] = s.game
	}
	for key, value := range Sanitize(fields, s.opts.Level, s.opts.PayloadChars) {
		line[key] = value
	}

	encoded, err := json.Marshal(line)
	if err != nil {
		s.countFailure()
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.writer == nil {
		s.failures++
		return
	}
	encoded = append(encoded, '\n')
	written, err := s.writer.Write(encoded)
	if err == nil {
		// Flushed per event, so a crash keeps what was seen.
		err = s.writer.Flush()
	}
	if err != nil {
		s.failures++
		return
	}

	s.written += int64(written)
	s.since++
	if s.since >= s.opts.RotateCheck || s.written >= s.opts.MaxBytes {
		s.rotateLocked()
	}
}

func (s *FileSink) countFailure() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures++
}

func (s *FileSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closeLocked()
}

func (s *FileSink) closeLocked() error {
	if s.writer != nil {
		_ = s.writer.Flush()
		s.writer = nil
	}
	if s.file != nil {
		err := s.file.Close()
		s.file = nil
		return err
	}
	return nil
}

func (s *FileSink) open() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return fmt.Errorf("create trace dir: %w", err)
	}
	file, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("open trace file: %w", err)
	}
	info, err := file.Stat()
	if err == nil {
		s.written = info.Size()
	}
	s.file = file
	s.writer = bufio.NewWriter(file)
	return nil
}

// rotateLocked shifts the numbered files up and starts a fresh live file.
func (s *FileSink) rotateLocked() {
	if s.written < s.opts.MaxBytes {
		return
	}

	_ = s.closeLocked()

	for index := s.opts.MaxFiles - 1; index >= 1; index-- {
		from := fmt.Sprintf("%s.%d", s.path, index)
		to := fmt.Sprintf("%s.%d", s.path, index+1)
		if _, err := os.Stat(from); err == nil {
			_ = os.Rename(from, to)
		}
	}
	_ = os.Rename(s.path, s.path+".1")
	_ = os.Remove(fmt.Sprintf("%s.%d", s.path, s.opts.MaxFiles+1))

	s.written = 0
	s.since = 0
	if err := s.open(); err != nil {
		s.writer = nil
		s.file = nil
	}
}

// Multi fans one event out to several loggers, so a CLI run can print to stderr
// and write the file from the same call sites.
func Multi(loggers ...Logger) Logger {
	active := make([]Logger, 0, len(loggers))
	for _, logger := range loggers {
		if logger != nil {
			active = append(active, logger)
		}
	}
	if len(active) == 0 {
		return Nop()
	}
	return multiLogger(active)
}

type multiLogger []Logger

func (m multiLogger) Enabled(level Level) bool {
	for _, logger := range m {
		if logger.Enabled(level) {
			return true
		}
	}
	return false
}

func (m multiLogger) Event(name string, fields map[string]interface{}) {
	for _, logger := range m {
		logger.Event(name, fields)
	}
}

func (m multiLogger) SetGame(gameID string) {
	for _, logger := range m {
		logger.SetGame(gameID)
	}
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -count=1 ./pkg/trace/ && gofmt -l pkg/trace/`
Expected: PASS. Rotation is checked after a write, so a file that passes the ceiling rotates on the write that crossed it, not the next one.

- [ ] **Step 5: Commit**

```bash
git add pkg/trace/file.go pkg/trace/file_test.go
git commit -m "feat(trace): append JSONL to one file, rotating by size"
```

---

### Task 4: Trace configuration

**Files:**
- Modify: `pkg/config/types.go`
- Test: `pkg/config/types_test.go`

**Interfaces:**
- Produces: `config.PreferencesConfig.TraceLevel string`
- Produces: `config.AgentsConfig` fields `TracePayloadChars`, `TraceMaxBytes`, `TraceMaxFiles`, `TraceRotateCheck`, `TraceChunkLimit`
- Produces: `(*Config).TraceLevel() string`, `(*Config).TracePayloadChars() int`, `(*Config).TraceMaxBytes() int64`, `(*Config).TraceMaxFiles() int`, `(*Config).TraceRotateCheck() int`, `(*Config).TraceChunkLimit() int`

This task also adds `agents.trace_chunk_limit` to the spec's config table in section 11, because the spec's levels table refers to it without listing it.

- [ ] **Step 1: Write the failing test**

Append to `pkg/config/types_test.go`:

```go
func TestTraceSettingsHaveDefaults(t *testing.T) {
	empty := &Config{}

	if got := empty.TraceLevel(); got != "off" {
		t.Errorf("TraceLevel() = %q, want off", got)
	}
	if got := empty.TracePayloadChars(); got != 20000 {
		t.Errorf("TracePayloadChars() = %d, want 20000", got)
	}
	// Generous on purpose: tracing is opt-in, so this bounds a debug session left
	// running rather than rationing normal play.
	if got := empty.TraceMaxBytes(); got != 268435456 {
		t.Errorf("TraceMaxBytes() = %d, want 268435456", got)
	}
	if got := empty.TraceMaxFiles(); got != 3 {
		t.Errorf("TraceMaxFiles() = %d, want 3", got)
	}
	if got := empty.TraceRotateCheck(); got != 200 {
		t.Errorf("TraceRotateCheck() = %d, want 200", got)
	}
	if got := empty.TraceChunkLimit(); got != 500 {
		t.Errorf("TraceChunkLimit() = %d, want 500", got)
	}

	configured := &Config{
		Preferences: PreferencesConfig{TraceLevel: "full"},
		Agents: AgentsConfig{
			TracePayloadChars: 5000,
			TraceMaxBytes:     1024,
			TraceMaxFiles:     1,
			TraceRotateCheck:  10,
			TraceChunkLimit:   20,
		},
	}
	if got := configured.TraceLevel(); got != "full" {
		t.Errorf("TraceLevel() = %q, want full", got)
	}
	if got := configured.TraceMaxBytes(); got != 1024 {
		t.Errorf("TraceMaxBytes() = %d, want 1024", got)
	}
	if got := configured.TraceChunkLimit(); got != 20 {
		t.Errorf("TraceChunkLimit() = %d, want 20", got)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestTraceSettingsHaveDefaults ./pkg/config/`
Expected: FAIL — `empty.TraceLevel undefined`.

- [ ] **Step 3: Write the implementation**

In `pkg/config/types.go`, extend `PreferencesConfig`:

```go
type PreferencesConfig struct {
	Streaming        bool   `yaml:"streaming" json:"streaming"`
	TypingSpeedMS    int    `yaml:"typing_speed_ms" json:"typing_speed_ms"`
	CinematicEffects bool   `yaml:"cinematic_effects" json:"cinematic_effects"`
	FontScale        string `yaml:"font_scale" json:"font_scale"`
	// TraceLevel is "off", "summary", or "full". Off is the default so normal
	// play writes nothing.
	TraceLevel string `yaml:"trace_level" json:"trace_level"`
}
```

Extend `AgentsConfig`:

```go
	// Tracing bounds. Opt-in, so these guard against a debug session left running
	// rather than rationing normal play.
	TracePayloadChars int   `yaml:"trace_payload_chars" json:"trace_payload_chars"`
	TraceMaxBytes     int64 `yaml:"trace_max_bytes" json:"trace_max_bytes"`
	TraceMaxFiles     int   `yaml:"trace_max_files" json:"trace_max_files"`
	TraceRotateCheck  int   `yaml:"trace_rotate_check" json:"trace_rotate_check"`
	TraceChunkLimit   int   `yaml:"trace_chunk_limit" json:"trace_chunk_limit"`
```

Add accessors beside the existing timeout accessors:

```go
// TraceLevel is the configured trace detail, defaulting to off.
func (c *Config) TraceLevel() string {
	if strings.TrimSpace(c.Preferences.TraceLevel) == "" {
		return "off"
	}
	return c.Preferences.TraceLevel
}

// TracePayloadChars caps any single recorded string.
func (c *Config) TracePayloadChars() int {
	if c.Agents.TracePayloadChars <= 0 {
		return 20000
	}
	return c.Agents.TracePayloadChars
}

// TraceMaxBytes is the size at which the trace rotates.
func (c *Config) TraceMaxBytes() int64 {
	if c.Agents.TraceMaxBytes <= 0 {
		return 268435456
	}
	return c.Agents.TraceMaxBytes
}

// TraceMaxFiles is how many rotated trace files are kept.
func (c *Config) TraceMaxFiles() int {
	if c.Agents.TraceMaxFiles <= 0 {
		return 3
	}
	return c.Agents.TraceMaxFiles
}

// TraceRotateCheck is how many events pass between rotation checks.
func (c *Config) TraceRotateCheck() int {
	if c.Agents.TraceRotateCheck <= 0 {
		return 200
	}
	return c.Agents.TraceRotateCheck
}

// TraceChunkLimit bounds how many wire or chunk events one provider call records.
func (c *Config) TraceChunkLimit() int {
	if c.Agents.TraceChunkLimit <= 0 {
		return 500
	}
	return c.Agents.TraceChunkLimit
}
```

`strings` is already imported in that file; if not, add it.

- [ ] **Step 4: Update the spec's config table**

In `docs/superpowers/specs/2026-09-22-narrative-coherence-and-trace-design.md`, section 11, add:

```markdown
| `agents.trace_chunk_limit` | 500 | Wire or chunk events recorded per provider call |
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test -count=1 ./pkg/config/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/config/types.go pkg/config/types_test.go docs/superpowers/specs/2026-09-22-narrative-coherence-and-trace-design.md
git commit -m "feat(config): add the trace level and its bounds"
```

---

### Task 5: Trace the provider call

**Files:**
- Modify: `pkg/harness/http_provider.go`
- Modify: `pkg/harness/cli_provider.go`
- Modify: `pkg/harness/oracle_provider.go`
- Modify: `pkg/harness/factory.go`
- Test: `pkg/harness/http_provider_test.go`, `pkg/harness/cli_provider_test.go`

**Interfaces:**
- Consumes: `trace.Logger`, `trace.Nop`, `trace.OrNil` (Task 1), `trace.Memory` (Task 1)
- Produces: `(*HTTPProvider).SetLogger(trace.Logger)`, `(*CLIProvider).SetLogger(trace.Logger)`, `(*NarrativeOracleProvider).SetLogger(trace.Logger)`
- Produces: `harness.NewHTTPProviderWithLogger(id, endpoint, model, apiKey string, opts GenerationOptions, logger trace.Logger) *HTTPProvider`
- Produces: `harness.NewCLIProviderWithLogger(id, command string, args []string, opts GenerationOptions, logger trace.Logger) *CLIProvider`
- Produces: `harness.NewModelProviderWithLogger(id string, cfg ProviderConfig, logger trace.Logger) (ModelProvider, error)`
- Produces: `harness.RouterFromConfigWithLogger(cfg *config.Config, logger trace.Logger) (*Router, error)`
- Produces: `harness.ExtractorFromConfigWithLogger(cfg *config.Config, router *Router, logger trace.Logger) *Extractor`
- Produces: a logger-aware extractor via `(*Extractor).SetLogger(trace.Logger)`

The existing constructors and factory stay as they are and delegate with `trace.Nop()`, so no existing test changes.

- [ ] **Step 1: Write the failing test**

Append to `pkg/harness/http_provider_test.go`:

```go
func TestHTTPProviderTracesTheEnvelopeButNeverThePromptAtSummary(t *testing.T) {
	sent := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprintf(w, "data: [DONE]\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		sent <- struct{}{}
	}))
	defer server.Close()

	memory := trace.NewMemory(trace.LevelSummary)
	provider := NewHTTPProviderWithLogger("gm", server.URL, "gemma", "sk-secret-key", GenerationOptions{MaxTokens: 256}, memory)

	out := make(chan StreamChunk, 10)
	errCh := make(chan error, 1)
	go func() {
		errCh <- provider.Stream(context.Background(), GenerateRequest{Prompt: "the whole prompt"}, out)
	}()
	for range out {
	}
	if err := <-errCh; err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	request, ok := memory.Find("provider.request")
	if !ok {
		t.Fatalf("expected a provider.request event, got %v", memory.Names())
	}
	if _, present := request.Fields["prompt"]; present {
		t.Errorf("summary level must not carry the prompt")
	}
	if request.Fields["prompt_sha256"] == nil || request.Fields["prompt_chars"] != 15 {
		t.Errorf("expected a prompt hash and length, got %+v", request.Fields)
	}
	if request.Fields["auth_set"] != true {
		t.Errorf("expected auth_set to record that a key was used, got %+v", request.Fields)
	}
	if request.Fields["max_tokens"] != 256 {
		t.Errorf("expected the envelope's max_tokens, got %+v", request.Fields)
	}

	// The key itself must not appear anywhere, at any level.
	for _, event := range memory.Events() {
		for key, value := range event.Fields {
			if text, ok := value.(string); ok && strings.Contains(text, "sk-secret-key") {
				t.Errorf("event %s field %s leaked the API key", event.Name, key)
			}
		}
	}

	if _, ok := memory.Find("provider.response"); !ok {
		t.Errorf("expected a provider.response event, got %v", memory.Names())
	}
}

func TestHTTPProviderRecordsRawWireLinesOnlyAtFull(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	call := func(level trace.Level) []string {
		memory := trace.NewMemory(level)
		provider := NewHTTPProviderWithLogger("gm", server.URL, "gemma", "", GenerationOptions{}, memory)
		out := make(chan StreamChunk, 10)
		errCh := make(chan error, 1)
		go func() { errCh <- provider.Stream(context.Background(), GenerateRequest{Prompt: "hi"}, out) }()
		for range out {
		}
		if err := <-errCh; err != nil {
			t.Fatalf("Stream failed: %v", err)
		}
		return memory.Names()
	}

	for _, name := range call(trace.LevelFull) {
		if name == "provider.wire" {
			return
		}
	}
	t.Errorf("expected provider.wire at full level, got %v", call(trace.LevelFull))

	for _, name := range call(trace.LevelSummary) {
		if name == "provider.wire" {
			t.Errorf("did not expect provider.wire at summary level")
		}
	}
}
```

Add `"github.com/darkliquid/localrpg/pkg/trace"` to that file's imports.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestHTTPProviderTraces ./pkg/harness/`
Expected: FAIL — `undefined: NewHTTPProviderWithLogger`.

- [ ] **Step 3: Implement provider tracing**

In `pkg/harness/http_provider.go`, add the field, the logger-aware constructor, `SetLogger`, and the two events:

```go
type HTTPProvider struct {
	id       string
	endpoint string
	model    string
	apiKey   string
	opts     GenerationOptions
	client   *http.Client
	logger   trace.Logger
}

func NewHTTPProviderWithLogger(id, endpoint, model, apiKey string, opts GenerationOptions, logger trace.Logger) *HTTPProvider {
	provider := NewHTTPProviderWithOptions(id, endpoint, model, apiKey, opts)
	provider.SetLogger(logger)
	return provider
}

// SetLogger attaches a trace sink. It is safe to pass nil.
func (h *HTTPProvider) SetLogger(logger trace.Logger) {
	h.logger = trace.OrNil(logger)
}
```

In `Stream`, initialise `h.logger = trace.OrNil(h.logger)` before the request, then record the envelope, the wire lines, and the outcome:

```go
	start := time.Now()
	h.logger = trace.OrNil(h.logger)
	wireLines := 0

	h.logger.Event("provider.request", map[string]interface{}{
		"role":          h.id,
		"kind":          "http",
		"model":         h.model,
		"endpoint":      h.endpoint,
		"temperature":   temperature,
		"max_tokens":    maxTokens,
		"stream":        true,
		"auth_set":      h.apiKey != "",
		"prompt_sha256": hashPrompt(req.Prompt),
		"prompt_chars":  len([]rune(req.Prompt)),
	})
```

Inside the scan loop, before parsing each line:

```go
		if h.logger.Enabled(trace.LevelFull) && wireLines < h.chunkLimit() {
			wireLines++
			h.logger.Event("provider.wire", map[string]interface{}{
				"role":      h.id,
				"direction": "recv",
				"line":      line,
			})
		}
```

After the loop, where `out <- StreamChunk{Done: true, FinishReason: finishReason}` is today:

```go
	h.logger.Event("provider.response", map[string]interface{}{
		"role":          h.id,
		"finish_reason": finishReason,
		"first_token_ms": firstToken.Milliseconds(),
		"total_ms":      time.Since(start).Milliseconds(),
		"chunks":        chunkCount,
		"bytes":         byteCount,
	})
```

where `firstToken` is set the first time a delta arrives, `chunkCount` counts content deltas, and `byteCount` accumulates `len(chunk.Text)`. On the three error returns in `Stream`, record first:

```go
	h.logger.Event("provider.error", map[string]interface{}{"role": h.id, "error": err.Error()})
```

Add the two shared helpers at the bottom of the file:

```go
// hashPrompt identifies a prompt without recording it twice. The prompt itself is
// recorded once, on context.assembled.
func hashPrompt(prompt string) string {
	sum := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(sum[:])
}

// chunkLimit is the per-call cap on wire or chunk events. The default applies when
// no configuration has been threaded through, which keeps tests honest.
func (h *HTTPProvider) chunkLimit() int {
	if h.chunkLimitOverride <= 0 {
		return 500
	}
	return h.chunkLimitOverride
}
```

with `chunkLimitOverride int` on the struct and a setter `SetChunkLimit(int)` so the orchestrator can pass `cfg.TraceChunkLimit()`.

- [ ] **Step 4: Implement CLI and oracle tracing the same way**

In `pkg/harness/cli_provider.go`: add a `logger trace.Logger` field, `NewCLIProviderWithLogger`, `SetLogger`, and record:

```go
	h.logger.Event("provider.request", map[string]interface{}{
		"role":        c.id,
		"kind":        "cli",
		"command":     c.command,
		"arg_count":   len(c.args) + 1,
		"prompt_chars": len([]rune(req.Prompt)),
		"system_set":  req.System != "",
	})
```

and on clean exit `provider.response` with `exit_code: 0`, `stdout_chars`, `total_ms`; on failure `provider.error` with the exit status. The prompt is **not** logged as an argument: it is the final argument and would duplicate `context.assembled`.

In `pkg/harness/oracle_provider.go`: add the field, `SetLogger`, and a single `provider.response` event with `finish_reason: "stop"` and the character count, so a deterministic provider is visible in the trace as such.

In `pkg/harness/factory.go`, add the logger-aware variants:

```go
// NewModelProviderWithLogger is NewModelProvider with a trace sink attached.
func NewModelProviderWithLogger(id string, cfg ProviderConfig, logger trace.Logger) (ModelProvider, error) {
	provider, err := NewModelProvider(id, cfg)
	if err != nil {
		return nil, err
	}
	setProviderLogger(provider, logger)
	return provider, nil
}

// setProviderLogger attaches a logger to any provider that accepts one. Providers
// that do not are left alone rather than requiring the interface to grow.
func setProviderLogger(provider ModelProvider, logger trace.Logger) {
	if aware, ok := provider.(interface{ SetLogger(trace.Logger) }); ok {
		aware.SetLogger(logger)
	}
	if aware, ok := provider.(interface{ SetChunkLimit(int) }); ok {
		aware.SetChunkLimit(500)
	}
}
```

`RouterFromConfig` keeps its signature and delegates:

```go
func RouterFromConfig(cfg *config.Config) (*Router, error) {
	return RouterFromConfigWithLogger(cfg, trace.Nop())
}

// RouterFromConfigWithLogger builds the role-routed provider registry and
// attaches a trace sink to every provider it registers. It also applies the
// configured chunk limit, so wire events are capped without the orchestrator
// reaching into providers it does not own.
func RouterFromConfigWithLogger(cfg *config.Config, logger trace.Logger) (*Router, error) {
	router := NewRouter()

	for role, roleCfg := range cfg.Agents.Roles {
		if roleCfg.Type == "inherit" {
			continue
		}

		provider, err := NewModelProviderWithLogger(role, ProviderConfig{
			Type:        roleCfg.Type,
			BuiltinName: roleCfg.BuiltinName,
			Command:     roleCfg.Command,
			Args:        roleCfg.Args,
			Endpoint:    roleCfg.Endpoint,
			Model:       roleCfg.Model,
			APIKey:      roleCfg.APIKey,
			Temperature: roleCfg.Temperature,
			MaxTokens:   roleCfg.MaxTokens,
		}, logger)
		if err != nil {
			continue
		}
		if aware, ok := provider.(interface{ SetChunkLimit(int) }); ok {
			aware.SetChunkLimit(cfg.TraceChunkLimit())
		}
		if aware, ok := provider.(interface{ SetGame(string) }); ok {
			// Providers do not know the campaign; the sink stamps it instead, so
			// nothing is needed here. This branch exists only to keep the shape of
			// the factory explicit for future per-campaign providers.
			_ = aware
		}

		router.RegisterProvider(provider)
		router.AssignRole(role, role)
	}

	for role, fallback := range cfg.Agents.Fallbacks {
		if fallback != "" {
			router.SetFallback(role, fallback)
		}
	}

	if _, err := router.GetProviderForRole(config.RoleGM); err != nil {
		router.RegisterProvider(NewCLIProviderWithLogger("default-echo", "echo", []string{}, GenerationOptions{}, logger))
		router.AssignRole(config.RoleGM, "default-echo")
	}

	return router, nil
}
```

`ExtractorFromConfigWithLogger` follows the same shape, attaching the logger to the extractor and to any provider it builds itself.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/harness/ && go vet ./pkg/harness/`
Expected: PASS, including every pre-existing provider test.

- [ ] **Step 6: Commit**

```bash
git add pkg/harness/
git commit -m "feat(harness): trace provider requests, wire lines, and responses"
```

---

### Task 6: Trace context assembly and the turn pipeline

**Files:**
- Modify: `pkg/harness/context.go`
- Modify: `pkg/harness/extractor.go`
- Modify: `pkg/engine/orchestrator.go`
- Test: `pkg/harness/context_test.go`, `pkg/engine/orchestrator_trace_test.go` (create)

**Interfaces:**
- Consumes: `trace.Logger`, `trace.Memory`, `trace.LevelFull` (Task 1), `harness.MatchExistingEntity` (existing), `harness.NewHTTPProviderWithLogger` etc. (Task 5)
- Produces: `(*ContextAssembler).SetLogger(trace.Logger)`
- Produces: `(*Extractor).SetLogger(trace.Logger)`
- Produces: `(*TurnOrchestrator).SetLogger(trace.Logger)`
- Produces: events `context.assembled`, `turn.begin`, `generation.complete`, `segment.build`, `extraction.request`, `extraction.result`, `extraction.reconcile`, `record.turn`

- [ ] **Step 1: Write the failing test**

Create `pkg/engine/orchestrator_trace_test.go`:

```go
package engine

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/trace"
)

func TestATurnProducesAnOrderedTrace(t *testing.T) {
	memory := trace.NewMemory(trace.LevelFull)
	provider := &scriptedStreamProvider{chunks: []string{"The docks are quiet."}}
	orchestrator, _, _ := streamingOrchestrator(t, provider)
	orchestrator.SetLogger(memory)

	if _, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I look around", nil); err != nil {
		t.Fatalf("ProcessActionStream failed: %v", err)
	}

	order := []string{
		"turn.begin",
		"context.assembled",
		"generation.complete",
		"segment.build",
		"record.turn",
	}
	names := memory.Names()
	position := 0
	for _, name := range names {
		if position < len(order) && name == order[position] {
			position++
		}
	}
	if position != len(order) {
		t.Fatalf("trace is missing events in order: got %v, wanted %v", names, order)
	}

	assembled, ok := memory.Find("context.assembled")
	if !ok {
		t.Fatal("expected context.assembled")
	}
	if _, present := assembled.Fields["prompt"]; !present {
		t.Errorf("full level must record the assembled prompt, got %+v", assembled.Fields)
	}
	if assembled.Fields["estimated_tokens"] == nil {
		t.Errorf("expected an estimated token count, got %+v", assembled.Fields)
	}

	generated, _ := memory.Find("generation.complete")
	if generated.Fields["narration_chars"] != 20 {
		t.Errorf("narration_chars = %v, want 20", generated.Fields["narration_chars"])
	}
	if generated.Fields["truncated"] != false {
		t.Errorf("truncated = %v, want false", generated.Fields["truncated"])
	}

	segments, _ := memory.Find("segment.build")
	if segments.Fields["count"] != 1 {
		t.Errorf("segment count = %v, want 1", segments.Fields["count"])
	}

	recorded, _ := memory.Find("record.turn")
	if recorded.Fields["number"] != 1 {
		t.Errorf("recorded turn number = %v, want 1", recorded.Fields["number"])
	}
}

func TestTraceIsSilentWhenOff(t *testing.T) {
	memory := trace.NewMemory(trace.LevelOff)
	provider := &scriptedStreamProvider{chunks: []string{"Silence."}}
	orchestrator, _, _ := streamingOrchestrator(t, provider)
	orchestrator.SetLogger(memory)

	if _, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I wait", nil); err != nil {
		t.Fatal(err)
	}
	if len(memory.Events()) != 0 {
		t.Errorf("expected no events at level off, got %v", memory.Names())
	}
}
```

Note: `streamingOrchestrator` builds the orchestrator without a logger, which is exactly the nil case the plan must tolerate.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestATurnProducesAnOrderedTrace ./pkg/engine/`
Expected: FAIL — `orchestrator.SetLogger undefined`.

- [ ] **Step 3: Implement assembler and orchestrator tracing**

In `pkg/harness/context.go`, add `logger trace.Logger` to `ContextAssembler`, a `SetLogger`, and at the end of `AssembleContextWithProfiles`:

```go
	result := AssembleResult{Prompt: prompt, EstimatedTokens: estimateTokens(prompt), Trimmed: trimmed}
	c.logger = trace.OrNil(c.logger)
	c.logger.Event("context.assembled", map[string]interface{}{
		"estimated_tokens": result.EstimatedTokens,
		"budget":           c.limits.TokenBudget,
		"trimmed":          trimmed,
		"recall_turns":     len(turns),
		"prompt":           prompt,
	})
	return result, nil
```

In `pkg/harness/extractor.go`, add `logger trace.Logger`, a `SetLogger`, and inside `Extract` record the request before the call and the parse after it:

```go
	e.logger = trace.OrNil(e.logger)
	e.logger.Event("extraction.request", map[string]interface{}{
		"role": e.id(),
		"prompt": prompt,
		"prompt_chars": len([]rune(prompt)),
	})
	// … existing call and parse …
	e.logger.Event("extraction.result", map[string]interface{}{
		"entities":        len(result.Entities),
		"dialogue":        len(result.Dialogue),
		"player_location": result.PlayerLocation,
	})
```

If `Extractor` has no identity today, add `id string` set by `NewExtractor(provider)` to `provider.ID()`.

In `pkg/engine/orchestrator.go`, add `logger trace.Logger`, a `SetLogger`, and emit around the existing steps:

```go
// SetLogger attaches a trace sink. A nil logger records nothing. The assembler is
// told too, because it emits context.assembled from inside itself.
func (o *TurnOrchestrator) SetLogger(logger trace.Logger) {
	o.logger = trace.OrNil(logger)
	o.assembler.SetLogger(o.logger)
}
```

At the start of `ProcessActionStream`, after `turnNum` is known:

```go
	o.logger.Event("turn.begin", map[string]interface{}{
		"number":      turnNum,
		"mode":        mode,
		"input_chars": len([]rune(actionInput)),
	})
```

After `generate` returns:

```go
	o.logger.Event("generation.complete", map[string]interface{}{
		"narration_chars": len([]rune(narration)),
		"finish_reason":   finishReason,
		"truncated":       finishReason == "length",
	})
```

After segments are built:

```go
	o.logger.Event("segment.build", map[string]interface{}{
		"count":      len(turn.Segments),
		"kinds":      segmentKinds(turn.Segments),
		"speakers":   segmentSpeakers(turn.Segments),
		"unresolved": unresolvedSpeakers(turn.Segments),
	})
```

Before `RecordTurn`, classify what extraction proposed. `MatchExistingEntity` needs the store and answers whether a note already exists, so the orchestrator can say which entities a turn created:

```go
	matched := make([]string, 0, len(extraction.Entities))
	created := make([]string, 0, len(extraction.Entities))
	for i := range extraction.Entities {
		proposed := extraction.Entities[i]
		if existing := harness.MatchExistingEntity(o.store, &proposed); existing != nil {
			matched = append(matched, existing.ID)
			continue
		}
		id := proposed.ID
		if id == "" {
			id = entity.Slugify(proposed.Name)
		}
		created = append(created, id)
	}
	o.logger.Event("extraction.reconcile", map[string]interface{}{
		"matched": matched,
		"created": created,
	})
```

After `RecordTurn` succeeds:

```go
	o.logger.Event("record.turn", map[string]interface{}{
		"number":    turn.Number,
		"location":  turn.Location,
		"entities":  len(turn.Entities),
		"outcome":   turn.Outcome,
		"narration_chars": len([]rune(turn.Narration)),
	})
```

Add the three small helpers at the bottom of the file:

```go
func segmentKinds(segments []entity.TurnSegment) []string {
	kinds := make([]string, 0, len(segments))
	for _, segment := range segments {
		kinds = append(kinds, segment.Kind)
	}
	return kinds
}

func segmentSpeakers(segments []entity.TurnSegment) []string {
	speakers := make([]string, 0, len(segments))
	for _, segment := range segments {
		if segment.Speaker != "" {
			speakers = append(speakers, segment.Speaker)
		}
	}
	return speakers
}

func unresolvedSpeakers(segments []entity.TurnSegment) []string {
	unresolved := make([]string, 0)
	for _, segment := range segments {
		if segment.Kind == entity.SegmentSpeech && segment.SpeakerID == "" {
			unresolved = append(unresolved, segment.Speaker)
		}
	}
	return unresolved
}
```

- [ ] **Step 4: Update the spec's event catalogue**

In the coherence spec section 9.1, change the `extraction.result` row to drop `matched[]` and `created[]`, and add:

```markdown
| `extraction.reconcile` | matched[], created[] |
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/engine/ ./pkg/harness/ && go vet ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/harness/ pkg/engine/ docs/superpowers/specs/
git commit -m "feat(engine): trace context, generation, segments, and the write"
```

---

### Task 7: Trace media and playback

**Files:**
- Modify: `pkg/media/tts.go`
- Modify: `pkg/media/playback/player.go`
- Modify: `pkg/gui/service.go`
- Test: `pkg/media/tts_test.go`, `pkg/media/playback/player_test.go`

**Interfaces:**
- Consumes: `trace.Logger`, `trace.OrNil` (Task 1), `trace.Memory` (Task 1)
- Produces: `(*TTSPipeline).SetLogger(trace.Logger)`
- Produces: `(*playback.Player).SetLogger(trace.Logger)`
- Produces: events `media.tts.request`, `media.tts.result`, `media.stt.request`, `media.stt.result`, `media.image.request`, `media.image.result`, `audio.play`

- [ ] **Step 1: Write the failing test**

Append to `pkg/media/tts_test.go`:

```go
func TestTTSPipelineTracesCacheHitsAndMisses(t *testing.T) {
	dir := t.TempDir()
	memory := trace.NewMemory(trace.LevelFull)
	client := &mockTTSClient{}
	pipeline := NewTTSPipeline(client, NewContentCache(dir))
	pipeline.SetLogger(memory)

	voice := &entity.VoiceConfig{VoiceID: "af_bella", Pitch: 1, SpeechRate: 1}
	if _, err := pipeline.SynthesizeUtterance(context.Background(), "elena", voice, "Hello."); err != nil {
		t.Fatalf("SynthesizeUtterance failed: %v", err)
	}
	if _, err := pipeline.SynthesizeUtterance(context.Background(), "elena", voice, "Hello."); err != nil {
		t.Fatalf("second SynthesizeUtterance failed: %v", err)
	}

	results := make([]trace.Event, 0)
	for _, event := range memory.Events() {
		if event.Name == "media.tts.result" {
			results = append(results, event)
		}
	}
	if len(results) != 2 {
		t.Fatalf("expected two TTS results, got %d", len(results))
	}
	if results[0].Fields["cache_hit"] != false {
		t.Errorf("first synthesis should be a miss, got %+v", results[0].Fields)
	}
	if results[1].Fields["cache_hit"] != true {
		t.Errorf("second synthesis should be a hit, got %+v", results[1].Fields)
	}

	request, ok := memory.Find("media.tts.request")
	if !ok {
		t.Fatalf("expected a TTS request event")
	}
	if request.Fields["voice_id"] != "af_bella" || request.Fields["chars"] != 6 {
		t.Errorf("unexpected request fields: %+v", request.Fields)
	}
	if request.Fields["cache_key"] == nil {
		t.Errorf("expected the cache key so a clip can be found, got %+v", request.Fields)
	}
}
```

Append to `pkg/media/playback/player_test.go`:

```go
func TestPlayerTracesWhatItPlayed(t *testing.T) {
	player, err := OpenWithBackends(0.5, []mago.Backend{mago.BackendNull})
	if err != nil {
		t.Fatalf("open player: %v", err)
	}
	defer func() { _ = player.Close() }()

	memory := trace.NewMemory(trace.LevelSummary)
	player.SetLogger(memory)

	path := writeToneWAV(t, t.TempDir(), "clip.wav", deviceSampleRate, 30*time.Millisecond)
	if err := player.PlayFiles([]string{path}); err != nil {
		t.Fatalf("PlayFiles failed: %v", err)
	}

	event, ok := memory.Find("audio.play")
	if !ok {
		t.Fatalf("expected an audio.play event, got %v", memory.Names())
	}
	if event.Fields["clips"] != 1 {
		t.Errorf("clips = %v, want 1", event.Fields["clips"])
	}
}
```

Add the `trace` import to both test files.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test -run 'TestTTSPipelineTraces|TestPlayerTraces' ./pkg/media/...`
Expected: FAIL — `pipeline.SetLogger undefined`.

- [ ] **Step 3: Implement TTS and playback tracing**

In `pkg/media/tts.go`, add `logger trace.Logger` to `TTSPipeline`, `SetLogger`, and trace both branches of `SynthesizeUtterance`:

```go
	p.logger = trace.OrNil(p.logger)
	start := time.Now()

	base := ComputeAudioCacheKey(speakerID, voiceHash, text)
	voiceID := ""
	if voice != nil {
		voiceID = voice.VoiceID
	}
	p.logger.Event("media.tts.request", map[string]interface{}{
		"speaker":   speakerID,
		"voice_id":  voiceID,
		"pitch":     pitch,
		"rate":      rate,
		"chars":     len([]rune(text)),
		"cache_key": base,
	})

	if path, ok := p.cachedClip(base); ok {
		p.logger.Event("media.tts.result", map[string]interface{}{
			"cache_hit": true,
			"duration_ms": time.Since(start).Milliseconds(),
		})
		return path, nil
	}

	// … existing synthesis …
	p.logger.Event("media.tts.result", map[string]interface{}{
		"cache_hit":    false,
		"bytes":        len(audioBytes),
		"content_type": AudioContentType(audioBytes),
		"duration_ms":  time.Since(start).Milliseconds(),
	})
```

In `pkg/media/playback/player.go`, add `logger trace.Logger`, `SetLogger`, and in `PlayFiles` after the queue is swapped:

```go
	p.logger = trace.OrNil(p.logger)
	p.logger.Event("audio.play", map[string]interface{}{
		"clips":  len(streamers),
		"volume": volume,
	})
```

- [ ] **Step 4: Implement media tracing at the service call sites**

Speech to text and image generation have several provider implementations, so instrumenting each would spread the same three lines across them. Trace at the call sites instead, in `pkg/gui/service.go`, which already owns both:

- in `TranscribeAudio`, emit `media.stt.request` with `bytes` before the call and `media.stt.result` with `chars` and `duration_ms` after it;
- in `GetLocationArt`, emit `media.image.request` with the `location`, `provider`, and `cache_key` before `store.SceneArt`, and `media.image.result` with bytes and `duration_ms` after.

`Service` gains the logger via `NewService(rootDir)` and a `SetLogger(trace.Logger)` method, stored as `s.logger`.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/media/... ./pkg/gui/`
Expected: PASS, including pre-existing media tests that never set a logger.

- [ ] **Step 6: Commit**

```bash
git add pkg/media/ pkg/gui/service.go
git commit -m "feat(media): trace speech synthesis, transcription, imagery, and playback"
```

---

### Task 8: The trace API

**Files:**
- Modify: `pkg/gui/service.go`
- Modify: `pkg/gui/server.go`
- Modify: `pkg/gui/types.go`
- Test: `pkg/gui/server_test.go`

**Interfaces:**
- Consumes: the file sink's path convention (`<cache>/trace/trace.jsonl`) and the `game` field the sink stamps (Task 3, Task 9)
- Produces: `(*Service).TraceEvents(limit int, gameID string) ([]TraceEventDTO, error)`
- Produces: `(*Service).ClearTrace() error`
- Produces: `TraceEventDTO{Time, Event, Level string; Fields map[string]interface{}}`
- Produces: routes `GET /api/trace`, `DELETE /api/trace`

- [ ] **Step 1: Write the failing test**

Append to `pkg/gui/server_test.go`:

```go
func TestTraceRouteReturnsTheMostRecentEvents(t *testing.T) {
	_, svc := turnFixture(t)
	server := NewServer(svc, http.NotFoundHandler())

	tracePath := filepath.Join(svc.GetResolver().CacheDir(), "trace", "trace.jsonl")
	if err := os.MkdirAll(filepath.Dir(tracePath), 0755); err != nil {
		t.Fatal(err)
	}
	lines := `{"ts":"2026-09-22T09:00:00.000Z","event":"turn.begin","level":"summary","game":"other","number":1}
{"ts":"2026-09-22T09:00:01.000Z","event":"context.assembled","level":"summary","game":"campaign-01","tokens":100}
{"ts":"2026-09-22T09:00:02.000Z","event":"record.turn","level":"summary","game":"campaign-01","number":1}
`
	if err := os.WriteFile(tracePath, []byte(lines), 0600); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/trace?limit=2&game=campaign-01", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("trace: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var events []TraceEventDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &events); err != nil {
		t.Fatalf("decode trace: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 filtered events, got %d: %+v", len(events), events)
	}
	if events[0].Event != "context.assembled" || events[1].Event != "record.turn" {
		t.Errorf("unexpected ordering: %+v", events)
	}
	if events[1].Fields["number"] != float64(1) {
		t.Errorf("fields must survive the mapping: %+v", events[1].Fields)
	}

	// Deleting clears what the view reads.
	req = httptest.NewRequest(http.MethodDelete, "/api/trace", nil)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/trace", nil)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	var after []TraceEventDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &after); err != nil {
		t.Fatal(err)
	}
	if len(after) != 0 {
		t.Errorf("expected the trace to be empty after delete, got %d", len(after))
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestTraceRoute ./pkg/gui/`
Expected: FAIL — `undefined: TraceEventDTO`.

- [ ] **Step 3: Implement the service methods**

In `pkg/gui/types.go`:

```go
// TraceEventDTO is one traced event. The event's own fields are nested rather than
// flattened so the envelope stays stable as the catalogue grows.
type TraceEventDTO struct {
	Time   string                 `json:"ts"`
	Event  string                 `json:"event"`
	Level  string                 `json:"level"`
	Fields map[string]interface{} `json:"fields,omitempty"`
}
```

In `pkg/gui/service.go`:

```go
// tracePath is where the sink appends, matching what the composition root builds.
func (s *Service) tracePath() string {
	return filepath.Join(s.resolver.CacheDir(), "trace", "trace.jsonl")
}

// TraceEvents returns the newest trace events, oldest first. Only the tail of the
// file is read: a full trace is tens of megabytes and a viewer never needs all of
// it.
func (s *Service) TraceEvents(limit int, gameID string) ([]TraceEventDTO, error) {
	if limit <= 0 || limit > 2000 {
		limit = 200
	}

	lines, err := tailLines(s.tracePath(), limit*4)
	if err != nil {
		if os.IsNotExist(err) {
			return []TraceEventDTO{}, nil
		}
		return nil, fmt.Errorf("read trace: %w", err)
	}

	events := make([]TraceEventDTO, 0, len(lines))
	for _, line := range lines {
		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue // a torn final line is not a failure
		}
		if gameID != "" {
			if game, ok := raw["game"].(string); ok && game != gameID {
				continue
			}
		}

		dto := TraceEventDTO{}
		if text, ok := raw["ts"].(string); ok {
			dto.Time = text
		}
		if text, ok := raw["event"].(string); ok {
			dto.Event = text
		}
		if text, ok := raw["level"].(string); ok {
			dto.Level = text
		}
		delete(raw, "ts")
		delete(raw, "event")
		delete(raw, "level")
		if len(raw) > 0 {
			dto.Fields = raw
		}
		events = append(events, dto)
	}

	if len(events) > limit {
		events = events[len(events)-limit:]
	}
	return events, nil
}

// ClearTrace removes the trace and its rotations.
func (s *Service) ClearTrace() error {
	base := s.tracePath()
	_ = os.Remove(base)
	for index := 1; index <= 32; index++ {
		_ = os.Remove(fmt.Sprintf("%s.%d", base, index))
	}
	return nil
}
```

Add `tailLines` in the same file. It reads the file backwards in blocks so a 256 MiB trace is not loaded to show the last page:

```go
// tailLines returns up to want lines from the end of a file, oldest first.
func tailLines(path string, want int) ([]string, error) {
	if want <= 0 {
		return nil, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}

	const block = 64 * 1024
	remaining := info.Size()
	buffer := make([]byte, 0, block)
	newlines := 0

	var chunk []byte
	for remaining > 0 && newlines <= want {
		readSize := int64(block)
		if remaining < readSize {
			readSize = remaining
		}
		remaining -= readSize

		chunk = make([]byte, readSize)
		if _, err := file.ReadAt(chunk, remaining); err != nil {
			return nil, err
		}
		buffer = append(chunk, buffer...)
		newlines = bytes.Count(buffer, []byte{'\n'})
	}

	lines := strings.Split(strings.TrimRight(string(buffer), "\n"), "\n")
	if len(lines) > want {
		lines = lines[len(lines)-want:]
	}
	return lines, nil
}
```

- [ ] **Step 4: Implement the routes**

In `pkg/gui/server.go`, register the exact path beside `/api/games`:

```go
	s.mux.HandleFunc("/api/trace", s.handleTraceRoute)
```

and implement:

```go
// handleTraceRoute serves the recorded trace. It is a developer view, so it is
// read-only apart from being able to clear it.
func (s *Server) handleTraceRoute(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		events, err := s.service.TraceEvents(limit, r.URL.Query().Get("game"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, events)

	case http.MethodDelete:
		if err := s.service.ClearTrace(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.NotFound(w, r)
	}
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test -count=1 ./pkg/gui/ && go vet ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/gui/
git commit -m "feat(gui): serve the recorded trace over the API"
```

---

### Task 9: The composition root and `--trace`

**Files:**
- Modify: `cmd/localrpg/gui.go`
- Modify: `cmd/localrpg/play.go`
- Modify: `pkg/gui/service.go`
- Test: `cmd/localrpg/gui_test.go` (create)

**Interfaces:**
- Consumes: `trace.NewFileSink`, `trace.FileOptions`, `trace.Multi`, `trace.ParseLevel` (Tasks 1-3), the config accessors (Task 4)
- Produces: `(*Service).SetLogger(trace.Logger)`
- Produces: `traceFlagLevel(args []string, configured string) (string, bool)` in `package main`

- [ ] **Step 1: Write the failing test**

Create `cmd/localrpg/gui_test.go`:

```go
package main

import "testing"

func TestTraceFlagDefaultsToFullWhenBare(t *testing.T) {
	cases := []struct {
		args       []string
		configured string
		want       string
		wantSet    bool
	}{
		{args: []string{"gui"}, configured: "off", want: "off", wantSet: false},
		{args: []string{"gui", "--trace"}, configured: "off", want: "full", wantSet: true},
		{args: []string{"gui", "--trace", "summary"}, configured: "off", want: "summary", wantSet: true},
		{args: []string{"gui", "--trace=full"}, configured: "off", want: "full", wantSet: true},
		{args: []string{"gui", "--trace", "--headless"}, configured: "off", want: "full", wantSet: true},
		{args: []string{"gui", "--port", "8080"}, configured: "summary", want: "summary", wantSet: false},
	}

	for _, testCase := range cases {
		got, set := traceFlagLevel(testCase.args, testCase.configured)
		if got != testCase.want || set != testCase.wantSet {
			t.Errorf("traceFlagLevel(%v, %q) = %q, %v; want %q, %v",
				testCase.args, testCase.configured, got, set, testCase.want, testCase.wantSet)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestTraceFlag ./cmd/localrpg/`
Expected: FAIL — `undefined: traceFlagLevel`.

- [ ] **Step 3: Implement the flag handling and the sink**

Create `cmd/localrpg/trace.go` in `package main`:

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// traceFlagLevel reads --trace from the argument list without registering it with
// flag, so a bare --trace can mean "full" rather than swallowing the next
// argument. The returned bool reports whether the flag was present at all.
func traceFlagLevel(args []string, configured string) (string, bool) {
	level := configured
	seen := false

	for index, arg := range args {
		if arg == "--trace" {
			seen = true
			level = "full"
			if index+1 < len(args) {
				next := args[index+1]
				if !strings.HasPrefix(next, "-") {
					if _, err := trace.ParseLevel(next); err == nil {
						level = next
					}
				}
			}
			continue
		}
		if strings.HasPrefix(arg, "--trace=") {
			seen = true
			level = strings.TrimPrefix(arg, "--trace=")
		}
	}
	return level, seen
}

// buildTraceLogger creates the sink a command should write to, under the cache
// directory the resolver owns. It returns a no-op logger when tracing is off, and
// never fails the command: a trace that cannot be opened is reported and dropped.
func buildTraceLogger(cfg *config.Config, args []string, cacheDir string) trace.Logger {
	levelName, explicit := traceFlagLevel(args, cfg.TraceLevel())
	if !explicit && levelName == "off" {
		return trace.Nop()
	}

	level, err := trace.ParseLevel(levelName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: unknown trace level %q, tracing is off\n", levelName)
		return trace.Nop()
	}
	if level == trace.LevelOff {
		return trace.Nop()
	}

	sink, err := trace.NewFileSink(filepath.Join(cacheDir, "trace", "trace.jsonl"), trace.FileOptions{
		Level:        level,
		MaxBytes:     cfg.TraceMaxBytes(),
		MaxFiles:     cfg.TraceMaxFiles(),
		RotateCheck:  cfg.TraceRotateCheck(),
		PayloadChars: cfg.TracePayloadChars(),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not open the trace file: %v\n", err)
		return trace.Nop()
	}

	fmt.Fprintf(os.Stderr, "Tracing at level %q to %s\n", level.String(), sink.Path())
	if explicit {
		return trace.Multi(sink, trace.NewStderr(level))
	}
	return sink
}
```

Add `trace.NewStderr(level)` in `pkg/trace` for the explicit flag case:

```go
// StderrLogger prints events as single lines. It is what makes `--trace` useful
// from a terminal without tailing a file.
type StderrLogger struct {
	mu    sync.Mutex
	level Level
	game  string
	out   io.Writer
}

func NewStderr(level Level) *StderrLogger {
	return &StderrLogger{level: level, out: os.Stderr}
}

func (l *StderrLogger) Enabled(level Level) bool {
	return l.level != LevelOff && level <= l.level
}

func (l *StderrLogger) Event(name string, fields map[string]interface{}) {
	if !l.Enabled(l.level) {
		return
	}
	if l.game != "" {
		fields["game"] = l.game
	}
	encoded, err := json.Marshal(Sanitize(fields, l.level, 20000))
	if err != nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintf(l.out, "trace %s %s\n", name, encoded)
}

// SetGame stamps later events with a campaign, matching the file sink.
func (l *StderrLogger) SetGame(gameID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.game = gameID
}
```

with a test in `pkg/trace/file_test.go` asserting one line per event and that payloads are omitted at summary.

- [ ] **Step 4: Attach the logger in both commands**

In `cmd/localrpg/gui.go`:

```go
	service := gui.NewService(*rootDir)
	// The resolver owns where caches live, so the sink follows it rather than
	// duplicating the relative-path resolution.
	logger := buildTraceLogger(cfg, os.Args, service.GetResolver().CacheDir())
	service.SetLogger(logger)
```

In `pkg/gui/service.go`, `SetLogger` stores it, and `prepareTurn` threads it through everything a turn touches:

```go
// SetLogger attaches a trace sink to the service and to every turn it prepares.
func (s *Service) SetLogger(logger trace.Logger) {
	s.logger = trace.OrNil(logger)
}
```

and inside `prepareTurn`, after the router and extractor exist:

```go
	orchestrator.SetLogger(s.logger)
	timeline.SetLogger(s.logger)
```

with `harness.RouterFromConfigWithLogger(cfg, s.logger)` and `harness.ExtractorFromConfigWithLogger(cfg, router, s.logger)` replacing the current calls. The router applies the chunk limit itself, so the orchestrator does not reach into providers it does not own. `prepareTurn` also stamps the campaign onto the trace:

```go
	s.logger.SetGame(gameID)
```

In `cmd/localrpg/play.go`, the same: build the logger, `timeline.SetLogger(logger)`, `orchestrator.SetLogger(logger)`, and pass the logger into `harness.RouterFromConfigWithLogger`.

- [ ] **Step 5: Run the tests and the full gate**

Run: `go test -count=1 ./... && go vet ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/localrpg/ pkg/trace/ pkg/gui/ pkg/engine/
git commit -m "feat(cmd): add --trace and wire the sink through both clients"
```

---

### Task 10: The Debug view

**Files:**
- Modify: `frontend/src/components/SettingsStudio.tsx`
- Modify: `frontend/src/api/client.ts`
- Modify: `frontend/src/types.ts`
- Test: `cd frontend && npx tsc --noEmit`

**Interfaces:**
- Consumes: `GET /api/trace`, `DELETE /api/trace` (Task 8), `preferences.trace_level` (Task 4)
- Produces: `APIClient.traceEvents(limit, gameID)`, `APIClient.clearTrace()`
- Produces: `TraceEvent` type in `frontend/src/types.ts`

- [ ] **Step 1: Add the client methods and types**

In `frontend/src/types.ts`:

```ts
export interface TraceEvent {
  ts: string;
  event: string;
  level: string;
  fields?: Record<string, unknown>;
}
```

and extend `PreferencesConfig` with `trace_level?: string`.

In `frontend/src/api/client.ts`:

```ts
  static async traceEvents(limit = 200, gameID?: string): Promise<TraceEvent[]> {
    const query = new URLSearchParams({ limit: String(limit) });
    if (gameID) query.set('game', gameID);
    const res = await fetch(`/api/trace?${query.toString()}`);
    if (!res.ok) throw new Error(`traceEvents: ${res.statusText}`);
    return res.json();
  }

  static async clearTrace(): Promise<void> {
    const res = await fetch('/api/trace', { method: 'DELETE' });
    if (!res.ok) throw new Error(`clearTrace: ${res.statusText}`);
  }
```

- [ ] **Step 2: Add the Debug tab**

In `SettingsStudio.tsx`, add `'debug'` to the sub-tab union and a fifth tab button beside Preferences. The panel:

- a level selector bound to `config.preferences.trace_level` with `off`, `summary`, and `full`, explaining that full records prompts and raw wire lines and that everything stays local;
- payload cap, rotation size, kept files, and chunk limit number inputs bound to the `config.agents` trace keys;
- a **Clear trace** button calling `APIClient.clearTrace()`;
- a **Refresh** button, a newest-first list of events showing `ts`, `event`, `level`, and a collapsible JSON body per event, and a separate collapsible **Raw provider lines** panel filtered to `provider.wire`, closed by default.

The list polls every second only while a turn is in flight; otherwise it loads on open and on Refresh. Keep the polling in one `useEffect` with a `setInterval` cleared on unmount.

- [ ] **Step 3: Typecheck and build**

Run: `cd frontend && npx tsc --noEmit && npm run build`
Expected: no type errors; a successful build. Restore `pkg/gui/dist/.gitkeep` afterwards and do not stage its deletion.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/
git commit -m "feat(frontend): add a Debug view for the trace"
```

---

## Self-Review

**Spec coverage** (coherence spec section 9, the trace increment):

| Spec item | Task |
| --- | --- |
| `trace.Logger` with `Enabled`/`Event`, `Nop` | 1 |
| Levels `off`/`summary`/`full` | 1, 4 |
| Redaction deny-list, applied by the sink | 2 |
| Payload cap with a marker | 2 |
| Single appended JSONL file, `0600`, flushed per event | 3 |
| Rotation by size, kept files, rotation-check cadence | 3 |
| Opt-in defaults, generous bounds | 3, 4 |
| Provider request/response/error, prompt recorded once as a hash | 5 |
| Raw wire lines at full, bounded by `trace_chunk_limit` | 5 |
| `context.assembled` with tokens, budget, trimmed, prompt | 6 |
| `turn.begin`, `generation.complete`, `segment.build`, `record.turn` | 6 |
| `extraction.request`/`result`, plus the new `extraction.reconcile` | 6 |
| `media.tts.*`, `media.stt.*`, `media.image.*`, `audio.play` | 7 |
| `GET`/`DELETE /api/trace` | 8 |
| `--trace` flag, stderr sink, level override | 9 |
| Debug view under Settings, with a separate wire panel | 10 |
| Tracing never fails a turn | 3, 9 |
| Deferred: `continuity.check`, `summary.regenerate`, `tool.*`, `sections[]` | stated in Scope |

**Placeholder scan:** no "TBD", no "add error handling", no "similar to Task N". The one substitution instruction (Task 3's `setProviderLogger`) shows the final form in the same step rather than leaving the reader to guess.

**Type consistency:** `trace.Logger`, `trace.Level`, `trace.FileOptions`, `trace.Sanitize`, `trace.Memory`, `trace.OrNil`, `trace.Multi`, and the `SetLogger` method name are used identically in every task. `TurnOrchestrator.SetLogger`, `Timeline.SetLogger`, `Service.SetLogger`, `TTSPipeline.SetLogger`, and `playback.Player.SetLogger` all take `trace.Logger` and are nil-safe.
