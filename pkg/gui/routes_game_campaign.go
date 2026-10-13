package gui

import (
	"net/http"
	"strings"

	"github.com/darkliquid/localrpg/pkg/engine"
)

// The campaign routes under /api/game/{id}: the ones that read or write the
// campaign as a whole rather than one entity within it.

// handleGameStateRoute answers GET /api/game/{id}/state.
func (s *Server) handleGameStateRoute(w http.ResponseWriter, r *http.Request, gameID string) {
	state, err := s.service.GetGameState(r.Context(), gameID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, state)
}

// handleGameContextRoute answers GET /api/game/{id}/context.
func (s *Server) handleGameContextRoute(w http.ResponseWriter, r *http.Request, gameID string) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	turnCtx, err := s.service.GetTurnContext(gameID)
	if err != nil {
		writeGameError(w, err)
		return
	}
	writeJSON(w, turnCtx)
}

// handleGameWorkingSetRoute answers GET /api/game/{id}/working-set.
func (s *Server) handleGameWorkingSetRoute(w http.ResponseWriter, r *http.Request, gameID string) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	workingSet, err := s.service.GetWorkingSet(gameID)
	if err != nil {
		writeGameError(w, err)
		return
	}
	writeJSON(w, workingSet)
}

// handleGameEntitiesRoute answers GET /api/game/{id}/entities.
func (s *Server) handleGameEntitiesRoute(w http.ResponseWriter, r *http.Request, gameID string) {
	entities, err := s.service.ListEntities(r.Context(), gameID)
	if err != nil {
		writeGameError(w, err)
		return
	}
	writeJSON(w, entities)
}

// handleGameRecapRoute answers GET /api/game/{id}/recap.
func (s *Server) handleGameRecapRoute(w http.ResponseWriter, r *http.Request, gameID string) {
	recap, err := s.service.GetRecap(r.Context(), gameID)
	if err != nil {
		writeGameError(w, err)
		return
	}
	writeJSON(w, recap)
}

// handleGameFindingsRoute answers GET and POST /api/game/{id}/findings: the
// rules the turn broke, and the acknowledgement of one.
func (s *Server) handleGameFindingsRoute(w http.ResponseWriter, r *http.Request, gameID string) {
	switch r.Method {
	case http.MethodGet:
		found, err := s.service.Findings(gameID)
		if err != nil {
			writeGameError(w, err)
			return
		}
		writeJSON(w, found)

	case http.MethodPost:
		var req AddressedFinding
		if !decodeJSONBody(w, r, maxTurnBody, &req) {
			return
		}
		if err := s.service.AddressFinding(gameID, req.Turn, req.Rule); err != nil {
			writeGameError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.NotFound(w, r)
	}
}

// handleGameGraphRoute answers GET /api/game/{id}/graph.
func (s *Server) handleGameGraphRoute(w http.ResponseWriter, r *http.Request, gameID string) {
	graph, err := s.service.GetGraph(r.Context(), gameID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, graph)
}

// handleGameChronicleRoute answers GET /api/game/{id}/chronicle.
func (s *Server) handleGameChronicleRoute(w http.ResponseWriter, r *http.Request, gameID string) {
	chronicle, err := s.service.GetChronicle(r.Context(), gameID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, chronicle)
}

// handleGameUsageRoute answers GET /api/game/{id}/usage.
func (s *Server) handleGameUsageRoute(w http.ResponseWriter, r *http.Request, gameID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	dto, err := s.service.GameUsage(r.Context(), gameID)
	if err != nil {
		writeGameError(w, err)
		return
	}
	writeJSON(w, dto)
}

// handleGameRestartRoute answers POST /api/game/{id}/restart.
func (s *Server) handleGameRestartRoute(w http.ResponseWriter, r *http.Request, gameID string) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	game, err := s.service.RestartGame(r.Context(), gameID)
	if err != nil {
		writeGameError(w, err)
		return
	}
	writeJSON(w, game)
}

// handleGameSettingsRoute answers PATCH /api/game/{id}/settings. Only the fields
// the client sent are written, so an absent field keeps its stored value.
func (s *Server) handleGameSettingsRoute(w http.ResponseWriter, r *http.Request, gameID string) {
	if r.Method != http.MethodPatch {
		http.NotFound(w, r)
		return
	}
	var patch GameSettingsPatchDTO
	if !decodeJSONBody(w, r, maxTurnBody, &patch) {
		return
	}
	values := map[string]any{}
	if patch.OpeningPrompt != nil {
		values[engine.OpeningPromptSetting] = strings.TrimSpace(*patch.OpeningPrompt)
	}
	if patch.NarratorVoice != nil {
		values["narrator_voice"] = strings.TrimSpace(*patch.NarratorVoice)
	}
	if patch.StartLocation != nil {
		values[engine.StartLocationSetting] = strings.TrimSpace(*patch.StartLocation)
	}
	if err := s.service.UpdateGameSettings(r.Context(), gameID, values); err != nil {
		writeGameError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleGameAdvanceRoute answers POST /api/game/{id}/advance: the unlock the
// player has just earned.
func (s *Server) handleGameAdvanceRoute(w http.ResponseWriter, r *http.Request, gameID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		UnlockID string `json:"unlock_id"`
	}
	if !decodeJSONBody(w, r, 64*1024, &req) {
		return
	}
	advancement, err := s.service.AdvanceUnlock(r.Context(), gameID, req.UnlockID)
	if err != nil {
		writeGameError(w, err)
		return
	}
	writeJSON(w, map[string]any{"advancement": advancement})
}
