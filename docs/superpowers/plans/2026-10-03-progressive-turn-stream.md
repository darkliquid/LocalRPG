# Progressive Turn Stream Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the terminal `submit_turn` payload with a line-framed turn stream that is parsed progressively, so attributed speech segments and their audio are available while the model is still writing, and rolls become stop-and-continue terminators.

**Architecture:** A new `pkg/turnstream` parser turns streamed text into ordered narration/speech/record events. The orchestrator owns a parser per turn, feeds it from `onChunk`, and forwards events to a segment observer and a grouped TTS scheduler. Finalise builds the turn's segments from the same events, so the streamed and recorded views agree. A `@roll` record ends the response; the engine resolves it (auto) or persists a pending check (ask) and continues.

**Tech Stack:** Go standard library, `modernc.org/sqlite`, React 19 + TypeScript frontend.

**Spec:** `docs/superpowers/specs/2026-10-03-progressive-turn-stream-design.md`

## Global Constraints

- Go standard library only for tests (`testing`, `t.TempDir()`); no testify.
- Use `interface{}`, not `any`; `go vet` must stay clean.
- Errors wrapped with `fmt.Errorf("...: %w", err)`.
- `pkg/turnstream` must not import `pkg/engine` or `pkg/gui`.
- TypeScript is `strict` with `noUnusedLocals`/`noUnusedParameters`; `npx tsc --noEmit` is the frontend gate.
- Commits are Conventional Commits with a scope, subject under 72 chars.
- Run a single Go test with `go test -run TestName ./pkg/...`.

---

### File Map

- **`pkg/turnstream/parser.go`** — the streaming line parser: `Parser`, `Event`, `Record`, `Roster`, `Feed`, `Flush`, `Records`.
- **`pkg/turnstream/records.go`** — record types and payload decoding into `harness` shapes.
- **`pkg/turnstream/parser_test.go`** — chunk-invariance, line classes, records, narration coalescing.
- **`pkg/engine/roster.go`** — a mutable speaker roster built from the store, voice profiles, and the player.
- **`pkg/engine/roster_test.go`** — roster resolution and `Declare`.
- **`pkg/engine/turnstream.go`** — glue: build a parser for a turn, convert events to `[]entity.TurnSegment`.
- **`pkg/engine/turnstream_test.go`** — event-to-segment conversion.
- **`pkg/engine/orchestrator.go`** — own the parser, feed it in `onChunk`, expose `SetSegmentObserver`, use events at finalise.
- **`pkg/gui/types.go`** — add a `Segment` field to `TurnEvent`.
- **`pkg/gui/service.go`** — wire the segment observer to a `segment` event.
- **`pkg/media/groupstream.go`** — incremental group fold and scheduler.
- **`pkg/media/groupstream_test.go`** — fold parity with `planGroups`, budget flush.
- **`pkg/gui/streaming_tts.go`** — generalise the streamer to grouped, voice-aware units.
- **`frontend/src/types.ts`**, **`frontend/src/App.tsx`** — render live segments.
- **`pkg/harness/context.go`** — framing protocol instruction.
- **`pkg/harness/turn_tools.go`** — remove `submit_turn`.

---

## Phase 1 — The `pkg/turnstream` parser

### Task 1: Line parser skeleton and chunk invariance

**Files:**
- Create: `pkg/turnstream/parser.go`
- Test: `pkg/turnstream/parser_test.go`

**Interfaces:**
- Produces: `type Roster interface { Resolve(name string) (string, bool); Declare(name, id string) }`
- Produces: `type Event struct { Kind, Speaker, SpeakerID, Text string; Record *Record }`
- Produces: `func NewParser(roster Roster) *Parser`, `func (p *Parser) Feed(text string) []Event`, `func (p *Parser) Flush() []Event`
- Produces: `const KindNarration = "narration"`, `KindSpeech = "speech"`, `KindRecord = "record"`

- [ ] **Step 1: Write the failing test**

```go
package turnstream

import "testing"

// mapRoster is a fixed roster for tests.
type mapRoster map[string]string

func (m mapRoster) Resolve(name string) (string, bool) {
	id, ok := m[name]
	return id, ok
}

func (m mapRoster) Declare(name, id string) { m[name] = id }

func TestFeedClassifiesLines(t *testing.T) {
	p := NewParser(mapRoster{"Kaelen": "kaelen"})
	events := p.Feed("The docks are quiet.\n\n> Kaelen: You didn't see me here.\n\nA gull cries.\n")
	events = append(events, p.Flush()...)

	want := []Event{
		{Kind: KindNarration, Text: "The docks are quiet."},
		{Kind: KindSpeech, Speaker: "Kaelen", SpeakerID: "kaelen", Text: "You didn't see me here."},
		{Kind: KindNarration, Text: "A gull cries."},
	}
	if len(events) != len(want) {
		t.Fatalf("events = %#v, want %#v", events, want)
	}
	for i := range want {
		if events[i].Kind != want[i].Kind || events[i].Text != want[i].Text ||
			events[i].SpeakerID != want[i].SpeakerID {
			t.Fatalf("event %d = %#v, want %#v", i, events[i], want[i])
		}
	}
}

func TestFeedIsChunkInvariant(t *testing.T) {
	input := "One.\n\n> Kaelen: Keep walking.\n\nTwo.\n"
	whole := NewParser(mapRoster{"Kaelen": "kaelen"})
	want := append(whole.Feed(input), whole.Flush()...)

	for split := 1; split < len(input); split++ {
		p := NewParser(mapRoster{"Kaelen": "kaelen"})
		got := append(p.Feed(input[:split]), p.Feed(input[split:])...)
		got = append(got, p.Flush()...)
		if len(got) != len(want) {
			t.Fatalf("split %d: %#v, want %#v", split, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("split %d event %d: %#v, want %#v", split, i, got[i], want[i])
			}
		}
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test -run 'TestFeed' ./pkg/turnstream/`
Expected: build failure, `undefined: NewParser`.

