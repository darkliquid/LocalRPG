package gui

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/engine"
	"github.com/darkliquid/localrpg/pkg/media"
)

type Server struct {
	service     *Service
	assetServer http.Handler
	mux         *http.ServeMux
}

func NewServer(service *Service, assetHandler http.Handler) *Server {
	s := &Server{
		service:     service,
		assetServer: assetHandler,
		mux:         http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("/api/game/", s.handleGameRoutes)
	s.mux.HandleFunc("/api/games", s.handleGamesRoutes)
	s.mux.HandleFunc("/api/systems", s.handleSystemsRoutes)
	s.mux.HandleFunc("/api/system/", s.handleSystemRoutes)
	s.mux.HandleFunc("/api/worlds", s.handleWorldsRoutes)
	s.mux.HandleFunc("/api/world/", s.handleWorldRoutes)
	s.mux.HandleFunc("/api/settings", s.handleSettingsRoutes)
	s.mux.HandleFunc("/api/settings/test-provider", s.handleTestProviderRoute)
	s.mux.HandleFunc("/api/audio/", s.handleAudioRoutes)
	s.mux.HandleFunc("/api/stt", s.handleSTTRoute)
	s.mux.HandleFunc("/api/trace", s.handleTraceRoute)
	if s.assetServer != nil {
		s.mux.Handle("/", s.assetServer)
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// writeGameError maps a campaign-lifecycle failure to a status. A turn in flight
// is a conflict, a missing campaign is a 404, and anything else is a server
// fault, which keeps the client from retrying a request that cannot succeed.
func writeGameError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrTurnInFlight):
		w.Header().Set("Retry-After", "1")
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, os.ErrNotExist):
		http.Error(w, err.Error(), http.StatusNotFound)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleGameRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/game/")
	parts := strings.Split(path, "/")

	// DELETE /api/game/{id} has no sub-action, so it is matched before the
	// two-part requirement every other game route satisfies.
	if len(parts) == 1 && r.Method == http.MethodDelete {
		if err := s.service.DeleteGame(r.Context(), parts[0]); err != nil {
			writeGameError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if len(parts) < 2 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	gameID := parts[0]
	action := parts[1]

	switch action {
	case "restart":
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

	case "settings":
		if r.Method != http.MethodPatch {
			http.NotFound(w, r)
			return
		}
		var patch GameSettingsPatchDTO
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnBody)).Decode(&patch); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		values := map[string]interface{}{}
		if patch.OpeningPrompt != nil {
			values[engine.OpeningPromptSetting] = strings.TrimSpace(*patch.OpeningPrompt)
		}
		if err := s.service.UpdateGameSettings(r.Context(), gameID, values); err != nil {
			writeGameError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	case "state":
		state, err := s.service.GetGameState(r.Context(), gameID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, state)

	case "entities":
		entities, err := s.service.ListEntities(r.Context(), gameID)
		if err != nil {
			writeGameError(w, err)
			return
		}
		writeJSON(w, entities)

	case "recap":
		recap, err := s.service.GetRecap(r.Context(), gameID)
		if err != nil {
			writeGameError(w, err)
			return
		}
		writeJSON(w, recap)

	case "graph":
		graph, err := s.service.GetGraph(r.Context(), gameID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, graph)

	case "chronicle":
		chronicle, err := s.service.GetChronicle(r.Context(), gameID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, chronicle)

	case "turn":
		// POST /api/game/{id}/turn submits a turn. GET
		// /api/game/{id}/turn/{n}/segment/{i}/audio serves one beat's clip; both
		// hang off a turn, so they share this case.
		if r.Method == http.MethodPost && len(parts) == 2 {
			s.handleTurnSubmit(w, r, gameID)
			return
		}

		// POST /api/game/{id}/turn/{n}/play plays the whole turn through the
		// application's audio device.
		if r.Method == http.MethodPost && len(parts) == 4 && parts[3] == "play" {
			turnNumber, err := strconv.Atoi(parts[2])
			if err != nil {
				http.Error(w, "invalid turn number", http.StatusBadRequest)
				return
			}
			if err := s.service.PlayTurnAudio(r.Context(), gameID, turnNumber); err != nil {
				writeGameError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// POST /api/game/{id}/turn/{n}/segment/{i}/play plays one beat.
		if r.Method == http.MethodPost && len(parts) == 6 && parts[3] == "segment" && parts[5] == "play" {
			turnNumber, err := strconv.Atoi(parts[2])
			if err != nil {
				http.Error(w, "invalid turn number", http.StatusBadRequest)
				return
			}
			segmentIndex, err := strconv.Atoi(parts[4])
			if err != nil {
				http.Error(w, "invalid segment index", http.StatusBadRequest)
				return
			}
			if err := s.service.PlaySegmentAudio(r.Context(), gameID, turnNumber, segmentIndex); err != nil {
				writeGameError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}

		if len(parts) < 6 || parts[3] != "segment" || parts[5] != "audio" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}

		turnNumber, err := strconv.Atoi(parts[2])
		if err != nil {
			http.Error(w, "invalid turn number", http.StatusBadRequest)
			return
		}
		segmentIndex, err := strconv.Atoi(parts[4])
		if err != nil {
			http.Error(w, "invalid segment index", http.StatusBadRequest)
			return
		}

		path, err := s.service.GetSegmentAudio(r.Context(), gameID, turnNumber, segmentIndex)
		switch {
		case errors.Is(err, ErrAudioUnavailable):
			w.WriteHeader(http.StatusNoContent)
		case errors.Is(err, os.ErrNotExist):
			http.Error(w, err.Error(), http.StatusNotFound)
		case err != nil:
			http.Error(w, err.Error(), http.StatusNotFound)
		default:
			// The clip's type comes from its bytes: a provider returns whatever its
			// engine produces, which is not necessarily what the configuration says.
			data, err := os.ReadFile(path)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", media.AudioContentType(data))
			_, _ = w.Write(data)
		}

	case "location":
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
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", contentType)
		http.ServeFile(w, r, path)

	case "entity":
		if len(parts) < 3 {
			http.Error(w, "missing entity id", http.StatusBadRequest)
			return
		}
		entityID := parts[2]
		if len(parts) >= 4 && parts[3] == "turns" && r.Method == http.MethodGet {
			turns, err := s.service.GetEntityTurns(r.Context(), gameID, entityID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, turns)
			return
		}
		if r.Method == http.MethodPut {
			var body struct {
				Markdown string `json:"markdown"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			if err := s.service.SaveEntity(r.Context(), gameID, entityID, body.Markdown); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}

		if len(parts) >= 4 && parts[3] == "art" && r.Method == http.MethodGet {
			path, contentType, err := s.service.GetLocationArt(r.Context(), gameID, entityID, r.URL.Query().Get("force") == "1")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", contentType)
			http.ServeFile(w, r, path)
			return
		}

		ent, err := s.service.GetEntity(r.Context(), gameID, entityID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, ent)

	default:
		http.NotFound(w, r)
	}
}

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func (s *Server) handleGamesRoutes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		games, err := s.service.ListGames(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, games)
	case http.MethodPost:
		var req CreateGameRequestDTO
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		game, err := s.service.CreateGame(r.Context(), req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(game)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSystemsRoutes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		systems, err := s.service.ListSystems(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, systems)
	case http.MethodPost:
		var req CreateSystemRequestDTO
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		sys, err := s.service.SaveSystem(r.Context(), req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(sys)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSystemRoutes(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/system/")
	id = strings.Trim(id, "/")
	if id == "" {
		http.Error(w, "missing system id", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		sys, err := s.service.GetSystem(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, sys)
	case http.MethodPut:
		var req CreateSystemRequestDTO
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		req.ID = id
		sys, err := s.service.SaveSystem(r.Context(), req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, sys)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleWorldsRoutes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		worlds, err := s.service.ListWorlds(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, worlds)
	case http.MethodPost:
		var req CreateWorldRequestDTO
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		world, err := s.service.SaveWorld(r.Context(), req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(world)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleWorldRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/world/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "missing world id", http.StatusBadRequest)
		return
	}

	worldID := parts[0]

	// Check if this is an entity route: /api/world/:worldID/entity/:entityID
	if len(parts) >= 3 && parts[1] == "entity" {
		entityID := parts[2]
		switch r.Method {
		case http.MethodGet:
			ent, err := s.service.GetWorldEntity(r.Context(), worldID, entityID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusNotFound)
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
			}
			if err := json.Unmarshal(data, &obj); err == nil && obj.Markdown != "" {
				content = obj.Markdown
			}
			if err := s.service.SaveWorldEntity(r.Context(), worldID, entityID, content); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
		case http.MethodDelete:
			if err := s.service.DeleteWorldEntity(r.Context(), worldID, entityID); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	// World root route: /api/world/:worldID
	switch r.Method {
	case http.MethodGet:
		world, err := s.service.GetWorld(r.Context(), worldID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
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
		world, err := s.service.SaveWorld(r.Context(), req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, world)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSettingsRoutes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings, err := s.service.GetSettings(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, settings)

	case http.MethodPut:
		var req struct {
			Config config.Config `json:"config"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		res, err := s.service.SaveSettings(r.Context(), req.Config)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, res)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleTestProviderRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req TestProviderRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	res, err := s.service.TestProvider(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, res)
}

func (s *Server) handleSTTRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 25<<20)

	var audioData []byte
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(25 << 20); err != nil {
			http.Error(w, "failed to parse multipart form: "+err.Error(), http.StatusBadRequest)
			return
		}
		file, _, err := r.FormFile("audio")
		if err != nil {
			file, _, err = r.FormFile("file")
		}
		if err != nil {
			http.Error(w, "missing audio or file in multipart form", http.StatusBadRequest)
			return
		}
		defer file.Close()
		data, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, "failed to read audio file: "+err.Error(), http.StatusInternalServerError)
			return
		}
		audioData = data
	} else {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body: "+err.Error(), http.StatusInternalServerError)
			return
		}
		audioData = data
	}

	if len(audioData) == 0 {
		http.Error(w, "empty audio payload", http.StatusBadRequest)
		return
	}

	text, err := s.service.TranscribeAudio(r.Context(), audioData)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, STTResponse{Text: text})
}

