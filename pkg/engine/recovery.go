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
