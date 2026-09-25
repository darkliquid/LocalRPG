# Turn Completion and Trimming Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Recover a narrator reply that stops mid-thought, so a turn is never recorded as a hard cut and a mid-stream failure never loses a turn.

**Architecture:** Three separable pieces. Pure prose helpers in `pkg/harness` answer "is this a complete ending?" and "where is the last balanced sentence boundary?". A shared `stream` pump in `pkg/engine` keeps the text that arrived before a provider failure. A recovery pass in `pkg/engine` runs after generation and before segmentation: it first tries one narrow "finish this" call, then trims to the last balanced boundary, and always records a turn.

**Tech Stack:** Go 1.27.1 (standard library only for tests), existing `harness.Router`/`ModelProvider`, `trace.Logger`, React 19 + TypeScript in `frontend/`.

**Spec:** `docs/superpowers/specs/2026-09-22-turn-completion-and-trimming-design.md`

## Global Constraints

- Tests use only `testing` and `t.TempDir()`; no testify, no new dependencies.
- Use `interface{}`, never `any`. `go vet ./...` must stay clean.
- Never write em dashes in source code; use commas, periods, parentheses, or semicolons.
- Follow the existing zero-safe config accessor pattern: an omitted value returns the default, so configuration written before a key existed keeps working.
- Errors are wrapped with `fmt.Errorf("...: %w", err)`.
- Conventional Commits with a scope, subject under 72 characters.
- Verification command for the whole plan: `mise run test` (which runs `go test -v -count=1 ./...` and `npx tsc --noEmit` in `frontend/`), plus `mise run lint` (`go vet ./...`).
- Single Go test example: `go test -run TestProseComplete ./pkg/harness/`.

### File Map

| Action | Path | Responsibility |
| :--- | :--- | :--- |
| Create | `pkg/harness/prose.go` | `ProseComplete`, `LastSentenceBoundary`, `TrimToLastSentence`, `StitchContinuation`, and their pure helpers |
| Create | `pkg/harness/prose_test.go` | Table tests for terminators, abbreviations, balance, trim, stitching |
| Modify | `pkg/config/types.go` | `CompletionConfig`, `RoleCompletion`, accessors, `DefaultConfig` block and role entry |
| Modify | `pkg/config/types_test.go` | Defaults and zero-safe accessors |
| Modify | `pkg/harness/router.go` | `FallbackForRole` |
| Modify | `pkg/harness/factory.go` | `CompletionFromConfig` |
| Modify | `pkg/harness/factory_test.go` | Completion role resolution, inheritance, and disable |
| Modify | `pkg/engine/orchestrator.go` | `streamResult`, `stream`, `generate` wrapper, `classifyCut`, recovery wiring, trace event |
| Modify | `pkg/engine/orchestrator_stream_test.go` | Interrupted stream keeps partial text; no text still fails |
| Create | `pkg/engine/recovery.go` | `RecoveryOutcome`, `CutCause`, `CompletionPolicy`, `recoverReply`, prompt and tail builders |
| Create | `pkg/engine/recovery_test.go` | Continue, trim fallback, short-reply skip, mode matrix, disconnect |
| Modify | `pkg/engine/history.go` | `Turn.Recovery` |
| Modify | `pkg/gui/types.go` | `TurnDTO.Recovery` |
| Modify | `pkg/gui/service.go` | `turnDTO` copies `Recovery`; `prepareTurn` wires the completion provider and policy |
| Modify | `cmd/localrpg/play.go` | Wire the completion provider and policy into the TUI orchestrator |
| Modify | `frontend/src/types.ts` | `recovery?: string` |
| Modify | `frontend/src/components/ChronicleView.tsx` | Trimmed note; broaden the truncated note |

---

## Task 1: Prose completeness and boundary detection

**Files:**
- Create: `pkg/harness/prose.go`
- Test: `pkg/harness/prose_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `harness.ProseComplete(text string) bool`, `harness.LastSentenceBoundary(text string, minChars int) (int, bool)`, `harness.TrimToLastSentence(text string, minChars int) (string, bool)`.

- [x] **Step 1: Write the failing test**

Create `pkg/harness/prose_test.go`:

```go
package harness

import "testing"

