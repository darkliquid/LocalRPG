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
	KindBOM           Kind = "bom"
	KindEscapeControl Kind = "escape_control"
	KindSmartQuote    Kind = "smart_quote"
)

// Result reports what a repair did. When OK is false, Payload holds the furthest
// repair attempted rather than the input: a caller salvaging what a cut-off reply
// managed to write needs the fence stripped and the prose trimmed, and an error
// that quotes the fence points at the wrong problem. Payload is the input only
// when no repair could be attempted at all.
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
	if out, ok := stripBOM(cur); ok {
		cur = out
		if json.Valid(cur) {
			return Result{Payload: cur, Kind: KindBOM, OK: true}
		}
	}
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
	if out, ok := escapeStringControlChars(cur); ok {
		cur = out
		if json.Valid(cur) {
			return Result{Payload: cur, Kind: KindEscapeControl, OK: true}
		}
	}
	if out, ok := normalizeStructuralQuotes(cur); ok {
		cur = out
		if json.Valid(cur) {
			return Result{Payload: cur, Kind: KindSmartQuote, OK: true}
		}
	}
	return Result{Payload: cur}
}

// stripBOM removes a leading UTF-8 byte-order mark, which some models place
// before the object.
func stripBOM(b []byte) ([]byte, bool) {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:], true
	}
	return b, false
}

// escapeStringControlChars replaces raw control bytes inside string literals with
// their JSON escapes, so a newline a model wrote inside a value becomes legal. It
// never touches a byte outside a string.
func escapeStringControlChars(b []byte) ([]byte, bool) {
	const hexDigits = "0123456789abcdef"
	out := make([]byte, 0, len(b))
	inString := false
	escaped := false
	changed := false
	for i := 0; i < len(b); i++ {
		c := b[i]
		if !inString {
			if c == '"' {
				inString = true
			}
			out = append(out, c)
			continue
		}
		if escaped {
			escaped = false
			out = append(out, c)
			continue
		}
		switch {
		case c == '\\':
			escaped = true
			out = append(out, c)
		case c == '"':
			inString = false
			out = append(out, c)
		case c < 0x20:
			changed = true
			switch c {
			case '\n':
				out = append(out, '\\', 'n')
			case '\r':
				out = append(out, '\\', 'r')
			case '\t':
				out = append(out, '\\', 't')
			default:
				out = append(out, '\\', 'u', '0', '0', hexDigits[c>>4], hexDigits[c&0x0F])
			}
		default:
			out = append(out, c)
		}
	}
	return out, changed
}

// normalizeStructuralQuotes replaces a curly quote that sits where a JSON
// delimiter belongs with a straight quote, and leaves one inside a value alone.
func normalizeStructuralQuotes(b []byte) ([]byte, bool) {
	out := make([]byte, 0, len(b))
	changed := false
	for i := 0; i < len(b); {
		if b[i] == 0xE2 && i+2 < len(b) && b[i+1] == 0x80 && (b[i+2] == 0x9C || b[i+2] == 0x9D) {
			if structuralQuotePosition(b, i) {
				out = append(out, '"')
				changed = true
			} else {
				out = append(out, b[i:i+3]...)
			}
			i += 3
			continue
		}
		out = append(out, b[i])
		i++
	}
	return out, changed
}

// structuralQuotePosition reports whether the quote at index i opens a value or
// closes a key, which is where a curly quote stands in for a straight one.
func structuralQuotePosition(b []byte, i int) bool {
	prev := prevNonSpace(b, i)
	next := nextNonSpace(b, i+3)
	opensValue := prev == '{' || prev == ',' || prev == '['
	closesKey := next == ':' || next == '}' || next == ']' || next == ','
	return opensValue || closesKey
}

// prevNonSpace returns the byte before index i, skipping insignificant
// whitespace, or 0 when none remains.
func prevNonSpace(b []byte, i int) byte {
	for j := i - 1; j >= 0; j-- {
		if b[j] != ' ' && b[j] != '\n' && b[j] != '\t' && b[j] != '\r' {
			return b[j]
		}
	}
	return 0
}

// nextNonSpace returns the byte at index i, skipping insignificant whitespace,
// or 0 when none remains.
func nextNonSpace(b []byte, i int) byte {
	for j := i; j < len(b); j++ {
		if b[j] != ' ' && b[j] != '\n' && b[j] != '\t' && b[j] != '\r' {
			return b[j]
		}
	}
	return 0
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

// ArrayElements returns the complete elements of the first array in payload, even
// when the array itself is unterminated. A reply cut off by a token limit is not
// a failed reply: the elements written before the cut are whole, and keeping them
// beats discarding a batch, which for a long import means discarding its work.
//
// Only objects and arrays are returned; a scalar element is skipped because a
// truncated number or string cannot be told from a complete one.
func ArrayElements(payload []byte) [][]byte {
	if len(payload) > maxPayload {
		return nil
	}
	open := bytes.IndexByte(payload, '[')
	if open < 0 {
		return nil
	}

	var elements [][]byte
	depth := 0
	inString := false
	escaped := false
	start := -1

	for i := open + 1; i < len(payload); i++ {
		c := payload[i]
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
			if depth == 0 {
				start = i
			}
			depth++
		case '}', ']':
			if depth == 0 {
				// The enclosing array closed without an open element.
				return elements
			}
			depth--
			if depth == 0 && start >= 0 {
				element := bytes.TrimSpace(payload[start : i+1])
				if len(element) > 0 && element[0] == '{' {
					elements = append(elements, element)
				}
				start = -1
			}
		}
	}
	return elements
}
