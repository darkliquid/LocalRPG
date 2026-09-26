package desktop

import (
	. "go.hasen.dev/shirei"

	"github.com/darkliquid/localrpg/pkg/desktop/markdown"
	"github.com/darkliquid/localrpg/pkg/ui"
)

// proseBlocks renders Markdown source as shirei text. base is the paragraph
// text style for plain runs; inline spans derive their style from it.
func proseBlocks(src string, base ...TextStyleFn) {
	p := ui.DefaultPalette()
	for _, b := range markdown.Parse(src) {
		switch b.Kind {
		case markdown.BlockHeading:
			mods := append([]TextStyleFn{FontWeight(WeightBold)}, base...)
			if b.Level <= 2 {
				mods = append([]TextStyleFn{FontSize(18)}, mods...)
			}
			Label(b.Text, mods...)
		case markdown.BlockList:
			for _, item := range b.Items {
				Label("• "+item, base...)
			}
		case markdown.BlockQuote:
			Container(Attrs(Pad2(4, 8), Corners(4), BackgroundVec(p.Border)), func() {
				Label(b.Text, base...)
			})
		case markdown.BlockRule:
			Element(Attrs(Expand, FixHeight(1), BackgroundVec(p.Border)))
		default:
			Text(b.Text, TextStyle(base...), proseSpans(b)...)
		}
	}
}

// proseSpans maps the parser's byte-range spans to shirei rune-range spans.
func proseSpans(b markdown.Block) []TextSpan {
	accent := ui.DefaultPalette().Accent
	var spans []TextSpan
	for _, s := range b.Spans {
		from := runeIndex(b.Text, s.From)
		to := runeIndex(b.Text, s.To)
		switch s.Kind {
		case markdown.SpanEmphasis:
			spans = append(spans, Span(from, to, FontStyle(StyleItalic)))
		case markdown.SpanStrong:
			spans = append(spans, Span(from, to, FontWeight(WeightBold)))
		case markdown.SpanCode:
			spans = append(spans, Span(from, to, Fonts(Monospace...)))
		case markdown.SpanWikilink:
			spans = append(spans, Span(from, to, TextColorVec(accent)))
		}
	}
	return spans
}

// runeIndex converts a byte offset to a rune index, clamping past the end.
func runeIndex(s string, byteOff int) int {
	if byteOff > len(s) {
		byteOff = len(s)
	}
	if byteOff < 0 {
		return 0
	}
	return len([]rune(s[:byteOff]))
}
