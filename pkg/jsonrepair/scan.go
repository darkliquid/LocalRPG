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