// handleAudioRoutes reports and controls application-side playback. It is what
// lets a client stop depending on a browser's autoplay policy.
func (s *Server) handleAudioRoutes(w http.ResponseWriter, r *http.Request) {
	action := strings.TrimPrefix(r.URL.Path, "/api/audio/")

	switch action {
	case "status":
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, AudioStatusDTO{
			Available: s.service.AudioAvailable(),
			Playing:   s.service.AudioPlaying(),
		})

	case "stop":
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		s.service.StopAudio()
		w.WriteHeader(http.StatusNoContent)

	default:
		http.NotFound(w, r)
	}
}

// handleTraceRoute serves the recorded trace. It is a developer view, so it can be
// read and cleared and nothing else.
func (s *Server) handleTraceRoute(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		events, err := s.service.TraceEvents(limit, r.URL.Query().Get("game"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, events)

	case http.MethodDelete:
		if err := s.service.ClearTrace(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.NotFound(w, r)
	}
}

// maxTurnBody bounds a player action so a runaway paste cannot allocate without
// limit. It is far above any plausible action.
const maxTurnBody = 64 << 10

// handleTurnSubmit streams a turn as newline-delimited JSON. Status codes can only
// be chosen before the first byte, so the campaign is prepared first: 409 for a
// turn already in flight, 503 for a campaign that cannot be played, and an `error`
// event for anything that happens once streaming has begun.
func (s *Server) handleTurnSubmit(w http.ResponseWriter, r *http.Request, gameID string) {
	var req TurnRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnBody)).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := req.validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	session, err := s.service.BeginTurn(gameID)
	switch {
	case errors.Is(err, ErrTurnInFlight):
		w.Header().Set("Retry-After", "1")
		http.Error(w, err.Error(), http.StatusConflict)
		return
	case errors.Is(err, fs.ErrNotExist):
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	case err != nil:
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer session.Close()

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported by this client", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	encoder := json.NewEncoder(w)
	writeEvent := func(event TurnEvent) error {
		if err := encoder.Encode(event); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	if err := session.Run(r.Context(), req, writeEvent); err != nil {
		_ = writeEvent(TurnEvent{Type: "error", Message: err.Error()})
	}
}
