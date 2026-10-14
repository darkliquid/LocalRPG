package gui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/darkliquid/localrpg/pkg/pathutil"
)

// The entity routes under /api/game/{id}: a location's art, a character's
// portrait, and one entity by id.

// handleGameLocationRoute serves GET /api/game/{id}/location/{id}/art.
func (s *Server) handleGameLocationRoute(w http.ResponseWriter, r *http.Request, gameID string, parts []string) {
	if len(parts) < 3 {
		http.Error(w, "missing location id", http.StatusBadRequest)
		return
	}
	locationID := parts[2]
	if len(parts) < 4 || parts[3] != "art" || r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}

	path, contentType, err := s.service.GetLocationArt(r.Context(), gameID, locationID, r.URL.Query().Get("force") == "1")
	if err != nil {
		s.writeAPIError(w, err)
		return
	}
	w.Header().Set("Content-Type", contentType)
	http.ServeFile(w, r, path)
}

// handleGameCharacterRoute serves GET and POST for
// /api/game/{id}/character/{id}/portrait: the portrait bytes, and the request to
// generate a fresh one.
func (s *Server) handleGameCharacterRoute(w http.ResponseWriter, r *http.Request, gameID string, parts []string) {
	if len(parts) < 3 {
		http.Error(w, "missing character id", http.StatusBadRequest)
		return
	}
	characterID := parts[2]
	if err := pathutil.ValidateID(characterID); err != nil {
		http.Error(w, "invalid character id", http.StatusBadRequest)
		return
	}
	if len(parts) >= 4 && parts[3] == "portrait" {
		if r.Method == http.MethodPost {
			dto, err := s.service.RegenerateCharacterPortrait(r.Context(), gameID, characterID)
			if err != nil {
				if writeGenerationFailure(w, err) {
					return
				}
				s.writeAPIError(w, err)
				return
			}
			writeJSON(w, dto)
			return
		}
		if r.Method == http.MethodGet {
			var version []int
			if vStr := r.URL.Query().Get("v"); vStr != "" {
				if v, err := strconv.Atoi(vStr); err == nil && v > 0 {
					version = append(version, v)
				}
			}
			data, contentType, err := s.service.GetCharacterPortrait(r.Context(), gameID, characterID, version...)
			if err != nil {
				s.writeAPIError(w, err)
				return
			}
			w.Header().Set("Content-Type", contentType)
			w.Header().Set("Cache-Control", "no-cache")
			_, _ = w.Write(data)
			return
		}
	}
	http.NotFound(w, r)
}

// handleGameEntityRoute serves one note by id: its turns, its memories, its
// markdown, its art, a merge into another note, and its deletion.
func (s *Server) handleGameEntityRoute(w http.ResponseWriter, r *http.Request, gameID string, parts []string) {
	if len(parts) < 3 {
		http.Error(w, "missing entity id", http.StatusBadRequest)
		return
	}
	entityID := parts[2]
	if err := pathutil.ValidateID(entityID); err != nil {
		http.Error(w, "invalid entity id", http.StatusBadRequest)
		return
	}
	if len(parts) >= 4 && parts[3] == "turns" && r.Method == http.MethodGet {
		turns, err := s.service.GetEntityTurns(r.Context(), gameID, entityID)
		if err != nil {
			s.writeAPIError(w, err)
			return
		}
		writeJSON(w, turns)
		return
	}
	if len(parts) >= 4 && parts[3] == "memories" && r.Method == http.MethodGet {
		limit := 20
		if raw := r.URL.Query().Get("limit"); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil {
				limit = parsed
			}
		}
		memories, err := s.service.ListEntityMemories(gameID, entityID, limit)
		if err != nil {
			s.writeAPIError(w, err)
			return
		}
		writeJSON(w, memories)
		return
	}
	if r.Method == http.MethodPut {
		var body struct {
			Markdown string `json:"markdown"`
			Folder   string `json:"folder"`
		}
		if !decodeJSONBody(w, r, maxTurnBody, &body) {
			return
		}
		if err := s.service.SaveEntityInFolder(r.Context(), gameID, entityID, body.Folder, body.Markdown); err != nil {
			s.writeSaveError(w, err, body.Markdown)
			return
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	if len(parts) >= 4 && parts[3] == "art" && r.Method == http.MethodGet {
		path, contentType, err := s.service.GetLocationArt(r.Context(), gameID, entityID, r.URL.Query().Get("force") == "1")
		if err != nil {
			s.writeAPIError(w, err)
			return
		}
		w.Header().Set("Content-Type", contentType)
		http.ServeFile(w, r, path)
		return
	}

	// POST /api/game/{id}/entity/{source}/merge folds one note into another.
	if r.Method == http.MethodPost && len(parts) >= 4 && parts[3] == "merge" {
		var req MergeEntityRequestDTO
		if !decodeJSONBody(w, r, maxTurnBody, &req) {
			return
		}
		if strings.TrimSpace(req.Into) == "" {
			http.Error(w, "a merge needs a note to merge into", http.StatusBadRequest)
			return
		}
		if err := pathutil.ValidateID(req.Into); err != nil {
			http.Error(w, "invalid target id", http.StatusBadRequest)
			return
		}
		if !req.Confirm {
			http.Error(w, "a merge must be explicitly confirmed", http.StatusBadRequest)
			return
		}

		merged, err := s.service.MergeEntities(r.Context(), gameID, parts[2], req.Into)
		if err != nil {
			s.writeAPIError(w, err)
			return
		}
		writeJSON(w, merged)
		return
	}

	if r.Method == http.MethodDelete {
		if err := s.service.DeleteGameEntity(r.Context(), gameID, entityID); err != nil {
			s.writeAPIError(w, err)
			return
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	ent, err := s.service.GetEntity(r.Context(), gameID, entityID)
	if err != nil {
		s.writeAPIError(w, err)
		return
	}
	writeJSON(w, ent)
}