- [ ] **Step 3: Write the parser**

```go
// Package turnstream parses the GM's reply as a line-framed stream of narration,
// speech, and control records, so segments can be attributed as they arrive.
package turnstream

import "strings"

// Line classes.
const (
	KindNarration = "narration"
	KindSpeech    = "speech"
	KindRecord    = "record"
)

// Roster resolves a speaker name to an entity id. Declare adds a speaker that a
// persona record introduced mid-stream, so their first line can be attributed.
type Roster interface {
	Resolve(name string) (id string, ok bool)
	Declare(name, id string)
}

// Event is one parsed unit in stream order.
type Event struct {
	Kind      string
	Speaker   string
	SpeakerID string
	Text      string
	Record    *Record
}

// Parser turns text deltas into ordered events. It buffers a partial line until
// its newline arrives, which is what makes the event stream independent of how
// the provider chunked the text.
type Parser struct {
	roster  Roster
	buf     string
	pending []string
	records []Record
}

// NewParser builds a parser that attributes speech against roster.
func NewParser(roster Roster) *Parser {
	return &Parser{roster: roster}
}

// Feed adds streamed text and returns every event the new text completed.
func (p *Parser) Feed(text string) []Event {
	var events []Event
	p.buf += text
	for {
		i := strings.IndexByte(p.buf, '\n')
		if i < 0 {
			break
		}
		line := p.buf[:i]
		p.buf = p.buf[i+1:]
		events = append(events, p.consume(line)...)
	}
	return events
}

// Flush returns the event for any trailing partial line at end of stream.
func (p *Parser) Flush() []Event {
	var events []Event
	if p.buf != "" {
		events = append(events, p.consume(p.buf)...)
		p.buf = ""
	}
	events = append(events, p.flushNarration()...)
	return events
}

// consume classifies one complete line.
func (p *Parser) consume(line string) []Event {
	line = strings.TrimSuffix(line, "\r")
	trimmed := strings.TrimSpace(line)
	switch {
	case trimmed == "":
		return p.flushNarration()
	case strings.HasPrefix(trimmed, ">"):
		return append(p.flushNarration(), p.speech(trimmed)...)
	default:
		p.pending = append(p.pending, trimmed)
		return nil
	}
}

// flushNarration emits the pending narration paragraph, if any.
func (p *Parser) flushNarration() []Event {
	if len(p.pending) == 0 {
		return nil
	}
	text := strings.Join(p.pending, "\n")
	p.pending = p.pending[:0]
	return []Event{{Kind: KindNarration, Text: text}}
}

// speech parses a blockquote line into a speech event when its speaker resolves,
// and into narration with the marker stripped otherwise.
func (p *Parser) speech(line string) []Event {
	body := strings.TrimSpace(strings.TrimPrefix(line, ">"))
	name, text, ok := splitSpeaker(body)
	if !ok {
		return []Event{{Kind: KindNarration, Text: body}}
	}
	id, ok := p.roster.Resolve(name)
	if !ok {
		return []Event{{Kind: KindNarration, Text: body}}
	}
	return []Event{{Kind: KindSpeech, Speaker: name, SpeakerID: id, Text: text}}
}

// maxSpeakerLength bounds a candidate so a sentence cannot masquerade as a name.
const maxSpeakerLength = 64

// splitSpeaker splits "Name: utterance" at the first colon, stripping one
// wrapping pair of quotes from the utterance.
func splitSpeaker(body string) (name, text string, ok bool) {
	i := strings.IndexByte(body, ':')
	if i <= 0 {
		return "", "", false
	}
	name = strings.TrimSpace(body[:i])
	if name == "" || len(name) > maxSpeakerLength || strings.ContainsAny(name, "\n\"") {
		return "", "", false
	}
	text = strings.TrimSpace(body[i+1:])
	text = trimQuotes(text)
	if text == "" {
		return "", "", false
	}
	return name, text, true
}

// trimQuotes removes one wrapping pair of straight or curly quotes.
func trimQuotes(s string) string {
	if len(s) < 2 {
		return s
	}
	first, last := s[0], s[len(s)-1]
	pairs := map[byte]byte{'"': '"', '\u201c': '\u201d', '\u2018': '\u2019'}
	if pairs[first] == last {
		return strings.TrimSpace(s[1 : len(s)-1])
	}
	return s
}
```

- [ ] **Step 4: Run it and watch it pass**

Run: `go test -run 'TestFeed' ./pkg/turnstream/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/turnstream/parser.go pkg/turnstream/parser_test.go
git commit -m "feat(turnstream): parse narration and speech from a streamed reply"
```

### Task 2: Records

**Files:**
- Modify: `pkg/turnstream/parser.go`
- Create: `pkg/turnstream/records.go`
- Test: `pkg/turnstream/parser_test.go`

**Interfaces:**
- Consumes: `Parser`, `Event`, `Roster` from Task 1.
- Produces: `type Record struct { Type string; Payload []byte; Line int; Err error }`
- Produces: `func (p *Parser) Records() []Record`
- Produces: `const RecordPersona = "persona"`, `RecordRoll = "roll"`, `RecordState = "state"`, `RecordMemory = "memory"`, `RecordMove = "move"`

- [ ] **Step 1: Write the failing test**

