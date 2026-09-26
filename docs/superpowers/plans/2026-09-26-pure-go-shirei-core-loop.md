# Pure-Go shirei GUI — Core Loop: Markdown, Chronicle, and Turns Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the play loop: render prose Markdown (emphasis/strong/code/wikilinks) in shirei, show the chronicle of turns with dice checks and audio controls, and submit turns in-process through `BeginTurn`/`TurnSession.Run`.

**Architecture:** `pkg/desktop/markdown` parses with goldmark into a small block/span model and renders it with shirei `Text`/`Label` (emphasis-only scope). `desktop.State` gains the chronicle and in-flight turn fields; `desktop.turnRunner` wraps `BeginTurn` + `Run` behind an injected interface so tests never call a model. The chronicle and action console read state and re-render on `RequestNextFrame`, so a fake runner's `emit` calls drive the streamed-prose path in tests.

**Tech Stack:** Go 1.27.1, `github.com/yuin/goldmark` v1.7.16 (present as indirect; becomes direct), `go.hasen.dev/shirei`, `pkg/gui` (`BeginTurn`, `TurnSession`, DTOs), standard-library tests.

**Spec:** `docs/superpowers/specs/2026-09-26-pure-go-shirei-gui-design.md` (Phase 4)

## Global Constraints

- Desktop only. Do not modify `pkg/gui/server.go`, `socket.go`, `assets.go`, or `middleware.go`.
- Markdown scope is what the SPA's `MarkdownProse` does: inline emphasis, strong, inline code, `[[wikilink]]`, performance-direction tags, scene rules, headings, unordered lists, blockquotes, paragraphs. No tables, ordered lists, links-with-URLs, or images.
- In-process submission only: `BeginTurn` → `errors.Is(err, gui.ErrTurnInFlight)` handling → `defer session.Close()` → `session.Run(ctx, req, emit)`. Never fetch `/api/...`.
- Go style: `any`; `go vet ./...` clean. Tests standard-library only.
- Every interactive container gets `NextAccessName(...)` + `AssignAccess()`.
- Commits: Conventional Commits with a scope; end with the attribution block shown in Task 1 Step 5.
- `mise run test` must pass (`pkg/gui` is flaky ~1 in 6; re-run before calling it a regression).

---

### Task 1: The Markdown block/span model

**Files:**
- Create: `pkg/desktop/markdown/markdown.go`
- Test: `pkg/desktop/markdown/markdown_test.go`
- Modify: `go.mod` (goldmark becomes direct via `go mod tidy`)

**Interfaces:**
- Consumes: `github.com/yuin/goldmark` (`goldmark.New().Parser().Parse(text.NewReader(src))`).
- Produces:
  - `type markdown.BlockKind int` with `BlockParagraph, BlockHeading, BlockList, BlockQuote, BlockRule`
  - `type markdown.SpanKind int` with `SpanEmphasis, SpanStrong, SpanCode, SpanWikilink`
  - `type markdown.SpanRange struct { From, To int; Kind SpanKind; Target string }`
  - `type markdown.Block struct { Kind BlockKind; Level int; Text string; Spans []SpanRange; Items []string }`
  - `func markdown.Parse(src string) []Block`

- [ ] **Step 1: Write the failing tests**

Create `pkg/desktop/markdown/markdown_test.go`:

