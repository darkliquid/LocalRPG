package tools

import "testing"

func TestBuildMatch(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  string
	}{
		{"single word", "warden", `"warden"*`},
		{"two words", "guard kael", `"guard" AND "kael"*`},
		{"punctuation is stripped", "kael's oath", `"kaels" AND "oath"*`},
		{"a bare operator cannot break the query", "guard AND", `"guard" AND "AND"*`},
		{"NEAR is quoted like any word", "NEAR(gate", `"NEARgate"*`},
		{"extra whitespace collapses", "  ancient   bridge  ", `"ancient" AND "bridge"*`},
		{"empty is empty", "   ", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := BuildMatch(tc.query); got != tc.want {
				t.Errorf("BuildMatch(%q) = %q, want %q", tc.query, got, tc.want)
			}
		})
	}
}
