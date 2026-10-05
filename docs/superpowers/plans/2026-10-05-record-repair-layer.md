# Record Repair Layer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop malformed `@` control records from silently disappearing, by repairing structural JSON damage and assembling multi-line records.

**Architecture:** A new standard-library-only leaf `pkg/jsonrepair` provides a deterministic, bounded structural repair and a shared brace-depth scanner. `pkg/turnstream` calls it from `Parser.record`, gains a `Repaired` field on `Record`, accumulates multi-line records, and exposes a `RepairReport`. The extractor's inline brace trim is rewritten to use the leaf.

**Tech Stack:** Go (standard library only; tests use `testing` and `t.TempDir()`), goja is not involved.

**Spec:** `docs/superpowers/specs/2026-10-05-record-repair-layer-design.md`

## Global Constraints

- Go standard library only for tests; no testify. Errors wrapped with `fmt.Errorf("...: %w", err)`.
- Use `interface{}`, not `any` (the codebase is uniform on this; `go vet` must stay clean).
- Commits: Conventional Commits with a scope, subject under 72 chars (for example `fix(turnstream): repair malformed control records`).
- Repair must never invent content: it may remove, truncate, or close structure, never add a key.
- A payload that is already valid JSON must be returned byte-identical.
- `maxPayload` is 64 KiB; `maxRecordLines` is 32.

---

### Task 1: The `jsonrepair` leaf with a shared brace scanner

**Files:**
- Create: `pkg/jsonrepair/scan.go`
- Test: `pkg/jsonrepair/scan_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `func BraceDepth(b []byte) int`.

- [ ] **Step 1: Write the failing test**

```go
package jsonrepair

import "testing"