```go
package markdown

import "testing"

func TestParseEmphasisAndStrong(t *testing.T) {
	blocks := Parse("A *soft* word and **hard** word.")
	if len(blocks) != 1 {
		t.Fatalf("blocks = %d, want 1", len(blocks))
	}
	b := blocks[0]
	if b.Text != "A soft word and hard word." {
		t.Fatalf("text = %q", b.Text)
	}
	if len(b.Spans) != 2 {
		t.Fatalf("spans = %d, want 2: %+v", len(b.Spans), b.Spans)
	}
	if b.Spans[0].Kind != SpanEmphasis || b.Text[b.Spans[0].From:b.Spans[0].To] != "soft" {
		t.Errorf("span 0 = %+v", b.Spans[0])
	}
	if b.Spans[1].Kind != SpanStrong || b.Text[b.Spans[1].From:b.Spans[1].To] != "hard" {
		t.Errorf("span 1 = %+v", b.Spans[1])
	}
}

func TestParseWikilinkAndCode(t *testing.T) {
	blocks := Parse("Meet [[hero|Vance]] using `1d20`.")
	b := blocks[0]
	if b.Text != "Meet Vance using 1d20." {
		t.Fatalf("text = %q", b.Text)
	}
	if len(b.Spans) != 2 {
		t.Fatalf("spans = %+v", b.Spans)
	}
	if b.Spans[0].Kind != SpanWikilink || b.Spans[0].Target != "hero" {
		t.Errorf("wikilink span = %+v", b.Spans[0])
	}
	if b.Spans[1].Kind != SpanCode {
		t.Errorf("code span = %+v", b.Spans[1])
	}
}

func TestParseBlocks(t *testing.T) {
	blocks := Parse("# Title\n\n- one\n- two\n\n> quoted\n\n---\n\nplain")
	if len(blocks) != 5 {
		t.Fatalf("blocks = %d, want 5: %+v", len(blocks), blocks)
	}
	if blocks[0].Kind != BlockHeading || blocks[0].Level != 1 || blocks[0].Text != "Title" {
		t.Errorf("heading = %+v", blocks[0])
	}
	if blocks[1].Kind != BlockList || len(blocks[1].Items) != 2 {
		t.Errorf("list = %+v", blocks[1])
	}
	if blocks[2].Kind != BlockQuote || blocks[2].Text != "quoted" {
		t.Errorf("quote = %+v", blocks[2])
	}
	if blocks[3].Kind != BlockRule {
		t.Errorf("rule = %+v", blocks[3])
	}
	if blocks[4].Kind != BlockParagraph || blocks[4].Text != "plain" {
		t.Errorf("paragraph = %+v", blocks[4])
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/desktop/markdown/ -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement the parser**

Create `pkg/desktop/markdown/markdown.go`:

```go
// Package markdown turns the small subset of Markdown LocalRPG shows in prose
// into flat blocks with styled rune ranges, which shirei can draw with Text.
package markdown

import (
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

type BlockKind int

const (
	BlockParagraph BlockKind = iota
	BlockHeading
	BlockList
	BlockQuote
	BlockRule
)

type SpanKind int

const (
	SpanEmphasis SpanKind = iota
	SpanStrong
	SpanCode
	SpanWikilink
)

type SpanRange struct {
	From, To int
	Kind     SpanKind
	Target   string // wikilink target, when Kind == SpanWikilink
}

type Block struct {
	Kind  BlockKind
	Level int
	Text  string
	Spans []SpanRange
	Items []string
}

var parser = goldmark.New()

// Parse converts src into blocks. Inline wikilinks are extracted first because
// goldmark does not know them.
func Parse(src string) []Block {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	doc := parser.Parser().Parse(text.NewReader([]byte(src)))
	var blocks []Block
	for node := doc.FirstChild(); node != nil; node = node.NextSibling() {
		blocks = append(blocks, blockFromNode(node, src)...)
	}
	return blocks
}

func blockFromNode(node ast.Node, src string) []Block {
	switch node.Kind() {
	case ast.KindHeading:
		level := node.(*ast.Heading).Level
		text, spans := inline(node, src)
		return []Block{{Kind: BlockHeading, Level: level, Text: text, Spans: spans}}
	case ast.KindBlockquote:
		var out []Block
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			out = append(out, blockFromNode(child, src)...)
		}
		for i := range out {
			out[i].Kind = BlockQuote
		}
		return out
	case ast.KindThematicBreak:
		return []Block{{Kind: BlockRule}}
	case ast.KindList:
		list := node.(*ast.List)
		var items []string
		for child := list.FirstChild(); child != nil; child = child.NextSibling() {
			text, _ := inline(child, src)
			items = append(items, text)
		}
		return []Block{{Kind: BlockList, Items: items}}
	case ast.KindParagraph:
		text, spans := inline(node, src)
		return []Block{{Kind: BlockParagraph, Text: text, Spans: spans}}
	default:
		return nil
	}
}

