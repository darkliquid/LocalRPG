package sysgen

import (
	"context"
	"testing"
)

func TestGenerateJSONRetriesOnceOnAMalformedReply(t *testing.T) {
	gen := &jsonGen{responses: []string{`{"a": not json}`, `{"a":1}`}}
	var out struct {
		A int `json:"a"`
	}

	if err := generateJSON(context.Background(), gen, "p", "s", &out); err != nil {
		t.Fatalf("generateJSON: %v", err)
	}
	if out.A != 1 {
		t.Fatalf("out = %+v, want the decoded reply after the retry", out)
	}
	if len(gen.prompts) != 2 {
		t.Fatalf("calls = %d, want 2", len(gen.prompts))
	}
}

func TestGenerateJSONDoesNotRetryAValidReply(t *testing.T) {
	gen := &jsonGen{responses: []string{`{"a":1}`}}
	var out struct {
		A int `json:"a"`
	}

	if err := generateJSON(context.Background(), gen, "p", "s", &out); err != nil {
		t.Fatalf("generateJSON: %v", err)
	}
	if len(gen.prompts) != 1 {
		t.Fatalf("calls = %d, want 1", len(gen.prompts))
	}
}

func TestGenerateJSONGivesUpAfterOneRetry(t *testing.T) {
	gen := &jsonGen{responses: []string{`{"a": not json}`, `{"a": still not json}`}}
	var out struct {
		A int `json:"a"`
	}

	if err := generateJSON(context.Background(), gen, "p", "s", &out); err == nil {
		t.Fatal("expected an error after two unreadable replies")
	}
	if len(gen.prompts) != 2 {
		t.Fatalf("calls = %d, want 2", len(gen.prompts))
	}
}

func TestGenerateJSONDoesNotRetryAGenerateError(t *testing.T) {
	gen := &jsonGen{}
	var out struct {
		A int `json:"a"`
	}

	if err := generateJSON(context.Background(), gen, "p", "s", &out); err == nil {
		t.Fatal("expected the generate error")
	}
	if len(gen.prompts) != 1 {
		t.Fatalf("calls = %d, want 1 (a generate error has no reply to re-ask about)", len(gen.prompts))
	}
}