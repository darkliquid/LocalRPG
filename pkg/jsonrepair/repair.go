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
// that validates, naming the step that produced it. It never adds a key.
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
	if out, ok := trimToValue(cur); ok && !bytes.Equal(out, cur) {
		cur = out
		if json.Valid(cur) {
			return Result{Payload: cur, Kind: KindTrim, OK: true}
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
