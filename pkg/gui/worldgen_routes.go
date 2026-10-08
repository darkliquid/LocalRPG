package gui

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// streamWorldGeneration runs a generation and streams its progress as NDJSON,
// one event per line: a step per pipeline stage and a final draft. A client that
// disconnects cancels the generation, and nothing is persisted for it.
func (s *Server) streamWorldGeneration(w http.ResponseWriter, r *http.Request, req WorldGenerateRequestDTO) {
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

	if _, err := s.service.GenerateWorld(r.Context(), req, emit); err != nil {
		event := TurnEvent{Type: "error", Message: err.Error()}
		if errors.Is(err, ErrWorldExists) || errors.Is(err, ErrEmptyDraftSelection) {
			event.Code = "worldgen"
		}
		_ = emit(event)
	}
}

// handleWorldGenerateRoute streams a whole-world generation.
func (s *Server) handleWorldGenerateRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req WorldGenerateRequestDTO
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnBody)).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	s.streamWorldGeneration(w, r, req)
}

// handleWorldIngestRoute streams a generation built from a folder or URLs.
func (s *Server) handleWorldIngestRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req WorldGenerateRequestDTO
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnBody)).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Source == nil || strings.TrimSpace(req.Source.Kind) == "" {
		http.Error(w, "a source is required", http.StatusBadRequest)
		return
	}
	s.streamWorldGeneration(w, r, req)
}

// handleWorldDraftCommitRoute commits the accepted set from a draft.
func (s *Server) handleWorldDraftCommitRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req DraftCommitRequestDTO
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnBody)).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	world, err := s.service.CommitDraft(r.Context(), req)
	switch {
	case errors.Is(err, ErrWorldExists):
		writeJSONError(w, http.StatusConflict, err.Error())
		return
	case errors.Is(err, ErrEmptyDraftSelection), errors.Is(err, ErrWorldEntityExists):
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	case err != nil:
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSONStatus(w, http.StatusCreated, world)
}

// handleWorldDraftDiscardRoute deletes a draft.
func (s *Server) handleWorldDraftDiscardRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req DraftDiscardRequestDTO
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnBody)).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := s.service.DiscardDraft(r.Context(), req.DraftID); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleWorldDraftRoutes serves a persisted draft, so a reload resumes review.
func (s *Server) handleWorldDraftRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/world/draft/"), "/")
	if id == "" {
		ids, err := s.service.ListDraftIDs(r.Context())
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, ids)
		return
	}
	draft, err := s.service.GetDraft(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, draft)
}

// handleWorldEntitiesPreview previews a batch of generated entities.
func (s *Server) handleWorldEntitiesPreview(w http.ResponseWriter, r *http.Request, worldID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req WorldEntityBatchRequestDTO
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnBody)).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	batch, err := s.service.PreviewWorldEntities(r.Context(), worldID, req)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, batch)
}

// handleWorldEntitiesAccept writes a reviewed batch.
func (s *Server) handleWorldEntitiesAccept(w http.ResponseWriter, r *http.Request, worldID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req WorldEntityAcceptRequestDTO
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnBody)).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	result, err := s.service.AcceptWorldEntities(r.Context(), worldID, req)
	switch {
	case errors.Is(err, ErrWorldEntityExists):
		writeJSONError(w, http.StatusConflict, err.Error())
		return
	case err != nil:
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, result)
}

// handleWorldEnhance returns enhancement proposals for an existing world.
func (s *Server) handleWorldEnhance(w http.ResponseWriter, r *http.Request, worldID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req WorldEnhanceRequestDTO
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnBody)).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	proposals, err := s.service.EnhanceWorld(r.Context(), worldID, req)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, proposals)
}

// handleWorldEnhanceApply writes the accepted proposals.
func (s *Server) handleWorldEnhanceApply(w http.ResponseWriter, r *http.Request, worldID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req WorldEnhanceApplyRequestDTO
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnBody)).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	result, err := s.service.ApplyWorldEnhancements(r.Context(), worldID, req)
	switch {
	case errors.Is(err, ErrWorldEntityExists):
		writeJSONError(w, http.StatusConflict, err.Error())
		return
	case err != nil:
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, result)
}
