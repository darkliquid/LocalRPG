package gui

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/engine"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/models"
	"github.com/darkliquid/localrpg/pkg/pathutil"
)

type Server struct {
	service     *Service
	assetServer http.Handler
	mux         *http.ServeMux
	handler     http.Handler
}

func NewServer(service *Service, assetHandler http.Handler) *Server {
	s := &Server{
		service:     service,
		assetServer: assetHandler,
		mux:         http.NewServeMux(),
	}
	s.registerRoutes()
	s.handler = otelhttp.NewHandler(ActionCorrelationMiddleware(s.mux), "localrpg.http",
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return routePattern(r.URL.Path)
		}),
	)
	return s
}

// routePattern maps a request path to a stable template so span names stay
// bounded: a campaign id in the path must not create one span name per id.
func routePattern(path string) string {
	switch {
	case path == "/api/games" || path == "/api/systems" || path == "/api/worlds" ||
		path == "/api/settings" || path == "/api/settings/test-provider" ||
		path == "/api/config/offline-preset" || path == "/api/config/offline-report" ||
		path == "/api/open-url" ||
		path == "/api/providers" || path == "/api/providers/models" ||
		path == "/api/reference-systems" ||
		path == "/api/tts/inspect" || path == "/api/tts/voices/search" ||
		path == "/api/media/inspect" ||
		path == "/api/tts/batch" ||
		path == "/api/content/export" || path == "/api/content/import" ||
		path == "/api/registry/search" || path == "/api/registry/install" || path == "/api/registry/updates" ||
		path == "/api/stt" || path == "/api/trace" || path == "/api/character/generate" ||
		path == "/api/generate-text" || path == "/api/generate-asset-preview" ||
		path == "/api/usage" || path == "/api/limits":
		return path
	case strings.HasPrefix(path, "/api/schema/"):
		return "/api/schema/{name}"
	case path == "/api/models" || strings.HasPrefix(path, "/api/models/"):
		return "/api/models"
	case path == "/api/export" || path == "/api/export/capabilities" || path == "/api/export/events" ||
		path == "/api/export/choose-directory":
		return path
	case strings.HasPrefix(path, "/api/export/"):
		return "/api/export/{gameID}"
	case strings.HasPrefix(path, "/api/tts/batch/"):
		return "/api/tts/batch"
	case strings.HasPrefix(path, "/api/game/"):
		rest := strings.TrimPrefix(path, "/api/game/")
		if _, suffix, ok := strings.Cut(rest, "/"); ok {
			return "/api/game/{id}/" + suffix
		}
		return "/api/game/{id}"
	case path == "/api/system/test":
		return "/api/system/test"
	case strings.HasPrefix(path, "/api/dialog/"):
		return "/api/dialog/{action}"
	case strings.HasPrefix(path, "/api/system/tests/"):
		return "/api/system/tests/{id}"
	case strings.HasPrefix(path, "/api/system/"):
		return "/api/system/{id}"
	case strings.HasPrefix(path, "/api/world/"):
		rest := strings.TrimPrefix(path, "/api/world/")
		if _, suffix, ok := strings.Cut(rest, "/"); ok {
			return "/api/world/{id}/" + suffix
		}
		return "/api/world/{id}"
	case strings.HasPrefix(path, "/api/audio/"):
		return "/api/audio"
	case strings.HasPrefix(path, "/api/docs"):
		return "/api/docs"
	default:
		return "http.request"
	}
}

