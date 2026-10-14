package gui

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/models"
	"github.com/darkliquid/localrpg/pkg/pathutil"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// The stable codes a client switches on. They are the contract the frontend reads
// out of the error envelope (frontend/src/api/client.ts), so they outlive any
// handler's choice of status.
const (
	codeInvalidRequest = "invalid_request"
	codeNotFound       = "not_found"
	codeConflict       = "conflict"
	codeTurnInFlight   = "turn_in_flight"
	codeNotImplemented = "not_implemented"
	codeUnavailable    = "unavailable"
	codeRateLimited    = "rate_limited"
	codeCancelled      = "cancelled"
	codeInternal       = "internal"
)

// internalMessage is what a client is told when the fault is this side's. The
// detail goes to the log instead, because these errors are wrapped on the way up
// and the wrap names filesystem paths and package structure.
const internalMessage = "the request could not be completed"

// statusClientClosedRequest is the non-standard code every proxy uses for a client
// that went away before its answer was ready.
const statusClientClosedRequest = 499

// ErrInvalidRequest marks a failure as the request's fault rather than this
// side's, so the mapper answers 400 and lets the message through.
var ErrInvalidRequest = errors.New("invalid request")

// invalidRequestError carries a request's own failure to the mapper without
// prefixing its message.
type invalidRequestError struct{ err error }

func (e invalidRequestError) Error() string { return e.err.Error() }
func (e invalidRequestError) Unwrap() error { return e.err }
func (e invalidRequestError) Is(target error) bool {
	return target == ErrInvalidRequest
}

// invalid marks err as the request's fault. A handler uses it where it knows the
// request was wrong and the service's error carries no sentinel for it, so the
// judgement is written down as one rather than encoded as a status number.
func invalid(err error) error { return invalidRequestError{err} }

// apiFault is the answer a failed call gets: the status, the stable code the
// client switches on, the message it is told, and a retry hint when there is one.
type apiFault struct {
	status     int
	code       string
	message    string
	retryAfter string
}

// apiFaultFor decides what a client is told about err. A failure the client caused
// carries its own message, because that names what to change. Anything else is this
// side's fault and is answered with a fixed message, so a wrapped error cannot leak
// out of the API; writeAPIError records the detail in that case.
//
// The classes are the sentinels the handlers already distinguish, which is why this
// table replaces their guesses rather than changing their answers.
func apiFaultFor(err error) apiFault {
	switch {
	// A turn already in flight is the one failure whose client has a next step.
	case errors.Is(err, ErrTurnInFlight):
		return apiFault{
			status:     http.StatusConflict,
			code:       codeTurnInFlight,
			message:    err.Error(),
			retryAfter: "1",
		}

	case errors.Is(err, ErrInvalidRequest),
		errors.Is(err, pathutil.ErrEmptyID),
		errors.Is(err, pathutil.ErrPathTraversal),
		errors.Is(err, pathutil.ErrInvalidID),
		errors.Is(err, ErrInvalidFolderPath),
		errors.Is(err, ErrAdvancementRefused),
		errors.Is(err, ErrEmptyDraftSelection),
		errors.Is(err, ErrInvalidCounter),
		errors.Is(err, ErrNegotiationLimit),
		errors.Is(err, ErrNoPendingCheck),
		errors.Is(err, ErrPendingCheckMismatch),
		errors.Is(err, ErrSourceTooLarge),
		errors.Is(err, ErrExportDirRequired),
		errors.Is(err, ErrNoGeneratorForImport):
		return apiFault{status: http.StatusBadRequest, code: codeInvalidRequest, message: err.Error()}

	case errors.Is(err, ErrDuplicateEntityID),
		errors.Is(err, ErrContentConflict),
		errors.Is(err, ErrRegistrySourceExists),
		errors.Is(err, ErrWorldEntityExists),
		errors.Is(err, ErrWorldExists),
		errors.Is(err, ErrSystemInUse),
		errors.Is(err, ErrWorldInUse),
		errors.Is(err, ErrDirectoryChoiceInFlight),
		errors.Is(err, ErrSaveFileChoiceInFlight),
		errors.Is(err, ErrExportInFlight),
		errors.Is(err, models.ErrDownloadActive):
		return apiFault{status: http.StatusConflict, code: codeConflict, message: err.Error()}

	case errors.Is(err, ErrWorldNotFound),
		errors.Is(err, ErrSystemNotFound),
		errors.Is(err, ErrReferenceSystemNotFound),
		errors.Is(err, ErrContentNotFound),
		errors.Is(err, ErrDocNotFound),
		errors.Is(err, ErrRegistrySourceNotFound),
		errors.Is(err, fs.ErrNotExist),
		errors.Is(err, os.ErrNotExist):
		return apiFault{status: http.StatusNotFound, code: codeNotFound, message: err.Error()}

	case errors.Is(err, ErrNoNativeDialog),
		errors.Is(err, ErrNoNativeSaveFileDialog),
		errors.Is(err, errNoURLOpener):
		return apiFault{status: http.StatusNotImplemented, code: codeNotImplemented, message: err.Error()}

	case errors.Is(err, context.Canceled):
		return apiFault{status: statusClientClosedRequest, code: codeCancelled, message: "the request was cancelled"}

	// Ours, but the user can act on it: a campaign whose files are not ready, a
	// provider with no batch API. The status says the call cannot succeed as things
	// stand, and the message says what to change.
	case errors.Is(err, ErrCampaignNotPlayable), errors.Is(err, ErrNoBatchAPI):
		return apiFault{status: http.StatusServiceUnavailable, code: codeUnavailable, message: err.Error()}
	}

	var limited *harness.ErrRateLimitedUntil
	if errors.As(err, &limited) {
		return apiFault{
			status:     http.StatusTooManyRequests,
			code:       codeRateLimited,
			message:    limited.Error(),
			retryAfter: strconv.Itoa(int(limited.RetryAfter().Seconds()) + 1),
		}
	}

	return apiFault{status: http.StatusInternalServerError, code: codeInternal, message: internalMessage}
}

