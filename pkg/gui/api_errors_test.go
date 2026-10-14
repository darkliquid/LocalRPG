package gui

// The mapper is the one place a failed call is answered, so these tests pin the two
// properties that matter: a class of failure keeps its status and gains a stable
// code, and a failure this side caused never puts its text in the body.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/pathutil"
)

func TestAPIFaultForMapsEachClass(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"turn in flight", ErrTurnInFlight, http.StatusConflict, codeTurnInFlight},
		{"duplicate entity", ErrDuplicateEntityID, http.StatusConflict, codeConflict},
		{"world exists", ErrWorldExists, http.StatusConflict, codeConflict},
		{"content conflict", ErrContentConflict, http.StatusConflict, codeConflict},
		{"identifier sentinel", fmt.Errorf("invalid game id: %w", pathutil.ErrPathTraversal), http.StatusBadRequest, codeInvalidRequest},
		{"marked invalid", invalid(errors.New("system name is required")), http.StatusBadRequest, codeInvalidRequest},
		{"world not found", ErrWorldNotFound, http.StatusNotFound, codeNotFound},
		{"missing file", fmt.Errorf("open campaign: %w", fs.ErrNotExist), http.StatusNotFound, codeNotFound},
		{"no native dialog", ErrNoNativeDialog, http.StatusNotImplemented, codeNotImplemented},
		{"client went away", context.Canceled, statusClientClosedRequest, codeCancelled},
		{"unknown is ours", errors.New("boom"), http.StatusInternalServerError, codeInternal},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fault := apiFaultFor(tc.err)
			if fault.status != tc.status {
				t.Errorf("status = %d, want %d", fault.status, tc.status)
			}
			if fault.code != tc.code {
				t.Errorf("code = %q, want %q", fault.code, tc.code)
			}
		})
	}
}

func TestTurnInFlightAsksTheClientToRetry(t *testing.T) {
	fault := apiFaultFor(ErrTurnInFlight)

	if fault.retryAfter == "" {
		t.Fatal("a turn in flight must carry a Retry-After, because the client's next step is to wait")
	}
}

// The leak this bead exists for: the error's own text must not reach the body when
// the fault is this side's, because these errors are wrapped on the way up and the
// wrap names filesystem paths and package structure.
func TestInternalDetailIsNotSentToTheClient(t *testing.T) {
	s := &Server{service: &Service{}}
	rec := httptest.NewRecorder()

	s.writeAPIError(rec, fmt.Errorf("open /home/someone/.config/localrpg/campaign.yaml: %w", fs.ErrPermission))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if body := rec.Body.String(); strings.Contains(body, "campaign.yaml") || strings.Contains(body, "/home/") {
		t.Errorf("the body carries internal detail: %q", body)
	}

	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("the body is not the shared envelope: %v (body %q)", err, rec.Body.String())
	}
	if envelope.Error.Code != codeInternal {
		t.Errorf("code = %q, want %q", envelope.Error.Code, codeInternal)
	}
	if envelope.Error.Message != internalMessage {
		t.Errorf("message = %q, want the fixed %q", envelope.Error.Message, internalMessage)
	}
}

// A request the client got wrong is the one case where the message is the point.
func TestInvalidRequestKeepsTheMessageThatSaysWhatToChange(t *testing.T) {
	s := &Server{service: &Service{}}
	rec := httptest.NewRecorder()

	s.writeAPIError(rec, invalid(errors.New("system name is required")))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if body := rec.Body.String(); !strings.Contains(body, "system name is required") {
		t.Errorf("body = %q, want the message the client can act on", body)
	}
}

func TestMethodNotAllowedNamesTheMethods(t *testing.T) {
	rec := httptest.NewRecorder()

	methodNotAllowed(rec, http.MethodGet, http.MethodPatch)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
	if got := rec.Header().Get("Allow"); got != "GET, PATCH" {
		t.Errorf("Allow = %q, want %q", got, "GET, PATCH")
	}
}
