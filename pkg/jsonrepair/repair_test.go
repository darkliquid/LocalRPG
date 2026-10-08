package jsonrepair

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestRepair(t *testing.T) {
	cases := []struct {
		name string
		in   string
		kind Kind
		ok   bool
	}{
		{"valid untouched", `{"actor":"x"}`, KindNone, true},
		{"fenced", "```json\n{\"actor\":\"x\"}\n```", KindFence, true},
		{"prose trim", `here you go: {"actor":"x"} done`, KindTrim, true},
		{"unclosed closed", `{"actor":"x"`, KindClose, true},
		{"trailing comma", `{"actor":"x",}`, KindTrailingComma, true},
		{"garbage", `not json at all`, KindNone, false},
	}
	for _, c := range cases {
		got := Repair([]byte(c.in))
		if got.OK != c.ok {
			t.Errorf("%s: OK = %v, want %v", c.name, got.OK, c.ok)
		}
		if c.ok && !json.Valid(got.Payload) {
			t.Errorf("%s: repaired payload is not valid JSON: %q", c.name, got.Payload)
		}
		if c.ok && got.Kind != c.kind {
			t.Errorf("%s: Kind = %q, want %q", c.name, got.Kind, c.kind)
		}
	}
}

func TestRepairValidIsByteIdentical(t *testing.T) {
	in := []byte(`{"a":1,"b":[2,3]}`)
	got := Repair(in)
	if string(got.Payload) != string(in) {
		t.Errorf("valid payload changed: %q -> %q", in, got.Payload)
	}
}

func TestRepairRefusesOversize(t *testing.T) {
	big := []byte(strings.Repeat(" ", 64*1024+1))
	if got := Repair(big); got.OK {
		t.Fatal("oversize payload should be refused")
	}
}

func TestArrayElementsKeepsWhatATruncatedReplyWrote(t *testing.T) {
	// A reply cut off mid-entity: the two before the cut are whole.
	payload := []byte(`{"entities":[{"name":"A","type":"location"},{"name":"B","type":"character"},{"name":"C","ty`)
	got := ArrayElements(payload)
	if len(got) != 2 {
		t.Fatalf("elements = %d, want 2: %q", len(got), got)
	}
	if string(got[0]) != `{"name":"A","type":"location"}` {
		t.Fatalf("first = %s", got[0])
	}
	if string(got[1]) != `{"name":"B","type":"character"}` {
		t.Fatalf("second = %s", got[1])
	}
}

func TestArrayElementsIgnoresBracketsInsideStrings(t *testing.T) {
	payload := []byte(`{"entities":[{"name":"A [not an array]","type":"location"},{"name":"B"}]}`)
	got := ArrayElements(payload)
	if len(got) != 2 {
		t.Fatalf("elements = %d, want 2: %q", len(got), got)
	}
}

func TestArrayElementsHandlesNestedArrays(t *testing.T) {
	payload := []byte(`{"entities":[{"name":"A","tags":["x","y"]},{"name":"B"}]}`)
	got := ArrayElements(payload)
	if len(got) != 2 {
		t.Fatalf("elements = %d, want 2: %q", len(got), got)
	}
	if !bytes.Contains(got[0], []byte(`"tags"`)) {
		t.Fatalf("first = %s", got[0])
	}
}

func TestArrayElementsFindsNothingWithoutAnArray(t *testing.T) {
	if got := ArrayElements([]byte(`{"name":"A"}`)); got != nil {
		t.Fatalf("elements = %q, want none", got)
	}
	if got := ArrayElements(nil); got != nil {
		t.Fatalf("elements = %q, want none", got)
	}
}

func TestArrayElementsStopsAtTheEnclosingArray(t *testing.T) {
	payload := []byte(`{"entities":[]}`)
	if got := ArrayElements(payload); len(got) != 0 {
		t.Fatalf("elements = %q, want none", got)
	}
}
