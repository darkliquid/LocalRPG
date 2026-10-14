package gui

// A response that cannot be encoded must not be a success. The status is ours
// until the first byte goes out, so these tests pin the moment the helpers
// decide: marshal first, then write, and a value json.Marshal rejects is a 500
// with the error envelope rather than a 200 whose body stops mid-object.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// unencodable is a value json.Marshal rejects, which is how a real one arrives:
// a field added to a response DTO that has no JSON representation.
func unencodable() any { return make(chan int) }

func TestWriteJSONUnencodableIsAServerError(t *testing.T) {
	rec := httptest.NewRecorder()

	writeJSON(rec, unencodable())

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d: an unencodable body must not be a success", rec.Code, http.StatusInternalServerError)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var envelope struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("the fault body is not the shared envelope: %v (body %q)", err, rec.Body.String())
	}
	if envelope.Error.Message == "" {
		t.Errorf("the envelope carries no message: %q", rec.Body.String())
	}
}

func TestWriteJSONStatusFailsBeforeItSendsTheStatus(t *testing.T) {
	rec := httptest.NewRecorder()

	writeJSONStatus(rec, http.StatusCreated, unencodable())

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d: the caller's status must not be sent for a body that cannot be written", rec.Code, http.StatusInternalServerError)
	}
}

func TestWriteJSONBytesMatchTheEncoder(t *testing.T) {
	value := map[string]any{"name": "<script>", "count": 3, "nested": []int{1, 2}}

	got := httptest.NewRecorder()
	writeJSON(got, value)

	want := httptest.NewRecorder()
	if err := json.NewEncoder(want).Encode(value); err != nil {
		t.Fatal(err)
	}

	if got.Body.String() != want.Body.String() {
		t.Errorf("body = %q, want %q: a well-formed response must not change shape", got.Body.String(), want.Body.String())
	}
	if got.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", got.Code, http.StatusOK)
	}
}

func TestWriteJSONStatusKeepsTheGivenStatus(t *testing.T) {
	rec := httptest.NewRecorder()

	writeJSONStatus(rec, http.StatusAccepted, map[string]string{"status": "downloading"})

	if rec.Code != http.StatusAccepted {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}
	if body := rec.Body.String(); !strings.Contains(body, "downloading") {
		t.Errorf("body = %q, want it to carry the payload", body)
	}
}
