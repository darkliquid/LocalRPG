package gui

import (
	"encoding/json"
	"net/http"

	"github.com/darkliquid/localrpg/pkg/harness"
)

// generationStatus maps a bounded failure code to the HTTP status that best
// describes it. A hard failure is a non-2xx; a partial success never reaches
// here because it is returned as a 200 with Warning set.
func generationStatus(code harness.FailureCode) int {
	switch code {
	case harness.FailureInvalidRequest:
		return http.StatusBadRequest
	case harness.FailureContextTooLarge:
		return http.StatusRequestEntityTooLarge
	case harness.FailureParseError:
		return http.StatusUnprocessableEntity
	case harness.FailureProviderUnavailable:
		return http.StatusServiceUnavailable
	case harness.FailureTimeout:
		return http.StatusGatewayTimeout
	case harness.FailureProviderError, harness.FailureEmptyResponse:
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}

// writeGenerationError writes a structured failure body and status.
func writeGenerationError(w http.ResponseWriter, failure *harness.GenerationFailure) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(generationStatus(failure.Code))
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": failure})
}

// writeGenerationFailure recognises a generation failure and writes it, so
// handlers can share one line. It reports whether it handled the error; a plain
// error must still go through the caller's existing path.
func writeGenerationFailure(w http.ResponseWriter, err error) bool {
	failure, ok := harness.FailureFrom(err)
	if !ok {
		return false
	}
	writeGenerationError(w, failure)
	return true
}
