package gui

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestGenerationStatus(t *testing.T) {
	tests := map[harness.FailureCode]int{
		harness.FailureProviderUnavailable: 503,
		harness.FailureProviderError:       502,
		harness.FailureEmptyResponse:       502,
		harness.FailureParseError:          422,
		harness.FailureTimeout:             504,
		harness.FailureContextTooLarge:     413,
		harness.FailureInvalidRequest:      400,
		harness.FailureCode("unknown"):     500,
	}
	for code, want := range tests {
		if got := generationStatus(code); got != want {
			t.Fatalf("generationStatus(%q) = %d, want %d", code, got, want)
		}
	}
}

func TestWriteGenerationFailure(t *testing.T) {
	rec := httptest.NewRecorder()
	failure := &harness.GenerationFailure{Code: harness.FailureEmptyResponse, Message: "no text"}
	if !writeGenerationFailure(rec, failure) {
		t.Fatal("writeGenerationFailure did not recognise a failure")
	}
	if rec.Code != 502 {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	var body struct {
		Error harness.GenerationFailure `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if body.Error.Code != harness.FailureEmptyResponse {
		t.Fatalf("error.code = %q, want empty_response", body.Error.Code)
	}
}

func TestWriteGenerationFailureIgnoresPlainErrors(t *testing.T) {
	rec := httptest.NewRecorder()
	if writeGenerationFailure(rec, errPlain{}) {
		t.Fatal("writeGenerationFailure claimed a plain error was a generation failure")
	}
}

type errPlain struct{}

func (errPlain) Error() string { return "plain" }