```go
func TestRecordsAreParsedAndPersonaeDeclared(t *testing.T) {
	roster := mapRoster{}
	p := NewParser(roster)
	events := p.Feed("@persona {\"name\":\"Kae\",\"type\":\"character\"}\n> Kae: Well met.\n")
	events = append(events, p.Flush()...)

	if len(events) != 2 || events[0].Kind != KindRecord || events[1].Kind != KindSpeech {
		t.Fatalf("events = %#v", events)
	}
	if events[1].SpeakerID != "kae" {
		t.Fatalf("a declared persona must be attributable: %#v", events[1])
	}
	records := p.Records()
	if len(records) != 1 || records[0].Type != RecordPersona || records[0].Err != nil {
		t.Fatalf("records = %#v", records)
	}
}

func TestMalformedRecordIsKeptButDoesNotFailTheStream(t *testing.T) {
	p := NewParser(mapRoster{})
	events := p.Feed("@bogus {not json}\nStill narrated.\n")
	events = append(events, p.Flush()...)

	records := p.Records()
	if len(records) != 1 || records[0].Err == nil {
		t.Fatalf("a bad record must be reported, got %#v", records)
	}
	if len(events) != 1 || events[0].Kind != KindNarration || events[0].Text != "Still narrated." {
		t.Fatalf("prose must survive a bad record, got %#v", events)
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test -run 'TestRecord|TestMalformed' ./pkg/turnstream/`
Expected: FAIL, `KindRecord` undefined / records not parsed.

- [ ] **Step 3: Implement records**

Add to `parser.go` in the `consume` switch, before the narration default:

```go
	case strings.HasPrefix(trimmed, "@"):
		return append(p.flushNarration(), p.record(trimmed)...)
```

Add the record handling and accessor:

```go
// record parses an "@type payload" line. A malformed record is recorded with an
// error and produces no event, so the surrounding prose still parses.
func (p *Parser) record(line string) []Event {
	rest := strings.TrimPrefix(line, "@")
	typ, payload, _ := strings.Cut(rest, " ")
	rec := Record{Type: strings.TrimSpace(typ), Payload: []byte(strings.TrimSpace(payload)), Line: len(p.records) + 1}
	switch {
	case !validRecordType(rec.Type):
		rec.Err = fmt.Errorf("unknown record type %q", rec.Type)
	case len(rec.Payload) == 0:
		rec.Err = fmt.Errorf("record %q has no payload", rec.Type)
	case !json.Valid(rec.Payload):
		rec.Err = fmt.Errorf("record %q payload is not JSON", rec.Type)
	}
	p.records = append(p.records, rec)
	if rec.Err != nil {
		return nil
	}
	if rec.Type == RecordPersona {
		p.declarePersona(rec)
	}
	out := p.records[len(p.records)-1]
	return []Event{{Kind: KindRecord, Record: &out}}
}

// declarePersona adds a declared character to the roster so their first line can
// be attributed even though the persona record precedes it.
func (p *Parser) declarePersona(rec Record) {
	var decl struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(rec.Payload, &decl); err != nil {
		return
	}
	if id := entity.Slugify(decl.Name); id != "" {
		p.roster.Declare(strings.ToLower(strings.TrimSpace(decl.Name)), id)
	}
}

// Records returns every record seen so far, in order.
func (p *Parser) Records() []Record {
	out := make([]Record, len(p.records))
	copy(out, p.records)
	return out
}
```

Add `"encoding/json"`, `"fmt"`, and `"github.com/darkliquid/localrpg/pkg/entity"` to the imports.

Create `pkg/turnstream/records.go`:

```go
package turnstream

// Record types the GM may emit as an "@type {json}" line.
const (
	RecordPersona = "persona"
	RecordRoll    = "roll"
	RecordState   = "state"
	RecordMemory  = "memory"
	RecordMove    = "move"
)

// Record is one control line: its type, its raw JSON payload, its ordinal, and
// the error that made it unusable, if any.
type Record struct {
	Type    string
	Payload []byte
	Line    int
	Err     error
}

// validRecordType reports whether a record type is recognised.
func validRecordType(typ string) bool {
	switch typ {
	case RecordPersona, RecordRoll, RecordState, RecordMemory, RecordMove:
		return true
	default:
		return false
	}
}
```

- [ ] **Step 4: Run it and watch it pass**

Run: `go test -run 'TestRecord|TestMalformed' ./pkg/turnstream/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/turnstream/
git commit -m "feat(turnstream): parse control records and declare personae mid-stream"
```

### Task 3: Record payload decoding

**Files:**
- Modify: `pkg/turnstream/records.go`
- Test: `pkg/turnstream/records_test.go`

**Interfaces:**
- Consumes: `Record` from Task 2, `harness.PersonaDecl`, `harness.CheckRequest`, `harness.StateChangeDecl`, `harness.MemoryDecl`.
- Produces: `func (r Record) DecodePersona() (harness.PersonaDecl, error)`, `DecodeRoll() (harness.CheckRequest, error)`, `DecodeState() (harness.StateChangeDecl, error)`, `DecodeMemory() (harness.MemoryDecl, error)`, `DecodeMove() (string, error)`

- [ ] **Step 1: Write the failing test**

```go
package turnstream

import "testing"

func TestRecordPayloadsDecode(t *testing.T) {
	roll := Record{Type: RecordRoll, Payload: []byte(`{"actor":"player","check_kind":"skill","stakes":"the bridge","outcomes":{"pass":"cross","fail":"fall"}}`)}
	req, err := roll.DecodeRoll()
	if err != nil || req.CheckKind != "skill" || req.Outcomes["pass"] != "cross" {
		t.Fatalf("DecodeRoll = %#v, %v", req, err)
	}

	move := Record{Type: RecordMove, Payload: []byte(`{"location":"[[aldon-harbour]]"}`)}
	loc, err := move.DecodeMove()
	if err != nil || loc != "[[aldon-harbour]]" {
		t.Fatalf("DecodeMove = %q, %v", loc, err)
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test -run TestRecordPayloadsDecode ./pkg/turnstream/`
Expected: FAIL, `DecodeRoll` undefined.

- [ ] **Step 3: Implement the decoders**

Append to `pkg/turnstream/records.go`:

