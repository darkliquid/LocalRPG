package jsonrepair

import "testing"

func TestBraceDepth(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{`{"a":1}`, 0},
		{`{"a":1`, 1},
		{`{"a":{"b":2`, 2},
		{`[1,2,3`, 1},
		{`{"a":"}{"}`, 0},
		{`{"a":"\""}`, 0},
		{`}`, -1},
	}
	for _, c := range cases {
		if got := BraceDepth([]byte(c.in)); got != c.want {
			t.Errorf("BraceDepth(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}
