package engine

import (
	"strings"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/turnstream"
)

// ReplyProblem classifies what is wrong with a generation reply.
type ReplyProblem int

const (
	// ProblemNone means the reply is valid and ready to use.
	ProblemNone ReplyProblem = iota
	// ProblemMalformed means the reply is structurally broken (empty, unparseable, invalid record).
	ProblemMalformed
	// ProblemCut means the reply was cut off by length or incomplete sentence.
	ProblemCut
	// ProblemProvider means the provider itself failed (handled by provider fallback).
	ProblemProvider
)

// String renders a ReplyProblem for tracing and logging.
func (p ReplyProblem) String() string {
	switch p {
	case ProblemMalformed:
		return "malformed"
	case ProblemCut:
		return "cut"
	case ProblemProvider:
		return "provider"
	default:
		return "none"
	}
}

// classifyReply decides whether a stream result is valid, cut, malformed, or a provider error.
// A length finish or an incomplete sentence is cut;
// an empty response with no usable text/record is malformed;
// otherwise ProblemNone.
func classifyReply(res streamResult, report turnstream.RepairReport) ReplyProblem {
	// A cut reply wins classification over malformed if it has text with length finish or broken grammar
	trimmed := strings.TrimSpace(res.Text)
	if res.FinishReason == "length" {
		return ProblemCut
	}
	if trimmed != "" && !harness.ProseComplete(res.Text) {
		return ProblemCut
	}
	if trimmed == "" && len(res.ToolCalls) == 0 && res.PendingCheck == nil && res.RollOutcome == "" {
		return ProblemMalformed
	}
	// If unrepairable record failed and there's no usable prose or valid record
	if report.Failed > 0 && trimmed == "" {
		return ProblemMalformed
	}
	return ProblemNone
}

// repairInstruction builds a short, specific nudge for a malformed reply.
func repairInstruction(p ReplyProblem, detail string) string {
	lower := strings.ToLower(detail)
	if strings.Contains(lower, "roll") {
		return "Your previous reply contained no valid @roll record. Re-emit the reply, ending with a single @roll {…} line whose JSON is valid."
	}
	if strings.Contains(lower, "tool") {
		return "The previous tool call had arguments that were not valid JSON. Call it again with valid JSON."
	}
	if detail != "" {
		return "Your previous reply was malformed: " + detail + ". Please write the turn again."
	}
	return "Your previous reply was empty. Write the turn now."
}