```go
// DecodePersona decodes a persona record's payload.
func (r Record) DecodePersona() (harness.PersonaDecl, error) {
	var decl harness.PersonaDecl
	err := json.Unmarshal(r.Payload, &decl)
	return decl, err
}

// DecodeRoll decodes a roll record's payload into a check request.
func (r Record) DecodeRoll() (harness.CheckRequest, error) {
	var req harness.CheckRequest
	err := json.Unmarshal(r.Payload, &req)
	return req, err
}

// DecodeState decodes a state-change record's payload.
func (r Record) DecodeState() (harness.StateChangeDecl, error) {
	var change harness.StateChangeDecl
	err := json.Unmarshal(r.Payload, &change)
	return change, err
}

// DecodeMemory decodes a memory record's payload.
func (r Record) DecodeMemory() (harness.MemoryDecl, error) {
	var memory harness.MemoryDecl
	err := json.Unmarshal(r.Payload, &memory)
	return memory, err
}

// DecodeMove decodes a move record's payload into a location reference.
func (r Record) DecodeMove() (string, error) {
	var move struct {
		Location string `json:"location"`
	}
	if err := json.Unmarshal(r.Payload, &move); err != nil {
		return "", err
	}
	return move.Location, nil
}
```

Add `"encoding/json"` and `"github.com/darkliquid/localrpg/pkg/harness"` to the imports of `records.go`.

- [ ] **Step 4: Run it and watch it pass**

Run: `go test ./pkg/turnstream/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/turnstream/
git commit -m "feat(turnstream): decode record payloads into harness shapes"
```

---

## Phase 2 — Engine integration

### Task 4: The mutable roster

**Files:**
- Create: `pkg/engine/roster.go`
- Test: `pkg/engine/roster_test.go`

**Interfaces:**
- Consumes: `storage.Store`, `entity.VoiceConfig`.
- Produces: `func newRoster(store *storage.Store, playerID, playerName string, profiles []entity.VoiceProfile) *roster`
- Produces: `func (r *roster) Resolve(name string) (string, bool)`, `func (r *roster) Declare(name, id string)`, `func (r *roster) Voice(id string) *entity.VoiceConfig`

- [ ] **Step 1: Write the failing test**

```go
package engine

import "testing"

func TestRosterResolvesNamesSlugsAndPlayer(t *testing.T) {
	r := &roster{byKey: map[string]string{}, voices: map[string]*entity.VoiceConfig{}}
	r.Declare("Lady Evelyn Vance", "lady-evelyn-vance")
	r.Declare("player", "sean")

	if id, ok := r.Resolve("Lady Evelyn Vance"); !ok || id != "lady-evelyn-vance" {
		t.Fatalf("name resolve = %q, %v", id, ok)
	}
	if id, ok := r.Resolve("lady evelyn vance"); !ok || id != "lady-evelyn-vance" {
		t.Fatalf("case-insensitive resolve = %q, %v", id, ok)
	}
	if id, ok := r.Resolve("Evelyn"); ok {
		t.Fatalf("a partial name must not resolve, got %q", id)
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test -run TestRosterResolves ./pkg/engine/`
Expected: FAIL, `roster` undefined.

- [ ] **Step 3: Implement the roster**

```go
package engine

import (
	"strings"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// roster resolves a speaker name to an entity id without a store query, so a
// streamed line can be attributed the moment its colon arrives. It is mutable:
// a persona record declares a speaker mid-stream.
type roster struct {
	byKey  map[string]string
	voices map[string]*entity.VoiceConfig
}

// newRoster seeds the roster from the store, the voice profiles, and the player.
// A missing store yields a roster that resolves only declared speakers.
func newRoster(store *storage.Store, playerID, playerName string, profiles []entity.VoiceProfile) *roster {
	r := &roster{byKey: map[string]string{}, voices: map[string]*entity.VoiceConfig{}}
	for i := range profiles {
		profile := profiles[i]
		if profile.ID == "" {
			continue
		}
		voice := profile.Voice
		r.voices[profile.ID] = &voice
	}
	if store != nil {
		if summaries, err := store.ListEntities(); err == nil {
			for _, summary := range summaries {
				r.Declare(summary.Name, summary.ID)
				r.byKey[strings.ToLower(summary.ID)] = summary.ID
			}
		}
	}
	if playerID != "" {
		r.Declare(playerName, playerID)
		r.Declare(playerID, playerID)
	}
	return r
}

// Resolve maps a speaker name or slug to an entity id, case-insensitively.
func (r *roster) Resolve(name string) (string, bool) {
	if r == nil {
		return "", false
	}
	key := strings.ToLower(strings.TrimSpace(name))
	if id, ok := r.byKey[key]; ok {
		return id, true
	}
	if id := entity.Slugify(name); id != "" {
		if resolved, ok := r.byKey[id]; ok {
			return resolved, true
		}
	}
	return "", false
}

// Declare adds a name and its slug as keys for an entity id.
func (r *roster) Declare(name, id string) {
	if r == nil || id == "" {
		return
	}
	if trimmed := strings.ToLower(strings.TrimSpace(name)); trimmed != "" {
		r.byKey[trimmed] = id
	}
	if slug := entity.Slugify(name); slug != "" {
		r.byKey[slug] = id
	}
}

// Voice returns the voice profile assigned to an entity, or nil.
func (r *roster) Voice(id string) *entity.VoiceConfig {
	if r == nil {
		return nil
	}
	return r.voices[id]
}
```

- [ ] **Step 4: Run it and watch it pass**

Run: `go test -run TestRosterResolves ./pkg/engine/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/roster.go pkg/engine/roster_test.go
git commit -m "feat(engine): add a mutable speaker roster for streamed attribution"
```

### Task 5: Events to turn segments

**Files:**
- Create: `pkg/engine/turnstream.go`
- Test: `pkg/engine/turnstream_test.go`

**Interfaces:**
- Consumes: `turnstream.Event`, `roster`.
- Produces: `func segmentsFromEvents(events []turnstream.Event) []entity.TurnSegment`

- [ ] **Step 1: Write the failing test**

