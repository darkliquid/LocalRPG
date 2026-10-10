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

func TestRepairReportsTheFurthestAttemptWhenNothingValidates(t *testing.T) {
	// A reply that was fenced and then cut off. The fence is gone, so an error
	// quotes the cut rather than the backticks, and a caller salvaging elements
	// starts from the JSON instead of from Markdown.
	// The cut is inside a string, which closing structures cannot repair.
	in := []byte("```json\n{\"entities\":[{\"name\":\"A\"},{\"name\":\"B")
	got := Repair(in)
	if got.OK {
		t.Fatalf("payload should not validate: %q", got.Payload)
	}
	if !bytes.HasPrefix(got.Payload, []byte("{")) {
		t.Fatalf("the fence should be stripped from the reported payload: %q", got.Payload)
	}
	if len(ArrayElements(got.Payload)) != 1 {
		t.Fatalf("elements = %q, want the one complete element", ArrayElements(got.Payload))
	}
}

func TestRepairLeavesAnUnrepairablePayloadAlone(t *testing.T) {
	in := []byte("no json here at all")
	if got := Repair(in); got.OK || string(got.Payload) != string(in) {
		t.Fatalf("payload = %q, want the input unchanged", got.Payload)
	}
}

func TestRepairStringFaults(t *testing.T) {
	cases := []struct {
		name string
		in   string
		kind Kind
	}{
		{"literal newline in a string", "{\"a\":\"one\ntwo\"}", KindEscapeControl},
		{"literal tab in a string", "{\"a\":\"one\ttwo\"}", KindEscapeControl},
		{"leading BOM", "\ufeff{\"a\":1}", KindBOM},
		{"smart quotes as delimiters", "{\u201ca\u201d:1}", KindSmartQuote},
	}
	for _, c := range cases {
		got := Repair([]byte(c.in))
		if !got.OK {
			t.Errorf("%s: not repaired: %q", c.name, got.Payload)
			continue
		}
		if !json.Valid(got.Payload) {
			t.Errorf("%s: payload invalid: %q", c.name, got.Payload)
		}
		if got.Kind != c.kind {
			t.Errorf("%s: Kind = %q, want %q", c.name, got.Kind, c.kind)
		}
	}
}

func TestRepairPreservesText(t *testing.T) {
	got := Repair([]byte("{\"a\":\"one\ntwo\"}"))
	var decoded map[string]string
	if err := json.Unmarshal(got.Payload, &decoded); err != nil {
		t.Fatalf("unmarshal repaired: %v", err)
	}
	if decoded["a"] != "one\ntwo" {
		t.Fatalf("decoded = %q, want the literal newline preserved", decoded["a"])
	}
}

func TestRepairLeavesASmartQuoteInsideAValue(t *testing.T) {
	// A smart quote inside a value is content, not a delimiter, so the payload is
	// already valid and must come back byte-identical.
	in := []byte("{\"a\":\"it\u2019s\"}")
	got := Repair(in)
	if !got.OK || string(got.Payload) != string(in) {
		t.Fatalf("payload = %q, want the input unchanged", got.Payload)
	}
}
