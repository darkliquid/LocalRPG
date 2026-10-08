package gui

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/adrg/xdg"
)

// ErrNoNativeDialog reports that no native directory picker is available, so the
// UI must fall back to a path field.
var ErrNoNativeDialog = errors.New("native directory dialog is not available")

// ErrDirectoryChoiceInFlight reports a second folder dialog while one is open.
// The native picker is modal, so only one can be pending at a time.
var ErrDirectoryChoiceInFlight = errors.New("a folder dialog is already open")

// ErrNoNativeSaveFileDialog reports that no native save file picker is available, so the
// UI must fall back to direct download.
var ErrNoNativeSaveFileDialog = errors.New("native save file dialog is not available")

// ErrSaveFileChoiceInFlight reports a second save file dialog while one is open.
var ErrSaveFileChoiceInFlight = errors.New("a save file dialog is already open")

// The states a directory choice moves through. Idle means nothing is pending and
// nothing has been chosen.
const (
	DirectoryChoiceIdle      = "idle"
	DirectoryChoicePending   = "pending"
	DirectoryChoiceSelected  = "selected"
	DirectoryChoiceCancelled = "cancelled"
)

const (
	SaveFileChoiceIdle      = "idle"
	SaveFileChoicePending   = "pending"
	SaveFileChoiceSelected  = "selected"
	SaveFileChoiceCancelled = "cancelled"
)

// ChooseDirectoryRequestDTO asks the desktop window for a directory. Title is
// what the picker shows, so one endpoint serves an export destination and a
// source folder without the picker guessing.
type ChooseDirectoryRequestDTO struct {
	Title      string `json:"title,omitempty"`
	DefaultDir string `json:"default_dir,omitempty"`
}

// DirectoryChoiceDTO is the state of a pending folder choice.
type DirectoryChoiceDTO struct {
	Status string `json:"status"`
	Path   string `json:"path,omitempty"`
}

// FileFilterDTO defines an allowed file filter in file pickers.
type FileFilterDTO struct {
	DisplayName string `json:"display_name"`
	Pattern     string `json:"pattern"`
}

// ChooseSaveFileRequestDTO asks the desktop window for a save file destination.
type ChooseSaveFileRequestDTO struct {
	Title           string          `json:"title,omitempty"`
	DefaultDir      string          `json:"default_dir,omitempty"`
	DefaultFilename string          `json:"default_filename,omitempty"`
	Filters         []FileFilterDTO `json:"filters,omitempty"`
}

// SaveFileChoiceDTO is the state of a pending save file choice.
type SaveFileChoiceDTO struct {
	Status string `json:"status"`
	Path   string `json:"path,omitempty"`
}

type saveFileChoice struct {
	mu     sync.Mutex
	status string
	path   string
}

// directoryChoice is the one native folder dialog that may be open. The dialog
// call blocks, so it runs in the background and the UI polls for the result.
// Holding the webview's own request open across a modal is what froze the app:
// the response the webview is waiting on cannot be written until the dialog
// closes, so nothing repaints while it is up.
type directoryChoice struct {
	mu     sync.Mutex
	status string
	path   string
}

// SetDirectoryPicker installs a native directory chooser, used only by the Wails
// desktop window. Without one the UI falls back to a path field.
func (s *Service) SetDirectoryPicker(picker func(title, defaultDir string) (string, error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.directoryPicker = picker
}

// hasDirectoryPicker reports whether a native chooser is installed.
func (s *Service) hasDirectoryPicker() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.directoryPicker != nil
}

