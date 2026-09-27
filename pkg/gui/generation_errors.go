package gui

import (
	"net/http"
	"strconv"

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
	case harness.FailureRateLimited:
		return http.StatusTooManyRequests
	case harness.FailureInsufficientFunds:
		return http.StatusPaymentRequired
	case harness.FailureProviderError, harness.FailureEmptyResponse:
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}

// writeGenerationError writes a structured failure body and status.
func writeGenerationError(w http.ResponseWriter, failure *harness.GenerationFailure) {
	if failure.RetryAfterMS > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(failure.RetryAfterMS/1000)+1))
	}
	writeJSONStatus(w, generationStatus(failure.Code), map[string]interface{}{"error": failure})
}

// writeInvalidRequest writes a 400 in the same shape as every other generation
// failure, so clients parse one error type.
func writeInvalidRequest(w http.ResponseWriter, message string) {
	writeGenerationError(w, &harness.GenerationFailure{
		Code:    harness.FailureInvalidRequest,
		Message: message,
	})
}

// writeJSONError writes a JSON error envelope with an explicit status, for
// non-generation failures such as a world conflict.
func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSONStatus(w, status, map[string]interface{}{"error": map[string]string{"message": message}})
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