func TestBraceDepth(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{`{"a":1}`, 0},
		{`{"a":1`, 1},
		{`{"a":{"b":2`, 2},
		{`[1,2,3`, 1},
		{`{"a":"}{"}`, 0},
		{`{"a":"\""}`, 0},
		{`}`, -1},
	}
	for _, c := range cases {
		if got := BraceDepth([]byte(c.in)); got != c.want {
			t.Errorf("BraceDepth(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/jsonrepair/ -run TestBraceDepth -v`
Expected: FAIL, `undefined: BraceDepth`.

- [ ] **Step 3: Write minimal implementation**

Create `pkg/jsonrepair/scan.go`:

```go
// Package jsonrepair performs bounded, deterministic structural repair of JSON
// that a language model emitted with small formatting mistakes. It never
// invents content: it only removes, truncates, or closes structure.
package jsonrepair

// BraceDepth returns the net brace/bracket depth of b, ignoring characters
// inside JSON string literals and honouring backslash escapes. A negative
// result means the payload closes more structure than it opens.
func BraceDepth(b []byte) int {
	depth := 0
	inString := false
	escaped := false
	for i := 0; i < len(b); i++ {
		c := b[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch c {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		}
	}
	return depth
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/jsonrepair/ -run TestBraceDepth -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/jsonrepair/scan.go pkg/jsonrepair/scan_test.go
git commit -m "feat(jsonrepair): add shared brace-depth scanner"
```

---

### Task 2: `Repair` and its structural steps

**Files:**
- Create: `pkg/jsonrepair/repair.go`
- Test: `pkg/jsonrepair/repair_test.go`

**Interfaces:**
- Consumes: `BraceDepth` (Task 1).
- Produces: `type Kind string`, `const KindNone/KindFence/KindTrim/KindClose/KindTrailingComma`, `type Result struct { Payload []byte; Kind Kind; OK bool }`, `func Repair(payload []byte) Result`.

- [ ] **Step 1: Write the failing test**

```go
package jsonrepair

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRepair(t *testing.T) {
	cases := []struct {
		name string
		in   string
		kind Kind
		ok   bool
	}{
		{"valid untouched", `{"actor":"x"}`, KindNone, true},
		{"fenced", "```json\n{\"actor\":\"x\"}\n```", KindFence, true},
		{"prose trim", `here you go: {"actor":"x"} done`, KindTrim, true},
		{"unclosed closed", `{"actor":"x"`, KindClose, true},
		{"trailing comma", `{"actor":"x",}`, KindTrailingComma, true},
		{"garbage", `not json at all`, KindNone, false},
	}
	for _, c := range cases {
		got := Repair([]byte(c.in))
		if got.OK != c.ok {
			t.Errorf("%s: OK = %v, want %v", c.name, got.OK, c.ok)
		}
		if c.ok && !json.Valid(got.Payload) {
			t.Errorf("%s: repaired payload is not valid JSON: %q", c.name, got.Payload)
		}
		if c.ok && got.Kind != c.kind {
			t.Errorf("%s: Kind = %q, want %q", c.name, got.Kind, c.kind)
		}
	}
}

func TestRepairValidIsByteIdentical(t *testing.T) {
	in := []byte(`{"a":1,"b":[2,3]}`)
	got := Repair(in)
	if string(got.Payload) != string(in) {
		t.Errorf("valid payload changed: %q -> %q", in, got.Payload)
	}
}

func TestRepairRefusesOversize(t *testing.T) {
	big := []byte(strings.Repeat(" ", 64*1024+1))
	if got := Repair(big); got.OK {
		t.Fatal("oversize payload should be refused")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/jsonrepair/ -run TestRepair -v`
Expected: FAIL, `undefined: Repair`.

- [ ] **Step 3: Write minimal implementation**

Create `pkg/jsonrepair/repair.go`:

```go
package jsonrepair

import (
	"bytes"
	"encoding/json"
	"strings"
)

// maxPayload bounds the input so repair never scans unbounded data.
const maxPayload = 64 * 1024

// Kind names the structural repair that produced a valid payload.
type Kind string

const (
	KindNone          Kind = ""
	KindFence         Kind = "fence"
	KindTrim          Kind = "trim"
	KindClose         Kind = "close"
	KindTrailingComma Kind = "trailing_comma"
)

// Result reports what a repair did. Payload equals the input when OK is false.
type Result struct {
	Payload []byte
	Kind    Kind
	OK      bool
}

// Repair attempts a bounded, deterministic structural repair of a payload that
// is expected to be one JSON object or array. It returns the first candidate
// that validates, naming the step that produced it.
func Repair(payload []byte) Result {
	if len(payload) > maxPayload {
		return Result{Payload: payload}
	}
	if json.Valid(payload) {
		return Result{Payload: payload, Kind: KindNone, OK: true}
	}
	cur := payload
	if out, ok := stripFence(cur); ok {
		cur = out
		if json.Valid(cur) {
			return Result{Payload: cur, Kind: KindFence, OK: true}
		}
	}
	kind := KindNone
	if out, ok := trimToValue(cur); ok && !bytes.Equal(out, cur) {
		cur, kind = out, KindTrim
		if json.Valid(cur) {
			return Result{Payload: cur, Kind: kind, OK: true}
		}
	}
	if out, ok := closeStructures(cur); ok {
		cur = out
		if json.Valid(cur) {
			return Result{Payload: cur, Kind: KindClose, OK: true}
		}
	}
	if out, ok := removeTrailingCommas(cur); ok {
		cur = out
		if json.Valid(cur) {
			return Result{Payload: cur, Kind: KindTrailingComma, OK: true}
		}
	}
	return Result{Payload: payload}
}

// stripFence removes one Markdown code fence around the payload.
func stripFence(b []byte) ([]byte, bool) {
	s := strings.TrimSpace(string(b))
	if !strings.HasPrefix(s, "```") {
		return b, false
	}
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[i+1:]
	} else {
		s = strings.TrimPrefix(s, "```")
	}
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "```"))
	if s == "" {
		return b, false
	}
	return []byte(s), true
}