// inline flattens a node's inline children into plain text plus spans, applying
// wikilink extraction to the result.
func inline(node ast.Node, src string) (string, []SpanRange) {
	var (
		b strings.Builder
		s []SpanRange
	)
	var walk func(n ast.Node, kind SpanKind)
	walk = func(n ast.Node, kind SpanKind) {
		for child := n.FirstChild(); child != nil; child = child.NextSibling() {
			switch child.Kind() {
			case ast.KindText:
				t := child.(*ast.Text)
				start := b.Len()
				b.Write(t.Segment.Value([]byte(src)))
				if kind != 0 {
					s = append(s, SpanRange{From: start, To: b.Len(), Kind: kind})
				}
				if t.SoftLineBreak() || t.HardLineBreak() {
					b.WriteByte(' ')
				}
			case ast.KindCodeSpan:
				start := b.Len()
				for c := child.FirstChild(); c != nil; c = c.NextSibling() {
					if t, ok := c.(*ast.Text); ok {
						b.Write(t.Segment.Value([]byte(src)))
					}
				}
				s = append(s, SpanRange{From: start, To: b.Len(), Kind: SpanCode})
			case ast.KindEmphasis:
				inner := child.(*ast.Emphasis)
				if inner.Level >= 2 {
					walk(child, SpanStrong)
				} else {
					walk(child, SpanEmphasis)
				}
			default:
				walk(child, kind)
			}
		}
	}
	walk(node, 0)
	text := b.String()
	return extractWikilinks(text, s)
}

// extractWikilinks rewrites [[target|label]] (and [[target]]) to label, maps
// existing byte-offset spans into the rewritten text, and appends wikilink
// spans.
func extractWikilinks(text string, spans []SpanRange) (string, []SpanRange) {
	var b strings.Builder
	// offsetMap maps an original byte offset to its new offset.
	offsetMap := make([]int, len(text)+1)
	i := 0
	for i < len(text) {
		offsetMap[i] = b.Len()
		if strings.HasPrefix(text[i:], "[[") {
			if end := strings.Index(text[i+2:], "]]"); end >= 0 {
				inner := text[i+2 : i+2+end]
				target, label := inner, inner
				if bar := strings.IndexByte(inner, '|'); bar >= 0 {
					target, label = inner[:bar], inner[bar+1:]
				}
				linkStart := b.Len()
				linkEnd := linkStart + len(label)
				tokenEnd := i + 4 + len(inner)
				for j := i; j <= tokenEnd; j++ {
					offsetMap[j] = linkStart
				}
				b.WriteString(label)
				offsetMap[tokenEnd] = linkEnd
				spans = append(spans, SpanRange{From: linkStart, To: linkEnd, Kind: SpanWikilink, Target: target})
				i = tokenEnd
				continue
			}
		}
		b.WriteByte(text[i])
		i++
	}
	offsetMap[len(text)] = b.Len()

	out := make([]SpanRange, 0, len(spans))
	for _, sp := range spans {
		if sp.Kind == SpanWikilink {
			continue
		}
		out = append(out, SpanRange{From: offsetMap[sp.From], To: offsetMap[sp.To], Kind: sp.Kind})
	}
	for _, sp := range spans {
		if sp.Kind == SpanWikilink {
			out = append(out, sp)
		}
	}
	return b.String(), out
}
```

Spans from goldmark are byte offsets, so `offsetMap` indexing by byte is correct. The wikilink spans are appended last so callers can rely on spans being otherwise source-ordered.

- [ ] **Step 4: Run the tests**

Run: `go test ./pkg/desktop/markdown/ -v`
Expected: PASS.

- [ ] **Step 5: Promote goldmark and commit**

```bash
go mod tidy
git add pkg/desktop/markdown go.mod go.sum
git commit -m "$(cat <<'EOF'
feat(gui): add a Markdown block parser for prose

Parse the subset of Markdown the app shows (emphasis, strong, code,
wikilinks, headings, lists, quotes, rules) into flat blocks with styled
rune ranges, so prose can render as shirei Text.

💘 Generated with Crush

Assisted-by: Crush:deepseek-v4.1-flash
EOF
)"
```

---

### Task 2: Render prose blocks with shirei Text

**Files:**
- Create: `pkg/desktop/prose.go`
- Test: `pkg/desktop/prose_test.go`
- Create: `pkg/desktop/testdata/snapshots/prose.png`

**Interfaces:**
- Consumes: `markdown.Parse`, `markdown.Block`, `markdown.SpanRange`.
- Produces: `func proseBlocks(src string, base ...TextStyleFn)` (unexported) and `func proseSpans(b markdown.Block) []TextSpan`.

- [ ] **Step 1: Write the failing test**

Create `pkg/desktop/prose_test.go`:

```go
package desktop

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/ui"
)

