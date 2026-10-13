package gui

import (
	"errors"
	"net/http"
)

// The turn routes under /api/game/{id}/turn: submitting a turn, resolving or
// renegotiating its pending check, its playback ledger, its scene image, and the
// audio for the turn and for one segment of it.

// handleGameTurnRoute dispatches everything that hangs off a turn.
func (s *Server) handleGameTurnRoute(w http.ResponseWriter, r *http.Request, gameID string, parts []string) {
	// POST /api/game/{id}/turn submits a turn. GET
	// /api/game/{id}/turn/{n}/segment/{i}/audio serves one beat's clip; both
	// hang off a turn, so they share this case.
	if r.Method == http.MethodPost && len(parts) == 2 {
		s.handleTurnSubmit(w, r, gameID)
		return
	}

	// POST /api/game/{id}/turn/{n}/resolve-check resolves the turn's pending
	// check without a fresh player action.
	if r.Method == http.MethodPost && len(parts) == 4 && parts[3] == "resolve-check" {
		turnNumber, ok := turnNumberFromPath(w, parts)
		if !ok {
			return
		}
		s.handleResolveCheck(w, r, gameID, turnNumber)
		return
	}

	// POST /api/game/{id}/turn/{n}/renegotiate asks the GM to rule on a
	// counter-proposal to the turn's pending check.
	if r.Method == http.MethodPost && len(parts) == 4 && parts[3] == "renegotiate" {
		turnNumber, ok := turnNumberFromPath(w, parts)
		if !ok {
			return
		}
		s.handleRenegotiate(w, r, gameID, turnNumber)
		return
	}

	// GET /api/game/{id}/turn/{n}/ledger or POST /api/game/{id}/turn/{n}/ledger
	if len(parts) == 4 && parts[3] == "ledger" {
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
		return
	}

	// GET /api/game/{id}/turn/{n}/scene-image
	if r.Method == http.MethodGet && len(parts) == 4 && parts[3] == "scene-image" {
		turnNumber, ok := turnNumberFromPath(w, parts)
		if !ok {
			return
		}
		data, contentType, err := s.service.GetTurnSceneImage(r.Context(), gameID, turnNumber)
		if err != nil {
			writeGameError(w, err)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(data)
		return
	}

	// POST /api/game/{id}/turn/{n}/scene-image generates one on demand,
	// regardless of the trigger policy.
	if r.Method == http.MethodPost && len(parts) == 4 && parts[3] == "scene-image" {
		turnNumber, ok := turnNumberFromPath(w, parts)
		if !ok {
			return
		}
		if err := s.service.GenerateTurnSceneImage(r.Context(), gameID, turnNumber); err != nil {
			writeGameError(w, err)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// POST /api/game/{id}/turn/{n}/play plays the whole turn through the
	// application's audio device.
	if r.Method == http.MethodPost && len(parts) == 4 && parts[3] == "play" {
		turnNumber, ok := turnNumberFromPath(w, parts)
		if !ok {
			return
		}
		force := r.URL.Query().Get("force") == "1" || r.URL.Query().Get("force") == "true"
		if err := s.service.PlayTurnAudio(gameID, turnNumber, force); err != nil {
			if writeGenerationFailure(w, err) {
				return
			}
			writeGameError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// POST /api/game/{id}/turn/{n}/segment/{i}/play plays one beat.
	if r.Method == http.MethodPost && len(parts) == 6 && parts[3] == "segment" && parts[5] == "play" {
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
			writeGameError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if len(parts) < 6 || parts[3] != "segment" || parts[5] != "audio" || r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}

	turnNumber, ok := turnNumberFromPath(w, parts)
	if !ok {
		return
	}
	segmentIndex, ok := segmentIndexFromPath(w, parts)
	if !ok {
		return
	}

	// POST /api/game/{id}/turn/{n}/segment/{i}/audio re-synthesizes one beat
	// and answers with its refreshed clip URLs. Playback itself reads clips by
	// content key, so this is only what a regenerate control needs.
	clips, err := s.service.GetSegmentClips(r.Context(), gameID, turnNumber, segmentIndex, true)
	switch {
	case errors.Is(err, ErrAudioUnavailable):
		w.WriteHeader(http.StatusNoContent)
	case err != nil:
		writeGameError(w, err)
	default:
		writeJSON(w, map[string]any{"audio_urls": clipURLs(clips)})
	}
}