func (s *Server) registerRoutes() {
	for _, mount := range mounts {
		serve := mount.serve
		s.mux.HandleFunc(mount.Pattern, func(w http.ResponseWriter, r *http.Request) {
			serve(s, w, r)
		})
	}
	if s.assetServer != nil {
		s.mux.Handle("/", s.assetServer)
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

// writeGameError maps a campaign-lifecycle failure to a status. A turn in flight
// is a conflict, a missing campaign is a 404, and anything else is a server
// fault, which keeps the client from retrying a request that cannot succeed.
func writeGameError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrTurnInFlight):
		w.Header().Set("Retry-After", "1")
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, ErrAdvancementRefused):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, ErrInvalidFolderPath):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, ErrDuplicateEntityID):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, os.ErrNotExist):
		http.Error(w, err.Error(), http.StatusNotFound)
	default:
		var limited *harness.ErrRateLimitedUntil
		if errors.As(err, &limited) {
			seconds := int(limited.RetryAfter().Seconds()) + 1
			w.Header().Set("Retry-After", strconv.Itoa(seconds))
			writeJSONError(w, http.StatusTooManyRequests, limited.Error())
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// writeSaveError answers a failed save. A frontmatter failure carries the line and
// column it happened on, so the editor can highlight it rather than showing a bare
// status; anything else falls back to the shared error mapping.
func writeSaveError(w http.ResponseWriter, err error, document string) {
	var fe *entity.FrontmatterError
	if !errors.As(err, &fe) {
		writeGameError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnprocessableEntity)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error":  fe.Msg,
		"line":   fe.Line(document),
		"column": fe.Column(document),
	})
}

// handleSchemaRoutes serves the generated schemas the editor and the new-entity
// wizard consume.
func (s *Server) handleSchemaRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	switch r.URL.Path {
	case "/api/schema/entity-frontmatter":
		schema, err := s.service.GetEntityFrontmatterSchema(r.Context())
		if err != nil {
			writeGameError(w, err)
			return
		}
		writeJSON(w, schema)
	case "/api/schema/entity-types":
		catalog, err := s.service.GetEntityTypeCatalog(r.Context())
		if err != nil {
			writeGameError(w, err)
			return
		}
		writeJSON(w, catalog)
	default:
		http.NotFound(w, r)
	}
}

// handleFolderRoutes serves the folder CRUD a tree UI needs. It takes the
// entities directory rather than a collection id, because the game and world
// trees are the same shape.
func (s *Server) handleFolderRoutes(w http.ResponseWriter, r *http.Request, entitiesDir string) {
	switch r.Method {
	case http.MethodGet:
		tree, err := s.service.ListFolders(entitiesDir)
		if err != nil {
			writeGameError(w, err)
			return
		}
		writeJSON(w, tree)

	case http.MethodPost:
		var body FolderRequestDTO
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnBody)).Decode(&body); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if err := s.service.CreateFolder(entitiesDir, body.Path); err != nil {
			writeGameError(w, err)
			return
		}
		writeJSON(w, map[string]string{"path": strings.TrimSpace(body.Path)})

	case http.MethodPut:
		var body FolderRequestDTO
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnBody)).Decode(&body); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if err := s.service.MoveFolder(entitiesDir, body.From, body.Path); err != nil {
			writeGameError(w, err)
			return
		}
		writeJSON(w, map[string]string{"path": strings.TrimSpace(body.Path)})

	case http.MethodDelete:
		recursive := r.URL.Query().Get("recursive") == "true"
		if err := s.service.DeleteFolder(entitiesDir, r.URL.Query().Get("path"), recursive); err != nil {
			writeGameError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleUsageRoute serves GET /api/usage: installation-wide spend with a
// per-campaign drilldown.
func (s *Server) handleUsageRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	dto, err := s.service.GlobalUsage(r.Context())
	if err != nil {
		writeGameError(w, err)
		return
	}
	writeJSON(w, dto)
}

// handleLimitsRoute serves GET /api/limits: the live blocks and funds failures.
func (s *Server) handleLimitsRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, LimitsDTO{Blocks: s.service.Limits()})
}

