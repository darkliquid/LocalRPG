package gui

import (
	"encoding/json"
	"net/http"
	"strings"
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
	s.mux.HandleFunc("/api/worlds", s.handleWorldsRoutes)
	if s.assetServer != nil {
		s.mux.Handle("/", s.assetServer)
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleGameRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/game/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	gameID := parts[0]
	action := parts[1]

	switch action {
	case "state":
		state, err := s.service.GetGameState(r.Context(), gameID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, state)

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

	case "entity":
		if len(parts) < 3 {
			http.Error(w, "missing entity id", http.StatusBadRequest)
			return
		}
		entityID := parts[2]
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
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	systems, err := s.service.ListSystems(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, systems)
}

func (s *Server) handleWorldsRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	worlds, err := s.service.ListWorlds(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, worlds)
}

