package debugger

import (
	"encoding/json"
	"net/http"
	"sync"
)

// Server provides the real-time debugger dashboard and JSON API.
type Server struct {
	addr       string
	collector  *Collector
	correlator *Correlator
	mux        *http.ServeMux
	mu         sync.RWMutex
	actions    []ActionRecord
}

// NewServer builds the HTTP debugger server.
func NewServer(collector *Collector, addr string) *Server {
	s := &Server{
		addr:       addr,
		collector:  collector,
		correlator: NewCorrelator(collector),
		mux:        http.NewServeMux(),
		actions:    make([]ActionRecord, 0),
	}
	s.registerRoutes()
	return s
}

// Handler returns the HTTP handler for the server.
func (s *Server) Handler() http.Handler {
	return s.mux
}

// RecordAction adds or updates an action record and enriches it with telemetry.
func (s *Server) RecordAction(rec ActionRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()

	enriched := s.correlator.EnrichAction(rec, nil)
	for i, existing := range s.actions {
		if existing.ID == enriched.ID {
			s.actions[i] = enriched
			return
		}
	}
	s.actions = append(s.actions, enriched)
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("/api/actions", s.handleActions)
	s.mux.HandleFunc("/api/actions/", s.handleActionDetail)
	s.mux.HandleFunc("/api/spans", s.handleSpans)
	s.mux.HandleFunc("/", s.handleDashboardUI)
}

func (s *Server) getEffectiveActions() []ActionRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]ActionRecord, len(s.actions))
	copy(result, s.actions)

	registeredTraceIDs := make(map[string]bool)
	for _, a := range result {
		if a.RootTraceID != "" {
			registeredTraceIDs[a.RootTraceID] = true
		}
	}

	if s.collector != nil {
		allSpans := s.collector.GetSpans()
		for _, span := range allSpans {
			if (span.Name == "turn" || span.Name == "POST /api/game/{id}/turn") && !registeredTraceIDs[span.TraceID] {
				registeredTraceIDs[span.TraceID] = true

				traceSpans := s.collector.FindSpansByTraceID(span.TraceID)
				status := "passed"
				if span.Status == "Error" || span.Status == "ERROR" || span.Attributes["turn.failure_code"] != "" || span.Attributes["localrpg.generation.failure_code"] != "" {
					status = "failed"
				}

				turnLabel := "turn"
				if num := span.Attributes["turn.number"]; num != "" {
					turnLabel = "turn #" + num
				}
				gameID := span.Attributes["game.id"]
				mode := span.Attributes["turn.mode"]

				synthetic := ActionRecord{
					ID:            span.SpanID,
					StepIndex:     len(result) + 1,
					ActionType:    turnLabel,
					Selector:      gameID,
					InputData:     mode,
					Timestamp:     span.StartTime,
					DurationMs:    span.Duration.Milliseconds(),
					RootTraceID:   span.TraceID,
					Status:        status,
					FailureReason: span.Attributes["turn.failure_code"],
					Spans:         traceSpans,
					Diagnostics:   extractDiagnostics(span, traceSpans),
				}
				result = append(result, synthetic)
			}
		}
	}

	return result
}

func (s *Server) handleActions(w http.ResponseWriter, r *http.Request) {
	actions := s.getEffectiveActions()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(actions)
}

func (s *Server) handleActionDetail(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/api/actions/"):]
	actions := s.getEffectiveActions()
	for _, a := range actions {
		if a.ID == id {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(a)
			return
		}
	}
	http.NotFound(w, r)
}

func (s *Server) handleSpans(w http.ResponseWriter, r *http.Request) {
	traceID := r.URL.Query().Get("trace_id")
	actionID := r.URL.Query().Get("action_id")

	var spans []SpanSummary
	if traceID != "" {
		spans = s.collector.FindSpansByTraceID(traceID)
	} else if actionID != "" {
		spans = s.collector.FindSpansByActionID(actionID)
	} else {
		spans = s.collector.GetSpans()
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(spans)
}

func (s *Server) handleDashboardUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(DashboardHTML))
}

const DashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>LocalRPG Debugger &amp; Action Correlator</title>
<style>
body { font-family: monospace; background: #0c0a09; color: #f5f5f4; margin: 0; display: flex; height: 100vh; }
#left { width: 340px; border-right: 1px solid #292524; display: flex; flex-direction: column; }
#header { padding: 12px; font-weight: bold; border-bottom: 1px solid #292524; background: #1c1917; color: #c084fc; }
#actions { flex: 1; overflow-y: auto; }
.action-item { padding: 10px; border-bottom: 1px solid #1c1917; cursor: pointer; }
.action-item:hover { background: #1c1917; }
.passed { color: #4ade80; }
.failed { color: #f87171; }
#right { flex: 1; display: flex; flex-direction: column; overflow: hidden; }
#detail-tabs { display: flex; background: #1c1917; border-bottom: 1px solid #292524; }
.tab { padding: 10px 16px; cursor: pointer; color: #a8a29e; }
.tab.active { color: #c084fc; font-weight: bold; border-bottom: 2px solid #a855f7; }
#detail-content { flex: 1; padding: 16px; overflow: auto; white-space: pre-wrap; word-break: break-all; }
</style>
</head>
<body>
<div id="left">
  <div id="header">LocalRPG Test Actions</div>
  <div id="actions"></div>
</div>
<div id="right">
  <div id="detail-tabs">
    <div class="tab active" onclick="showTab('spans')">Spans Waterfall</div>
    <div class="tab" onclick="showTab('prompt')">Prompt &amp; LLM</div>
    <div class="tab" onclick="showTab('raw')">Raw JSON</div>
  </div>
  <div id="detail-content">Select an action to inspect correlated telemetry.</div>
</div>
<script>
let actions = [];
let currentAction = null;
let currentTab = 'spans';

async function refresh() {
  const res = await fetch('/api/actions');
  actions = await res.json();
  const list = document.getElementById('actions');
  list.innerHTML = actions.map((a, i) => ` + "`" + `
    <div class="action-item" onclick="selectAction(${i})">
      <span class="${a.status}">[${a.status.toUpperCase()}]</span> <b>${a.action_type}</b> ${a.selector || ''} (${a.duration_ms}ms)
    </div>
  ` + "`" + `).join('');
}

function selectAction(index) {
  currentAction = actions[index];
  renderDetail();
}

function showTab(tab) {
  currentTab = tab;
  document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
  event.target.classList.add('active');
  renderDetail();
}

function renderDetail() {
  if (!currentAction) return;
  const out = document.getElementById('detail-content');
  if (currentTab === 'spans') {
    out.textContent = JSON.stringify(currentAction.spans || [], null, 2);
  } else if (currentTab === 'prompt') {
    out.textContent = currentAction.diagnostics ? 
      "=== ASSEMBLED PROMPT ===\n" + (currentAction.diagnostics.assembled_prompt || "N/A") + 
      "\n\n=== RAW COMPLETION ===\n" + (currentAction.diagnostics.raw_completion || "N/A") +
      "\n\n=== FAILURE CODE ===\n" + (currentAction.diagnostics.failure_code || "N/A") : "No turn diagnostics recorded.";
  } else {
    out.textContent = JSON.stringify(currentAction, null, 2);
  }
}

setInterval(refresh, 1000);
refresh();
</script>
</body>
</html>`