// handleOpenURLRoute hands a link to the desktop window, which opens it in the
// system browser. Browser and socket mode have no window to ask, so they answer
// 501 and the frontend opens a tab itself.
func (s *Server) handleOpenURLRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req OpenURLRequestDTO
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxOpenURLBody)).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := s.service.OpenURL(req.URL); err != nil {
		if errors.Is(err, errNoURLOpener) {
			http.Error(w, err.Error(), http.StatusNotImplemented)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleTTSBatchRoute serves the global batch job manager: GET /api/tts/batch
// lists every campaign's jobs, and POST /api/tts/batch/cancel cancels one.
func (s *Server) handleTTSBatchRoute(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && (r.URL.Path == "/api/tts/batch" || r.URL.Path == "/api/tts/batch/"):
		jobs, err := s.service.AllTTSBatchJobs(r.Context())
		if err != nil {
			writeGameError(w, err)
			return
		}
		writeJSON(w, jobs)
	case r.Method == http.MethodPost && r.URL.Path == "/api/tts/batch/cancel":
		var req TTSBatchCancelRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnBody)).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if err := s.service.CancelTTSBatch(r.Context(), req.GameID, req.JobID); err != nil {
			writeGameError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost && r.URL.Path == "/api/tts/batch/delete":
		var req TTSBatchCancelRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnBody)).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if err := s.service.DeleteTTSBatch(r.Context(), req.GameID, req.JobID); err != nil {
			writeGameError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost && r.URL.Path == "/api/tts/batch/clear":
		var req TTSBatchClearRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnBody)).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		removed, err := s.service.ClearTTSBatch(r.Context(), req.GameID)
		if err != nil {
			writeGameError(w, err)
			return
		}
		writeJSON(w, map[string]int{"removed": removed})
	case r.Method == http.MethodPost && r.URL.Path == "/api/tts/batch/resume":
		var req TTSBatchCancelRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnBody)).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		job, err := s.service.ResumeTTSBatch(r.Context(), req.GameID, req.JobID)
		if err != nil {
			writeGameError(w, err)
			return
		}
		writeJSON(w, job)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleGameRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/game/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "missing game id", http.StatusBadRequest)
		return
	}

	gameID := parts[0]
	if err := pathutil.ValidateID(gameID); err != nil {
		http.Error(w, "invalid game id", http.StatusBadRequest)
		return
	}

	// DELETE /api/game/{id} has no sub-action, so it is matched before the
	// two-part requirement every other game route satisfies.
	if len(parts) == 1 && r.Method == http.MethodDelete {
		if err := s.service.DeleteGame(r.Context(), gameID); err != nil {
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

	action := parts[1]

	switch action {
	case "folders":
		s.handleFolderRoutes(w, r, filepath.Join(s.service.resolver.GameDir(gameID), "entities"))
		return

	case "banner", "icon":
		if r.Method == http.MethodGet {
			filePath, contentType, err := s.service.GetGameAsset(gameID, action)
			if err != nil {
				writeGameError(w, err)
				return
			}
			w.Header().Set("Content-Type", contentType)
			http.ServeFile(w, r, filePath)
			return
		} else if r.Method == http.MethodPost {
			file, header, err := r.FormFile("file")
			if err != nil {
				http.Error(w, "missing file in form data", http.StatusBadRequest)
				return
			}
			defer file.Close()
			data, err := io.ReadAll(http.MaxBytesReader(w, file, 15*1024*1024))
			if err != nil {
				http.Error(w, "file too large or read failed", http.StatusBadRequest)
				return
			}
			ext := filepath.Ext(header.Filename)
			url, err := s.service.SaveGameAsset(gameID, action, data, ext)
			if err != nil {
				writeGameError(w, err)
				return
			}
			writeJSON(w, map[string]string{"url": url})
			return
		} else if r.Method == http.MethodDelete {
			// Clearing the campaign's own artwork is how the world's is restored.
			if err := s.service.DeleteGameAsset(gameID, action); err != nil {
				writeGameError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return

	case "generate-asset":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req GenerateAssetRequestDTO
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024*1024)).Decode(&req); err != nil {
			writeInvalidRequest(w, "invalid request body")
			return
		}
		url, err := s.service.GenerateGameAsset(r.Context(), gameID, req)
		if err != nil {
			if writeGenerationFailure(w, err) {
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"url": url})
		return

	case "tts":
		if len(parts) < 3 {
			http.NotFound(w, r)
			return
		}
		switch parts[2] {
		case "uncached":
			if r.Method != http.MethodGet {
				http.NotFound(w, r)
				return
			}
			cached, uncached, err := s.service.CountUncachedBeats(gameID)
			if err != nil {
				writeGameError(w, err)
				return
			}
			writeJSON(w, map[string]int{"cached": cached, "uncached": uncached})
		case "batch":
			switch r.Method {
			case http.MethodGet:
				jobs, err := s.service.TTSBatchJobs(gameID)
				if err != nil {
					writeGameError(w, err)
					return
				}
				writeJSON(w, jobs)
			case http.MethodPost:
				force := r.URL.Query().Get("force") == "1" || r.URL.Query().Get("force") == "true"
				job, err := s.service.StartTTSBatch(r.Context(), gameID, force)
				if err != nil {
					writeGameError(w, err)
					return
				}
				writeJSON(w, job)
			default:
				http.NotFound(w, r)
			}
		default:
			http.NotFound(w, r)
		}

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

	case "state":
		state, err := s.service.GetGameState(r.Context(), gameID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, state)

	case "context":
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

	case "working-set":
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

	case "findings":
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
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnBody)).Decode(&req); err != nil {
				http.Error(w, "invalid request body", http.StatusBadRequest)
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

	case "usage":
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
		return

	case "advance":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			UnlockID string `json:"unlock_id"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&req); err != nil {
			writeInvalidRequest(w, "invalid request body")
			return
		}
		advancement, err := s.service.AdvanceUnlock(r.Context(), gameID, req.UnlockID)
		if err != nil {
			writeGameError(w, err)
			return
		}
		writeJSON(w, map[string]interface{}{"advancement": advancement})
		return

	case "turn":
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
			turnNumber, err := strconv.Atoi(parts[2])
			if err != nil {
				http.Error(w, "invalid turn number", http.StatusBadRequest)
				return
			}
			s.handleResolveCheck(w, r, gameID, turnNumber)
			return
		}

		// GET /api/game/{id}/turn/{n}/scene-image
		if r.Method == http.MethodGet && len(parts) == 4 && parts[3] == "scene-image" {
			turnNumber, err := strconv.Atoi(parts[2])
			if err != nil {
				http.Error(w, "invalid turn number", http.StatusBadRequest)
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
			turnNumber, err := strconv.Atoi(parts[2])
			if err != nil {
				http.Error(w, "invalid turn number", http.StatusBadRequest)
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
		// POST /api/game/{id}/turn/{n}/play plays the whole turn through the
		// application's audio device.
		if r.Method == http.MethodPost && len(parts) == 4 && parts[3] == "play" {
			turnNumber, err := strconv.Atoi(parts[2])
			if err != nil {
				http.Error(w, "invalid turn number", http.StatusBadRequest)
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
			writeJSON(w, map[string]interface{}{"audio_urls": clipURLs(clips)})
		}
		return

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

	case "character":
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
					writeGameError(w, err)
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
					writeGameError(w, err)
					return
				}
				w.Header().Set("Content-Type", contentType)
				w.Header().Set("Cache-Control", "no-cache")
				_, _ = w.Write(data)
				return
			}
		}
		http.NotFound(w, r)
		return

	case "entity":
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
				http.Error(w, err.Error(), http.StatusInternalServerError)
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
				http.Error(w, err.Error(), http.StatusInternalServerError)
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
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnBody)).Decode(&body); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			if err := s.service.SaveEntityInFolder(r.Context(), gameID, entityID, body.Folder, body.Markdown); err != nil {
				writeSaveError(w, err, body.Markdown)
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

		// POST /api/game/{id}/entity/{source}/merge folds one note into another.
		if r.Method == http.MethodPost && len(parts) >= 4 && parts[3] == "merge" {
			var req MergeEntityRequestDTO
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnBody)).Decode(&req); err != nil {
				http.Error(w, "invalid request body", http.StatusBadRequest)
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
				writeGameError(w, err)
				return
			}
			writeJSON(w, merged)
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

// writeJSONStatus writes a JSON body with an explicit status, so header
// handling stays in one place for error envelopes.
func writeJSONStatus(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
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

func (s *Server) handleReferenceSystemsRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	res, err := s.service.ListReferenceSystems(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, res)
}

func (s *Server) handleSystemTestRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req SystemTestRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	resp, err := s.service.TestSystem(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, resp)
}

func (s *Server) handleSystemTestsRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/system/tests/"), "/")
	if id == "" {
		http.Error(w, "missing system id", http.StatusBadRequest)
		return
	}
	resp, err := s.service.SystemScenarios(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, resp)
}

func (s *Server) handleSystemRoutes(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/system/")
	id = strings.Trim(id, "/")
	if id == "" {
		http.Error(w, "missing system id", http.StatusBadRequest)
		return
	}
	if err := pathutil.ValidateID(id); err != nil {
		http.Error(w, "invalid system id", http.StatusBadRequest)
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
	case http.MethodDelete:
		force := r.URL.Query().Get("force") == "true"
		err := s.service.DeleteSystem(r.Context(), id, force)
		if errors.Is(err, ErrSystemNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if errors.Is(err, ErrSystemInUse) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
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
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnBody)).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		world, err := s.service.CreateWorld(r.Context(), req)
		if errors.Is(err, ErrWorldExists) {
			writeJSONError(w, http.StatusConflict, err.Error())
			return
		}
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
	if err := pathutil.ValidateID(worldID); err != nil {
		http.Error(w, "invalid world id", http.StatusBadRequest)
		return
	}

	// Check if this is an entity route: /api/world/:worldID/entity/:entityID
	if len(parts) >= 3 && parts[1] == "entity" {
		entityID := parts[2]
		if err := pathutil.ValidateID(entityID); err != nil {
			http.Error(w, "invalid entity id", http.StatusBadRequest)
			return
		}
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
				Folder   string `json:"folder"`
			}
			if err := json.Unmarshal(data, &obj); err == nil && obj.Markdown != "" {
				content = obj.Markdown
			}
			if err := s.service.SaveWorldEntity(r.Context(), worldID, entityID, obj.Folder, content); err != nil {
				writeGameError(w, err)
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

	if len(parts) >= 2 && parts[1] == "folders" {
		s.handleFolderRoutes(w, r, filepath.Join(s.service.resolver.WorldDir(worldID), "entities"))
		return
	}

	// AI generation on a world: a previewed batch of entities, and enhancement
	// proposals with an apply step. None of them writes without an accept.
	if len(parts) >= 2 && parts[1] == "generate-entities" {
		s.handleWorldEntitiesPreview(w, r, worldID)
		return
	}
	if len(parts) >= 3 && parts[1] == "entities" && parts[2] == "accept" {
		s.handleWorldEntitiesAccept(w, r, worldID)
		return
	}
	if len(parts) >= 2 && parts[1] == "enhance" {
		if len(parts) >= 3 && parts[2] == "apply" {
			s.handleWorldEnhanceApply(w, r, worldID)
			return
		}
		s.handleWorldEnhance(w, r, worldID)
		return
	}

	if len(parts) >= 2 && (parts[1] == "banner" || parts[1] == "icon") {
		action := parts[1]
		if r.Method == http.MethodGet {
			filePath, contentType, err := s.service.GetWorldAsset(worldID, action)
			if err != nil {
				writeGameError(w, err)
				return
			}
			w.Header().Set("Content-Type", contentType)
			http.ServeFile(w, r, filePath)
			return
		} else if r.Method == http.MethodPost {
			file, header, err := r.FormFile("file")
			if err != nil {
				http.Error(w, "missing file in form data", http.StatusBadRequest)
				return
			}
			defer file.Close()
			data, err := io.ReadAll(http.MaxBytesReader(w, file, 15*1024*1024))
			if err != nil {
				http.Error(w, "file too large or read failed", http.StatusBadRequest)
				return
			}
			ext := filepath.Ext(header.Filename)
			url, err := s.service.SaveWorldAsset(worldID, action, data, ext)
			if err != nil {
				writeGameError(w, err)
				return
			}
			writeJSON(w, map[string]string{"url": url})
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if len(parts) >= 2 && parts[1] == "generate-asset" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req GenerateAssetRequestDTO
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024*1024)).Decode(&req); err != nil {
			writeInvalidRequest(w, "invalid request body")
			return
		}
		url, err := s.service.GenerateWorldAsset(r.Context(), worldID, req)
		if err != nil {
			if writeGenerationFailure(w, err) {
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"url": url})
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
		world, err := s.service.UpdateWorld(r.Context(), req)
		if errors.Is(err, ErrWorldNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, world)
	case http.MethodDelete:
		force := r.URL.Query().Get("force") == "true"
		err := s.service.DeleteWorld(r.Context(), worldID, force)
		if errors.Is(err, ErrWorldNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if errors.Is(err, ErrWorldInUse) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
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

func (s *Server) handleOfflinePresetRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req OfflinePresetRequestDTO
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	res, err := s.service.ApplyOfflinePreset(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, res)
}

func (s *Server) handleOfflineReportRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	res, err := s.service.CheckOffline(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, res)
}

func (s *Server) handleTTSInspectRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req TTSInspectRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	res, err := s.service.InspectTTS(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, res)
}

func (s *Server) handleMediaInspectRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req MediaInspectRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	res, err := s.service.InspectMedia(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, res)
}

func (s *Server) handleProviderCatalogRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	res, err := s.service.ListProviders(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, res)
}

func (s *Server) handleModelCatalogueRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ModelCatalogueRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	res, err := s.service.ListModels(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, res)
}

func (s *Server) handleVoiceSearchRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req VoiceSearchRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	res, err := s.service.SearchTTSVoices(r.Context(), req)
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
		if writeGenerationFailure(w, err) {
			return
		}
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

	case "events":
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		s.serveAudioEvents(w, r)

	default:
		if r.Method == http.MethodGet && strings.HasPrefix(action, "clip/") {
			s.serveClip(w, r, strings.TrimPrefix(action, "clip/"))
			return
		}
		http.NotFound(w, r)
	}
}

// serveAudioEvents streams playback status changes as server-sent events, so a
// client advances on a real completion rather than polling the status endpoint.
func (s *Server) serveAudioEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported by this client", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	write := func(status AudioStatusDTO) {
		data, err := json.Marshal(status)
		if err != nil {
			return
		}
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	ch, cancel := s.service.SubscribeAudioStatus()
	defer cancel()
	for {
		select {
		case <-r.Context().Done():
			return
		case status := <-ch:
			write(status)
		}
	}
}

// serveClip serves one stored clip by its content key. The key names the file and
// nothing else does, so a malformed or unknown key is a 404 rather than a lookup.
func (s *Server) serveClip(w http.ResponseWriter, r *http.Request, key string) {
	path, ok := s.service.ClipPath(key)
	if !ok {
		http.NotFound(w, r)
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// The clip's type comes from its bytes: a provider returns whatever its engine
	// produces, which is not necessarily what the configuration says. The key is
	// content-addressed, so the URL can be cached hard and the ETag follows the
	// bytes in case a clip is ever rewritten.
	setClipHeaders(w, data)
	_, _ = w.Write(data)
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

// maxOpenURLBody bounds a link request. A URL that does not fit in a few
// kilobytes is not one the app would ever open.
const maxOpenURLBody = 4 << 10

// handleTurnSubmit streams a turn as newline-delimited JSON. Status codes can only
// be chosen before the first byte, so the campaign is prepared first: 409 for a
// turn already in flight, 503 for a campaign that cannot be played, and an `error`
// event for anything that happens once streaming has begun.
func (s *Server) handleTurnSubmit(w http.ResponseWriter, r *http.Request, gameID string) {
	var req TurnRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnBody)).Decode(&req); err != nil {
		writeInvalidRequest(w, "invalid request body")
		return
	}
	if err := req.validate(); err != nil {
		writeInvalidRequest(w, err.Error())
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
		var limited *harness.ErrRateLimitedUntil
		if errors.As(err, &limited) {
			seconds := int(limited.RetryAfter().Seconds()) + 1
			w.Header().Set("Retry-After", strconv.Itoa(seconds))
			writeJSONError(w, http.StatusTooManyRequests, limited.Error())
			return
		}
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
		event := TurnEvent{Type: "error", Message: err.Error()}
		if failure, ok := harness.FailureFrom(err); ok {
			event.Code = string(failure.Code)
			event.Detail = failure.Message
			event.Failure = failure
		}
		_ = writeEvent(event)
	}
}

// handleResolveCheck streams the resolution of a turn's pending check as a
// continuation turn. Status codes are chosen before the first byte, mirroring
// handleTurnSubmit: 404 for an unknown turn, 400 for no or mismatched pending
// check, and 409 for a turn already in flight.
func (s *Server) handleResolveCheck(w http.ResponseWriter, r *http.Request, gameID string, turnNumber int) {
	var req ResolveCheckRequestDTO
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnBody)).Decode(&req); err != nil {
		writeInvalidRequest(w, "invalid request body")
		return
	}

	session, pendingRef, err := s.service.BeginResolveCheck(gameID, turnNumber, req)
	switch {
	case errors.Is(err, ErrTurnInFlight):
		w.Header().Set("Retry-After", "1")
		http.Error(w, err.Error(), http.StatusConflict)
		return
	case errors.Is(err, fs.ErrNotExist):
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	case errors.Is(err, ErrNoPendingCheck), errors.Is(err, ErrPendingCheckMismatch):
		writeInvalidRequest(w, err.Error())
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

	err = session.Run(r.Context(), TurnRequest{
		Mode:            "roll",
		Input:           req.Note,
		PendingCheckRef: pendingRef,
		ForcedTotal:     req.ManualResult,
	}, writeEvent)
	if err != nil {
		event := TurnEvent{Type: "error", Message: err.Error()}
		if failure, ok := harness.FailureFrom(err); ok {
			event.Code = string(failure.Code)
			event.Detail = failure.Message
			event.Failure = failure
		}
		_ = writeEvent(event)
	}
}

func (s *Server) handleModelsRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/models")
	path = strings.TrimPrefix(path, "/")

	if path == "" && r.Method == http.MethodGet {
		statuses := s.service.GetModelsStatus()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(statuses)
		return
	}

	if path == "events" && r.Method == http.MethodGet {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		ch := s.service.SubscribeModelEvents()
		defer s.service.UnsubscribeModelEvents(ch)

		for {
			select {
			case <-r.Context().Done():
				return
			case status, ok := <-ch:
				if !ok {
					return
				}
				data, _ := json.Marshal(status)
				_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			}
		}
	}

	parts := strings.Split(path, "/")
	if len(parts) == 2 && parts[1] == "download" && r.Method == http.MethodPost {
		modelID := parts[0]
		if err := s.service.DownloadModel(context.WithoutCancel(r.Context()), modelID); err != nil {
			if errors.Is(err, models.ErrDownloadActive) {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "downloading"})
		return
	}

	http.NotFound(w, r)
}

// handleExportRoutes serves the story-export API. Progress streams over SSE,
// mirroring the model-download events.
func (s *Server) handleExportRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/export")
	path = strings.TrimPrefix(path, "/")

	if path == "capabilities" && r.Method == http.MethodGet {
		writeJSON(w, s.service.ExportCapabilities())
		return
	}

	if path == "events" && r.Method == http.MethodGet {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		ch := s.service.SubscribeExportEvents()
		defer s.service.UnsubscribeExportEvents(ch)

		for {
			select {
			case <-r.Context().Done():
				return
			case event, ok := <-ch:
				if !ok {
					return
				}
				data, _ := json.Marshal(event)
				_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			}
		}
	}

	if path == "" && r.Method == http.MethodPost {
		var req ExportRequestDTO
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		job, err := s.service.StartExport(r.Context(), req)
		switch {
		case errors.Is(err, ErrExportInFlight):
			http.Error(w, err.Error(), http.StatusConflict)
			return
		case errors.Is(err, ErrExportFormat), errors.Is(err, ErrExportDirRequired):
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		case err != nil:
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(job)
		return
	}

	if path != "" && r.Method == http.MethodDelete {
		s.service.CancelExport(path)
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "cancelling"})
		return
	}

	http.NotFound(w, r)
}

// handleGenerateAssetPreview serves POST /api/generate-asset-preview. It renders
// image bytes from inline form metadata and writes them back without touching
// disk.
func (s *Server) handleGenerateAssetPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req GenerateAssetPreviewRequestDTO
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024*1024)).Decode(&req); err != nil {
		writeInvalidRequest(w, "invalid request body")
		return
	}
	if req.Kind != "banner" && req.Kind != "icon" {
		writeInvalidRequest(w, "kind must be 'banner' or 'icon'")
		return
	}
	data, contentType, err := s.service.GenerateAssetPreview(r.Context(), req)
	if err != nil {
		if writeGenerationFailure(w, err) {
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(data)
}

// setClipHeaders marks a synthesized clip as cacheable. The clip URL already
// carries the audio cache key, so a changed voice or text is a new URL; the ETag
// is derived from the bytes so a regenerated clip on the same URL is not reused.
func setClipHeaders(w http.ResponseWriter, data []byte) {
	sum := sha256.Sum256(data)
	w.Header().Set("ETag", fmt.Sprintf(`"%x"`, sum[:8]))
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Header().Set("Content-Type", media.AudioContentType(data))
}

func (s *Server) handleDocsRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/docs")
	path = strings.Trim(path, "/")

	if path == "" {
		docs, err := s.service.GetDocsList(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, docs)
		return
	}

	article, err := s.service.GetDocArticle(r.Context(), path)
	if err != nil {
		if errors.Is(err, ErrDocNotFound) {
			http.Error(w, "document not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, article)
}

func (s *Server) handleContentExportRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req ExportContentRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.ID == "" || (req.Type != "world" && req.Type != "system") {
		http.Error(w, "type and id are required", http.StatusBadRequest)
		return
	}

	var buf bytes.Buffer
	m, err := s.service.ExportContent(r.Context(), req.Type, req.ID, &buf)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ext := ".lrpgworld"
	if req.Type == "system" {
		ext = ".lrpgsystem"
	}

	if req.TargetPath != "" {
		cleanTarget, err := pathutil.ValidateUserPath(req.TargetPath)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid target path: %v", err), http.StatusBadRequest)
			return
		}
		absTarget, err := filepath.Abs(cleanTarget)
		if err != nil {
			http.Error(w, fmt.Sprintf("resolve target path: %v", err), http.StatusBadRequest)
			return
		}

		targetExt := strings.ToLower(filepath.Ext(absTarget))
		if targetExt == "" {
			absTarget += ext
		} else if targetExt != ext && targetExt != ".lrpgpack" {
			http.Error(w, fmt.Sprintf("target path %q has invalid extension for %s (expected %s or .lrpgpack)", absTarget, req.Type, ext), http.StatusBadRequest)
			return
		}

		if err := os.MkdirAll(filepath.Dir(absTarget), 0o755); err != nil {
			http.Error(w, fmt.Sprintf("create destination directory: %v", err), http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(absTarget, buf.Bytes(), 0o644); err != nil {
			http.Error(w, fmt.Sprintf("write package file: %v", err), http.StatusInternalServerError)
			return
		}

		writeJSON(w, ExportContentResultDTO{
			Path:    absTarget,
			ID:      m.ID,
			Version: m.Version,
			Type:    req.Type,
		})
		return
	}

	filename := fmt.Sprintf("%s-%s%s", m.ID, m.Version, ext)
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

func (s *Server) handleContentImportRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	onConflict := r.URL.Query().Get("on_conflict")

	r.Body = http.MaxBytesReader(w, r.Body, 510*1024*1024)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "failed to parse multipart form: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	if onConflict == "" {
		onConflict = r.FormValue("on_conflict")
	}

	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing 'file' in multipart form", http.StatusBadRequest)
		return
	}
	defer file.Close()

	expectedType := r.URL.Query().Get("expected_type")
	if expectedType == "" {
		expectedType = r.FormValue("expected_type")
	}

	var filename string
	if fileHeader != nil {
		filename = fileHeader.Filename
	}

	res, err := s.service.ImportContentWithOptions(r.Context(), file, ImportContentOptions{
		ConflictMode: onConflict,
		Filename:     filename,
		ExpectedType: expectedType,
	})
	if err != nil {
		if errors.Is(err, ErrContentConflict) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, res)
}

func (s *Server) handleRegistrySearchRoute(w http.ResponseWriter, r *http.Request) {
	s.service.HandleRegistrySearch(w, r)
}

func (s *Server) handleRegistryInstallRoute(w http.ResponseWriter, r *http.Request) {
	s.service.HandleRegistryInstall(w, r)
}

func (s *Server) handleRegistryUpdatesRoute(w http.ResponseWriter, r *http.Request) {
	s.service.HandleRegistryUpdates(w, r)
}