```go
package engine

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/turnstream"
)

func TestSegmentsFromEventsKeepsOrderAndSpeakers(t *testing.T) {
	events := []turnstream.Event{
		{Kind: turnstream.KindNarration, Text: "The hall is quiet."},
		{Kind: turnstream.KindSpeech, Speaker: "Garrick", SpeakerID: "garrick", Text: "Keep walking."},
		{Kind: turnstream.KindRecord, Record: &turnstream.Record{Type: turnstream.RecordRoll}},
	}
	got := segmentsFromEvents(events)

	want := []entity.TurnSegment{
		{Kind: entity.SegmentNarration, Text: "The hall is quiet."},
		{Kind: entity.SegmentSpeech, Speaker: "Garrick", SpeakerID: "garrick", Text: "Keep walking."},
	}
	if len(got) != len(want) {
		t.Fatalf("segments = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("segment %d = %#v, want %#v", i, got[i], want[i])
		}
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test -run TestSegmentsFromEvents ./pkg/engine/`
Expected: FAIL, `segmentsFromEvents` undefined.

- [ ] **Step 3: Implement the conversion**

```go
package engine

import (
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/turnstream"
)

// segmentsFromEvents maps parser events to the turn's playback script. Records
// carry no speech and are skipped here; the orchestrator consumes them
// separately.
func segmentsFromEvents(events []turnstream.Event) []entity.TurnSegment {
	segments := make([]entity.TurnSegment, 0, len(events))
	for _, event := range events {
		switch event.Kind {
		case turnstream.KindSpeech:
			segments = append(segments, entity.TurnSegment{
				Kind:      entity.SegmentSpeech,
				Speaker:   event.Speaker,
				SpeakerID: event.SpeakerID,
				Text:      event.Text,
			})
		case turnstream.KindNarration:
			if text := strings.TrimSpace(event.Text); text != "" {
				segments = append(segments, entity.TurnSegment{Kind: entity.SegmentNarration, Text: text})
			}
		}
	}
	return segments
}
```

Add `"strings"` to the imports.

- [ ] **Step 4: Run it and watch it pass**

Run: `go test -run TestSegmentsFromEvents ./pkg/engine/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/turnstream.go pkg/engine/turnstream_test.go
git commit -m "feat(engine): build turn segments from parsed stream events"
```

### Task 6: Feed the parser and observe segments

**Files:**
- Modify: `pkg/engine/orchestrator.go`
- Test: `pkg/engine/orchestrator_stream_test.go`

**Interfaces:**
- Consumes: `newRoster`, `turnstream.NewParser`, `segmentsFromEvents`.
- Produces: `func (o *TurnOrchestrator) SetSegmentObserver(func(turnstream.Event))`

- [ ] **Step 1: Write the failing test**

```go
func TestProcessActionStreamEmitsSegmentsAsTheyArrive(t *testing.T) {
	o, _ := newStreamTestOrchestrator(t, []string{
		"The docks are quiet.\n\n> Kaelen: You didn't see me here.\n",
	})

	var kinds []string
	o.SetSegmentObserver(func(event turnstream.Event) {
		kinds = append(kinds, event.Kind)
	})
	if _, err := o.ProcessActionStream(context.Background(), "Do", "I wait.", func(string) error { return nil }); err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}
	if len(kinds) < 2 || kinds[0] != turnstream.KindNarration {
		t.Fatalf("segment observer saw %#v", kinds)
	}
}
```

Note: `newStreamTestOrchestrator` is the existing helper used by
`orchestrator_stream_test.go`; extend it if it does not already seed a roster
entity named Kaelen, and register that entity in the test store.

- [ ] **Step 2: Run it and watch it fail**

Run: `go test -run TestProcessActionStreamEmitsSegments ./pkg/engine/`
Expected: FAIL, `SetSegmentObserver` undefined.

- [ ] **Step 3: Implement the wiring**

Add the field and setter near the existing `SetToolObserver`:

```go
// SetSegmentObserver attaches a sink for parsed turn-stream events, so a client
// can render attributed segments while the model is still writing. A nil
// observer records nothing.
func (o *TurnOrchestrator) SetSegmentObserver(observer func(turnstream.Event)) {
	o.segmentObserver = observer
}

// observeSegments forwards parsed events, if a sink is attached.
func (o *TurnOrchestrator) observeSegments(events []turnstream.Event) {
	if o.segmentObserver == nil {
		return
	}
	for _, event := range events {
		o.segmentObserver(event)
	}
}
```

Add `segmentObserver func(turnstream.Event)` and `parser *turnstream.Parser` to the
`TurnOrchestrator` struct.

In `ProcessActionStream`, immediately before the `runGenerationLoop` call, build
the roster and parser, and wrap `onChunk` so every delta is fed:

```go
	roster := newRoster(o.store, o.playerID, o.playerDisplayName(), o.timeline.VoiceProfiles())
	o.parser = turnstream.NewParser(roster)
	if onChunk != nil {
		inner := onChunk
		onChunk = func(text string) error {
			o.observeSegments(o.parser.Feed(text))
			return inner(text)
		}
	} else {
		onChunk = func(text string) error {
			o.observeSegments(o.parser.Feed(text))
			return nil
		}
	}
```

Note: place this after the existing `turn.ttft` wrapper so TTFT still measures the
first real client chunk.

- [ ] **Step 4: Run it and watch it pass**

Run: `go test -run TestProcessActionStream ./pkg/engine/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/orchestrator.go pkg/engine/orchestrator_stream_test.go
git commit -m "feat(engine): observe parsed segments while the turn streams"
```

### Task 7: Use parsed events at finalise, with the legacy fallback

**Files:**
- Modify: `pkg/engine/orchestrator.go`
- Test: `pkg/engine/turnstream_test.go`

**Interfaces:**
- Consumes: `o.parser`, `segmentsFromEvents`, `buildTurnSegments`.

