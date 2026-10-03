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

// StitchContinuation joins a continuation onto the text it resumes, removing a
// repeated overlap and repairing the seam. It is the only place the two halves
// are combined, so the seam rules live in one test.
func StitchContinuation(existing, continuation string) string {
	cont := stripContinuationPreamble(continuation)
	if strings.TrimSpace(cont) == "" {
		return existing
	}
	cont = removeOverlap(existing, cont)
	if strings.TrimSpace(cont) == "" {
		return existing
	}
	if strings.TrimSpace(existing) == "" {
		return collapseNewlines(cont)
	}

	switch {
	case endsWithSpace(existing):
		return collapseNewlines(existing + strings.TrimLeft(cont, " \t\r\n"))
	case startsWithPunctuation(cont):
		return collapseNewlines(strings.TrimRight(existing, " \t\r\n") + cont)
	case endsAlphanumeric(existing) && startsAlphanumeric(cont):
		if resumesWord(cont) {
			return collapseNewlines(existing + cont)
		}
		// A word-boundary cut starts a new word, so it needs a space: "the" and
		// "whisper" must not read as "thewhisper".
		return collapseNewlines(existing + " " + cont)
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

// resumesWord reports whether a continuation opens with the tail of a word the cut
// split, rather than a new word. A short lowercase run is a suffix ("hinge" +
// "s"); anything longer starts a new word and needs a space ("the" + "whisper").
func resumesWord(cont string) bool {
	run := 0
	for _, r := range cont {
		if !unicode.IsLetter(r) {
			break
		}
		run++
	}
	if run == 0 || run > 2 {
		return false
	}
	return unicode.IsLower([]rune(cont)[0])
}

func startsWithPunctuation(text string) bool {
	if text == "" {
		return false
	}
	switch []rune(text)[0] {
	case ',', '.', ';', ':', '!', '?':
		return true
	default:
		return false
	}
}