func TestProseSnapshot(t *testing.T) {
	appState = &State{
		Loaded:  true,
		Screen:  ScreenChronicle,
		Prose:   "A *quiet* room.\n\n**Vance** waits.\n\n- one\n- two\n\n> a whisper\n\n---\n\nMeet [[hero|Vance]].",
		ChronicleSource: true,
	}
	ui.Snapshot(t, "prose", 600, 500, RootView)
}
```

(`ScreenChronicle` and `State.Prose` are added by Task 3; if writing this test first, add the one-line `Prose` field and `ScreenChronicle` constant in `state.go` as part of Task 2 to keep the test compiling.)

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/desktop/ -run TestProseSnapshot -v`
Expected: FAIL — `proseBlocks` undefined.

- [ ] **Step 3: Implement the renderer**

Create `pkg/desktop/prose.go`:

```go
package desktop

import (
	. "go.hasen.dev/shirei"

	"github.com/darkliquid/localrpg/pkg/desktop/markdown"
)

// proseBlocks renders Markdown source as shirei text. base is the paragraph
// text style for plain runs; spans derive their style from it.
func proseBlocks(src string, base ...TextStyleFn) {
	for _, b := range markdown.Parse(src) {
		switch b.Kind {
		case markdown.BlockHeading:
			if b.Level <= 2 {
				Label(b.Text, append([]TextStyleFn{FontWeight(WeightBold), FontSize(18)}, base...)...)
			} else {
				Label(b.Text, append([]TextStyleFn{FontWeight(WeightBold)}, base...)...)
			}
		case markdown.BlockList:
			for _, item := range b.Items {
				Label("• "+item, base...)
			}
		case markdown.BlockQuote:
			Container(Attrs(Pad2(4, 8), BackgroundVec(quoteBackground())), func() {
				Label(b.Text, base...)
			})
		case markdown.BlockRule:
			Element(Attrs(Expand, FixHeight(1), BackgroundVec(ruleColor())))
		default:
			Text(b.Text, TextStyle(base...), proseSpans(b)...)
		}
	}
}

// proseSpans maps the parser's rune-range spans to shirei text spans. Indices
// from the parser are byte offsets; convert to rune offsets shirei expects.
func proseSpans(b markdown.Block) []TextSpan {
	var spans []TextSpan
	for _, s := range b.Spans {
		from := runeIndex(b.Text, s.From)
		to := runeIndex(b.Text, s.To)
		switch s.Kind {
		case markdown.SpanEmphasis:
			spans = append(spans, Span(from, to, FontStyle(StyleItalic)))
		case markdown.SpanStrong:
			spans = append(spans, Span(from, to, FontWeight(WeightBold)))
		case markdown.SpanCode:
			spans = append(spans, Span(from, to, Fonts(Monospace...)))
		case markdown.SpanWikilink:
			spans = append(spans, Span(from, to, TextColorVec(wikilinkColor())))
		}
	}
	return spans
}

func runeIndex(s string, byteOff int) int {
	return len([]rune(s[:min(byteOff, len(s))]))
}

func quoteBackground() Vec4  { return DefaultPalette().Border }
func ruleColor() Vec4        { return DefaultPalette().Border }
func wikilinkColor() Vec4    { return DefaultPalette().Accent }
```

Confirm `FontStyle`, `StyleItalic`, `Fonts`, `Monospace`, `TextColorVec`, and `Span` against `attrs.go`/`text.go` before compiling; use the vendored spellings. If converting byte→rune offsets proves fiddly, change `markdown.Parse` to emit rune offsets directly (its tests remain the contract).

- [ ] **Step 4: Run the test and regenerate goldens**

Run: `go test ./pkg/desktop/ -v && mise run desktop:snapshots`
Expected: PASS; `prose` golden created. View it to confirm emphasis, strong, list, quote, rule, and wikilink styling.

- [ ] **Step 5: Commit**

```bash
git add pkg/desktop
git commit -m "feat(gui): render prose Markdown with shirei text spans"
```

---

### Task 3: The chronicle screen

