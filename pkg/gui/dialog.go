package gui

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"

	"github.com/adrg/xdg"
)

// ErrNoNativeDialog reports that no native directory picker is available, so the
// UI must fall back to a path field.
var ErrNoNativeDialog = errors.New("native directory dialog is not available")

// ChooseDirectoryRequestDTO asks the desktop window for a directory. Title is
// what the picker shows, so one endpoint serves an export destination and a
// source folder without the picker guessing.
type ChooseDirectoryRequestDTO struct {
	Title      string `json:"title,omitempty"`
	DefaultDir string `json:"default_dir,omitempty"`
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

// ChooseDirectory opens the native picker, or reports that none is available so
// the UI can fall back to a text field. An empty path with no error means the
// user cancelled.
func (s *Service) ChooseDirectory(ctx context.Context, req ChooseDirectoryRequestDTO) (string, error) {
	s.mu.RLock()
	picker := s.directoryPicker
	s.mu.RUnlock()
	if picker == nil {
		return "", ErrNoNativeDialog
	}
	title := req.Title
	if title == "" {
		title = "Choose a folder"
	}
	start := req.DefaultDir
	if start == "" {
		start = s.defaultFolderDir()
	}
	return picker(title, start)
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
	if action != "directory" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	req := ChooseDirectoryRequestDTO{}
	if r.Body != nil {
		// A body is optional: the picker has sensible defaults.
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnBody)).Decode(&req)
	}

	chosen, err := s.service.ChooseDirectory(r.Context(), req)
	switch {
	case errors.Is(err, ErrNoNativeDialog):
		http.Error(w, err.Error(), http.StatusNotImplemented)
		return
	case err != nil:
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"path": chosen})
}