func TestProseComplete(t *testing.T) {
	cases := []struct {
		name string
		text string
		want bool
	}{
		{"sentence end", "The gate stands open.", true},
		{"question", "Do you hear it?", true},
		{"exclamation", "Run!", true},
		{"ellipsis rune", "The road ends here…", true},
		{"three dots", "The road ends here...", true},
		{"trailing closing quote", `He said "stop."`, true},
		{"scene break", "The road ends here.\n\n---", true},
		{"heading ending", "## The Gate", true},
		{"mid word", "The gate stands op", false},
		{"mid sentence no terminator", "The gate stands open, and then", false},
		{"decimal is not a terminator", "The gate is 3.5", false},
		{"domain is not a terminator", "Visit example.com", false},
		{"abbreviation", "Dr. Halloway arrived", false},
		{"initial", "H. P. Lovecraft wrote", false},
		{"open quotation", `He said "stop`, false},
		{"open emphasis", "The *gate", false},
		{"unterminated wikilink", "The [[gate", false},
		{"empty", "", false},
		{"whitespace", "   \n  ", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ProseComplete(tc.text); got != tc.want {
				t.Errorf("ProseComplete(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}

func TestLastSentenceBoundary(t *testing.T) {
	text := "The gate stands open before us all. Its hinges groan, and then"
	offset, ok := LastSentenceBoundary(text, 24)
	if !ok {
		t.Fatalf("expected a boundary in %q", text)
	}
	if got := text[:offset]; got != "The gate stands open before us all." {
		t.Errorf("boundary text = %q", got)
	}

	if _, ok := LastSentenceBoundary("Too short. And more", 24); ok {
		t.Errorf("expected no boundary when the first ends before minChars")
	}
	if _, ok := LastSentenceBoundary("no terminator anywhere", 4); ok {
		t.Errorf("expected no boundary without a terminator")
	}
}

func TestTrimToLastSentence(t *testing.T) {
	trimmed, ok := TrimToLastSentence("The gate stands open before us all. Its hinges groan, and then", 24)
	if !ok {
		t.Fatalf("expected a trim")
	}
	if trimmed != "The gate stands open before us all." {
		t.Errorf("TrimToLastSentence = %q", trimmed)
	}

	if got, ok := TrimToLastSentence("no terminator anywhere", 4); ok {
		t.Errorf("expected no trim, got %q", got)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run 'TestProseComplete|TestLastSentenceBoundary|TestTrimToLastSentence' ./pkg/harness/ -v`
Expected: FAIL with "undefined: ProseComplete".

- [x] **Step 3: Write minimal implementation**

Create `pkg/harness/prose.go`:

```go
package harness

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// abbreviations are word tokens whose trailing period does not end a sentence.
// The list is deliberately short: a missed abbreviation costs one sentence of
// trim, which is cheaper than treating a real sentence end as an abbreviation.
var abbreviations = map[string]bool{
	"mr": true, "mrs": true, "ms": true, "dr": true, "prof": true,
	"sr": true, "jr": true, "st": true, "vs": true, "etc": true,
	"e.g": true, "i.e": true, "approx": true, "dept": true, "no": true,
}

// ProseComplete reports whether text ends at a natural boundary: a sentence
// terminator, or a scene break or heading line, with every construct the text
// opened (quote, emphasis, wikilink) closed.
func ProseComplete(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	if completeEnding(trimmed) {
		return true
	}
	if !balancedText(trimmed) {
		return false
	}
	return endsAtTerminator(trimmed)
}

// LastSentenceBoundary returns the byte offset just past the last sentence
// terminator in text that leaves all open constructs balanced. ok is false when
// no such boundary exists, or when the boundary sits before minChars.
func LastSentenceBoundary(text string, minChars int) (int, bool) {
	if minChars < 0 {
		minChars = 0
	}
	trimmed := strings.TrimRight(text, " \t\r\n")
	if trimmed == "" {
		return 0, false
	}
	best := -1
	for _, offset := range sentenceTerminators(trimmed) {
		prefix := trimmed[:offset]
		if utf8.RuneCountInString(prefix) < minChars {
			continue
		}
		if !balancedText(prefix) {
			continue
		}
		best = offset
	}
	if best < 0 {
		return 0, false
	}
	return best, true
}

// TrimToLastSentence returns text cut back to its last balanced sentence
// boundary. It returns ("", false) when there is no usable boundary.
func TrimToLastSentence(text string, minChars int) (string, bool) {
	offset, ok := LastSentenceBoundary(text, minChars)
	if !ok {
		return "", false
	}
	trimmed := strings.TrimRight(text[:offset], " \t\r\n")
	if strings.TrimSpace(trimmed) == "" {
		return "", false
	}
	return trimmed, true
}

// balancedText reports whether every construct the text opens is closed. An
// unbalanced marker means a candidate boundary cannot be trusted, because a
// reply cut inside an open construct must trim to the sentence before it.
func balancedText(text string) bool {
	if strings.Count(text, "[[") != strings.Count(text, "]]") {
		return false
	}
	final := finalParagraph(text)
	if unescapedCount(final, '"')%2 != 0 {
		return false
	}
	return emphasisBalanced(final)
}

// finalParagraph is the text after the last blank-line break. An open quote in an
// earlier paragraph was closed by the paragraph break, so only the final one is
// inspected.
func finalParagraph(text string) string {
	trimmed := strings.TrimRight(text, " \t\r\n")
	if index := strings.LastIndex(trimmed, "\n\n"); index >= 0 {
		return trimmed[index+2:]
	}
	return trimmed
}

// unescapedCount counts occurrences of mark that are not preceded by a backslash.
func unescapedCount(text string, mark byte) int {
	count := 0
	for i := 0; i < len(text); i++ {
		if text[i] == mark && (i == 0 || text[i-1] != '\\') {
			count++
		}
	}
	return count
}

// emphasisBalanced reports whether asterisk runs pair up under the same
// no-internal-whitespace rule media.SpeakableText uses: a marker binds only when
// its inner side touches non-space text.
func emphasisBalanced(text string) bool {
	for _, line := range strings.Split(text, "\n") {
		runes := []rune(line)
		singles, doubles := 0, 0
		i := 0
		for i < len(runes) {
			if runes[i] != '*' {
				i++
				continue
			}
			start := i
			for i < len(runes) && runes[i] == '*' {
				i++
			}
			run := i - start
			prevSpace := start == 0 || unicode.IsSpace(runes[start-1])
			nextSpace := i >= len(runes) || unicode.IsSpace(runes[i])

			if !prevSpace {
				for pairs := run / 2; pairs > 0; pairs-- {
					if doubles == 0 {
						return false
					}
					doubles--
				}
				if run%2 == 1 {
					if singles == 0 {
						return false
					}
					singles--
				}
				continue
			}
			if !nextSpace {
				doubles += run / 2
				singles += run % 2
			}
		}
		if singles != 0 || doubles != 0 {
			return false
		}
	}
	return true
}

// completeEnding reports whether the text ends on a scene break or a heading,
// either of which is a complete ending without a sentence terminator.
func completeEnding(text string) bool {
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		return isSceneBreakLine(line) || isHeadingLine(line)
	}
	return false
}

func isSceneBreakLine(line string) bool {
	if len(line) < 3 {
		return false
	}
	for i := 0; i < len(line); i++ {
		if line[i] != line[0] {
			return false
		}
	}
	switch line[0] {
	case '-', '*', '_':
		return true
	default:
		return false
	}
}

func isHeadingLine(line string) bool {
	hashes := 0
	for hashes < len(line) && line[hashes] == '#' {
		hashes++
	}
	return hashes >= 1 && hashes <= 6 && hashes < len(line) && line[hashes] == ' '
}

// endsAtTerminator reports whether the text finishes on a sentence terminator,
// allowing one trailing closing quote or bracket.
func endsAtTerminator(text string) bool {
	trimmed := strings.TrimRight(text, " \t\r\n")
	trimmed = strings.TrimRight(trimmed, "\"'”’)")
	if trimmed == "" {
		return false
	}
	runes := []rune(trimmed)
	switch runes[len(runes)-1] {
	case '!', '?', '…':
		return true
	case '.':
		start := len(runes)
		for start > 0 && runes[start-1] == '.' {
			start--
		}
		count := len(runes) - start
		if count >= 3 {
			return true
		}
		return !isAbbreviation(runes[:start])
	default:
		return false
	}
}

// sentenceTerminators returns, in order, the byte offsets just past each
// sentence terminator that is followed by whitespace or the end of the text.
func sentenceTerminators(text string) []int {
	offsets := make([]int, 0)
	runes := []rune(text)
	for i := 0; i < len(runes); {
		r := runes[i]
		if r != '.' && r != '!' && r != '?' && r != '…' {
			i++
			continue
		}
		start := i
		for i < len(runes) && runes[i] == r {
			i++
		}
		if r == '!' || r == '?' {
			for i < len(runes) && (runes[i] == '!' || runes[i] == '?') {
				i++
			}
		}
		count := i - start

		// A terminator sits before a break, possibly with closing quotes between.
		end := i
		for end < len(runes) && isClosingRune(runes[end]) {
			end++
		}
		if end < len(runes) && !unicode.IsSpace(runes[end]) {
			continue
		}
		if r == '.' && count < 3 && isAbbreviation(runes[:start]) {
			continue
		}
		offsets = append(offsets, byteOffset(runes, end))
	}
	return offsets
}

// byteOffset is the byte index of rune index in text.
func byteOffset(runes []rune, index int) int {
	offset := 0
	for i := 0; i < index && i < len(runes); i++ {
		offset += utf8.RuneLen(runes[i])
	}
	return offset
}

// isClosingRune reports whether a rune may close a quotation or bracket.
func isClosingRune(r rune) bool {
	switch r {
	case '"', '\'', '”', '’', ')', ']', '}':
		return true
	default:
		return false
	}
}

// isAbbreviation reports whether the word ending at the end of prefix is a known
// abbreviation or a single-letter initial.
func isAbbreviation(prefix []rune) bool {
	end := len(prefix)
	for end > 0 && unicode.IsSpace(prefix[end-1]) {
		end--
	}
	begin := end
	for begin > 0 && (unicode.IsLetter(prefix[begin-1]) || prefix[begin-1] == '.') {
		begin--
	}
	token := strings.ToLower(string(prefix[begin:end]))
	if token == "" {
		return false
	}
	if utf8.RuneCountInString(token) == 1 {
		return true
	}
	return abbreviations[token]
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run 'TestProseComplete|TestLastSentenceBoundary|TestTrimToLastSentence' ./pkg/harness/ -v`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/harness/prose.go pkg/harness/prose_test.go
git commit -m "feat(harness): detect complete endings and trim to a safe boundary"
```

---

## Task 2: Stitch a continuation onto the text it resumes

**Files:**
- Modify: `pkg/harness/prose.go`
- Test: `pkg/harness/prose_test.go`

**Interfaces:**
- Consumes: nothing from Task 1.
- Produces: `harness.StitchContinuation(existing, continuation string) string`.

- [x] **Step 1: Write the failing test**

Append to `pkg/harness/prose_test.go`:

```go
func TestStitchContinuation(t *testing.T) {
	cases := []struct {
		name         string
		existing     string
		continuation string
		want         string
	}{
		{
			name:         "mid word cut resumes the word",
			existing:     "The old hinge",
			continuation: "s groan in the wind.",
			want:         "The old hinges groan in the wind.",
		},
		{
			name:         "trailing space is not doubled",
			existing:     "The gate stands open, and the hinges groan ",
			continuation: "in the rising wind.",
			want:         "The gate stands open, and the hinges groan in the rising wind.",
		},
		{
			name:         "sentence end joins with one space",
			existing:     "The gate stands open.",
			continuation: "Its hinges groan.",
			want:         "The gate stands open. Its hinges groan.",
		},
		{
			name:         "leading punctuation attaches directly",
			existing:     "The gate stands open",
			continuation: ", and the hinges groan.",
			want:         "The gate stands open, and the hinges groan.",
		},
		{
			name:         "overlap is removed",
			existing:     "The gate stands open before us all and the hinges",
			continuation: "and the hinges groan in the wind.",
			want:         "The gate stands open before us all and the hinges groan in the wind.",
		},
		{
			name:         "short overlap is ignored",
			existing:     "The gate stands open",
			continuation: "en and the hinges groan.",
			want:         "The gate stands openen and the hinges groan.",
		},
		{
			name:         "preamble label is dropped",
			existing:     "The gate stands open, ",
			continuation: "Continuation: and the hinges groan.",
			want:         "The gate stands open, and the hinges groan.",
		},
		{
			name:         "empty continuation changes nothing",
			existing:     "The gate stands open",
			continuation: "   ",
			want:         "The gate stands open",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := StitchContinuation(tc.existing, tc.continuation); got != tc.want {
				t.Errorf("StitchContinuation(%q, %q) = %q, want %q", tc.existing, tc.continuation, got, tc.want)
			}
		})
	}
}
```

The overlap case uses a 14-rune repeated phrase (`and the hinges`); a shorter overlap such as `hinges` (6 runes) is below the 8-rune floor and is intentionally ignored.

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestStitchContinuation ./pkg/harness/ -v`
Expected: FAIL with "undefined: StitchContinuation".

- [x] **Step 3: Write minimal implementation**

Append to `pkg/harness/prose.go`:

```go
// StitchContinuation joins a continuation onto the text it resumes, removing a
// repeated overlap and repairing the seam. It is the only place the two halves
// are combined, so the seam rules live in one test.
func StitchContinuation(existing, continuation string) string {
	cont := stripContinuationPreamble(continuation)
	if strings.TrimSpace(cont) == "" {
		return existing
	}
	cont = removeOverlap(existing, cont)
	if strings.TrimSpace(cont) == "" || strings.TrimSpace(existing) == "" {
		if strings.TrimSpace(existing) == "" {
			return collapseNewlines(cont)
		}
		return existing
	}

	switch {
	case endsWithSpace(existing):
		return collapseNewlines(existing + strings.TrimLeft(cont, " \t\r\n"))
	case strings.HasPrefix(cont, ",") || strings.HasPrefix(cont, ".") ||
		strings.HasPrefix(cont, ";") || strings.HasPrefix(cont, ":") ||
		strings.HasPrefix(cont, "!") || strings.HasPrefix(cont, "?"):
		return collapseNewlines(strings.TrimRight(existing, " \t\r\n") + cont)
	case endsAlphanumeric(existing) && startsAlphanumeric(cont):
		return collapseNewlines(existing + cont)
	case endsWithSentencePunctuation(existing):
		return collapseNewlines(existing + " " + strings.TrimLeft(cont, " \t\r\n"))
	default:
		return collapseNewlines(existing + " " + strings.TrimLeft(cont, " \t\r\n"))
	}
}

// stripContinuationPreamble removes a leading artifact or label a model may add
// even when told to output only the continuation.
func stripContinuationPreamble(text string) string {
	trimmed := strings.TrimLeft(text, " \t\r\n")
	if strings.HasPrefix(trimmed, "[") {
		if end := strings.Index(trimmed, "]"); end >= 0 {
			trimmed = strings.TrimLeft(trimmed[end+1:], " \t\r\n")
		}
	}
	for _, label := range []string{"Continuation:", "Continuing:", "Continue:"} {
		if strings.HasPrefix(trimmed, label) {
			return strings.TrimLeft(trimmed[len(label):], " \t\r\n")
		}
	}
	return trimmed
}

// removeOverlap drops the longest suffix of existing that also prefixes the
// continuation, when that overlap is at least eight runes. Shorter candidates
// match too readily and are ignored.
func removeOverlap(existing, continuation string) string {
	existingRunes := []rune(existing)
	contRunes := []rune(continuation)
	limit := len(existingRunes)
	if len(contRunes) < limit {
		limit = len(contRunes)
	}
	for length := limit; length >= 8; length-- {
		if string(existingRunes[len(existingRunes)-length:]) == string(contRunes[:length]) {
			return string(contRunes[length:])
		}
	}
	return continuation
}

// collapseNewlines bounds a run of blank lines at one, which is what a stitched
// seam should look like.
func collapseNewlines(text string) string {
	var sb strings.Builder
	newlines := 0
	for _, r := range text {
		if r == '\n' {
			newlines++
			if newlines > 2 {
				continue
			}
		} else {
			newlines = 0
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

func endsWithSpace(text string) bool {
	if text == "" {
		return false
	}
	runes := []rune(text)
	return unicode.IsSpace(runes[len(runes)-1])
}

func endsAlphanumeric(text string) bool {
	if text == "" {
		return false
	}
	runes := []rune(text)
	return unicode.IsLetter(runes[len(runes)-1]) || unicode.IsDigit(runes[len(runes)-1])
}

func startsAlphanumeric(text string) bool {
	if text == "" {
		return false
	}
	r := []rune(text)[0]
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

func endsWithSentencePunctuation(text string) bool {
	if text == "" {
		return false
	}
	runes := []rune(text)
	switch runes[len(runes)-1] {
	case '.', '!', '?', '…', '"', '\'', '”', '’', ')', ']', '}':
		return true
	default:
		return false
	}
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run TestStitchContinuation ./pkg/harness/ -v`
Expected: PASS. Fix the expected strings and the overlap case if the seam rules produce a different but correct join; the implementation, not the test, defines the seam, so align the test to the spec's rules.

- [x] **Step 5: Commit**

```bash
git add pkg/harness/prose.go pkg/harness/prose_test.go
git commit -m "feat(harness): stitch a continuation onto a cut-off reply"
```

---

## Task 3: Completion configuration

**Files:**
- Modify: `pkg/config/types.go`
- Test: `pkg/config/types_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `config.RoleCompletion`, `config.CompletionConfig`, `(*Config).CompletionMode() string`, `(*Config).CompletionAttempts() int`, `(*Config).CompletionTailChars() int`, `(*Config).CompletionMinChars() int`, `(*Config).CompletionTimeout() time.Duration`.

- [x] **Step 1: Write the failing test**

Append to `pkg/config/types_test.go`:

```go
func TestCompletionSettingsHaveDefaults(t *testing.T) {
	empty := &Config{}

	if got := empty.CompletionMode(); got != "auto" {
		t.Errorf("CompletionMode() = %q, want auto for an omitted setting", got)
	}
	if got := empty.CompletionAttempts(); got != 1 {
		t.Errorf("CompletionAttempts() = %d, want 1", got)
	}
	if got := empty.CompletionTailChars(); got != 1500 {
		t.Errorf("CompletionTailChars() = %d, want 1500", got)
	}
	if got := empty.CompletionMinChars(); got != 24 {
		t.Errorf("CompletionMinChars() = %d, want 24", got)
	}
	if got := empty.CompletionTimeout(); got != 45*time.Second {
		t.Errorf("CompletionTimeout() = %v, want 45s", got)
	}

	configured := &Config{Agents: AgentsConfig{Completion: CompletionConfig{
		Mode:               "trim",
		MaxAttempts:        3,
		TailChars:          800,
		MinIncompleteChars: 40,
		TimeoutSeconds:     10,
	}}}
	if got := configured.CompletionMode(); got != "trim" {
		t.Errorf("CompletionMode() = %q, want trim", got)
	}
	if got := configured.CompletionAttempts(); got != 3 {
		t.Errorf("CompletionAttempts() = %d, want 3", got)
	}
	if got := configured.CompletionTailChars(); got != 800 {
		t.Errorf("CompletionTailChars() = %d, want 800", got)
	}
	if got := configured.CompletionMinChars(); got != 40 {
		t.Errorf("CompletionMinChars() = %d, want 40", got)
	}
	if got := configured.CompletionTimeout(); got != 10*time.Second {
		t.Errorf("CompletionTimeout() = %v, want 10s", got)
	}

	// An unrecognised mode falls back to auto rather than silently disabling.
	unknown := &Config{Agents: AgentsConfig{Completion: CompletionConfig{Mode: "banana"}}}
	if got := unknown.CompletionMode(); got != "auto" {
		t.Errorf("CompletionMode() = %q, want auto for an unknown mode", got)
	}
}

func TestDefaultConfigCarriesCompletionKnobs(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Agents.Completion.Mode != "auto" {
		t.Errorf("default completion mode = %q, want auto", cfg.Agents.Completion.Mode)
	}
	role, ok := cfg.Agents.Roles[RoleCompletion]
	if !ok {
		t.Fatalf("default config has no %q role", RoleCompletion)
	}
	if role.Type != "inherit" || role.InheritFrom != RoleGM {
		t.Errorf("completion role = %+v, want inherit gm", role)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run 'TestCompletionSettings|TestDefaultConfigCarriesCompletion' ./pkg/config/ -v`
Expected: FAIL with "undefined: RoleCompletion" and "unknown field Completion".

- [x] **Step 3: Write minimal implementation**

In `pkg/config/types.go`, add `RoleCompletion` to the role constants block:

```go
// Agent role names routed by the harness router.
const (
	RoleGM         = "gm"
	RoleNarrator   = "narrator"
	RoleExtractor  = "extractor"
	RoleCompletion = "completion"
)
```

Add the struct after `AgentsConfig`:

```go
// CompletionConfig governs how a narrator reply that stops mid-thought is
// repaired. The zero value means "auto" with the documented defaults, so a
// configuration written before these keys existed keeps working.
type CompletionConfig struct {
	// Mode is "auto", "continue", "trim", or "off". Empty means "auto".
	Mode string `yaml:"mode,omitempty" json:"mode,omitempty"`
	// MaxAttempts caps continuation calls per turn. Zero means one.
	MaxAttempts int `yaml:"max_attempts,omitempty" json:"max_attempts,omitempty"`
	// TailChars is how much of the partial reply the continuation call sees.
	TailChars int `yaml:"tail_chars,omitempty" json:"tail_chars,omitempty"`
	// MinIncompleteChars skips recovery for replies shorter than this.
	MinIncompleteChars int `yaml:"min_incomplete_chars,omitempty" json:"min_incomplete_chars"`
	// TimeoutSeconds bounds one continuation call.
	TimeoutSeconds int `yaml:"timeout_seconds,omitempty" json:"timeout_seconds"`
}
```

Add the field to `AgentsConfig` (before the closing brace, after `ContinuityChecks`):

```go
	// Completion governs how a narrator reply that stops mid-thought is repaired.
	Completion CompletionConfig `yaml:"completion" json:"completion"`
```

Add accessors after `ContinuityChecks`:

```go
// CompletionMode is the recovery policy: "auto", "continue", "trim", or "off".
func (c *Config) CompletionMode() string {
	mode := strings.ToLower(strings.TrimSpace(c.Agents.Completion.Mode))
	switch mode {
	case "auto", "continue", "trim", "off":
		return mode
	default:
		return "auto"
	}
}

// CompletionAttempts caps continuation calls per turn.
func (c *Config) CompletionAttempts() int {
	if c.Agents.Completion.MaxAttempts <= 0 {
		return 1
	}
	return c.Agents.Completion.MaxAttempts
}

// CompletionTailChars is how much of the partial reply the continuation call sees.
func (c *Config) CompletionTailChars() int {
	if c.Agents.Completion.TailChars <= 0 {
		return 1500
	}
	return c.Agents.Completion.TailChars
}

// CompletionMinChars is the shortest incomplete reply worth recovering.
func (c *Config) CompletionMinChars() int {
	if c.Agents.Completion.MinIncompleteChars <= 0 {
		return 24
	}
	return c.Agents.Completion.MinIncompleteChars
}

// CompletionTimeout bounds one continuation call.
func (c *Config) CompletionTimeout() time.Duration {
	seconds := c.Agents.Completion.TimeoutSeconds
	if seconds <= 0 {
		seconds = 45
	}
	return time.Duration(seconds) * time.Second
}
```

In `DefaultConfig`, inside the `Agents: AgentsConfig{...}` literal, add the completion block (for example after `SummaryCharLimit: 2000,`) and the role entry (after the `RoleExtractor` entry):

```go
			Completion: CompletionConfig{
				Mode:               "auto",
				MaxAttempts:        1,
				TailChars:          1500,
				MinIncompleteChars: 24,
				TimeoutSeconds:     45,
			},
			Roles: map[string]AgentRoleConfig{
				"gm": {
					Type:        "cli",
					Command:     "echo",
					Args:        []string{},
					Temperature: 0.7,
					MaxTokens:   1024,
				},
				"narrator": {
					Type: "disabled",
				},
				RoleExtractor: {
					Type:        "inherit",
					InheritFrom: RoleGM,
				},
				RoleCompletion: {
					Type:        "inherit",
					InheritFrom: RoleGM,
				},
			},
```

Do not duplicate the `Roles` key; edit the existing map in place to add only the `RoleCompletion` entry, and add the `Completion:` field to the same literal.

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run 'TestCompletionSettings|TestDefaultConfigCarriesCompletion' ./pkg/config/ -v`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/config/types.go pkg/config/types_test.go
git commit -m "feat(config): add reply-recovery settings and the completion role"
```

---

## Task 4: Resolve the completion role

**Files:**
- Modify: `pkg/harness/router.go`
- Modify: `pkg/harness/factory.go`
- Test: `pkg/harness/factory_test.go`

**Interfaces:**
- Consumes: `config.RoleCompletion`, `config.CompletionConfig` (Task 3).
- Produces: `(*Router).FallbackForRole(role string) (ModelProvider, bool)`, `harness.CompletionFromConfig(cfg *config.Config, router *Router, logger trace.Logger) ModelProvider`.

- [x] **Step 1: Write the failing test**

Append to `pkg/harness/factory_test.go`:

```go
func TestCompletionResolvesThroughTheInheritedRole(t *testing.T) {
	cfg := config.DefaultConfig()
	router, err := RouterFromConfig(cfg)
	if err != nil {
		t.Fatalf("RouterFromConfig: %v", err)
	}

	provider := CompletionFromConfig(cfg, router, trace.Nop())
	if provider == nil {
		t.Fatalf("expected completion to inherit the gm provider")
	}
	gm, err := router.GetProviderForRole(config.RoleGM)
	if err != nil {
		t.Fatalf("GetProviderForRole: %v", err)
	}
	if provider.ID() != gm.ID() {
		t.Errorf("completion provider = %q, want the gm provider %q", provider.ID(), gm.ID())
	}
}

func TestCompletionDisabledReturnsNil(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agents.Roles[config.RoleCompletion] = config.AgentRoleConfig{Type: "disabled"}
	router, err := RouterFromConfig(cfg)
	if err != nil {
		t.Fatalf("RouterFromConfig: %v", err)
	}
	if provider := CompletionFromConfig(cfg, router, trace.Nop()); provider != nil {
		t.Errorf("expected a disabled completion role to resolve to nil, got %q", provider.ID())
	}
}
```

Ensure `factory_test.go` imports `config` and `trace` (add them to the import block if missing).

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run 'TestCompletion' ./pkg/harness/ -v`
Expected: FAIL with "undefined: CompletionFromConfig".

- [x] **Step 3: Write minimal implementation**

In `pkg/harness/router.go`, add after `SetFallback`:

```go
// FallbackForRole reports the provider a role falls back to, if one is set.
func (r *Router) FallbackForRole(role string) (ModelProvider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	id, ok := r.fallbacks[role]
	if !ok {
		return nil, false
	}
	provider, ok := r.providers[id]
	return provider, ok
}
```

In `pkg/harness/factory.go`, add after `ExtractorFromConfigWithLogger`:

```go
// CompletionFromConfig resolves the role that finishes a cut-off reply. It
// inherits gm unless configured otherwise, and a nil result disables the
// continuation half of recovery, leaving trimming.
func CompletionFromConfig(cfg *config.Config, router *Router, logger trace.Logger) ModelProvider {
	if cfg == nil || router == nil {
		return nil
	}

	roleCfg, configured := cfg.Agents.Roles[config.RoleCompletion]
	if !configured {
		roleCfg = config.AgentRoleConfig{Type: "inherit", InheritFrom: config.RoleGM}
	}

	switch roleCfg.Type {
	case "disabled":
		return nil
	case "inherit", "":
		source := roleCfg.InheritFrom
		if source == "" {
			source = config.RoleGM
		}
		provider, err := router.GetProviderForRole(source)
		if err != nil {
			return nil
		}
		return provider
	}

	provider, err := NewModelProviderWithLogger(config.RoleCompletion, ProviderConfig{
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
		return nil
	}
	setProviderChunkLimit(provider, cfg.TraceChunkLimit())
	return provider
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run 'TestCompletion' ./pkg/harness/ -v`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/harness/router.go pkg/harness/factory.go pkg/harness/factory_test.go
git commit -m "feat(harness): resolve the completion role and its fallback"
```

---

## Task 5: Keep the text that arrived

**Files:**
- Modify: `pkg/engine/orchestrator.go:589-662`
- Test: `pkg/engine/orchestrator_stream_test.go`

**Interfaces:**
- Consumes: `harness.ProseComplete` (Task 1), `Router.FallbackForRole` (Task 4).
- Produces: `streamResult` (unexported) and `(*TurnOrchestrator).stream(ctx, provider, req, onChunk) (streamResult, error)`; `(*TurnOrchestrator).generate` now returns `(streamResult, error)`.

- [x] **Step 1: Write the failing test**

Replace `TestMidStreamProviderFailureRecordsNothing` in `pkg/engine/orchestrator_stream_test.go` with the two tests below:

```go
func TestMidStreamProviderFailureKeepsPartialText(t *testing.T) {
	provider := &scriptedStreamProvider{chunks: []string{"Steel rings, "}, err: errors.New("model exploded")}
	orchestrator, timeline, _ := streamingOrchestrator(t, provider)

	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I swing", nil)
	if err != nil {
		t.Fatalf("expected the partial reply to survive, got %v", err)
	}
	if turn.Narration != "Steel rings, " {
		t.Errorf("Narration = %q, want the text that arrived", turn.Narration)
	}
	if !turn.Truncated {
		t.Errorf("expected the unrepaired reply to be marked incomplete")
	}

	turns, err := timeline.history.LoadHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 {
		t.Errorf("expected the partial turn recorded, got %+v", turns)
	}
}

func TestProviderFailureBeforeAnyTextRecordsNothing(t *testing.T) {
	provider := &scriptedStreamProvider{err: errors.New("model exploded")}
	orchestrator, timeline, _ := streamingOrchestrator(t, provider)

	if _, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I swing", nil); err == nil {
		t.Fatalf("expected a failure with no text to surface")
	}

	turns, err := timeline.history.LoadHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 0 {
		t.Errorf("expected nothing recorded, got %+v", turns)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run 'TestMidStreamProviderFailureKeepsPartialText|TestProviderFailureBeforeAnyText' ./pkg/engine/ -v`
Expected: FAIL, `TestMidStreamProviderFailureKeepsPartialText` errors with "gm generation failed", `TestProviderFailureBeforeAnyText` passes already.

- [x] **Step 3: Write minimal implementation**

In `pkg/engine/orchestrator.go`, add a sentinel next to `ErrGenerationStalled`:

```go
// errStreamListener marks a failure caused by the caller's onChunk listener, so
// a fallback provider is never tried on top of a disconnected client.
var errStreamListener = errors.New("stream listener failed")
```

Replace the body of `generate` (currently lines 596-662) with `streamResult` plus the shared pump:

```go
// streamResult is what one provider stream produced: its accumulated text, the
// provider's finish reason when it reported one, and the failure that stopped it
// after some text had already arrived.
type streamResult struct {
	Text         string
	FinishReason string
	Interrupted  error
}

// stream pumps one provider stream, forwarding each delta to onChunk and
// accumulating the text. A failure after text has arrived is returned as
// Interrupted with the text intact, so a turn can be repaired rather than lost;
// a failure before any text is a hard failure, as is a listener error.
func (o *TurnOrchestrator) stream(ctx context.Context, provider harness.ModelProvider, req harness.GenerateRequest, onChunk func(string) error) (streamResult, error) {
	timeout := o.chunkTimeout
	if timeout <= 0 {
		timeout = defaultChunkTimeout
	}

	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	chunks := make(chan harness.StreamChunk, 32)
	streamErr := make(chan error, 1)
	go func() {
		streamErr <- provider.Stream(streamCtx, req, chunks)
	}()

	idle := time.NewTimer(timeout)
	defer idle.Stop()

	var sb strings.Builder
	finishReason := ""
	sawText := false
	interrupted := func(err error) (streamResult, error) {
		if !sawText {
			return streamResult{}, err
		}
		return streamResult{Text: sb.String(), FinishReason: finishReason, Interrupted: err}, nil
	}

	for {
		select {
		case <-idle.C:
			cancel()
			// Draining until the provider closes lets its goroutines exit rather
			// than block forever on a channel nobody reads.
			go func() {
				for range chunks {
				}
			}()
			<-streamErr
			return interrupted(fmt.Errorf("%w after %s", ErrGenerationStalled, timeout))

		case chunk, ok := <-chunks:
			if !ok {
				if err := <-streamErr; err != nil {
					// A cancelled client is final: the turn will be discarded, so
					// there is no point keeping text nobody is waiting for.
					if errors.Is(err, context.Canceled) {
						return streamResult{}, err
					}
					return interrupted(err)
				}
				return streamResult{Text: sb.String(), FinishReason: finishReason}, nil
			}
			if chunk.Error != nil {
				<-streamErr
				return interrupted(chunk.Error)
			}
			if chunk.FinishReason != "" {
				finishReason = chunk.FinishReason
			}
			if !idle.Stop() {
				select {
				case <-idle.C:
				default:
				}
			}
			idle.Reset(timeout)

			if chunk.Text == "" {
				continue
			}
			sb.WriteString(chunk.Text)
			sawText = true
			if onChunk != nil {
				if err := onChunk(chunk.Text); err != nil {
					return streamResult{}, fmt.Errorf("%w: %w", errStreamListener, err)
				}
			}
		}
	}
}

// generate streams the gm reply through stream, falling back to the configured
// fallback provider when the primary fails before producing any text.
func (o *TurnOrchestrator) generate(ctx context.Context, prompt string, onChunk func(string) error) (streamResult, error) {
	provider, err := o.router.GetProviderForRole("gm")
	if err != nil {
		return streamResult{}, err
	}

	result, err := o.stream(ctx, provider, harness.GenerateRequest{Prompt: prompt}, onChunk)
	if err == nil || errors.Is(err, errStreamListener) || ctx.Err() != nil {
		return result, err
	}
	if fallback, ok := o.router.FallbackForRole("gm"); ok {
		return o.stream(ctx, fallback, harness.GenerateRequest{Prompt: prompt}, onChunk)
	}
	return result, err
}
```

Update the call site in `ProcessActionStream` (currently lines 458-461) to the new signature; the full recovery wiring is Task 7, so for now use the text and keep the existing finish-reason behaviour:

```go
	result, err := o.generate(ctx, contextPrompt, onChunk)
	if err != nil {
		return nil, fmt.Errorf("gm generation failed: %w", err)
	}

	narration := result.Text
	finishReason := result.FinishReason
	// A reply is incomplete when the stream was cut short or the prose does not
	// end at a natural boundary, not only when the provider declared a token cap.
	truncated := finishReason == "length" || result.Interrupted != nil || !harness.ProseComplete(narration)

	o.logger.Event("generation.complete", map[string]interface{}{
		"narration_chars": len([]rune(narration)),
		"finish_reason":   finishReason,
		"truncated":       truncated,
	})

	turn := Turn{
		Number:       turnNum,
		Timestamp:    time.Now(),
		Mode:         mode,
		Input:        actionInput,
		Roll:         rollRes,
		Narration:    narration,
		Location:     locationID,
		Outcome:      outcome,
		Truncated:    truncated,
		ContextNotes: assembly.Trimmed,
	}
```

The recovery pass in Task 7 replaces the `truncated` computation with its own `stillIncomplete` result. The generic `map[string]interface{}` lines elsewhere are unchanged.

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/engine/ -run 'TestMidStream|TestProviderFailure|TestChunkFailure|TestGenerationStalls|TestCancellation' -v`
Expected: PASS. `TestChunkFailureAbortsBeforeRecording` must still see the listener error through `errors.Is(err, clientGone)`; the double `%w` wrapping preserves it.

- [x] **Step 5: Commit**

```bash
git add pkg/engine/orchestrator.go pkg/engine/orchestrator_stream_test.go
git commit -m "fix(engine): keep the narrator text that arrives before a stream fails"
```

---

## Task 6: The recovery pass

**Files:**
- Create: `pkg/engine/recovery.go`
- Test: `pkg/engine/recovery_test.go`

**Interfaces:**
- Consumes: `harness.ProseComplete`, `harness.TrimToLastSentence`, `harness.StitchContinuation` (Tasks 1-2); `(*TurnOrchestrator).stream` and `streamResult` (Task 5); the `harness.ModelProvider` and `trace.Logger` types.
- Produces: `engine.RecoveryOutcome` (values `RecoveryNone`, `RecoveryContinued`, `RecoveryTrimmed`, `RecoveryKept`), `engine.CutCause`, `engine.CompletionPolicy`, `(*TurnOrchestrator).SetCompletionProvider(harness.ModelProvider)`, `(*TurnOrchestrator).SetCompletionPolicy(CompletionPolicy)`, `(*TurnOrchestrator).recoverReply(ctx, partial string, cut CutCause, onChunk func(string) error) (string, RecoveryOutcome, bool)`, and `(*TurnOrchestrator).classifyCut(result streamResult) CutCause`.

- [x] **Step 1: Write the failing test**

Create `pkg/engine/recovery_test.go`:

```go
package engine

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

// replyProvider returns a different scripted reply per call, so a test can give
// the narrator one reply and the completion role another.
type replyProvider struct {
	id      string
	replies []*scriptedStreamProvider
	calls   int
}

func (p *replyProvider) ID() string { return p.id }

func (p *replyProvider) Generate(ctx context.Context, req harness.GenerateRequest) (*harness.GenerateResponse, error) {
	return nil, fmt.Errorf("replyProvider does not implement Generate")
}

func (p *replyProvider) Stream(ctx context.Context, req harness.GenerateRequest, out chan<- harness.StreamChunk) error {
	if p.calls >= len(p.replies) {
		defer close(out)
		return fmt.Errorf("no scripted reply for call %d", p.calls)
	}
	reply := p.replies[p.calls]
	p.calls++
	return reply.Stream(ctx, req, out)
}

func recoveryOrchestrator(t *testing.T, gm *scriptedStreamProvider, completion *replyProvider, policy CompletionPolicy) (*TurnOrchestrator, *Timeline) {
	t.Helper()
	orchestrator, timeline, _ := streamingOrchestrator(t, gm)
	if completion != nil {
		orchestrator.SetCompletionProvider(completion)
	}
	orchestrator.SetCompletionPolicy(policy)
	return orchestrator, timeline
}

func TestCompleteReplyIsRecordedUnchanged(t *testing.T) {
	gm := &scriptedStreamProvider{chunks: []string{"The gate stands open before us all."}}
	orchestrator, _ := recoveryOrchestrator(t, gm, nil, CompletionPolicy{})

	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I look", nil)
	if err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}
	if turn.Narration != "The gate stands open before us all." {
		t.Errorf("Narration = %q", turn.Narration)
	}
	if turn.Recovery != "" || turn.Truncated {
		t.Errorf("complete reply should not be recovered: recovery=%q truncated=%v", turn.Recovery, turn.Truncated)
	}
}

func TestStructuralCutIsContinued(t *testing.T) {
	gm := &scriptedStreamProvider{chunks: []string{"The gate stands open before us all and the hinges groan "}}
	completion := &replyProvider{id: "completion", replies: []*scriptedStreamProvider{
		{chunks: []string{"in the rising wind."}},
	}}
	orchestrator, _ := recoveryOrchestrator(t, gm, completion, CompletionPolicy{})

	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I look", nil)
	if err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}
	want := "The gate stands open before us all and the hinges groan in the rising wind."
	if turn.Narration != want {
		t.Errorf("Narration = %q, want %q", turn.Narration, want)
	}
	if turn.Recovery != string(RecoveryContinued) {
		t.Errorf("Recovery = %q, want continued", turn.Recovery)
	}
	if turn.Truncated {
		t.Errorf("a continued reply is not truncated")
	}
}

func TestInterruptedStreamKeepsTextAndContinues(t *testing.T) {
	gm := &scriptedStreamProvider{chunks: []string{"The gate stands open before us all. The old hinge"}, err: errors.New("stream died")}
	completion := &replyProvider{id: "completion", replies: []*scriptedStreamProvider{
		{chunks: []string{"s groan in the wind."}},
	}}
	orchestrator, _ := recoveryOrchestrator(t, gm, completion, CompletionPolicy{})

	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I look", nil)
	if err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}
	want := "The gate stands open before us all. The old hinges groan in the wind."
	if turn.Narration != want {
		t.Errorf("Narration = %q, want %q", turn.Narration, want)
	}
	if turn.Recovery != string(RecoveryContinued) {
		t.Errorf("Recovery = %q, want continued", turn.Recovery)
	}
}

func TestContinuationFailureTrimsToBoundary(t *testing.T) {
	gm := &scriptedStreamProvider{chunks: []string{"The gate stands open before us all. The hinges groan and then"}}
	completion := &replyProvider{id: "completion", replies: []*scriptedStreamProvider{
		{err: errors.New("completion failed")},
	}}
	orchestrator, _ := recoveryOrchestrator(t, gm, completion, CompletionPolicy{})

	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I look", nil)
	if err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}
	if turn.Narration != "The gate stands open before us all." {
		t.Errorf("Narration = %q, want the trimmed sentence", turn.Narration)
	}
	if turn.Recovery != string(RecoveryTrimmed) {
		t.Errorf("Recovery = %q, want trimmed", turn.Recovery)
	}
	if turn.Truncated {
		t.Errorf("a trimmed reply is not truncated")
	}
}

func TestShortIncompleteReplyIsKept(t *testing.T) {
	gm := &scriptedStreamProvider{chunks: []string{"Mid-sentence cut"}}
	completion := &replyProvider{id: "completion"}
	orchestrator, _ := recoveryOrchestrator(t, gm, completion, CompletionPolicy{})

	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I look", nil)
	if err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}
	if turn.Narration != "Mid-sentence cut" {
		t.Errorf("Narration = %q, want the partial unchanged", turn.Narration)
	}
	if turn.Recovery != string(RecoveryKept) || !turn.Truncated {
		t.Errorf("expected kept and truncated, got recovery=%q truncated=%v", turn.Recovery, turn.Truncated)
	}
	if completion.calls != 0 {
		t.Errorf("too-short reply should not call completion, calls = %d", completion.calls)
	}
}

func TestModeOffKeepsRawText(t *testing.T) {
	gm := &scriptedStreamProvider{chunks: []string{"The gate stands open before us all. The hinges groan and then"}}
	completion := &replyProvider{id: "completion"}
	orchestrator, _ := recoveryOrchestrator(t, gm, completion, CompletionPolicy{Mode: "off"})

	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I look", nil)
	if err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}
	if turn.Narration != "The gate stands open before us all. The hinges groan and then" {
		t.Errorf("Narration = %q, want the raw reply", turn.Narration)
	}
	if turn.Recovery != "" || !turn.Truncated {
		t.Errorf("mode off should keep the raw reply: recovery=%q truncated=%v", turn.Recovery, turn.Truncated)
	}
	if completion.calls != 0 {
		t.Errorf("mode off must not call completion, calls = %d", completion.calls)
	}
}

func TestModeTrimNeverCallsCompletion(t *testing.T) {
	gm := &scriptedStreamProvider{chunks: []string{"The gate stands open before us all. The hinges groan and then"}}
	completion := &replyProvider{id: "completion"}
	orchestrator, _ := recoveryOrchestrator(t, gm, completion, CompletionPolicy{Mode: "trim"})

	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I look", nil)
	if err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}
	if turn.Narration != "The gate stands open before us all." {
		t.Errorf("Narration = %q, want the trimmed sentence", turn.Narration)
	}
	if turn.Recovery != string(RecoveryTrimmed) {
		t.Errorf("Recovery = %q, want trimmed", turn.Recovery)
	}
	if completion.calls != 0 {
		t.Errorf("mode trim must not call completion, calls = %d", completion.calls)
	}
}

func TestModeContinueKeepsPartialWhenContinuationFails(t *testing.T) {
	gm := &scriptedStreamProvider{chunks: []string{"The gate stands open before us all. The hinges groan and then"}}
	completion := &replyProvider{id: "completion", replies: []*scriptedStreamProvider{
		{err: errors.New("completion failed")},
	}}
	orchestrator, _ := recoveryOrchestrator(t, gm, completion, CompletionPolicy{Mode: "continue"})

	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I look", nil)
	if err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}
	if turn.Narration != "The gate stands open before us all. The hinges groan and then" {
		t.Errorf("Narration = %q, want the partial kept", turn.Narration)
	}
	if turn.Recovery != string(RecoveryKept) || !turn.Truncated {
		t.Errorf("expected kept and truncated, got recovery=%q truncated=%v", turn.Recovery, turn.Truncated)
	}
}

func TestContinuationDeltaIsStreamed(t *testing.T) {
	gm := &scriptedStreamProvider{chunks: []string{"The gate stands open before us all and the hinges groan "}}
	completion := &replyProvider{id: "completion", replies: []*scriptedStreamProvider{
		{chunks: []string{"in the rising wind."}},
	}}
	orchestrator, _ := recoveryOrchestrator(t, gm, completion, CompletionPolicy{})

	var received []string
	if _, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I look", func(text string) error {
		received = append(received, text)
		return nil
	}); err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}
	if len(received) != 2 || received[1] != "in the rising wind." {
		t.Errorf("streamed chunks = %v, want the continuation forwarded", received)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run 'TestCompleteReply|TestStructuralCut|TestInterruptedStreamKeeps|TestContinuationFailure|TestShortIncomplete|TestModeOff|TestModeTrim|TestModeContinue|TestContinuationDelta' ./pkg/engine/ -v`
Expected: FAIL with "undefined: CompletionPolicy" and "unknown field Recovery".

- [x] **Step 3: Write minimal implementation**

Create `pkg/engine/recovery.go`:

```go
package engine

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/darkliquid/localrpg/pkg/harness"
)

// RecoveryOutcome describes what the recovery pass did to a reply, for the trace
// and the recorded turn.
type RecoveryOutcome string

const (
	// RecoveryNone means the reply was already complete.
	RecoveryNone RecoveryOutcome = ""
	// RecoveryContinued means a second call finished the reply.
	RecoveryContinued RecoveryOutcome = "continued"
	// RecoveryTrimmed means the unfinished tail was dropped.
	RecoveryTrimmed RecoveryOutcome = "trimmed"
	// RecoveryKept means recovery was skipped or failed.
	RecoveryKept RecoveryOutcome = "kept"
)

// CutCause explains why a reply is not a complete ending.
type CutCause int

const (
	cutNone CutCause = iota
	cutLength
	cutInterrupted
	cutStructural
)

// String renders a cause for the trace.
func (c CutCause) String() string {
	switch c {
	case cutLength:
		return "length"
	case cutInterrupted:
		return "interrupted"
	case cutStructural:
		return "structural"
	default:
		return "none"
	}
}

// CompletionPolicy is the reply-recovery configuration the orchestrator applies.
// The zero value is the default policy: auto, one attempt, a 1500-rune tail, a
// 24-rune minimum, and a 45-second bound.
type CompletionPolicy struct {
	Mode        string
	MaxAttempts int
	TailChars   int
	MinChars    int
	Timeout     time.Duration
}

// SetCompletionProvider attaches the provider that finishes a cut-off reply. A
// nil provider disables the continuation half of recovery, leaving trimming.
func (o *TurnOrchestrator) SetCompletionProvider(provider harness.ModelProvider) {
	o.completion = provider
}

// SetCompletionPolicy applies the recovery configuration.
func (o *TurnOrchestrator) SetCompletionPolicy(policy CompletionPolicy) {
	o.completionPolicy = policy
}

func (o *TurnOrchestrator) completionMode() string {
	switch strings.ToLower(strings.TrimSpace(o.completionPolicy.Mode)) {
	case "auto", "continue", "trim", "off":
		return strings.ToLower(strings.TrimSpace(o.completionPolicy.Mode))
	default:
		return "auto"
	}
}

func (o *TurnOrchestrator) completionAttempts() int {
	if o.completionPolicy.MaxAttempts <= 0 {
		return 1
	}
	return o.completionPolicy.MaxAttempts
}

func (o *TurnOrchestrator) completionTailChars() int {
	if o.completionPolicy.TailChars <= 0 {
		return 1500
	}
	return o.completionPolicy.TailChars
}

func (o *TurnOrchestrator) completionMinChars() int {
	if o.completionPolicy.MinChars <= 0 {
		return 24
	}
	return o.completionPolicy.MinChars
}

func (o *TurnOrchestrator) completionTimeout() time.Duration {
	if o.completionPolicy.Timeout <= 0 {
		return 45 * time.Second
	}
	return o.completionPolicy.Timeout
}

// classifyCut decides why a reply is not a complete ending. An interrupted
// stream wins, because a failure after text is the case that used to lose a turn.
func (o *TurnOrchestrator) classifyCut(result streamResult) CutCause {
	if result.Interrupted != nil {
		return cutInterrupted
	}
	if result.FinishReason == "length" {
		return cutLength
	}
	if !harness.ProseComplete(result.Text) {
		return cutStructural
	}
	return cutNone
}

// recoverReply runs once, after generation and before anything is segmented. It
// continues the cut-off thought with a second call when the policy allows, then
// trims to the last balanced boundary. It returns the narration to record, what
// it did, and whether the recorded prose is still incomplete.
func (o *TurnOrchestrator) recoverReply(ctx context.Context, partial string, cut CutCause, onChunk func(string) error) (string, RecoveryOutcome, bool) {
	mode := o.completionMode()
	if mode == "off" {
		return partial, RecoveryNone, !harness.ProseComplete(partial)
	}
	if cut == cutNone {
		return partial, RecoveryNone, false
	}
	if utf8.RuneCountInString(strings.TrimSpace(partial)) < o.completionMinChars() {
		return partial, RecoveryKept, !harness.ProseComplete(partial)
	}

	text := partial
	continued := false
	if mode != "trim" && o.completion != nil {
		for attempt := 0; attempt < o.completionAttempts(); attempt++ {
			if harness.ProseComplete(text) {
				break
			}
			continuation, ok := o.continueReply(ctx, text, onChunk)
			if !ok {
				break
			}
			stitched := harness.StitchContinuation(text, continuation)
			if stitched == text || strings.TrimSpace(stitched) == "" {
				break
			}
			text = stitched
			continued = true
			if harness.ProseComplete(text) {
				break
			}
		}
	}

	if harness.ProseComplete(text) {
		if continued {
			return text, RecoveryContinued, false
		}
		return text, RecoveryKept, false
	}

	if mode != "continue" {
		if trimmed, ok := harness.TrimToLastSentence(text, o.completionMinChars()); ok {
			return trimmed, RecoveryTrimmed, false
		}
	}
	return text, RecoveryKept, true
}

// continueReply makes one narrow "finish this" call. It never re-sends the
// assembled scene, because that invites a fresh scene rather than a finished
// thought. Deltas are forwarded through onChunk so the player watches the
// sentence close.
func (o *TurnOrchestrator) continueReply(ctx context.Context, partial string, onChunk func(string) error) (string, bool) {
	callCtx, cancel := context.WithTimeout(ctx, o.completionTimeout())
	defer cancel()

	tail := tailForContinuation(partial, o.completionTailChars())
	result, err := o.stream(callCtx, o.completion, harness.GenerateRequest{Prompt: completionPrompt(tail)}, onChunk)
	if err != nil {
		return "", false
	}
	text := strings.TrimSpace(result.Text)
	if text == "" {
		return "", false
	}
	return text, true
}

// tailForContinuation is the last tailChars runes of the partial reply, backed
// up to a paragraph boundary when one falls inside the window.
func tailForContinuation(text string, tailChars int) string {
	if tailChars <= 0 {
		tailChars = 1500
	}
	runes := []rune(strings.TrimRight(text, " \t\r\n"))
	if len(runes) <= tailChars {
		return string(runes)
	}
	tail := string(runes[len(runes)-tailChars:])
	if index := strings.Index(tail, "\n\n"); index >= 0 {
		return strings.TrimLeft(tail[index:], "\n")
	}
	return tail
}

// completionPrompt is the continuation instruction. It is identical for every
// cause, because the model cannot act on "your stream failed"; it only needs to
// know the text stops mid-thought.
func completionPrompt(tail string) string {
	var sb strings.Builder
	sb.WriteString("[CONTINUATION]\n")
	sb.WriteString("The narrator's reply was cut off mid-thought. Finish it.\n\n")
	sb.WriteString("- Continue the text below from exactly where it stops. Do not repeat any of it.\n")
	sb.WriteString("- Do not start a new scene, introduce characters, or resolve anything the\n")
	sb.WriteString("  cut-off text had not already begun.\n")
	sb.WriteString("- End at the first natural boundary: the end of the sentence or paragraph you\n")
	sb.WriteString("  are completing.\n")
	sb.WriteString("- Output only the continuation. No preamble, no headings, no wrapping the whole\n")
	sb.WriteString("  reply in quotation marks.\n\n")
	sb.WriteString("Cut-off text:\n")
	sb.WriteString(tail)
	return sb.String()
}
```

In `pkg/engine/orchestrator.go`, add the two fields to `TurnOrchestrator` (after `continuityChecks`):

```go
	completion       harness.ModelProvider
	completionPolicy CompletionPolicy
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run 'TestCompleteReply|TestStructuralCut|TestInterruptedStreamKeeps|TestContinuationFailure|TestShortIncomplete|TestModeOff|TestModeTrim|TestModeContinue|TestContinuationDelta' ./pkg/engine/ -v`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/engine/recovery.go pkg/engine/recovery_test.go pkg/engine/orchestrator.go
git commit -m "feat(engine): continue or trim a narrator reply that stops mid-thought"
```

---

## Task 7: Wire recovery into the turn and record its outcome

**Files:**
- Modify: `pkg/engine/orchestrator.go:458-484`
- Modify: `pkg/engine/history.go:26-29`
- Test: `pkg/engine/recovery_test.go`, `pkg/engine/orchestrator_stream_test.go`

**Interfaces:**
- Consumes: `recoverReply` and `classifyCut` (Task 6).
- Produces: `Turn.Recovery string` becomes part of the recorded turn; `Turn.Truncated` now describes the recorded prose.

- [x] **Step 1: Write the failing test**

Append to `pkg/engine/recovery_test.go`:

```go
func TestRecoveryOutcomeIsPersisted(t *testing.T) {
	gm := &scriptedStreamProvider{chunks: []string{"The gate stands open before us all and the hinges groan "}}
	completion := &replyProvider{id: "completion", replies: []*scriptedStreamProvider{
		{chunks: []string{"in the rising wind."}},
	}}
	orchestrator, timeline := recoveryOrchestrator(t, gm, completion, CompletionPolicy{})

	if _, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I look", nil); err != nil {
		t.Fatalf("ProcessActionStream: %v", err)
	}

	turns, err := timeline.history.LoadHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 {
		t.Fatalf("expected one recorded turn, got %d", len(turns))
	}
	if turns[0].Recovery != string(RecoveryContinued) {
		t.Errorf("recorded recovery = %q, want continued", turns[0].Recovery)
	}
	if turns[0].Truncated {
		t.Errorf("a continued reply should not be recorded truncated")
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestRecoveryOutcomeIsPersisted ./pkg/engine/ -v`
Expected: FAIL with "unknown field Recovery".

- [x] **Step 3: Write minimal implementation**

In `pkg/engine/history.go`, add the field to `Turn` before `LegacyOutput`:

```go
	// Recovery records how a reply that stopped mid-thought was repaired:
	// "continued" (a second call finished it), "trimmed" (the unfinished tail
	// was dropped), "kept" (recovery was skipped or failed), or empty (nothing
	// was wrong). Truncated is true only when the recorded prose is still
	// incomplete.
	Recovery string `json:"recovery,omitempty"`
```

In `pkg/engine/orchestrator.go`, replace the generation block in `ProcessActionStream` (the `result, err := o.generate(...)` block added in Task 5) with:

```go
	result, err := o.generate(ctx, contextPrompt, onChunk)
	if err != nil {
		return nil, fmt.Errorf("gm generation failed: %w", err)
	}

	cause := o.classifyCut(result)
	narration, recovery, stillIncomplete := o.recoverReply(ctx, result.Text, cause, onChunk)
	if strings.TrimSpace(narration) == "" {
		return nil, fmt.Errorf("gm returned no narration")
	}

	o.logger.Event("generation.complete", map[string]interface{}{
		"narration_chars": len([]rune(narration)),
		"finish_reason":   result.FinishReason,
		"cause":           cause.String(),
		"recovery":        string(recovery),
		"truncated":       stillIncomplete,
	})
```

Remove the previous `o.logger.Event("generation.complete", ...)` block, and set the `Turn` fields:

```go
	turn := Turn{
		Number:       turnNum,
		Timestamp:    time.Now(),
		Mode:         mode,
		Input:        actionInput,
		Roll:         rollRes,
		Narration:    narration,
		Location:     locationID,
		Outcome:      outcome,
		Truncated:    stillIncomplete,
		Recovery:     string(recovery),
		ContextNotes: assembly.Trimmed,
	}
```

Remove the now-duplicated `if strings.TrimSpace(turn.Narration) == ""` check that followed the old block.

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/engine/ -v`
Expected: PASS for the whole engine package.

- [x] **Step 5: Commit**

```bash
git add pkg/engine/orchestrator.go pkg/engine/history.go pkg/engine/recovery_test.go
git commit -m "feat(engine): record how a cut-off reply was repaired"
```

---

## Task 8: Expose the outcome and wire the provider

**Files:**
- Modify: `pkg/gui/types.go:57-74`
- Modify: `pkg/gui/service.go:822-837` and `pkg/gui/service.go:1083-1101`
- Modify: `cmd/localrpg/play.go`

**Interfaces:**
- Consumes: `Turn.Recovery` (Task 7), `engine.CompletionPolicy`, `harness.CompletionFromConfig` (Tasks 3-4), config accessors (Task 3).
- Produces: `TurnDTO.Recovery string` on the API; the completion provider and policy reach both turn pipelines.

- [x] **Step 1: Add the DTO field and copy it**

In `pkg/gui/types.go`, add to `TurnDTO` after `Truncated`:

```go
	Recovery        string       `json:"recovery,omitempty"`
```

In `pkg/gui/service.go`, inside `turnDTO`'s `TurnDTO{...}` literal, add after `Truncated: turn.Truncated,`:

```go
		Recovery:        turn.Recovery,
```

- [x] **Step 2: Wire the provider and policy into the GUI turn pipeline**

In `pkg/gui/service.go`, after `orchestrator.SetExtractor(...)` (line 1086), add:

```go
	orchestrator.SetCompletionProvider(harness.CompletionFromConfig(cfg, router, logger))
	orchestrator.SetCompletionPolicy(engine.CompletionPolicy{
		Mode:        cfg.CompletionMode(),
		MaxAttempts: cfg.CompletionAttempts(),
		TailChars:   cfg.CompletionTailChars(),
		MinChars:    cfg.CompletionMinChars(),
		Timeout:     cfg.CompletionTimeout(),
	})
```

- [x] **Step 3: Wire the provider and policy into the TUI pipeline**

In `cmd/localrpg/play.go`, after `orchestrator.SetExtractor(...)`, add:

```go
	orchestrator.SetCompletionProvider(harness.CompletionFromConfig(cfg, router, logger))
	orchestrator.SetCompletionPolicy(engine.CompletionPolicy{
		Mode:        cfg.CompletionMode(),
		MaxAttempts: cfg.CompletionAttempts(),
		TailChars:   cfg.CompletionTailChars(),
		MinChars:    cfg.CompletionMinChars(),
		Timeout:     cfg.CompletionTimeout(),
	})
```

- [x] **Step 4: Verify the backend builds and its tests pass**

Run: `go build ./... && go test ./pkg/gui/ ./cmd/... -count=1`
Expected: build succeeds and tests pass.

- [x] **Step 5: Commit**

```bash
git add pkg/gui/types.go pkg/gui/service.go cmd/localrpg/play.go
git commit -m "feat(gui): carry the recovery outcome to the client and wire the role"
```

---

## Task 9: Show the trimmed note in the chronicle

**Files:**
- Modify: `frontend/src/types.ts:49-53`
- Modify: `frontend/src/components/ChronicleView.tsx:120-124`

**Interfaces:**
- Consumes: the `recovery` field on the API turn.
- Produces: no new exports.

- [x] **Step 1: Add the field to the frontend type**

In `frontend/src/types.ts`, in `interface Turn`, after the `truncated` comment and field:

```ts
  // Set when the model hit its token limit mid-reply.
  truncated?: boolean;
  // How a reply that stopped mid-thought was repaired: 'continued', 'trimmed',
  // or 'kept'. Absent when nothing was wrong.
  recovery?: string;
```

- [x] **Step 2: Render the trimmed note and broaden the truncated note**

In `frontend/src/components/ChronicleView.tsx`, replace the truncated block (lines 120-124) with:

```tsx
            {turn.recovery === 'trimmed' && (
              <div className="text-xs font-mono text-amber-400/80 pt-1">
                The narrator's reply ended mid-thought; the unfinished tail was dropped.
              </div>
            )}

            {turn.truncated && (
              <div className="text-xs font-mono text-amber-400/80 pt-1">
                The narrator's reply could not be completed. Raise the response limit for the gm role in Settings, or
                check the provider.
              </div>
            )}
```

`continued` renders nothing: the reply reads as complete, which is the point.

- [x] **Step 3: Typecheck the frontend**

Run: `npx tsc --noEmit`
Expected: no errors.

- [x] **Step 4: Run the full verification gate**

Run: `mise run test && mise run lint`
Expected: `go test` passes, `npx tsc --noEmit` passes, and `go vet ./...` is clean.

- [x] **Step 5: Commit**

```bash
git add frontend/src/types.ts frontend/src/components/ChronicleView.tsx
git commit -m "feat(frontend): explain a reply that ended mid-thought"
```

---

## Self-Review

**Spec coverage:**

- 3.1 completeness and boundary detection: Task 1 (`ProseComplete`, `LastSentenceBoundary`, `TrimToLastSentence`), with the terminator, abbreviation, balance, scene-break, and heading rules.
- 3.2 the completion prompt: Task 6 (`completionPrompt`, `tailForContinuation`).
- 3.3 stitching: Task 2 (`StitchContinuation`).
- 3.4 the recovery pass and decision ladder: Task 6 (`recoverReply`, `CompletionPolicy`, modes `auto`/`continue`/`trim`/`off`, short-reply skip, attempt budget, time budget, disconnect-final, streaming deltas, never fabricate).
- 3.5 keeping the text that arrived: Task 5 (`streamResult`, `stream`, `generate`).
- 3.6 recording the outcome: Task 7 (`Turn.Recovery`, `Truncated = stillIncomplete`).
- 3.7 configuration and role: Tasks 3 and 4 (accessors, `RoleCompletion`, default entry, `CompletionFromConfig`), wired in Task 8 for GUI and TUI.
- 3.8 API and frontend: Tasks 8 and 9 (`TurnDTO.Recovery`, `types.ts`, `ChronicleView`).
- Acceptance 1-12 map to Tasks 5-9 and the final gate; the `mode: off` criterion is covered by `TestModeOffKeepsRawText`.

**Placeholder scan:** no "TBD"/"implement later" text; every code step carries real code. The only deliberately flexible item is the exact expected string in `TestStitchContinuation`'s overlap case, which Step 4 instructs the implementer to align to the spec's seam rules.

**Type consistency:** `streamResult` (`Text`, `FinishReason`, `Interrupted`) is defined in Task 5 and consumed in Tasks 6-7. `RecoveryOutcome` values are `RecoveryNone`/`RecoveryContinued`/`RecoveryTrimmed`/`RecoveryKept`, used verbatim in tests and in `Turn.Recovery`. `CutCause` constants `cutNone`/`cutLength`/`cutInterrupted`/`cutStructural` are defined and consumed only in `pkg/engine`. `CompletionPolicy` field names (`Mode`, `MaxAttempts`, `TailChars`, `MinChars`, `Timeout`) match the config accessors used in Task 8. `SetCompletionProvider` and `SetCompletionPolicy` are defined in Task 6 and called in Task 8. `Router.FallbackForRole` is defined in Task 4 and called in Task 5.
