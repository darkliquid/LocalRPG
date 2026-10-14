package gui

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/darkliquid/localrpg/pkg/pathutil"
)

// handleGameRoutes serves every URL under /api/game/. It parses the path once and
// hands each resource to the handler named for it; the bodies live in the
// routes_game_* files beside this one.
func (s *Server) handleGameRoutes(w http.ResponseWriter, r *http.Request) {
	gameID, parts, ok := gameRoutePath(w, r)
	if !ok {
		return
	}

	// DELETE /api/game/{id} has no sub-action, so it is matched before the
	// two-part requirement every other game route satisfies.
	if len(parts) == 1 && r.Method == http.MethodDelete {
		if err := s.service.DeleteGame(r.Context(), gameID); err != nil {
			s.writeAPIError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if len(parts) < 2 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	switch parts[1] {
	case "audio":
		s.handleGameAudioRoute(w, r, gameID, parts)
	case "folders":
		s.handleGameFoldersRoute(w, r, gameID)
	case "banner", "icon":
		s.handleGameAssetRoute(w, r, gameID, parts)
	case "generate-asset":
		s.handleGameGenerateAssetRoute(w, r, gameID)
	case "tts":
		s.handleGameTTSRoute(w, r, gameID, parts)
	case "restart":
		s.handleGameRestartRoute(w, r, gameID)
	case "settings":
		s.handleGameSettingsRoute(w, r, gameID)
	case "state":
		s.handleGameStateRoute(w, r, gameID)
	case "context":
		s.handleGameContextRoute(w, r, gameID)
	case "working-set":
		s.handleGameWorkingSetRoute(w, r, gameID)
	case "entities":
		s.handleGameEntitiesRoute(w, r, gameID)
	case "recap":
		s.handleGameRecapRoute(w, r, gameID)
	case "findings":
		s.handleGameFindingsRoute(w, r, gameID)
	case "graph":
		s.handleGameGraphRoute(w, r, gameID)
	case "chronicle":
		s.handleGameChronicleRoute(w, r, gameID)
	case "usage":
		s.handleGameUsageRoute(w, r, gameID)
	case "advance":
		s.handleGameAdvanceRoute(w, r, gameID)
	case "turn":
		s.handleGameTurnRoute(w, r, gameID, parts)
	case "location":
		s.handleGameLocationRoute(w, r, gameID, parts)
	case "character":
		s.handleGameCharacterRoute(w, r, gameID, parts)
	case "entity":
		s.handleGameEntityRoute(w, r, gameID, parts)
	default:
		http.NotFound(w, r)
	}
}

// gameRoutePath splits a /api/game/ path into the campaign id and the remaining
// segments, validating the id. It reports whether the caller should continue; a
// malformed path has already been answered.
func gameRoutePath(w http.ResponseWriter, r *http.Request) (string, []string, bool) {
	path := strings.TrimPrefix(r.URL.Path, "/api/game/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "missing game id", http.StatusBadRequest)
		return "", nil, false
	}
	if err := pathutil.ValidateID(parts[0]); err != nil {
		http.Error(w, "invalid game id", http.StatusBadRequest)
		return "", nil, false
	}
	return parts[0], parts, true
}

// decodeJSONBody decodes a JSON request body under a byte cap. It reports whether
// the caller should continue; a malformed body has already been answered with the
// shared invalid-request envelope.
func decodeJSONBody(w http.ResponseWriter, r *http.Request, limit int64, dst any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit)).Decode(dst); err != nil {
		writeInvalidRequest(w, "invalid request body")
		return false
	}
	return true
}

// turnNumberFromPath reads the turn number at parts[2] of a turn route, answering
// 400 when it is not a number. It reports whether the caller should continue.
func turnNumberFromPath(w http.ResponseWriter, parts []string) (int, bool) {
	n, err := strconv.Atoi(parts[2])
	if err != nil {
		http.Error(w, "invalid turn number", http.StatusBadRequest)
		return 0, false
	}
	return n, true
}

// segmentIndexFromPath reads the segment index at parts[4] of a segment route,
// answering 400 when it is not a number. It reports whether the caller should
// continue.
func segmentIndexFromPath(w http.ResponseWriter, parts []string) (int, bool) {
	n, err := strconv.Atoi(parts[4])
	if err != nil {
		http.Error(w, "invalid segment index", http.StatusBadRequest)
		return 0, false
	}
	return n, true
}
