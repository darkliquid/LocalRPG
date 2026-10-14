package gui

import (
	"errors"
	"net/http"
)

// The turn routes under /api/game/{id}/turn: submitting a turn, resolving or
// renegotiating its pending check, its playback ledger, its scene image, and the
// audio for the turn and for one segment of it.

// handleGameTurnRoute dispatches everything that hangs off a turn. A turn's own
// routes are four segments long, with the turn number at parts[2]; the segment
// routes are longer and name the beat at parts[4].
func (s *Server) handleGameTurnRoute(w http.ResponseWriter, r *http.Request, gameID string, parts []string) {
	// POST /api/game/{id}/turn submits a turn.
	if r.Method == http.MethodPost && len(parts) == 2 {
		s.handleTurnSubmit(w, r, gameID)
		return
	}

	if len(parts) == 4 {
		s.handleTurnActionRoute(w, r, gameID, parts)
		return
	}

	// POST /api/game/{id}/turn/{n}/segment/{i}/play plays one beat.
	if r.Method == http.MethodPost && len(parts) == 6 && parts[3] == "segment" && parts[5] == "play" {
		s.handleTurnSegmentPlay(w, r, gameID, parts)
		return
	}

	// POST /api/game/{id}/turn/{n}/segment/{i}/audio re-synthesizes one beat
	// and answers with its refreshed clip URLs. Playback itself reads clips by
	// content key, so this is only what a regenerate control needs.
	if r.Method == http.MethodPost && len(parts) >= 6 && parts[3] == "segment" && parts[5] == "audio" {
		s.handleTurnSegmentAudio(w, r, gameID, parts)
		return
	}

	http.NotFound(w, r)
}

// handleTurnActionRoute serves the routes that hang off one turn: resolving or
// renegotiating its pending check, its playback ledger, its scene image, and
// playing it.
//
// An arm whose method does not match falls out of the switch to the not-found,
// which is what the guard chain this replaces produced for those shapes.
func (s *Server) handleTurnActionRoute(w http.ResponseWriter, r *http.Request, gameID string, parts []string) {
	switch parts[3] {
	case "resolve-check":
		// POST /api/game/{id}/turn/{n}/resolve-check resolves the turn's pending
		// check without a fresh player action.
		if r.Method == http.MethodPost {
			s.handleTurnResolveCheckRoute(w, r, gameID, parts)
			return
		}
	case "renegotiate":
		// POST /api/game/{id}/turn/{n}/renegotiate asks the GM to rule on a
		// counter-proposal to the turn's pending check.
		if r.Method == http.MethodPost {
			s.handleTurnRenegotiateRoute(w, r, gameID, parts)
			return
		}
	case "ledger":
		s.handleTurnLedger(w, r, gameID, parts)
		return
	case "scene-image":
		s.handleTurnSceneImageRoute(w, r, gameID, parts)
		return
	case "play":
		if r.Method == http.MethodPost {
			s.handleTurnPlay(w, r, gameID, parts)
			return
		}
	}
	http.NotFound(w, r)
}

// handleTurnResolveCheckRoute reads the turn number and resolves its pending
// check.
func (s *Server) handleTurnResolveCheckRoute(w http.ResponseWriter, r *http.Request, gameID string, parts []string) {
	turnNumber, ok := turnNumberFromPath(w, parts)
	if !ok {
		return
	}
	s.handleResolveCheck(w, r, gameID, turnNumber)
}

// handleTurnRenegotiateRoute reads the turn number and asks the GM to rule on a
// counter-proposal to its pending check.
func (s *Server) handleTurnRenegotiateRoute(w http.ResponseWriter, r *http.Request, gameID string, parts []string) {
	turnNumber, ok := turnNumberFromPath(w, parts)
	if !ok {
		return
	}
	s.handleRenegotiate(w, r, gameID, turnNumber)
}

// handleTurnLedger serves the playback ledger of one turn: the stored ledger on
// GET, and a merge of an update on POST.
func (s *Server) handleTurnLedger(w http.ResponseWriter, r *http.Request, gameID string, parts []string) {
	turnNumber, ok := turnNumberFromPath(w, parts)
	if !ok {
		return
	}
	if r.Method == http.MethodGet {
		writeJSON(w, s.service.PlaybackLedger(gameID, turnNumber))
		return
	}
	if r.Method == http.MethodPost {
		var req PlaybackLedgerRequest
		if !decodeJSONBody(w, r, 1024*1024, &req) {
			return
		}
		if req.Turn <= 0 {
			req.Turn = turnNumber
		}
		dto, err := s.service.MergePlaybackLedger(gameID, req)
		if err != nil {
			writeInvalidRequest(w, err.Error())
			return
		}
		writeJSON(w, dto)
		return
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

// handleTurnSceneImageRoute serves the scene image of one turn: the stored image
// on GET, and a fresh generation on POST regardless of the trigger policy. Any
// other method is a shape the route does not have.
func (s *Server) handleTurnSceneImageRoute(w http.ResponseWriter, r *http.Request, gameID string, parts []string) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	turnNumber, ok := turnNumberFromPath(w, parts)
	if !ok {
		return
	}
	if r.Method == http.MethodGet {
		data, contentType, err := s.service.GetTurnSceneImage(r.Context(), gameID, turnNumber)
		if err != nil {
			s.writeAPIError(w, err)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(data)
		return
	}
	if err := s.service.GenerateTurnSceneImage(r.Context(), gameID, turnNumber); err != nil {
		s.writeAPIError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// handleTurnPlay plays the whole turn through the application's audio device.
func (s *Server) handleTurnPlay(w http.ResponseWriter, r *http.Request, gameID string, parts []string) {
	turnNumber, ok := turnNumberFromPath(w, parts)
	if !ok {
		return
	}
	force := r.URL.Query().Get("force") == "1" || r.URL.Query().Get("force") == "true"
	if err := s.service.PlayTurnAudio(gameID, turnNumber, force); err != nil {
		if writeGenerationFailure(w, err) {
			return
		}
		s.writeAPIError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleTurnSegmentPlay plays one beat of one turn.
func (s *Server) handleTurnSegmentPlay(w http.ResponseWriter, r *http.Request, gameID string, parts []string) {
	turnNumber, ok := turnNumberFromPath(w, parts)
	if !ok {
		return
	}
	segmentIndex, ok := segmentIndexFromPath(w, parts)
	if !ok {
		return
	}
	force := r.URL.Query().Get("force") == "1" || r.URL.Query().Get("force") == "true"
	if err := s.service.PlaySegmentAudio(r.Context(), gameID, turnNumber, segmentIndex, force); err != nil {
		if writeGenerationFailure(w, err) {
			return
		}
		s.writeAPIError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleTurnSegmentAudio re-synthesizes one beat and answers with its refreshed
// clip URLs.
func (s *Server) handleTurnSegmentAudio(w http.ResponseWriter, r *http.Request, gameID string, parts []string) {
	turnNumber, ok := turnNumberFromPath(w, parts)
	if !ok {
		return
	}
	segmentIndex, ok := segmentIndexFromPath(w, parts)
	if !ok {
		return
	}
	clips, err := s.service.GetSegmentClips(r.Context(), gameID, turnNumber, segmentIndex, true)
	switch {
	case errors.Is(err, ErrAudioUnavailable):
		w.WriteHeader(http.StatusNoContent)
	case err != nil:
		s.writeAPIError(w, err)
	default:
		writeJSON(w, map[string]any{"audio_urls": clipURLs(clips)})
	}
}