// trimToValue takes the substring from the first { or [ to its matching close,
// discarding prose before and after. When the structure is unterminated it
// returns the tail so closeStructures can finish it.
func trimToValue(b []byte) ([]byte, bool) {
	start := -1
	for i := 0; i < len(b); i++ {
		if b[i] == '{' || b[i] == '[' {
			start = i
			break
		}
	}
	if start < 0 {
		return b, false
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(b); i++ {
		c := b[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch c {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth == 0 {
				return b[start : i+1], true
			}
		}
	}
	return b[start:], true
}

// closeStructures appends the closing brackets implied by any open structures.
func closeStructures(b []byte) ([]byte, bool) {
	var stack []byte
	inString := false
	escaped := false
	for i := 0; i < len(b); i++ {
		c := b[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch c {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			stack = append(stack, '}')
		case '[':
			stack = append(stack, ']')
		case '}', ']':
			if len(stack) == 0 {
				return b, false
			}
			stack = stack[:len(stack)-1]
		}
	}
	if inString || len(stack) == 0 {
		return b, false
	}
	out := make([]byte, 0, len(b)+len(stack))
	out = append(out, b...)
	for i := len(stack) - 1; i >= 0; i-- {
		out = append(out, stack[i])
	}
	return out, true
}

// removeTrailingCommas drops commas that sit immediately before a } or ].
func removeTrailingCommas(b []byte) ([]byte, bool) {
	out := make([]byte, 0, len(b))
	inString := false
	escaped := false
	changed := false
	for i := 0; i < len(b); i++ {
		c := b[i]
		if inString {
			out = append(out, c)
			if escaped {
				escaped = false
				continue
			}
			switch c {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
			out = append(out, c)
		case ',':
			j := i + 1
			for j < len(b) && (b[j] == ' ' || b[j] == '\t' || b[j] == '\n' || b[j] == '\r') {
				j++
			}
			if j < len(b) && (b[j] == '}' || b[j] == ']') {
				changed = true
				continue
			}
			out = append(out, c)
		default:
			out = append(out, c)
		}
	}
	if !changed {
		return b, false
	}
	return out, true
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/jsonrepair/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/jsonrepair/repair.go pkg/jsonrepair/repair_test.go
git commit -m "feat(jsonrepair): add bounded structural JSON repair"
```

---

### Task 3: Parser integration and the `Repaired` field

**Files:**
- Modify: `pkg/turnstream/records.go:21-26`
- Modify: `pkg/turnstream/parser.go:216-241`
- Test: `pkg/turnstream/parser_test.go` (append)

**Interfaces:**
- Consumes: `jsonrepair.Repair`, `jsonrepair.Kind` (Task 2).
- Produces: `Record.Repaired jsonrepair.Kind`.

- [ ] **Step 1: Write the failing test**

```go
func TestParserRepairsMalformedRecord(t *testing.T) {
	p := NewParser(newTestRoster())
	p.Feed("@roll {\"actor\":\"x\",\"check_kind\":\"do\",}\n")
	recs := p.Records()
	if len(recs) != 1 {
		t.Fatalf("got %d records, want 1", len(recs))
	}
	if recs[0].Err != nil {
		t.Fatalf("record errored: %v", recs[0].Err)
	}
	if recs[0].Repaired != jsonrepair.KindTrailingComma {
		t.Fatalf("Repaired = %q, want %q", recs[0].Repaired, jsonrepair.KindTrailingComma)
	}
	if _, err := recs[0].DecodeRoll(); err != nil {
		t.Fatalf("decode repaired roll: %v", err)
	}
}

func TestParserKeepsUnrepairableRecord(t *testing.T) {
	p := NewParser(newTestRoster())
	p.Feed("@roll not json at all\n")
	recs := p.Records()
	if len(recs) != 1 || recs[0].Err == nil {
		t.Fatalf("expected one errored record, got %+v", recs)
	}
	if recs[0].Repaired != jsonrepair.KindNone {
		t.Fatalf("Repaired = %q, want empty", recs[0].Repaired)
	}
}
```

Add the import `"github.com/darkliquid/localrpg/pkg/jsonrepair"` to the test file. If
`newTestRoster` does not exist in the file, add a minimal roster:

```go
type testRoster struct{ m map[string]string }

func newTestRoster() *testRoster { return &testRoster{m: map[string]string{}} }
func (r *testRoster) Resolve(name string) (string, bool) { id, ok := r.m[name]; return id, ok }
func (r *testRoster) Declare(name, id string)            { r.m[name] = id }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/turnstream/ -run TestParserRepairs -v`
Expected: FAIL, `recs[0].Repaired undefined`.

- [ ] **Step 3: Write minimal implementation**

In `pkg/turnstream/records.go`, add the import `"github.com/darkliquid/localrpg/pkg/jsonrepair"` and
extend the struct:

```go
type Record struct {
	Type     string
	Payload  []byte
	Line     int
	Err      error
	Repaired jsonrepair.Kind
}
```

In `pkg/turnstream/parser.go`, replace the switch in `record` (lines 224-231) with:

```go
	switch {
	case !validRecordType(rec.Type):
		rec.Err = fmt.Errorf("unknown record type %q", rec.Type)
	case len(rec.Payload) == 0:
		rec.Err = fmt.Errorf("record %q has no payload", rec.Type)
	default:
		if res := jsonrepair.Repair(rec.Payload); res.OK {
			if res.Kind != jsonrepair.KindNone {
				rec.Repaired = res.Kind
				rec.Payload = res.Payload
			}
		} else {
			rec.Err = fmt.Errorf("record %q payload is not JSON", rec.Type)
		}
	}
```

Add `"github.com/darkliquid/localrpg/pkg/jsonrepair"` to the parser imports.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/turnstream/ -v`
Expected: PASS (existing parser tests plus the two new ones).

- [ ] **Step 5: Commit**

```bash
git add pkg/turnstream/records.go pkg/turnstream/parser.go pkg/turnstream/parser_test.go
git commit -m "fix(turnstream): repair malformed control records instead of dropping them"
```

---

### Task 4: Multi-line record accumulation

**Files:**
- Modify: `pkg/turnstream/parser.go` (struct, `Feed`, `consume`, `Flush`, `Reset`)
- Test: `pkg/turnstream/parser_test.go` (append)

**Interfaces:**
- Consumes: `jsonrepair.BraceDepth` (Task 1), `Parser.record` (Task 3).
- Produces: no new exported symbols; changes grouping only.

- [ ] **Step 1: Write the failing test**

```go
func TestParserAssemblesMultiLineRecord(t *testing.T) {
	p := NewParser(newTestRoster())
	p.Feed("@roll {\n")
	p.Feed("  \"actor\": \"x\",\n")
	p.Feed("  \"check_kind\": \"do\"\n")
	p.Feed("}\n")
	recs := p.Records()
	if len(recs) != 1 {
		t.Fatalf("got %d records, want 1", len(recs))
	}
	if recs[0].Err != nil {
		t.Fatalf("record errored: %v", recs[0].Err)
	}
	if recs[0].Repaired != jsonrepair.KindNone {
		t.Fatalf("Repaired = %q, want empty", recs[0].Repaired)
	}
	if req, err := recs[0].DecodeRoll(); err != nil || req.Actor != "x" {
		t.Fatalf("decode multi-line roll: %+v %v", req, err)
	}
}

func TestParserFlushReportsUnterminatedRecord(t *testing.T) {
	p := NewParser(newTestRoster())
	p.Feed("@roll {\n")
	p.Flush()
	recs := p.Records()
	if len(recs) != 1 || recs[0].Err == nil {
		t.Fatalf("expected one errored record, got %+v", recs)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/turnstream/ -run TestParserAssembles -v`
Expected: FAIL (the `{` line is treated as one malformed record and the rest as narration).

- [ ] **Step 3: Write minimal implementation**

Add fields and bounds to `Parser`:

```go
// record accumulation bounds
const (
	maxRecordLines = 32
	maxRecordBytes = 64 * 1024
)

// in Parser struct, alongside buf/pending/records/events:
	openRecord   string // accumulated "@type …" text awaiting balance
	openRecordLn int    // lines accumulated so far
```

In `consume`, before the `@` case, add a branch that either continues or starts an accumulation:

```go
	if p.openRecord != "" {
		p.openRecord += "\n" + trimmed
		p.openRecordLn++
		if jsonrepair.BraceDepth([]byte(p.openRecord)) <= 0 ||
			p.openRecordLn >= maxRecordLines || len(p.openRecord) > maxRecordBytes {
			line := p.openRecord
			p.openRecord, p.openRecordLn = "", 0
			return append(p.flushNarration(), p.record(line)...)
		}
		return nil
	}
```

Change the `@` case to defer when the remainder is unbalanced:

```go
	case strings.HasPrefix(trimmed, "@"):
		body := strings.TrimPrefix(trimmed, "@")
		if _, payload, _ := strings.Cut(body, " "); jsonrepair.BraceDepth([]byte(payload)) > 0 {
			p.openRecord = body
			p.openRecordLn = 1
			return p.flushNarration()
		}
		return append(p.flushNarration(), p.record(trimmed)...)
```

Note `p.record` expects the line including the leading `@`, so pass `trimmed`; the accumulated
`openRecord` stores the text after `@` and is re-prefixed before parsing:

```go
			line := "@" + p.openRecord
```

In `Flush`, parse any open accumulator before flushing narration:

```go
func (p *Parser) Flush() []Event {
	var out []Event
	if p.buf != "" {
		out = append(out, p.consume(p.buf)...)
		p.buf = ""
	}
	if p.openRecord != "" {
		line := "@" + p.openRecord
		p.openRecord, p.openRecordLn = "", 0
		out = append(out, p.record(line)...)
	}
	out = append(out, p.flushNarration()...)
	p.events = append(p.events, out...)
	return out
}
```

In `Reset`, clear the accumulator:

```go
	p.openRecord, p.openRecordLn = "", 0
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/turnstream/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/turnstream/parser.go pkg/turnstream/parser_test.go
git commit -m "fix(turnstream): assemble multi-line control records"
```

---

### Task 5: `RepairReport`

**Files:**
- Modify: `pkg/turnstream/parser.go`
- Test: `pkg/turnstream/parser_test.go` (append)

**Interfaces:**
- Consumes: `Record.Repaired`, `Record.Err` (Task 3).
- Produces: `type RepairReport struct { Total, Repaired, Failed int; Kinds map[jsonrepair.Kind]int }`, `func (p *Parser) RepairReport() RepairReport`.

- [ ] **Step 1: Write the failing test**

```go
func TestParserRepairReport(t *testing.T) {
	p := NewParser(newTestRoster())
	p.Feed("@roll {\"actor\":\"x\",}\n")
	p.Feed("@persona {\"name\":\"Vex\"}\n")
	p.Feed("@roll not json at all\n")
	r := p.RepairReport()
	if r.Total != 3 || r.Repaired != 1 || r.Failed != 1 {
		t.Fatalf("report = %+v, want total 3 repaired 1 failed 1", r)
	}
	if r.Kinds[jsonrepair.KindTrailingComma] != 1 {
		t.Fatalf("kinds = %+v, want one trailing_comma", r.Kinds)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/turnstream/ -run TestParserRepairReport -v`
Expected: FAIL, `undefined: RepairReport`.

- [ ] **Step 3: Write minimal implementation**

Append to `pkg/turnstream/parser.go`:

```go
// RepairReport summarises how records fared: how many were seen, repaired, and
// unrepairable, and the repair kinds applied. It is the data a diagnostics
// surface renders; it has no side effects.
type RepairReport struct {
	Total    int
	Repaired int
	Failed   int
	Kinds    map[jsonrepair.Kind]int
}

// RepairReport returns the current record-repair summary.
func (p *Parser) RepairReport() RepairReport {
	rep := RepairReport{Total: len(p.records), Kinds: map[jsonrepair.Kind]int{}}
	for _, rec := range p.records {
		if rec.Err != nil {
			rep.Failed++
			continue
		}
		if rec.Repaired != jsonrepair.KindNone {
			rep.Repaired++
			rep.Kinds[rec.Repaired]++
		}
	}
	return rep
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/turnstream/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/turnstream/parser.go pkg/turnstream/parser_test.go
git commit -m "feat(turnstream): expose a record repair report"
```

---

### Task 6: Move the extractor onto `jsonrepair`

**Files:**
- Modify: `pkg/harness/extractor.go:467-494`
- Test: `pkg/harness/extractor_test.go` (append)

**Interfaces:**
- Consumes: `jsonrepair.Repair` (Task 2).
- Produces: no signature change; the extractor's decode path now shares the leaf.

- [ ] **Step 1: Write the failing test**

```go
func TestDecodeExtractedRepairsFencedJSON(t *testing.T) {
	var out map[string]string
	err := decodeExtractedJSON([]byte("```json\n{\"name\":\"Vex\"}\n```"), &out)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["name"] != "Vex" {
		t.Fatalf("name = %q, want Vex", out["name"])
	}
}
```

Adjust the helper name to the real one in `pkg/harness/extractor.go` (the current code slices
braces inline inside the decode function; extract that inline logic into
`decodeExtractedJSON(payload []byte, v interface{}) error` as part of this task).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/harness/ -run TestDecodeExtracted -v`
Expected: FAIL, `undefined: decodeExtractedJSON`.

- [ ] **Step 3: Write minimal implementation**

In `pkg/harness/extractor.go`, replace the inline brace-slicing with:

```go
// decodeExtractedJSON decodes a JSON value a model returned, repairing fences,
// surrounding prose, and unclosed structure first.
func decodeExtractedJSON(payload []byte, v interface{}) error {
	res := jsonrepair.Repair(payload)
	if !res.OK {
		return fmt.Errorf("parse extracted json: not valid JSON")
	}
	if err := json.Unmarshal(res.Payload, v); err != nil {
		return fmt.Errorf("parse extracted json: %w", err)
	}
	return nil
}
```

Call it from the existing decode site and delete the inline `IndexAny`/`LastIndexAny` slicing.
Add `"github.com/darkliquid/localrpg/pkg/jsonrepair"` to the imports.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/harness/ -v`
Expected: PASS (existing extractor tests plus the new one).

- [ ] **Step 5: Commit**

```bash
git add pkg/harness/extractor.go pkg/harness/extractor_test.go
git commit -m "refactor(harness): reuse jsonrepair for extracted JSON"
```

---

### Task 7: Engine regression and fuzz target

**Files:**
- Test: `pkg/engine/streamsegments_test.go` (append)
- Create: `pkg/jsonrepair/fuzz_test.go`

**Interfaces:**
- Consumes: everything above.
- Produces: no production symbols.

- [ ] **Step 1: Write the failing test**

```go
func TestRepairedRollEndsTurn(t *testing.T) {
	o := &TurnOrchestrator{parser: turnstream.NewParser(newTestRoster())}
	o.parser.Feed("@roll {\"actor\":\"x\",\"check_kind\":\"do\",}\n")
	req, ok := o.pendingRoll()
	if !ok {
		t.Fatal("a repaired @roll should be pending")
	}
	if req.Actor != "x" {
		t.Fatalf("actor = %q, want x", req.Actor)
	}
}
```

Use the existing test roster helper in `pkg/engine` if one exists; otherwise add the same minimal
`testRoster` used in `pkg/turnstream`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/engine/ -run TestRepairedRollEndsTurn -v`
Expected: PASS once Tasks 3-4 land (this test documents the guarantee; if it fails, the parser
integration is wrong).

- [ ] **Step 3: Add the fuzz target**

Create `pkg/jsonrepair/fuzz_test.go`:

```go
package jsonrepair

import (
	"encoding/json"
	"testing"
)

func FuzzRepair(f *testing.F) {
	for _, s := range []string{
		`{"a":1}`, "```json\n{}\n```", `{"a":1`, `{"a":1,}`, `nope`,
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, in []byte) {
		res := Repair(in)
		if res.OK && !json.Valid(res.Payload) {
			t.Fatalf("Repair reported OK but payload invalid: %q -> %q", in, res.Payload)
		}
	})
}
```

- [ ] **Step 4: Run the fuzz target briefly**

Run: `go test ./pkg/jsonrepair/ -run FuzzRepair -fuzz FuzzRepair -fuzztime 20s`
Expected: no failures.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/streamsegments_test.go pkg/jsonrepair/fuzz_test.go
git commit -m "test: cover repaired rolls and fuzz jsonrepair"
```

---

### Task 8: Wire the repair report into the turn trace

**Files:**
- Modify: `pkg/engine/orchestrator.go` (the point after the generation loop where `applyRecords`
  is called)
- Test: `pkg/engine/orchestrator_test.go` (append)

**Interfaces:**
- Consumes: `Parser.RepairReport` (Task 5), the existing `o.logger.Event` call site
  (`pkg/engine/streamsegments.go:51`).
- Produces: one `turn.records_repaired` trace event per turn that had repairs.

- [ ] **Step 1: Write the failing test**

```go
func TestTurnEmitsRepairTrace(t *testing.T) {
	logger := newRecordingLogger() // use the existing test logger helper in pkg/engine
	o := &TurnOrchestrator{parser: turnstream.NewParser(newTestRoster()), logger: logger}
	o.parser.Feed("@roll {\"actor\":\"x\",\"check_kind\":\"do\",}\n")
	o.logRepairReport()
	if !logger.sawEvent("turn.records_repaired") {
		t.Fatal("expected a turn.records_repaired trace event")
	}
}
```

Adapt to the real logger type in `pkg/engine` tests.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/engine/ -run TestTurnEmitsRepairTrace -v`
Expected: FAIL, `undefined: logRepairReport`.

- [ ] **Step 3: Write minimal implementation**

Add to `pkg/engine/streamsegments.go`:

```go
// logRepairReport emits a trace event when any control record needed repair, so
// a degraded turn is visible in the debug trace even before the UI surfaces it.
func (o *TurnOrchestrator) logRepairReport() {
	if o.parser == nil || o.logger == nil {
		return
	}
	rep := o.parser.RepairReport()
	if rep.Repaired == 0 && rep.Failed == 0 {
		return
	}
	o.logger.Event("turn.records_repaired", map[string]interface{}{
		"repaired": rep.Repaired,
		"failed":   rep.Failed,
		"total":    rep.Total,
	})
}
```

Call `o.logRepairReport()` next to the existing `applyRecords` call site in `orchestrator.go`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/engine/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/streamsegments.go pkg/engine/orchestrator.go pkg/engine/orchestrator_test.go
git commit -m "feat(engine): trace turns whose records were repaired"
```

---

### Task 9: Full verification

- [ ] **Step 1: Run the full suite**

Run: `mise run test:backend`
Expected: PASS.

- [ ] **Step 2: Run the linters**

Run: `mise run lint`
Expected: clean (no `go vet` findings, docs lint unaffected).

- [ ] **Step 3: Confirm the acceptance criteria**

- A malformed `@roll`, `@persona`, `@state`, `@memory`, or `@move` that is structurally damaged is
  repaired and used.
- A pretty-printed multi-line record parses as one record.
- An unrepairable record keeps its error, produces no event, and is counted in `RepairReport`.
- A valid payload is byte-identical after `Repair`.
- The extractor's behaviour is unchanged for the cases it handled before.

- [ ] **Step 4: Commit any test-only adjustments**

```bash
git add -A
git commit -m "test: finalise record repair coverage"
```
