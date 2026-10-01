package scene

import (
	"regexp"
	"strings"
)

// RunStyle is the inline style of a run of prose.
type RunStyle int

const (
	// StyleRegular is unstyled body prose.
	StyleRegular RunStyle = iota
	// StyleBold is **strong** text.
	StyleBold
	// StyleItalic is *emphasised* or _emphasised_ text.
	StyleItalic
	// StyleCode is `inline code`.
	StyleCode
	// StyleLink is the label of a [[wikilink]].
	StyleLink
	// StyleDirection is a [performance direction].
	StyleDirection
)

// Run is a span of prose with one inline style.
type Run struct {
	Text  string
	Style RunStyle
}

// BlockKind is the block structure of a paragraph of prose.
type BlockKind int

const (
	// BlockParagraph is ordinary prose.
	BlockParagraph BlockKind = iota
	// BlockHeading is a # or ## line.
	BlockHeading
	// BlockList is a run of - or * items.
	BlockList
	// BlockQuote is a > line.
	BlockQuote
	// BlockRule is a --- line.
	BlockRule
)

// Block is one paragraph of prose, split into styled runs.
type Block struct {
	Kind  BlockKind
	Runs  []Run
	Level int // heading level, for BlockHeading
}

// inlinePattern mirrors the theatre's MarkdownProse grammar exactly, so the video
// formats the same string the exported page does.
var inlinePattern = regexp.MustCompile(`(\[\[[^\]]+\]\])|(\[[a-zA-Z][a-zA-Z\s_-]{1,28}\])|(` + "`[^`]+`" + `)|(\*\*[^*]+\*\*)|(\*[^*]+\*)|(_[^_]+_)`)

var (
	headingPattern   = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	listItemPattern  = regexp.MustCompile(`^\s*[-*]\s+`)
	rulePattern      = regexp.MustCompile(`^(-{3,}|\*{3,}|_{3,})$`)
	hiddenDirections = regexp.MustCompile(`(?:^|\s)\[[a-zA-Z][a-zA-Z\s_-]{1,28}\](?:\s|$)`)
	quotePrefix      = regexp.MustCompile(`^>\s?`)
)

// ParseProse splits text into blocks and their inline runs, applying the same
// grammar MarkdownProse applies in the player.
func ParseProse(text string, mode DisplayMode) []Block {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	if mode == DisplayHidden {
		normalized = hiddenDirections.ReplaceAllString(normalized, " ")
	}
	if strings.TrimSpace(normalized) == "" {
		return nil
	}

	raw := strings.Split(normalized, "\n\n")
	blocks := make([]Block, 0, len(raw))
	for _, chunk := range raw {
		trimmed := strings.TrimSpace(chunk)
		if trimmed == "" {
			continue
		}
		if rulePattern.MatchString(trimmed) {
			blocks = append(blocks, Block{Kind: BlockRule})
			continue
		}
		if m := headingPattern.FindStringSubmatch(trimmed); m != nil {
			blocks = append(blocks, Block{Kind: BlockHeading, Level: len(m[1]), Runs: inlineRuns(m[2], mode)})
			continue
		}
		if lines := strings.Split(chunk, "\n"); allMatch(lines, listItemPattern) {
			for _, line := range lines {
				blocks = append(blocks, Block{Kind: BlockList, Runs: inlineRuns(listItemPattern.ReplaceAllString(line, ""), mode)})
			}
			continue
		}
		if strings.HasPrefix(trimmed, ">") {
			joined := strings.Join(mapLines(strings.Split(chunk, "\n"), quotePrefix), "\n")
			blocks = append(blocks, Block{Kind: BlockQuote, Runs: inlineRuns(joined, mode)})
			continue
		}
		blocks = append(blocks, Block{Kind: BlockParagraph, Runs: inlineRuns(chunk, mode)})
	}
	return blocks
}

func allMatch(lines []string, pattern *regexp.Regexp) bool {
	if len(lines) == 0 {
		return false
	}
	for _, line := range lines {
		if !pattern.MatchString(line) {
			return false
		}
	}
	return true
}

func mapLines(lines []string, pattern *regexp.Regexp) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = pattern.ReplaceAllString(line, "")
	}
	return out
}

func inlineRuns(text string, mode DisplayMode) []Run {
	runs := make([]Run, 0, 8)
	last := 0
	for _, match := range inlinePattern.FindAllStringIndex(text, -1) {
		if match[0] > last {
			runs = append(runs, Run{Text: text[last:match[0]], Style: StyleRegular})
		}
		token := text[match[0]:match[1]]
		runs = append(runs, styleToken(token, mode))
		last = match[1]
	}
	if last < len(text) {
		runs = append(runs, Run{Text: text[last:], Style: StyleRegular})
	}
	return runs
}

func styleToken(token string, mode DisplayMode) Run {
	switch {
	case strings.HasPrefix(token, "[["):
		inner := token[2 : len(token)-2]
		label := inner
		if sep := strings.Index(inner, "|"); sep != -1 {
			label = strings.TrimSpace(inner[sep+1:])
		}
		return Run{Text: label, Style: StyleLink}
	case strings.HasPrefix(token, "["):
		switch mode {
		case DisplayHidden:
			return Run{Text: "", Style: StyleDirection}
		case DisplayRaw:
			return Run{Text: token, Style: StyleDirection}
		default:
			return Run{Text: token[1 : len(token)-1], Style: StyleDirection}
		}
	case strings.HasPrefix(token, "`"):
		return Run{Text: token[1 : len(token)-1], Style: StyleCode}
	case strings.HasPrefix(token, "**"):
		return Run{Text: token[2 : len(token)-2], Style: StyleBold}
	default:
		return Run{Text: token[1 : len(token)-1], Style: StyleItalic}
	}
}
