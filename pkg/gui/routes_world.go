package gui

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/darkliquid/localrpg/pkg/pathutil"
)

// The world routes under /api/world/: the world itself, one of its entity notes,
// its folder tree, its banner and icon, and the AI generation steps that never
// write without an accept.

// handleWorldRoutes serves every URL under /api/world/. It parses the path once
// and hands each resource to the handler named for it; the generation steps live
// in worldgen_routes.go beside it.
func (s *Server) handleWorldRoutes(w http.ResponseWriter, r *http.Request) {
	worldID, parts, ok := worldRoutePath(w, r)
	if !ok {
		return
	}

	// Each arm restates the segment requirement its resource needs, so a path
	// that names none of them falls through to the world root, which is what the
	// guard chain did before this was a switch.
	switch {
	case len(parts) >= 3 && parts[1] == "entity":
		s.handleWorldEntityRoute(w, r, worldID, parts)
	case len(parts) >= 2 && parts[1] == "folders":
		s.handleWorldFoldersRoute(w, r, worldID)
	case len(parts) >= 2 && parts[1] == "generate-entities":
		s.handleWorldEntitiesPreview(w, r, worldID)
	case len(parts) >= 3 && parts[1] == "entities" && parts[2] == "accept":
		s.handleWorldEntitiesAccept(w, r, worldID)
	case len(parts) >= 2 && parts[1] == "enhance":
		s.handleWorldEnhanceRoute(w, r, worldID, parts)
	case len(parts) >= 2 && (parts[1] == "banner" || parts[1] == "icon"):
		s.handleWorldAssetRoute(w, r, worldID, parts)
	case len(parts) >= 2 && parts[1] == "generate-asset":
		s.handleWorldGenerateAssetRoute(w, r, worldID)
	default:
		s.handleWorldRootRoute(w, r, worldID)
	}
}

// worldRoutePath splits a /api/world/ path into the world id and the remaining
// segments, validating the id. It reports whether the caller should continue; a
// malformed path has already been answered.
func worldRoutePath(w http.ResponseWriter, r *http.Request) (string, []string, bool) {
	path := strings.TrimPrefix(r.URL.Path, "/api/world/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "missing world id", http.StatusBadRequest)
		return "", nil, false
	}
	if err := pathutil.ValidateID(parts[0]); err != nil {
		http.Error(w, "invalid world id", http.StatusBadRequest)
		return "", nil, false
	}
	return parts[0], parts, true
}

// handleWorldEntityRoute serves one world note by id: reading it, saving its
// markdown into a folder, and deleting it.
func (s *Server) handleWorldEntityRoute(w http.ResponseWriter, r *http.Request, worldID string, parts []string) {
	entityID := parts[2]
	if err := pathutil.ValidateID(entityID); err != nil {
		http.Error(w, "invalid entity id", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodGet:
		ent, err := s.service.GetWorldEntity(r.Context(), worldID, entityID)
		if err != nil {
			s.writeAPIError(w, err)
			return
		}
		writeJSON(w, ent)
	case http.MethodPut:
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read body failed", http.StatusBadRequest)
			return
		}
		content := string(data)
		var obj struct {
			Markdown string `json:"markdown"`
			Folder   string `json:"folder"`
		}
		if err := json.Unmarshal(data, &obj); err == nil && obj.Markdown != "" {
			content = obj.Markdown
		}
		if err := s.service.SaveWorldEntity(r.Context(), worldID, entityID, obj.Folder, content); err != nil {
			s.writeAPIError(w, err)
			return
		}
		w.WriteHeader(http.StatusOK)
	case http.MethodDelete:
		if err := s.service.DeleteWorldEntity(r.Context(), worldID, entityID); err != nil {
			s.writeAPIError(w, err)
			return
		}
		w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleWorldFoldersRoute serves the folder tree over a world's entity notes.
func (s *Server) handleWorldFoldersRoute(w http.ResponseWriter, r *http.Request, worldID string) {
	s.handleFolderRoutes(w, r, filepath.Join(s.service.resolver.WorldDir(worldID), "entities"))
}

// handleWorldEnhanceRoute serves a world's enhancement proposals, and the apply
// step that accepts one.
func (s *Server) handleWorldEnhanceRoute(w http.ResponseWriter, r *http.Request, worldID string, parts []string) {
	if len(parts) >= 3 && parts[2] == "apply" {
		s.handleWorldEnhanceApply(w, r, worldID)
		return
	}
	s.handleWorldEnhance(w, r, worldID)
}

// handleWorldAssetRoute serves a world's banner or icon: the stored file on GET,
// and an upload on POST.
func (s *Server) handleWorldAssetRoute(w http.ResponseWriter, r *http.Request, worldID string, parts []string) {
	action := parts[1]
	if r.Method == http.MethodGet {
		filePath, contentType, err := s.service.GetWorldAsset(worldID, action)
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
		url, err := s.service.SaveWorldAsset(worldID, action, data, ext)
		if err != nil {
			s.writeAPIError(w, err)
			return
		}
		writeJSON(w, map[string]string{"url": url})
		return
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

// handleWorldGenerateAssetRoute generates a world's banner or icon through the
// configured image provider.
func (s *Server) handleWorldGenerateAssetRoute(w http.ResponseWriter, r *http.Request, worldID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req GenerateAssetRequestDTO
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024*1024)).Decode(&req); err != nil {
		writeInvalidRequest(w, "invalid request body")
		return
	}
	url, err := s.service.GenerateWorldAsset(r.Context(), worldID, req)
	if err != nil {
		if writeGenerationFailure(w, err) {
			return
		}
		s.writeAPIError(w, err)
		return
	}
	writeJSON(w, map[string]string{"url": url})
}

// handleWorldRootRoute serves /api/world/:worldID itself: reading a world,
// updating it, and deleting it, optionally by force.
func (s *Server) handleWorldRootRoute(w http.ResponseWriter, r *http.Request, worldID string) {
	switch r.Method {
	case http.MethodGet:
		world, err := s.service.GetWorld(r.Context(), worldID)
		if err != nil {
			s.writeAPIError(w, err)
			return
		}
		writeJSON(w, world)
	case http.MethodPut:
		var req CreateWorldRequestDTO
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		req.ID = worldID
		world, err := s.service.UpdateWorld(r.Context(), req)
		if errors.Is(err, ErrWorldNotFound) {
			s.writeAPIError(w, err)
			return
		}
		if err != nil {
			s.writeAPIError(w, invalid(err))
			return
		}
		writeJSON(w, world)
	case http.MethodDelete:
		force := r.URL.Query().Get("force") == "true"
		err := s.service.DeleteWorld(r.Context(), worldID, force)
		if errors.Is(err, ErrWorldNotFound) {
			s.writeAPIError(w, err)
			return
		}
		if errors.Is(err, ErrWorldInUse) {
			s.writeAPIError(w, err)
			return
		}
		if err != nil {
			s.writeAPIError(w, invalid(err))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
