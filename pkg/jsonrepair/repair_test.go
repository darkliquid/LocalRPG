package jsonrepair

import (
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
