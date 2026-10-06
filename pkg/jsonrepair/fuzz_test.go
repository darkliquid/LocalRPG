package jsonrepair

import (
	"encoding/json"
	"testing"
)

func FuzzRepair(f *testing.F) {
	for _, s := range []string{
		`{"a":1}`, "```json\n{}\n```", `{"a":1`, `{"a":1,}`, `nope`,
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, in []byte) {
		res := Repair(in)
		if res.OK && !json.Valid(res.Payload) {
			t.Fatalf("Repair reported OK but payload invalid: %q -> %q", in, res.Payload)
		}
	})
}
