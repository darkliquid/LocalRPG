package gui

import (
	"encoding/json"
	"net/http"
	"strings"
)

// streamSystemGeneration runs a system generation and streams its progress as NDJSON,
// one event per line: steps and final draft. A client that disconnects cancels the generation.
func (s *Server) streamSystemGeneration(w http.ResponseWriter, r *http.Request, req SystemGenerateRequestDTO) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported by this client", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	encoder := json.NewEncoder(w)
	emit := func(event TurnEvent) error {
		if err := encoder.Encode(event); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	if _, err := s.service.GenerateSystem(r.Context(), req, emit); err != nil {
		event := TurnEvent{Type: "error", Message: err.Error(), Code: generationLimitCode(err)}
		_ = emit(event)
	}
}

// handleSystemGenerateRoute streams a system generation.
func (s *Server) handleSystemGenerateRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req SystemGenerateRequestDTO
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnBody)).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	s.streamSystemGeneration(w, r, req)
}

// handleSystemDraftCommitRoute commits a system draft into systems/<id>/.
func (s *Server) handleSystemDraftCommitRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req SystemDraftCommitRequestDTO
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnBody)).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	detail, err := s.service.CommitSystemDraft(r.Context(), req)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSONStatus(w, http.StatusCreated, detail)
}

// handleSystemDraftDiscardRoute deletes a system draft.
func (s *Server) handleSystemDraftDiscardRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req SystemDraftDiscardRequestDTO
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnBody)).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := s.service.DiscardSystemDraft(r.Context(), req.DraftID); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleSystemDraftRoutes serves persisted system drafts.
func (s *Server) handleSystemDraftRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/system/draft/"), "/")
	if id == "" {
		ids, err := s.service.ListSystemDraftIDs(r.Context())
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, ids)
		return
	}
	draft, err := s.service.GetSystemDraft(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, draft)
}

// handleSystemEnhance returns additive proposals for an existing system.
func (s *Server) handleSystemEnhance(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req SystemEnhanceRequestDTO
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnBody)).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	resp, err := s.service.EnhanceSystem(r.Context(), id, req)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, resp)
}

// handleSystemEnhanceApply writes the accepted proposals.
func (s *Server) handleSystemEnhanceApply(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req SystemEnhanceApplyRequestDTO
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnBody)).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	result, err := s.service.ApplySystemEnhancements(r.Context(), id, req)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, result)
}

// handleSystemExplain returns a plain-language description of a system.
func (s *Server) handleSystemExplain(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	resp, err := s.service.ExplainSystem(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, resp)
}
