package desktop

import "testing"

func TestParseConsolePrefix(t *testing.T) {
	cases := []struct{ in, mode, text string }{
		{"/say hello", "say", "hello"},
		{"/do look", "do", "look"},
		{"/story once upon", "story", "once upon"},
		{"/roll 1d20", "roll", "1d20"},
		{"just looking", "do", "just looking"},
	}
	for _, c := range cases {
		mode, text := parseConsole(c.in, "do")
		if mode != c.mode || text != c.text {
			t.Errorf("parseConsole(%q) = (%q,%q), want (%q,%q)", c.in, mode, text, c.mode, c.text)
		}
	}
}
