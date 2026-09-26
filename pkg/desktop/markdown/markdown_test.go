package markdown

import "testing"

func TestParseEmphasisAndStrong(t *testing.T) {
	blocks := Parse("A *soft* word and **hard** word.")
	if len(blocks) != 1 {
		t.Fatalf("blocks = %d, want 1", len(blocks))
	}
	b := blocks[0]
	if b.Text != "A soft word and hard word." {
		t.Fatalf("text = %q", b.Text)
	}
	if len(b.Spans) != 2 {
		t.Fatalf("spans = %d, want 2: %+v", len(b.Spans), b.Spans)
	}
	if b.Spans[0].Kind != SpanEmphasis || b.Text[b.Spans[0].From:b.Spans[0].To] != "soft" {
		t.Errorf("span 0 = %+v", b.Spans[0])
	}
	if b.Spans[1].Kind != SpanStrong || b.Text[b.Spans[1].From:b.Spans[1].To] != "hard" {
		t.Errorf("span 1 = %+v", b.Spans[1])
	}
}

func TestParseWikilinkAndCode(t *testing.T) {
	blocks := Parse("Meet [[hero|Vance]] using `1d20`.")
	b := blocks[0]
	if b.Text != "Meet Vance using 1d20." {
		t.Fatalf("text = %q", b.Text)
	}
	if len(b.Spans) != 2 {
		t.Fatalf("spans = %+v", b.Spans)
	}
	if b.Spans[0].Kind != SpanWikilink || b.Spans[0].Target != "hero" {
		t.Errorf("wikilink span = %+v", b.Spans[0])
	}
	if b.Spans[1].Kind != SpanCode {
		t.Errorf("code span = %+v", b.Spans[1])
	}
}

func TestParseBlocks(t *testing.T) {
	blocks := Parse("# Title\n\n- one\n- two\n\n> quoted\n\n---\n\nplain")
	if len(blocks) != 5 {
		t.Fatalf("blocks = %d, want 5: %+v", len(blocks), blocks)
	}
	if blocks[0].Kind != BlockHeading || blocks[0].Level != 1 || blocks[0].Text != "Title" {
		t.Errorf("heading = %+v", blocks[0])
	}
	if blocks[1].Kind != BlockList || len(blocks[1].Items) != 2 {
		t.Errorf("list = %+v", blocks[1])
	}
	if blocks[2].Kind != BlockQuote || blocks[2].Text != "quoted" {
		t.Errorf("quote = %+v", blocks[2])
	}
	if blocks[3].Kind != BlockRule {
		t.Errorf("rule = %+v", blocks[3])
	}
	if blocks[4].Kind != BlockParagraph || blocks[4].Text != "plain" {
		t.Errorf("paragraph = %+v", blocks[4])
	}
}