// StartDirectoryChoice opens the picker in the background and returns at once.
// The caller polls DirectoryChoice for the result.
func (s *Service) StartDirectoryChoice(req ChooseDirectoryRequestDTO) error {
	s.mu.RLock()
	picker := s.directoryPicker
	s.mu.RUnlock()
	if picker == nil {
		return ErrNoNativeDialog
	}

	s.directoryChoice.mu.Lock()
	if s.directoryChoice.status == DirectoryChoicePending {
		s.directoryChoice.mu.Unlock()
		return ErrDirectoryChoiceInFlight
	}
	s.directoryChoice.status = DirectoryChoicePending
	s.directoryChoice.path = ""
	s.directoryChoice.mu.Unlock()

	title := req.Title
	if title == "" {
		title = "Choose a folder"
	}
	start := req.DefaultDir
	if start == "" {
		start = s.defaultFolderDir()
	}

	go func() {
		// A dialog that reports an error is a cancellation, not a failure: the
		// user dismissed it, or the window is going away.
		chosen, err := picker(title, start)
		status := DirectoryChoiceSelected
		if err != nil || chosen == "" {
			status = DirectoryChoiceCancelled
		}
		s.directoryChoice.mu.Lock()
		defer s.directoryChoice.mu.Unlock()
		s.directoryChoice.status = status
		s.directoryChoice.path = chosen
	}()
	return nil
}

// DirectoryChoice reports the current state. A finished state is delivered once
// and then cleared, so a poll that arrives late does not replay an old choice.
func (s *Service) DirectoryChoice() DirectoryChoiceDTO {
	s.directoryChoice.mu.Lock()
	defer s.directoryChoice.mu.Unlock()

	dto := DirectoryChoiceDTO{Status: s.directoryChoice.status, Path: s.directoryChoice.path}
	switch s.directoryChoice.status {
	case DirectoryChoiceSelected, DirectoryChoiceCancelled:
		s.directoryChoice.status = DirectoryChoiceIdle
		s.directoryChoice.path = ""
	case "":
		dto.Status = DirectoryChoiceIdle
	}
	return dto
}

// defaultFolderDir is where a picker opens when the caller names nowhere: the
// XDG Documents folder, then Videos, then the home directory. Only existing
// directories are offered.
func (s *Service) defaultFolderDir() string {
	for _, candidate := range []string{xdg.UserDirs.Documents, xdg.UserDirs.Videos} {
		if candidate == "" {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return ""
}

// defaultExportDir is where an export picker should open. A replay lands in the
// videos folder when one exists, so that is offered first.
func (s *Service) defaultExportDir() string {
	if candidate := xdg.UserDirs.Videos; candidate != "" {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	return s.defaultFolderDir()
}

// handleDialogRoutes serves the native-dialog endpoints.
func (s *Server) handleDialogRoutes(w http.ResponseWriter, r *http.Request) {
	action := r.URL.Path[len("/api/dialog/"):]
	switch action {
	case "directory":
		switch r.Method {
		case http.MethodPost:
			s.handleStartDirectoryChoice(w, r)
		case http.MethodGet:
			writeJSON(w, s.service.DirectoryChoice())
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	case "save-file":
		switch r.Method {
		case http.MethodPost:
			s.handleStartSaveFileChoice(w, r)
		case http.MethodGet:
			writeJSON(w, s.service.SaveFileChoice())
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	default:
		http.NotFound(w, r)
	}
}

// handleStartDirectoryChoice opens the picker and answers immediately, so the
// webview is never left waiting on a modal dialog.
func (s *Server) handleStartDirectoryChoice(w http.ResponseWriter, r *http.Request) {
	req := ChooseDirectoryRequestDTO{}
	if r.Body != nil {
		// A body is optional: the picker has sensible defaults.
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnBody)).Decode(&req)
	}

	switch err := s.service.StartDirectoryChoice(req); {
	case errors.Is(err, ErrNoNativeDialog):
		http.Error(w, err.Error(), http.StatusNotImplemented)
	case errors.Is(err, ErrDirectoryChoiceInFlight):
		http.Error(w, err.Error(), http.StatusConflict)
	case err != nil:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	default:
		writeJSONStatus(w, http.StatusAccepted, DirectoryChoiceDTO{Status: DirectoryChoicePending})
	}
}

// handleStartSaveFileChoice opens the save picker and answers immediately.
func (s *Server) handleStartSaveFileChoice(w http.ResponseWriter, r *http.Request) {
	req := ChooseSaveFileRequestDTO{}
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnBody)).Decode(&req)
	}

	switch err := s.service.StartSaveFileChoice(req); {
	case errors.Is(err, ErrNoNativeSaveFileDialog):
		http.Error(w, err.Error(), http.StatusNotImplemented)
	case errors.Is(err, ErrSaveFileChoiceInFlight):
		http.Error(w, err.Error(), http.StatusConflict)
	case err != nil:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	default:
		writeJSONStatus(w, http.StatusAccepted, SaveFileChoiceDTO{Status: SaveFileChoicePending})
	}
}

