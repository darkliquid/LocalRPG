package tools

import "strings"

// BuildMatch turns a model's words into an FTS5 MATCH expression that cannot be a
// syntax error. Terms are stripped of FTS punctuation, quoted, joined with AND,
// and the last is given a prefix star so 'guard kae' finds 'Guard Kael'. FTS5's
// own grammar is unreachable from here on purpose: 'kael's oath' and 'guard AND'
// are both errors a model can neither see nor fix.
func BuildMatch(query string) string {
	words := strings.Fields(query)
	terms := make([]string, 0, len(words))
	for _, word := range words {
		cleaned := stripFTSPunctuation(word)
		if cleaned == "" {
			continue
		}
		terms = append(terms, `"`+cleaned+`"`)
	}
	if len(terms) == 0 {
		return ""
	}
	last := len(terms) - 1
	terms[last] = terms[last] + "*"
	return strings.Join(terms, " AND ")
}

// stripFTSPunctuation removes everything FTS5 gives meaning to, leaving letters,
// digits, and hyphens.
func stripFTSPunctuation(word string) string {
	var sb strings.Builder
	for _, r := range word {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
			sb.WriteRune(r)
		default:
			continue
		}
	}
	return sb.String()
}
