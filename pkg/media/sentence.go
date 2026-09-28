package media

import (
	"strings"
	"unicode"
)

// abbreviations that end with a period but do not end a sentence.
var abbreviations = map[string]bool{
	"mr": true, "mrs": true, "ms": true, "dr": true, "prof": true, "st": true,
	"vs": true, "etc": true, "eg": true, "ie": true, "no": true, "sr": true, "jr": true,
}

// SplitSentences returns the sentences in text, in order. A trailing fragment
// without a terminator is returned as its own element when it is non-empty.
func SplitSentences(text string) []string {
	complete, remainder := SplitCompleteSentences(text)
	if strings.TrimSpace(remainder) != "" {
		complete = append(complete, remainder)
	}
	return complete
}

// SplitCompleteSentences returns the sentences terminated within text, and the
// unterminated tail, so a streaming caller can hold the tail until more text
// arrives. It never splits inside an inline code span or a common abbreviation.
func SplitCompleteSentences(text string) (complete []string, remainder string) {
	var sentence strings.Builder
	inCode := false
	runes := []rune(text)

	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '`' {
			inCode = !inCode
		}
		sentence.WriteRune(r)
		if inCode {
			continue
		}
		if !isSentenceBoundary(r, runes, i) {
			continue
		}
		// Absorb the punctuation that closes the sentence, so a closing quote
		// belongs to the sentence it ends rather than opening the next one.
		for i+1 < len(runes) && isClosingPunctuation(runes[i+1]) {
			i++
			sentence.WriteRune(runes[i])
		}
		if trimmed := strings.TrimSpace(sentence.String()); trimmed != "" {
			complete = append(complete, trimmed)
		}
		sentence.Reset()
	}

	return complete, sentence.String()
}

// isSentenceBoundary reports whether the rune at index i ends a sentence. A
// terminator only counts when it is followed by whitespace, the end of the text,
// or closing punctuation then whitespace or the end.
func isSentenceBoundary(r rune, runes []rune, i int) bool {
	switch r {
	case '\n':
		return true
	case '.', '!', '?', '…':
	default:
		return false
	}

	next := i + 1
	for next < len(runes) && isClosingPunctuation(runes[next]) {
		next++
	}
	if next < len(runes) && !unicode.IsSpace(runes[next]) {
		return false
	}

	if r == '.' && abbreviations[strings.ToLower(trailingWord(runes[:i]))] {
		return false
	}
	return true
}

func isClosingPunctuation(r rune) bool {
	switch r {
	case '"', '\'', ')', ']', '}', '”', '’':
		return true
	default:
		return false
	}
}

// trailingWord returns the run of letters immediately before the terminator.
func trailingWord(runes []rune) string {
	start := len(runes)
	for start > 0 && unicode.IsLetter(runes[start-1]) {
		start--
	}
	return string(runes[start:])
}