**Files:**
- Modify: `pkg/desktop/state.go` (add `ScreenChronicle`, `Turns`, `Prose`, `ChronicleSource`)
- Create: `pkg/desktop/chronicle.go`
- Modify: `pkg/desktop/data.go` (load the chronicle on game open)
- Test: `pkg/desktop/chronicle_test.go`
- Create: `pkg/desktop/testdata/snapshots/chronicle.png`

**Interfaces:**
- Consumes: `(*gui.Service).GetChronicle(ctx, gameID string) ([]gui.TurnDTO, error)` (`pkg/gui/service.go:903`), `TurnDTO`/`SegmentDTO`/`CheckResult` (`pkg/gui/types.go:66/53/89`), `proseBlocks` from Task 2.
- Produces: `ScreenChronicle`, `State.Turns []gui.TurnDTO`, `State.OpenGame string`, `func chronicleView()`, `func loadChronicle(ctx, svc, gameID) []gui.TurnDTO`.

- [ ] **Step 1: Write the failing snapshot test**

Create `pkg/desktop/chronicle_test.go`:

```go
package desktop

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/ui"
)

func TestChronicleSnapshot(t *testing.T) {
	appState = &State{
		Loaded:   true,
		Screen:   ScreenChronicle,
		OpenGame: "campaign-01",
		Turns: []gui.TurnDTO{
			{
				TurnNumber: 1,
				InputText:  "I open the door.",
				Mode:       "do",
				LocationID: "hall",
				Segments: []gui.SegmentDTO{
					{Kind: "narration", Text: "The *door* groans open."},
					{Kind: "speech", Speaker: "Vance", Text: "Hello?", Player: true},
				},
				Checks: []harness.CheckResult{
					{CheckID: "c1", CheckKind: "skill", Stakes: "notice the trap", Outcome: "failure",
						Roll: &harness.RollSummary{Notation: "1d20", Total: 4, RollCount: 1}},
				},
			},
		},
	}
	ui.Snapshot(t, "chronicle", 800, 600, RootView)
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/desktop/ -run TestChronicleSnapshot -v`
Expected: FAIL — `ScreenChronicle`/`chronicleView` undefined.

- [ ] **Step 3: Add state and the loader**

In `state.go`, add `ScreenChronicle` to the screen constants and to `State`: `Turns []gui.TurnDTO`, `Prose string`, `ChronicleSource bool`, `OpenGame string`.

In `data.go`, add:

```go
// loadChronicle reads the campaign's turn history.
func loadChronicle(ctx context.Context, svc *gui.Service, gameID string) []gui.TurnDTO {
	turns, err := svc.GetChronicle(ctx, gameID)
	if err != nil || turns == nil {
		return []gui.TurnDTO{}
	}
	return turns
}
```

- [ ] **Step 4: Build the chronicle view**

Create `pkg/desktop/chronicle.go` with `chronicleView()`: a `Container(Attrs(Viewport))` calling `ScrollOnInput()` and `ScrollBars()` (confirm the exact names in the vendored source), then for each `gui.TurnDTO`:

- If `t.InputText != ""` and no player segment covers it, render a muted chip `[mode]` + the input text.
- Render `t.LocationName` art via `Image` only when `t.LocationID` differs from the previous turn's.
- Render segments in order: narration via `proseBlocks(seg.Text, TextColorVec(p.Text))`; speech with speaker name, a portrait `Image` when `seg.PortraitURL` resolves to a local file, and `proseBlocks`.
- Before a segment whose `SegmentDTO.CheckRef` matches a `t.Checks[i].CheckID`, render `checkCard(p, check)`.
- Render `t.ToolCalls` as a muted summary, `t.Rejected`+`t.Verdict.Reason`, `t.Truncated`, and `t.ContextNotes`.
- Finish with the in-flight block: if `appState.TurnInFlight` is true, show `appState.PendingAction` and `appState.Prose`.

Add a `checkCard` that draws notation/total/outcome/stakes from `harness.CheckResult` (mirroring `DiceCheckCard.tsx`, simplified to text plus a coloured outcome chip).

- [ ] **Step 5: Dispatch and run**

In `RootView`, add `case ScreenChronicle: chronicleView()`. Run the test and regenerate goldens:

Run: `go test ./pkg/desktop/ -v && mise run desktop:snapshots`
Expected: PASS; `chronicle` golden created. View it.

- [ ] **Step 6: Commit**