// writeAPIError answers a failed call: the fault the error maps to, the detail
// recorded when the fault is this side's, and the envelope the client parses. The
// mapping and the message policy exist once, here, so no handler can reconsider
// either.
//
// A nil error is a programming mistake, and answering the generic fault is better
// than panicking inside a handler.
func writeAPIError(w http.ResponseWriter, logger trace.Logger, err error) {
	if err == nil {
		err = errors.New("handler reported a failure with no error")
	}
	fault := apiFaultFor(err)
	if fault.status >= http.StatusInternalServerError {
		trace.OrNil(logger).Event("api.error", map[string]any{
			"status": fault.status,
			"code":   fault.code,
			"error":  err.Error(),
		})
	}
	if fault.retryAfter != "" {
		w.Header().Set("Retry-After", fault.retryAfter)
	}
	writeErrorEnvelope(w, fault.status, fault.code, fault.message)
}

// Both types here serve routes: the server for the API's own handlers, and the
// service for the routes it owns. Each answers through the one core, with the
// logger it holds.
func (s *Server) writeAPIError(w http.ResponseWriter, err error) {
	writeAPIError(w, s.serviceLogger(), err)
}

func (s *Service) writeAPIError(w http.ResponseWriter, err error) {
	writeAPIError(w, s.logger, err)
}

// serviceLogger is the server's service logger, or nil when a test built a server
// without one.
func (s *Server) serviceLogger() trace.Logger {
	if s.service == nil {
		return nil
	}
	return s.service.logger
}

// writeErrorEnvelope writes the error envelope the client parses, with a stable
// code when there is one.
func writeErrorEnvelope(w http.ResponseWriter, status int, code, message string) {
	body := map[string]string{"message": message}
	if code != "" {
		body["code"] = code
	}
	writeJSONStatus(w, status, map[string]any{"error": body})
}

// methodNotAllowed answers a request whose method this route does not serve, and
// names the ones it does. Some branches answered 404 instead, which tells a client
// the route does not exist rather than that it used the wrong verb.
func methodNotAllowed(w http.ResponseWriter, allowed ...string) {
	w.Header().Set("Allow", strings.Join(allowed, ", "))
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}