- [ ] **Step 1: Write the failing test**

```go
func TestFinalisePrefersParsedSegments(t *testing.T) {
	o, store := newStreamTestOrchestrator(t, []string{
		"The docks are quiet.\n\n> Kaelen: You didn't see me here.\n",
	})
	seedEntity(t, store, "Kaelen", "character")

	turn, err := o.ProcessActionStream(context.Background(), "Do", "I wait.", nil)
	if err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}
	if len(turn.Segments) != 2 || turn.Segments[1].Kind != entity.SegmentSpeech ||
		turn.Segments[1].SpeakerID != "kaelen" {
		t.Fatalf("segments = %#v", turn.Segments)
	}
}

func TestFinaliseFallsBackWhenThereIsNoFraming(t *testing.T) {
	o, store := newStreamTestOrchestrator(t, []string{`He says, "Hold the line."`})
	seedEntity(t, store, "Garrick", "character")

	turn, err := o.ProcessActionStream(context.Background(), "Do", "I wait.", nil)
	if err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}
	if len(turn.Segments) == 0 {
		t.Fatal("a reply with no framing must still produce segments")
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test -run 'TestFinalise' ./pkg/engine/`
Expected: FAIL, the parsed segments are not yet used.

- [ ] **Step 3: Use the events at finalise**

Replace the non-structured segment build:

```go
	} else {
		events := o.parser.Flush()
		if parsed := segmentsFromEvents(events); len(parsed) > 0 {
			turn.Segments = parsed
		} else {
			turn.Segments = buildTurnSegments(o.store, turn.Narration, extraction)
		}
	}
```

The structured branch is deleted in Task 16; until then it stays as written.

- [ ] **Step 4: Run it and watch it pass**

Run: `go test ./pkg/engine/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/
git commit -m "feat(engine): finalise turn segments from the parsed stream"
```

---

## Phase 3 — GUI streaming

### Task 8: A `segment` turn event

**Files:**
- Modify: `pkg/gui/types.go`
- Modify: `pkg/gui/service.go`
- Test: `pkg/gui/service_test.go`

**Interfaces:**
- Consumes: `TurnOrchestrator.SetSegmentObserver`.
- Produces: `TurnEvent{Type: "segment", Segment: *SegmentDTO}`.

- [ ] **Step 1: Write the failing test**

```go
func TestTurnSessionEmitsSegmentEvents(t *testing.T) {
	// Build a session whose provider streams an indented speech line, collect
	// events, and assert at least one has Type "segment" and a speaker.
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test -run TestTurnSessionEmitsSegment ./pkg/gui/`
Expected: FAIL.

- [ ] **Step 3: Add the field and wire the observer**

In `pkg/gui/types.go`, extend `TurnEvent`:

```go
	// Live segment, present when Type is "segment": one parsed narration or
	// speech unit, emitted while the model is still writing.
	Segment *SegmentDTO `json:"segment,omitempty"`
```

In `pkg/gui/service.go`, inside `TurnSession.Run`, before
`ProcessActionStream`:

```go
	t.orchestrator.SetSegmentObserver(func(event turnstream.Event) {
		segment, ok := liveSegmentDTO(event)
		if !ok {
			return
		}
		_ = announce(TurnEvent{Type: "segment", Segment: &segment})
	})
```

Add the mapper in `pkg/gui/service.go`:

```go
// liveSegmentDTO renders one parsed event as a client segment, or reports false
// for an event a client should not render.
func liveSegmentDTO(event turnstream.Event) (SegmentDTO, bool) {
	switch event.Kind {
	case turnstream.KindSpeech:
		return SegmentDTO{Kind: "speech", Speaker: event.Speaker, SpeakerID: event.SpeakerID, Text: event.Text}, true
	case turnstream.KindNarration:
		if strings.TrimSpace(event.Text) == "" {
			return SegmentDTO{}, false
		}
		return SegmentDTO{Kind: "narration", Text: event.Text}, true
	default:
		return SegmentDTO{}, false
	}
}
```

- [ ] **Step 4: Run it and watch it pass**

Run: `go test ./pkg/gui/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/
git commit -m "feat(gui): stream parsed segments to the client as they arrive"
```

### Task 9: Render live segments in the client

**Files:**
- Modify: `frontend/src/types.ts`
- Modify: `frontend/src/App.tsx`

**Interfaces:**
- Consumes: the `segment` event.

- [ ] **Step 1: Add the type**

In `frontend/src/types.ts`, add to the stream event type:

```ts
  segment?: TurnSegment;
```

- [ ] **Step 2: Handle the event**

In `frontend/src/App.tsx`, extend the stream handler:

```tsx
          } else if (event.type === 'segment' && event.segment) {
            setStreamedSegments((prev) => [...prev, event.segment!]);
```

Replace the `streamedProse`-as-narration render with the accumulated segments,
falling back to `streamedProse` when none have arrived:

```tsx
          segments={
            streamedSegments.length > 0
              ? streamedSegments
              : [{ kind: 'narration' as const, text: streamedProse }]
          }
```

Reset `streamedSegments` alongside `setStreamedProse('')` when the `turn` event
arrives.

- [ ] **Step 3: Verify the frontend builds**

Run: `cd frontend && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/types.ts frontend/src/App.tsx
git commit -m "feat(frontend): render live segments during a streaming turn"
```

---

## Phase 4 — Grouped streaming TTS

### Task 10: The incremental group fold

**Files:**
- Create: `pkg/media/groupstream.go`
- Test: `pkg/media/groupstream_test.go`

**Interfaces:**
- Consumes: `TTSCapabilities`, `SpeakerLine`, `canonicalGroupLines`, `linesFit`, `splitLineToFit`.
- Produces: `type GroupFolder struct{...}`, `func NewGroupFolder(caps TTSCapabilities, budget int) *GroupFolder`, `func (f *GroupFolder) Add(line SpeakerLine) (flushed []SpeakerLine)`, `func (f *GroupFolder) Flush() []SpeakerLine`.

