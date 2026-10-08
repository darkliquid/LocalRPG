package gui

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// chosenDir drives a picker that answers immediately.
func pickerReturning(path string) func(title, defaultDir string) (string, error) {
	return func(_, _ string) (string, error) { return path, nil }
}

func TestStartDirectoryChoiceWithoutPicker(t *testing.T) {
	svc := NewService(t.TempDir())

	if err := svc.StartDirectoryChoice(ChooseDirectoryRequestDTO{}); !errors.Is(err, ErrNoNativeDialog) {
		t.Fatalf("StartDirectoryChoice = %v, want ErrNoNativeDialog", err)
	}
	if svc.ExportCapabilities().NativeDialog {
		t.Fatal("a headless service must not report a native dialog")
	}
}

func TestDirectoryChoiceReportsAPick(t *testing.T) {
	svc := NewService(t.TempDir())
	want := t.TempDir()
	svc.SetDirectoryPicker(pickerReturning(want))

	if err := svc.StartDirectoryChoice(ChooseDirectoryRequestDTO{Title: "Choose a source folder"}); err != nil {
		t.Fatal(err)
	}

	choice, err := svc.WaitForDirectoryChoice(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if choice.Status != DirectoryChoiceSelected || choice.Path != want {
		t.Fatalf("choice = %+v", choice)
	}

	// A finished choice is delivered once, then cleared.
	if again := svc.DirectoryChoice(); again.Status != DirectoryChoiceIdle {
		t.Fatalf("second read = %+v, want idle", again)
	}
}

func TestDirectoryChoiceReportsACancel(t *testing.T) {
	svc := NewService(t.TempDir())
	// An empty path with no error is how the Wails picker reports a dismissal.
	svc.SetDirectoryPicker(pickerReturning(""))

	if err := svc.StartDirectoryChoice(ChooseDirectoryRequestDTO{}); err != nil {
		t.Fatal(err)
	}
	choice, err := svc.WaitForDirectoryChoice(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if choice.Status != DirectoryChoiceCancelled {
		t.Fatalf("choice = %+v, want cancelled", choice)
	}
}

func TestDirectoryChoiceReportsAPickerErrorAsACancel(t *testing.T) {
	svc := NewService(t.TempDir())
	svc.SetDirectoryPicker(func(_, _ string) (string, error) { return "", errors.New("boom") })

	if err := svc.StartDirectoryChoice(ChooseDirectoryRequestDTO{}); err != nil {
		t.Fatal(err)
	}
	choice, err := svc.WaitForDirectoryChoice(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if choice.Status != DirectoryChoiceCancelled {
		t.Fatalf("choice = %+v, want cancelled", choice)
	}
}

func TestStartDirectoryChoiceRefusesASecondDialog(t *testing.T) {
	svc := NewService(t.TempDir())
	release := make(chan struct{})
	svc.SetDirectoryPicker(func(_, _ string) (string, error) {
		<-release
		return "/tmp/picked", nil
	})

	if err := svc.StartDirectoryChoice(ChooseDirectoryRequestDTO{}); err != nil {
		t.Fatal(err)
	}
	if err := svc.StartDirectoryChoice(ChooseDirectoryRequestDTO{}); !errors.Is(err, ErrDirectoryChoiceInFlight) {
		t.Fatalf("second start = %v, want ErrDirectoryChoiceInFlight", err)
	}
	close(release)
	if _, err := svc.WaitForDirectoryChoice(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestStartDirectoryChoiceReturnsAtOnce(t *testing.T) {
	svc := NewService(t.TempDir())
	release := make(chan struct{})
	svc.SetDirectoryPicker(func(_, _ string) (string, error) {
		<-release
		return "/tmp/picked", nil
	})
	defer close(release)

	// The whole point: a modal dialog must not hold the caller open.
	started := time.Now()
	if err := svc.StartDirectoryChoice(ChooseDirectoryRequestDTO{}); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("StartDirectoryChoice blocked for %s", elapsed)
	}
	if choice := svc.DirectoryChoice(); choice.Status != DirectoryChoicePending {
		t.Fatalf("choice = %+v, want pending", choice)
	}
}

func TestStartDirectoryChoiceOffersADefaultTitleAndFolder(t *testing.T) {
	svc := NewService(t.TempDir())
	var seenTitle, seenDefault string
	svc.SetDirectoryPicker(func(title, defaultDir string) (string, error) {
		seenTitle, seenDefault = title, defaultDir
		return "", nil
	})

	if err := svc.StartDirectoryChoice(ChooseDirectoryRequestDTO{}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.WaitForDirectoryChoice(context.Background()); err != nil {
		t.Fatal(err)
	}
	if seenTitle == "" {
		t.Fatal("expected a default title")
	}
	if seenDefault == "" {
		t.Fatal("expected a default directory to be offered")
	}
}

func TestStartDirectoryChoicePassesTheTitleAndFolderThrough(t *testing.T) {
	svc := NewService(t.TempDir())
	var seenTitle, seenDefault string
	svc.SetDirectoryPicker(func(title, defaultDir string) (string, error) {
		seenTitle, seenDefault = title, defaultDir
		return "", nil
	})

	req := ChooseDirectoryRequestDTO{Title: "Choose a source folder", DefaultDir: "/tmp/notes"}
	if err := svc.StartDirectoryChoice(req); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.WaitForDirectoryChoice(context.Background()); err != nil {
		t.Fatal(err)
	}
	if seenTitle != "Choose a source folder" || seenDefault != "/tmp/notes" {
		t.Fatalf("picker got title %q default %q", seenTitle, seenDefault)
	}
}

func TestDirectoryChoiceRouteWithoutPicker(t *testing.T) {
	svc := NewService(t.TempDir())
	server := NewServer(svc, http.NotFoundHandler())

	req := httptest.NewRequest(http.MethodPost, "/api/dialog/directory", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", rec.Code)
	}
}

func TestDirectoryChoiceRouteStartsAndPolls(t *testing.T) {
	svc := NewService(t.TempDir())
	svc.SetDirectoryPicker(pickerReturning("/tmp/picked"))
	server := NewServer(svc, http.NotFoundHandler())

	body := strings.NewReader(`{"title":"Choose a source folder"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/dialog/directory", body)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("start status = %d, want 202 (%s)", rec.Code, rec.Body.String())
	}
	var started DirectoryChoiceDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	if started.Status != DirectoryChoicePending {
		t.Fatalf("start body = %+v", started)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		poll := httptest.NewRequest(http.MethodGet, "/api/dialog/directory", nil)
		pollRec := httptest.NewRecorder()
		server.ServeHTTP(pollRec, poll)
		if pollRec.Code != http.StatusOK {
			t.Fatalf("poll status = %d", pollRec.Code)
		}
		var choice DirectoryChoiceDTO
		if err := json.Unmarshal(pollRec.Body.Bytes(), &choice); err != nil {
			t.Fatal(err)
		}
		if choice.Status == DirectoryChoiceSelected {
			if choice.Path != "/tmp/picked" {
				t.Fatalf("path = %q", choice.Path)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the choice never completed: %+v", choice)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestDirectoryChoiceRouteRejectsASecondDialog(t *testing.T) {
	svc := NewService(t.TempDir())
	release := make(chan struct{})
	svc.SetDirectoryPicker(func(_, _ string) (string, error) {
		<-release
		return "/tmp/picked", nil
	})
	defer close(release)
	server := NewServer(svc, http.NotFoundHandler())

	first := httptest.NewRequest(http.MethodPost, "/api/dialog/directory", nil)
	firstRec := httptest.NewRecorder()
	server.ServeHTTP(firstRec, first)
	if firstRec.Code != http.StatusAccepted {
		t.Fatalf("first status = %d", firstRec.Code)
	}

	second := httptest.NewRequest(http.MethodPost, "/api/dialog/directory", nil)
	secondRec := httptest.NewRecorder()
	server.ServeHTTP(secondRec, second)
	if secondRec.Code != http.StatusConflict {
		t.Fatalf("second status = %d, want 409", secondRec.Code)
	}
}

func TestDirectoryChoiceRouteRejectsAnUnknownAction(t *testing.T) {
	svc := NewService(t.TempDir())
	server := NewServer(svc, http.NotFoundHandler())

	req := httptest.NewRequest(http.MethodPost, "/api/dialog/nonsense", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
