package jsonrepair

import (
	"bytes"
	"encoding/json"
	"testing"
)

func FuzzRepair(f *testing.F) {
	for _, s := range []string{
		`{"a":1}`, "```json\n{}\n```", `{"a":1`, `{"a":1,}`, `nope`,
		`{"a":"}"`, `{"a":1} trailing`, "[1,2,", `{"a":[{"b":`,
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, in []byte) {
		res := Repair(in)
		if res.OK && !json.Valid(res.Payload) {
			t.Fatalf("Repair reported OK but payload invalid: %q -> %q", in, res.Payload)
		}
		if res.OK {
			again := Repair(res.Payload)
			if !again.OK {
				t.Fatalf("Repair is not idempotent: %q -> %q -> not OK", in, res.Payload)
			}
			if !bytes.Equal(again.Payload, res.Payload) {
				t.Fatalf("Repair is not idempotent: %q -> %q -> %q", in, res.Payload, again.Payload)
			}
		}
		_ = BraceDepth(in)
	})
}