- [ ] **Step 1: Write the failing test**

```go
func TestGroupFolderMatchesPlanGroupsWithoutABudget(t *testing.T) {
	lines := []SpeakerLine{
		{SpeakerID: "narrator", Label: "Narrator", Text: "The hall is quiet."},
		{SpeakerID: "narrator", Label: "Narrator", Text: "Cold air rushes in."},
		{SpeakerID: "garrick", Label: "Garrick", Text: "Keep walking."},
	}
	caps := TTSCapabilities{MaxSpeakers: 1}

	folder := NewGroupFolder(caps, 0)
	var flushed [][]SpeakerLine
	for _, line := range lines {
		if out := folder.Add(line); out != nil {
			flushed = append(flushed, out)
		}
	}
	if out := folder.Flush(); out != nil {
		flushed = append(flushed, out)
	}
	if len(flushed) != 2 {
		t.Fatalf("folded %d groups, want 2: %#v", len(flushed), flushed)
	}
	if len(flushed[0]) != 2 || len(flushed[1]) != 1 {
		t.Fatalf("group sizes = %d,%d", len(flushed[0]), len(flushed[1]))
	}
}

func TestGroupFolderFlushesAtTheBudget(t *testing.T) {
	folder := NewGroupFolder(TTSCapabilities{MaxSpeakers: 1}, 10)
	if out := folder.Add(SpeakerLine{SpeakerID: "n", Label: "Narrator", Text: "One two."}); out != nil {
		t.Fatalf("premature flush: %#v", out)
	}
	out := folder.Add(SpeakerLine{SpeakerID: "n", Label: "Narrator", Text: "Three four five."})
	if len(out) == 0 {
		t.Fatal("the budget must force a flush")
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test -run TestGroupFolder ./pkg/media/`
Expected: FAIL, `NewGroupFolder` undefined.

- [ ] **Step 3: Implement the fold**

```go
package media

import "strings"

// GroupFolder folds a turn's speaker lines into groups incrementally, applying
// the same rules as planGroups: a group ends when the speaker changes, when the
// speaker budget is reached, or when the request limits would be exceeded. A
// positive budget additionally forces a sentence-aligned flush so live audio is
// not held until a long block ends.
type GroupFolder struct {
	caps    TTSCapabilities
	budget  int
	pending []SpeakerLine
	chars   int
}

// NewGroupFolder builds a folder. A budget of zero flushes only on a boundary or
// the provider limits.
func NewGroupFolder(caps TTSCapabilities, budget int) *GroupFolder {
	return &GroupFolder{caps: normalizeCaps(caps), budget: budget}
}

// Add appends a line and returns the group to flush, or nil.
func (f *GroupFolder) Add(line SpeakerLine) []SpeakerLine {
	if len(f.pending) > 0 && !canJoinGroup(ClipGroup{Lines: f.pending}, line, f.caps) {
		out := f.Flush()
		f.append(line)
		return out
	}
	f.append(line)
	if f.budget > 0 && f.chars >= f.budget {
		return f.Flush()
	}
	return nil
}

// Flush returns the pending group and clears it.
func (f *GroupFolder) Flush() []SpeakerLine {
	if len(f.pending) == 0 {
		return nil
	}
	out := f.pending
	f.pending = nil
	f.chars = 0
	return out
}

func (f *GroupFolder) append(line SpeakerLine) {
	f.pending = append(f.pending, line)
	f.chars += len([]rune(strings.TrimSpace(line.Text))) + 1
}
```

- [ ] **Step 4: Run it and watch it pass**

Run: `go test ./pkg/media/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/media/groupstream.go pkg/media/groupstream_test.go
git commit -m "feat(media): fold streamed speaker lines into TTS groups incrementally"
```

### Task 11: Generalise the streamer to grouped, voice-aware units

**Files:**
- Modify: `pkg/gui/streaming_tts.go`
- Test: `pkg/gui/streaming_tts_test.go`

**Interfaces:**
- Consumes: `media.GroupFolder`, `turnstream.Event`.
- Produces: `func (s *sentenceStreamer) FeedSegment(event turnstream.Event)`, keeping `Feed` for narration-only providers.

- [ ] **Step 1: Write the failing test**

```go
func TestStreamerUsesTheSpeakerVoiceForGroupedSpeech(t *testing.T) {
	// A fake pipeline records (speakerID, text) per request. Feed a narration
	// sentence then a speech sentence for a distinct speaker and assert two
	// requests with the right voices.
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test -run TestStreamerUsesTheSpeakerVoice ./pkg/gui/`
Expected: FAIL.

- [ ] **Step 3: Implement**

Give the streamer a `media.GroupFolder` and a voice resolver, split each incoming
segment's text into sentences with `media.SplitCompleteSentences`, and on a
speaker change call `folder.Flush()` and synthesize the group with
`pipeline.SynthesizeGroups`. Keep `Feed` as a thin wrapper that feeds narration
sentences, so a provider that emits no framing is unchanged. Remove the
`TTSGrouping() == "always"` early return in `sentenceStreamerFor`; grouping and
streaming now cooperate.

- [ ] **Step 4: Run it and watch it pass**

Run: `go test ./pkg/gui/ ./pkg/media/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/streaming_tts.go pkg/gui/streaming_tts_test.go
git commit -m "feat(gui): voice streamed speech with the speaker's profile"
```

---

## Phase 5 — Records become declarations

### Task 12: Apply personae, memories, state, and moves from records

**Files:**
- Modify: `pkg/engine/orchestrator.go`
- Test: `pkg/engine/turnstream_test.go`

**Interfaces:**
- Consumes: `turnstream.Record` decoders.
- Produces: `func (o *TurnOrchestrator) applyRecords(turn *Turn, records []turnstream.Record) ([]harness.PersonaDecl, []harness.MemoryDecl)`