// WaitForDirectoryChoice blocks until the pending choice finishes or the context
// is done. It exists for tests and for a caller that would rather wait than poll.
func (s *Service) WaitForDirectoryChoice(ctx context.Context) (DirectoryChoiceDTO, error) {
	for {
		if choice := s.DirectoryChoice(); choice.Status != DirectoryChoicePending {
			return choice, nil
		}
		select {
		case <-ctx.Done():
			return DirectoryChoiceDTO{Status: DirectoryChoicePending}, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// SetSaveFilePicker installs a native save file chooser, used only by the Wails
// desktop window. Without one the UI falls back to direct browser download.
func (s *Service) SetSaveFilePicker(picker func(req ChooseSaveFileRequestDTO) (string, error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saveFilePicker = picker
}

// hasSaveFilePicker reports whether a native save file chooser is installed.
func (s *Service) hasSaveFilePicker() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.saveFilePicker != nil
}

// StartSaveFileChoice opens the save picker in the background and returns at once.
// The caller polls SaveFileChoice for the result.
func (s *Service) StartSaveFileChoice(req ChooseSaveFileRequestDTO) error {
	s.mu.RLock()
	picker := s.saveFilePicker
	s.mu.RUnlock()
	if picker == nil {
		return ErrNoNativeSaveFileDialog
	}

	s.saveFileChoice.mu.Lock()
	if s.saveFileChoice.status == SaveFileChoicePending {
		s.saveFileChoice.mu.Unlock()
		return ErrSaveFileChoiceInFlight
	}
	s.saveFileChoice.status = SaveFileChoicePending
	s.saveFileChoice.path = ""
	s.saveFileChoice.mu.Unlock()

	title := req.Title
	if title == "" {
		title = "Save file"
	}
	start := req.DefaultDir
	if start == "" {
		start = s.defaultFolderDir()
	}
	req.Title = title
	req.DefaultDir = start

	go func() {
		chosen, err := picker(req)
		status := SaveFileChoiceSelected
		if err != nil || chosen == "" {
			status = SaveFileChoiceCancelled
		}
		s.saveFileChoice.mu.Lock()
		defer s.saveFileChoice.mu.Unlock()
		s.saveFileChoice.status = status
		s.saveFileChoice.path = chosen
	}()
	return nil
}

// SaveFileChoice reports the current state. A finished state is delivered once
// and then cleared, so a poll that arrives late does not replay an old choice.
func (s *Service) SaveFileChoice() SaveFileChoiceDTO {
	s.saveFileChoice.mu.Lock()
	defer s.saveFileChoice.mu.Unlock()

	dto := SaveFileChoiceDTO{Status: s.saveFileChoice.status, Path: s.saveFileChoice.path}
	switch s.saveFileChoice.status {
	case SaveFileChoiceSelected, SaveFileChoiceCancelled:
		s.saveFileChoice.status = SaveFileChoiceIdle
		s.saveFileChoice.path = ""
	case "":
		dto.Status = SaveFileChoiceIdle
	}
	return dto
}

// WaitForSaveFileChoice blocks until the pending choice finishes or the context
// is done. It exists for tests and for a caller that would rather wait than poll.
func (s *Service) WaitForSaveFileChoice(ctx context.Context) (SaveFileChoiceDTO, error) {
	for {
		if choice := s.SaveFileChoice(); choice.Status != SaveFileChoicePending {
			return choice, nil
		}
		select {
		case <-ctx.Done():
			return SaveFileChoiceDTO{Status: SaveFileChoicePending}, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}
