package gui

import (
	"io"
	"net/http"
	"path/filepath"
)

// The asset routes under /api/game/{id}: the campaign's banner and icon, the
// generated artwork, and the folder tree the notes live in.

// handleGameAssetRoute serves GET, POST and DELETE for /api/game/{id}/banner and
// /api/game/{id}/icon. GET reads the stored artwork, POST replaces it with an
// upload, and DELETE clears the campaign's own copy.
func (s *Server) handleGameAssetRoute(w http.ResponseWriter, r *http.Request, gameID string, parts []string) {
	action := parts[1]
	if r.Method == http.MethodGet {
		filePath, contentType, err := s.service.GetGameAsset(gameID, action)
		if err != nil {
			s.writeAPIError(w, err)
			return
		}
		w.Header().Set("Content-Type", contentType)
		http.ServeFile(w, r, filePath)
		return
	} else if r.Method == http.MethodPost {
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "missing file in form data", http.StatusBadRequest)
			return
		}
		defer file.Close()
		data, err := io.ReadAll(http.MaxBytesReader(w, file, 15*1024*1024))
		if err != nil {
			http.Error(w, "file too large or read failed", http.StatusBadRequest)
			return
		}
		ext := filepath.Ext(header.Filename)
		url, err := s.service.SaveGameAsset(gameID, action, data, ext)
		if err != nil {
			s.writeAPIError(w, err)
			return
		}
		writeJSON(w, map[string]string{"url": url})
		return
	} else if r.Method == http.MethodDelete {
		// Clearing the campaign's own artwork is how the world's is restored.
		if err := s.service.DeleteGameAsset(gameID, action); err != nil {
			s.writeAPIError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

// handleGameGenerateAssetRoute answers POST /api/game/{id}/generate-asset: an
// illustration generated from a prompt for this campaign.
func (s *Server) handleGameGenerateAssetRoute(w http.ResponseWriter, r *http.Request, gameID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req GenerateAssetRequestDTO
	if !decodeJSONBody(w, r, 1024*1024, &req) {
		return
	}
	url, err := s.service.GenerateGameAsset(r.Context(), gameID, req)
	if err != nil {
		if writeGenerationFailure(w, err) {
			return
		}
		s.writeAPIError(w, err)
		return
	}
	writeJSON(w, map[string]string{"url": url})
}

// handleGameFoldersRoute answers the folder branch of /api/game/{id}/folders,
// reading and writing the campaign's own entity folder tree.
func (s *Server) handleGameFoldersRoute(w http.ResponseWriter, r *http.Request, gameID string) {
	s.handleFolderRoutes(w, r, filepath.Join(s.service.resolver.GameDir(gameID), "entities"))
}
