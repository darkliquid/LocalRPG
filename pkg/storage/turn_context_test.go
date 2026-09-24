package storage

import (
	"bytes"
	"testing"
)

func TestTurnContextRoundTrip(t *testing.T) {
	store := openTestDB(t)
	raw := []byte(`{"turn_number":1,"prompt_hash":"abc","strategy":"full_prompt"}`)
	if err := store.SaveTurnContext(1, "the prompt", raw); err != nil {
		t.Fatalf("SaveTurnContext: %v", err)
	}
	got, prompt, err := store.GetTurnContext(1)
	if err != nil {
		t.Fatalf("GetTurnContext: %v", err)
	}
	if prompt != "the prompt" || !bytes.Equal(got, raw) {
		t.Fatalf("round trip mismatch: %q %s", prompt, string(got))
	}
}
