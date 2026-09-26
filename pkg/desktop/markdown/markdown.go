// Package markdown turns the small subset of Markdown LocalRPG shows in prose
// into flat blocks with styled rune ranges, which shirei can draw with Text.
package markdown

import (
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// BlockKind identifies the kind of a top-level prose block.
type BlockKind int

const (
	BlockParagraph BlockKind = iota
	BlockHeading
	BlockList
	BlockQuote
	BlockRule
)

// SpanKind identifies an inline style run.
type SpanKind int

const (
	// SpanEmphasis starts at 1 so the zero value means "no style".
	SpanEmphasis SpanKind = iota + 1
	SpanStrong
	SpanCode
	SpanWikilink
)

// SpanRange is a styled byte range within a Block's Text.
type SpanRange struct {
	From, To int
	Kind     SpanKind
	Target   string // wikilink target, when Kind == SpanWikilink
}

// Block is a flat prose block.
type Block struct {
	Kind  BlockKind
	Level int
	Text  string
	Spans []SpanRange
	Items []string
}

var parser = goldmark.New()

// Parse converts src into blocks. Inline wikilinks are extracted because
// goldmark does not know them.
func Parse(src string) []Block {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	doc := parser.Parser().Parse(text.NewReader([]byte(src)))
	var blocks []Block
	for node := doc.FirstChild(); node != nil; node = node.NextSibling() {
		blocks = append(blocks, blockFromNode(node, src)...)
	}
	return blocks
}

func blockFromNode(node ast.Node, src string) []Block {
	switch node.Kind() {
	case ast.KindHeading:
		level := node.(*ast.Heading).Level
		text, spans := inline(node, src)
		return []Block{{Kind: BlockHeading, Level: level, Text: text, Spans: spans}}
	case ast.KindBlockquote:
		var out []Block
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			out = append(out, blockFromNode(child, src)...)
		}
		for i := range out {
			out[i].Kind = BlockQuote
		}
		return out
	case ast.KindThematicBreak:
		return []Block{{Kind: BlockRule}}
	case ast.KindList:
		var items []string
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			item, _ := inline(child, src)
			items = append(items, item)
		}
		return []Block{{Kind: BlockList, Items: items}}
	case ast.KindParagraph:
		text, spans := inline(node, src)
		return []Block{{Kind: BlockParagraph, Text: text, Spans: spans}}
	default:
		return nil
	}
}

// inline flattens a node's inline children into plain text plus spans, then
// applies wikilink extraction to the result. Indices are byte offsets.
func inline(node ast.Node, src string) (string, []SpanRange) {
	var (
		b strings.Builder
		s []SpanRange
	)
	var walk func(n ast.Node, kind SpanKind)
	walk = func(n ast.Node, kind SpanKind) {
		for child := n.FirstChild(); child != nil; child = child.NextSibling() {
			switch child.Kind() {
			case ast.KindText:
				t := child.(*ast.Text)
				start := b.Len()
				b.Write(t.Segment.Value([]byte(src)))
				if kind != 0 {
					s = append(s, SpanRange{From: start, To: b.Len(), Kind: kind})
				}
				if t.SoftLineBreak() || t.HardLineBreak() {
					b.WriteByte(' ')
				}
			case ast.KindCodeSpan:
				start := b.Len()
				for c := child.FirstChild(); c != nil; c = c.NextSibling() {
					if t, ok := c.(*ast.Text); ok {
						b.Write(t.Segment.Value([]byte(src)))
					}
				}
				s = append(s, SpanRange{From: start, To: b.Len(), Kind: SpanCode})
			case ast.KindEmphasis:
				inner := child.(*ast.Emphasis)
				if inner.Level >= 2 {
					walk(child, SpanStrong)
				} else {
					walk(child, SpanEmphasis)
				}
			default:
				walk(child, kind)
			}
		}
	}
	walk(node, 0)
	return extractWikilinks(b.String(), s)
}

// extractWikilinks rewrites [[target|label]] (and [[target]]) to label, maps
// existing byte-offset spans into the rewritten text, and appends wikilink
// spans. The result is sorted by source position.
func extractWikilinks(text string, spans []SpanRange) (string, []SpanRange) {
	var b strings.Builder
	offsetMap := make([]int, len(text)+1)
	i := 0
	for i < len(text) {
		offsetMap[i] = b.Len()
		if strings.HasPrefix(text[i:], "[[") {
			if end := strings.Index(text[i+2:], "]]"); end >= 0 {
				inner := text[i+2 : i+2+end]
				target, label := inner, inner
				if bar := strings.IndexByte(inner, '|'); bar >= 0 {
					target, label = inner[:bar], inner[bar+1:]
				}
				linkStart := b.Len()
				linkEnd := linkStart + len(label)
				tokenEnd := i + 4 + len(inner)
				for j := i; j <= tokenEnd; j++ {
					offsetMap[j] = linkStart
				}
				b.WriteString(label)
				offsetMap[tokenEnd] = linkEnd
				spans = append(spans, SpanRange{From: linkStart, To: linkEnd, Kind: SpanWikilink, Target: target})
				i = tokenEnd
				continue
			}
		}
		b.WriteByte(text[i])
		i++
	}
	offsetMap[len(text)] = b.Len()

	out := make([]SpanRange, 0, len(spans))
	for _, sp := range spans {
		if sp.Kind == SpanWikilink {
			out = append(out, sp)
			continue
		}
		out = append(out, SpanRange{From: offsetMap[sp.From], To: offsetMap[sp.To], Kind: sp.Kind})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].From < out[j].From })
	return b.String(), out
}