- [ ] **Step 1: Write the failing test**

```go
func TestRecordsIntroducePersonaeAndMoveThePlayer(t *testing.T) {
	o, _ := newStreamTestOrchestrator(t, []string{
		"@persona {\"name\":\"Kae\",\"type\":\"character\",\"new\":true}\n> Kae: Well met.\n@move {\"location\":\"[[aldon-harbour]]\"}\n",
	})
	turn, err := o.ProcessActionStream(context.Background(), "Do", "I arrive.", nil)
	if err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}
	if len(turn.Personae) != 1 || turn.Personae[0] != "kae" {
		t.Fatalf("personae = %#v", turn.Personae)
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test -run TestRecordsIntroduce ./pkg/engine/`
Expected: FAIL.

- [ ] **Step 3: Implement**

After the parser is flushed at finalise, decode records and populate the turn's
personae, memories, state changes, and location move, mirroring what the
structured branch did with `result.Submission`. State changes route through
`o.rulesEngine` exactly as `submission.go` does. A record that fails to decode is
logged (`turn.record_error`) and skipped.

- [ ] **Step 4: Run it and watch it pass**

Run: `go test ./pkg/engine/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/
git commit -m "feat(engine): apply personae, memories, and state from stream records"
```

---

## Phase 6 — Rolls as terminators

### Task 13: A `@roll` ends the response and the auto loop continues

**Files:**
- Modify: `pkg/engine/orchestrator.go`
- Test: `pkg/engine/turnstream_test.go`

**Interfaces:**
- Produces: `func (o *TurnOrchestrator) resolveStreamedRoll(ctx, req harness.CheckRequest) (*harness.CheckResult, error)` and a bounded continuation loop in `ProcessActionStream`.

- [ ] **Step 1: Write the failing test**

```go
func TestAutoRollTerminatesAndContinuesInOneTurn(t *testing.T) {
	o, _ := newStreamTestOrchestrator(t, []string{
		"Kaelen crosses the bridge.\n@roll {\"actor\":\"kaelen\",\"check_kind\":\"skill\",\"stakes\":\"the bridge\",\"outcomes\":{\"pass\":\"crosses\",\"fail\":\"falls\"}}\n",
		"Kaelen makes it across.\n",
	})
	turn, err := o.ProcessActionStream(context.Background(), "Do", "I follow.", nil)
	if err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}
	if len(turn.Checks) != 1 {
		t.Fatalf("checks = %#v", turn.Checks)
	}
	if !strings.Contains(turn.Narration, "makes it across") {
		t.Fatalf("the continuation must be recorded: %q", turn.Narration)
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test -run TestAutoRoll ./pkg/engine/`
Expected: FAIL.

- [ ] **Step 3: Implement**

When the flushed records contain a `roll`, resolve it with `resolveCheck` under
the `auto` policy, append the result to `turn.Checks`, and issue a continuation
call with a directive naming the outcome and the pre-committed outcome text. Feed
the continuation through the same parser and append its events' segments to the
turn. Bound the loop with a constant (three continuations). Under the `ask`
policy, instead persist a `PendingCheck` and return, as today.

Persist the resolved roll on the turn so a retry cannot re-roll: store the
`CheckResult` keyed by the pending ref and reuse it when the same ref is seen.

- [ ] **Step 4: Run it and watch it pass**

Run: `go test ./pkg/engine/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/
git commit -m "feat(engine): end the turn on a roll record and continue with the result"
```

---

## Phase 7 — Prompt and tool surface

### Task 14: The framing protocol instruction

**Files:**
- Modify: `pkg/harness/context.go`
- Test: `pkg/harness/context_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestAssembledPromptTeachesTheTurnFraming(t *testing.T) {
	// Assemble a prompt and assert it contains "> Speaker: utterance" and "@roll".
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test -run TestAssembledPromptTeaches ./pkg/harness/`
Expected: FAIL.

- [ ] **Step 3: Replace the instruction**

Replace `turnProtocolInstruction` with a framing instruction that states: speech
is a blockquote line `> Speaker: utterance`; records are `@type {json}` lines for
`persona`, `roll`, `state`, `memory`, and `move`; a `@roll` ends the response;
and one worked example. Keep it a non-droppable, high-rank section.

- [ ] **Step 4: Run it and watch it pass**

Run: `go test ./pkg/harness/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/harness/
git commit -m "feat(harness): teach the progressive turn framing in the prompt"
```

### Task 15: Remove `submit_turn`

**Files:**
- Modify: `pkg/harness/turn_tools.go`
- Modify: `pkg/engine/orchestrator.go`
- Modify: `pkg/harness/turn.go`, `pkg/engine/submission.go`

- [ ] **Step 1: Remove the tool and the structured path**

Delete `submit_turn` from `TurnToolSpecsFor`, `TurnToolNames`, and the orchestrator
dispatch. Delete the `structured` branch in `ProcessActionStream`. Remove
`TurnSubmission`, `TurnSubmissionSchema`, `ParseSubmission`, `buildSegments`,
`validateSubmission`, and `TurnSubmissionSchema` tests once nothing reads them.
`request_check`/`propose_check` are replaced by `@roll`; delete them too.

- [ ] **Step 2: Run the suite**

Run: `mise run test:backend`
Expected: PASS.

- [ ] **Step 3: Regenerate embedded docs**

Run: `go test ./pkg/gui -update-docs`
Expected: the provider catalogue and config reference regenerate without diff
beyond the protocol change.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "refactor(engine): retire submit_turn for the progressive turn stream"
```

### Task 16: Full verification

- [ ] **Step 1: Backend and frontend tests**

Run: `mise run test`
Expected: PASS.

- [ ] **Step 2: Lint**

Run: `mise run lint`
Expected: clean.

- [ ] **Step 3: Commit any fixes**

```bash
git add -A
git commit -m "chore: verify progressive turn stream milestone"
```
