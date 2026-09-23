package harness

import "testing"

func TestProseComplete(t *testing.T) {
	cases := []struct {
		name string
		text string
		want bool
	}{
		{"sentence end", "The gate stands open.", true},
		{"question", "Do you hear it?", true},
		{"exclamation", "Run!", true},
		{"ellipsis rune", "The road ends here…", true},
		{"three dots", "The road ends here...", true},
		{"trailing closing quote", `He said "stop."`, true},
		{"scene break", "The road ends here.\n\n---", true},
		{"heading ending", "## The Gate", true},
		{"mid word", "The gate stands op", false},
		{"mid sentence no terminator", "The gate stands open, and then", false},
		{"decimal is not a terminator", "The gate is 3.5", false},
		{"domain is not a terminator", "Visit example.com", false},
		{"abbreviation", "Dr. Halloway arrived", false},
		{"initial", "H. P. Lovecraft wrote", false},
		{"open quotation", `He said "stop`, false},
		{"open emphasis", "The *gate", false},
		{"unterminated wikilink", "The [[gate", false},
		{"empty", "", false},
		{"whitespace", "   \n  ", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ProseComplete(tc.text); got != tc.want {
				t.Errorf("ProseComplete(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}

func TestLastSentenceBoundary(t *testing.T) {
	text := "The gate stands open before us all. Its hinges groan, and then"
	offset, ok := LastSentenceBoundary(text, 24)
	if !ok {
		t.Fatalf("expected a boundary in %q", text)
	}
	if got := text[:offset]; got != "The gate stands open before us all." {
		t.Errorf("boundary text = %q", got)
	}

	if _, ok := LastSentenceBoundary("Too short. And more", 24); ok {
		t.Errorf("expected no boundary when the first ends before minChars")
	}
	if _, ok := LastSentenceBoundary("no terminator anywhere", 4); ok {
		t.Errorf("expected no boundary without a terminator")
	}
}

func TestTrimToLastSentence(t *testing.T) {
	trimmed, ok := TrimToLastSentence("The gate stands open before us all. Its hinges groan, and then", 24)
	if !ok {
		t.Fatalf("expected a trim")
	}
	if trimmed != "The gate stands open before us all." {
		t.Errorf("TrimToLastSentence = %q", trimmed)
	}

	if got, ok := TrimToLastSentence("no terminator anywhere", 4); ok {
		t.Errorf("expected no trim, got %q", got)
	}
}