```bash
git add pkg/desktop
git commit -m "feat(gui): add the chronicle screen with dice check cards"
```

---

### Task 4: In-process turn submission

**Files:**
- Modify: `pkg/desktop/state.go` (in-flight fields)
- Create: `pkg/desktop/turn.go`
- Test: `pkg/desktop/turn_test.go`

**Interfaces:**
- Consumes: `(*gui.Service).BeginTurn(gameID string) (*gui.TurnSession, error)` (`pkg/gui/service.go:1008`), `(*TurnSession).Run(ctx, req gui.TurnRequest, emit func(gui.TurnEvent) error) error` (`:1286`), `(*TurnSession).Close()` (`:1033`), `gui.ErrTurnInFlight`/`gui.ErrCampaignNotPlayable` (`:970/:974`), `gui.TurnEvent` (`types.go:405`).
- Produces:
  - `type desktop.TurnRunner interface { Begin(gameID string) (TurnSessionCloser, error) }` (small seam), or a function var `runTurn` — see Step 3.
  - `func submitTurn(ctx context.Context, svc *gui.Service, gameID, mode, input string)`
  - `State.TurnInFlight bool`, `State.PendingAction string`, `State.StreamedProse string`, `State.ToolActivity string`, `State.TurnError string`

- [ ] **Step 1: Write the failing test**

Create `pkg/desktop/turn_test.go`:

```go
package desktop

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/gui"
)

func TestSubmitTurnAppendsChunksAndTurn(t *testing.T) {
	appState = &State{Loaded: true, OpenGame: "campaign-01", Screen: ScreenChronicle}

	runTurn = func(ctx context.Context, svc *gui.Service, gameID string, req gui.TurnRequest, emit func(gui.TurnEvent) error) error {
		if req.Mode != "do" || req.Input != "look" {
			t.Fatalf("req = %+v", req)
		}
		_ = emit(gui.TurnEvent{Type: "chunk", Text: "The "})
		_ = emit(gui.TurnEvent{Type: "chunk", Text: "door opens."})
		_ = emit(gui.TurnEvent{Type: "turn", Turn: &gui.TurnDTO{TurnNumber: 2, InputText: "look"}})
		return nil
	}
	t.Cleanup(func() { runTurn = nil })

	submitTurn(context.Background(), nil, "campaign-01", "do", "look")

	if appState.TurnInFlight {
		t.Error("turn should not still be in flight")
	}
	if len(appState.Turns) != 1 || appState.Turns[0].TurnNumber != 2 {
		t.Errorf("turns = %+v", appState.Turns)
	}
	if appState.StreamedProse != "" {
		t.Errorf("streamed prose should be cleared after the turn event, got %q", appState.StreamedProse)
	}
}

func TestSubmitTurnReportsInFlight(t *testing.T) {
	appState = &State{Loaded: true, OpenGame: "campaign-01"}
	runTurn = func(context.Context, *gui.Service, string, gui.TurnRequest, func(gui.TurnEvent) error) error {
		return gui.ErrTurnInFlight
	}
	t.Cleanup(func() { runTurn = nil })

	submitTurn(context.Background(), nil, "campaign-01", "do", "look")
	if appState.TurnError == "" {
		t.Fatal("expected an in-flight error message")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/desktop/ -run TestSubmitTurn -v`
Expected: FAIL — `runTurn`/`submitTurn` undefined.

- [ ] **Step 3: Implement submission behind a seam**

Create `pkg/desktop/turn.go`:

```go
package desktop

import (
	"context"
	"errors"

	"go.hasen.dev/shirei"

	"github.com/darkliquid/localrpg/pkg/gui"
)

// runTurn is the turn seam. It mirrors BeginTurn + TurnSession.Run so tests can
// drive the stream without a model or a service.
var runTurn = func(ctx context.Context, svc *gui.Service, gameID string, req gui.TurnRequest, emit func(gui.TurnEvent) error) error {
	session, err := svc.BeginTurn(gameID)
	if err != nil {
		return err
	}
	defer session.Close()
	return session.Run(ctx, req, emit)
}

// submitTurn runs one turn, publishing streamed progress under the frame lock.
func submitTurn(ctx context.Context, svc *gui.Service, gameID, mode, input string) {
	shirei.WithFrameLock(func() {
		appState.TurnInFlight = true
		appState.PendingAction = input
		appState.StreamedProse = ""
		appState.ToolActivity = ""
		appState.TurnError = ""
	})
	shirei.RequestNextFrame()

	emit := func(ev gui.TurnEvent) error {
		shirei.WithFrameLock(func() {
			switch ev.Type {
			case "chunk":
				appState.StreamedProse += ev.Text
			case "tool":
				appState.ToolActivity = ev.ToolSummary
			case "turn":
				if ev.Turn != nil {
					appState.Turns = append(appState.Turns, *ev.Turn)
				}
				appState.StreamedProse = ""
			case "model_missing":
				appState.TurnError = "model missing: " + ev.Name
			case "error":
				appState.TurnError = ev.Message
			}
		})
		shirei.RequestNextFrame()
		return nil
	}

	err := runTurn(ctx, svc, gameID, gui.TurnRequest{Mode: mode, Input: input}, emit)
	shirei.WithFrameLock(func() {
		appState.TurnInFlight = false
		appState.PendingAction = ""
		switch {
		case err == nil:
		case errors.Is(err, gui.ErrTurnInFlight):
			appState.TurnError = "A turn is already in flight."
		case errors.Is(err, gui.ErrCampaignNotPlayable):
			appState.TurnError = "The campaign could not be prepared."
		default:
			appState.TurnError = err.Error()
		}
	})
	shirei.RequestNextFrame()
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./pkg/desktop/ -run TestSubmitTurn -v`
Expected: PASS.

- [ ] **Step 5: Wire the live runner**

In `Run`, when `cfg.Service != nil`, keep the default `runTurn` and set `liveService = cfg.Service`. (The default already closes over the passed service, so no extra wiring is needed beyond `liveService`.)

- [ ] **Step 6: Commit**

```bash
git add pkg/desktop
git commit -m "feat(gui): submit turns in-process through BeginTurn"
```

---

### Task 5: The action console and prologue

**Files:**
- Create: `pkg/desktop/console.go`
- Modify: `pkg/desktop/chronicle.go` (render the console and the prologue)
- Test: `pkg/desktop/console_test.go`

**Interfaces:**
- Consumes: `submitTurn` from Task 4.
- Produces: `State.ConsoleMode string`, `State.ConsoleText string`, `func actionConsole()`, `func prologuePanel()`.

- [ ] **Step 1: Write the failing test**

Create `pkg/desktop/console_test.go`:

```go
package desktop

import "testing"

func TestParseConsolePrefix(t *testing.T) {
	cases := []struct{ in, mode, text string }{
		{"/say hello", "say", "hello"},
		{"/do look", "do", "look"},
		{"/story once upon", "story", "once upon"},
		{"/roll 1d20", "roll", "1d20"},
		{"just looking", "do", "just looking"},
	}
	for _, c := range cases {
		mode, text := parseConsole(c.in, "do")
		if mode != c.mode || text != c.text {
			t.Errorf("parseConsole(%q) = (%q,%q), want (%q,%q)", c.in, mode, text, c.mode, c.text)
		}
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/desktop/ -run TestParseConsolePrefix -v`
Expected: FAIL — `parseConsole` undefined.

- [ ] **Step 3: Implement the console**

Create `pkg/desktop/console.go`:

