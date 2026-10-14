package gui

import "net/http"

// The audio routes under /api/game/{id}: the playback ledger and the batches that
// synthesize a turn's speech ahead of playback.

// handleGameAudioRoute serves the ledger branch of /api/game/{id}/audio. The GET
// reads the campaign-wide ledger; the POST merges one in.
func (s *Server) handleGameAudioRoute(w http.ResponseWriter, r *http.Request, gameID string, parts []string) {
	if len(parts) >= 3 && parts[2] == "ledger" {
		if r.Method == http.MethodGet {
			writeJSON(w, s.service.PlaybackLedger(gameID, 0))
			return
		}
		if r.Method == http.MethodPost {
			var req PlaybackLedgerRequest
			if !decodeJSONBody(w, r, 1024*1024, &req) {
				return
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
	http.NotFound(w, r)
}

// handleGameTTSRoute serves /api/game/{id}/tts: the count of beats without a
// cached clip, and the batch jobs that fill the cache.
func (s *Server) handleGameTTSRoute(w http.ResponseWriter, r *http.Request, gameID string, parts []string) {
	if len(parts) < 3 {
		http.NotFound(w, r)
		return
	}
	switch parts[2] {
	case "uncached":
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		cached, uncached, err := s.service.CountUncachedBeats(gameID)
		if err != nil {
			s.writeAPIError(w, err)
			return
		}
		writeJSON(w, map[string]int{"cached": cached, "uncached": uncached})
	case "batch":
		switch r.Method {
		case http.MethodGet:
			jobs, err := s.service.TTSBatchJobs(gameID)
			if err != nil {
				s.writeAPIError(w, err)
				return
			}
			writeJSON(w, jobs)
		case http.MethodPost:
			force := r.URL.Query().Get("force") == "1" || r.URL.Query().Get("force") == "true"
			job, err := s.service.StartTTSBatch(r.Context(), gameID, force)
			if err != nil {
				s.writeAPIError(w, err)
				return
			}
			writeJSON(w, job)
		default:
			http.NotFound(w, r)
		}
	default:
		http.NotFound(w, r)
	}
}