```go
package desktop

import (
	"context"
	"strings"

	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/darkliquid/localrpg/pkg/ui"
)

// parseConsole applies the /say, /do, /story, /roll prefixes, falling back to
// the supplied mode.
func parseConsole(input, fallback string) (string, string) {
	input = strings.TrimSpace(input)
	for _, p := range []struct{ prefix, mode string }{
		{"/say", "say"}, {"/do", "do"}, {"/story", "story"}, {"/roll", "roll"},
	} {
		if strings.HasPrefix(input, p.prefix) {
			return p.mode, strings.TrimSpace(strings.TrimPrefix(input, p.prefix))
		}
	}
	return fallback, input
}

func actionConsole() {
	p := ui.DefaultPalette()
	Container(Attrs(Expand, Pad(12), Gap(8), BackgroundVec(p.Panel)), func() {
		Container(Attrs(Row, Gap(6)), func() {
			for _, m := range []string{"do", "say", "story", "roll"} {
				mode := m
				selected := appState.ConsoleMode == mode
				Container(Attrs(Pad2(4, 10), Corners(6), BackgroundVec(p.Bg)), func() {
					if selected {
						ModAttrs(BackgroundVec(p.Accent))
					}
					NextAccessName("console.mode." + mode)
					if PressAction() {
						appState.ConsoleMode = mode
					}
					AssignAccess()
					Label(mode, FontSize(12), TextColorVec(p.Text))
				})
			}
		})

		TextInput(&appState.ConsoleText)

		Container(Attrs(Row, CrossMid, Gap(10)), func() {
			if appState.TurnError != "" {
				Label(appState.TurnError, FontSize(12), TextColorVec(p.Danger))
			}
			Filler(1)
			NextAccessName("console.submit")
			if !appState.TurnInFlight && appState.ConsoleText != "" && Button(NoIcon, "Act") {
				mode, text := parseConsole(appState.ConsoleText, appState.ConsoleMode)
				appState.ConsoleText = ""
				svc := liveService
				gameID := appState.OpenGame
				go submitTurn(context.Background(), svc, gameID, mode, text)
			}
			AssignAccess()
		})
	})
}

func prologuePanel() {
	p := ui.DefaultPalette()
	Container(Attrs(Expand, Grow(1), Center, Gap(10), Pad(24)), func() {
		Label(appState.GameName(), FontSize(22), FontWeight(WeightBold), TextColorVec(p.Text))
		Label("Begin the story", FontSize(14), TextColorVec(p.Muted))
		NextAccessName("prologue.begin")
		if Button(NoIcon, "Begin the story") {
			svc := liveService
			gameID := appState.OpenGame
			go submitTurn(context.Background(), svc, gameID, "story", "")
		}
		AssignAccess()
	})
}
```

Add `func (s *State) GameName() string` (first matching `Games` entry, else "LocalRPG"). In `chronicleView`, render `prologuePanel()` when `len(appState.Turns) == 0`, else the turn list plus `actionConsole()` at the bottom.

- [ ] **Step 4: Run tests and goldens**

Run: `go test ./pkg/desktop/ -v && mise run desktop:snapshots`
Expected: PASS.

- [ ] **Step 5: Full gate and manual play**

Run: `mise run test`
Expected: PASS (re-run if the `pkg/gui` flake triggers).

Run: `LOCALRPG_UI=shirei go run ./cmd/localrpg gui --dir <dir-with-a-campaign>` and open a campaign.
Expected: the chronicle loads; typing an action streams prose then the turn; dice cards appear where checks resolve; audio controls are not yet wired (Task deferred below).

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat(gui): add the action console and prologue"
```

---

## Self-Review

**Spec coverage (Phase 4, core loop):**

| Spec item | Task |
| --- | --- |
| `MarkdownProse` / goldmark renderer | Tasks 1-2 |
| `ChronicleView`, `TurnHistoryList` (turn list) | Task 3 |
| `TurnSegments` (segments), `DiceCheckCard` (checks) | Task 3 |
| `ActionConsole` | Task 5 |
| `ProloguePanel` | Task 5 |
| in-process turn submission (replaces NDJSON) | Task 4 |
| `SegmentAudioControls` | deferred: needs the portability/audio plan's player surfaced to the GUI; add a follow-up task once Plan 3's player is GUI-wired |
| `ContextDrawer`, `GraphDrawer`, `LivingWorldDrawer`, `CharacterSheetDrawer`, `CodexDrawer` | deferred to the next plan (drawers) |

**Placeholder scan:** No TBDs. Tasks 2-4 instruct the implementer to confirm shirei identifier spellings (`FontStyle`, `Fonts`, `Monospace`, `ScrollOnInput`, `ScrollBars`) and DTO field names against the source; that is factual verification.

**Type consistency:** `markdown.Block`/`SpanRange`/`BlockKind`/`SpanKind`, `proseBlocks`, `proseSpans`, `ScreenChronicle`, `Turns`, `StreamedProse`, `runTurn`, `submitTurn`, `parseConsole`, `actionConsole`, `prologuePanel` are each defined once and referenced consistently. Service signatures come from `pkg/gui/service.go:1008/1286/1033/903` and DTOs from `pkg/gui/types.go:66/53/405`.

**Known deferrals (not gaps):** audio controls, the drawers, and the shared theatre are later plans. Wikilinks render as styled spans, not clickable links, in v1.
