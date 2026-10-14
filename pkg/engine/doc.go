// Package engine coordinates campaign turn execution, session bootstrapping, and
// durable timeline persistence.
//
// The core orchestrator (TurnOrchestrator in orchestrator.go) drives player turn
// cycles: assembling prompt context, executing LLM streaming queries, evaluating
// mechanics checks, extracting entity mentions, and dispatching background tasks
// for portrait, scene art, and speech generation. The Timeline (timeline.go) acts
// as the authoritative single writer for durable campaign data on disk, writing
// append-only history (history.jsonl) and markdown entity files. Session (game.go)
// manages campaign lifecycle and bootstrapping.
//
// What engine is NOT:
//   - It is NOT the query index or full-text/vector search cache; that is the
//     responsibility of pkg/storage, which maintains a disposable, derived SQLite
//     database rebuildable from the engine's source files on disk.
//   - It is NOT a rules interpreter; game mechanics and script hooks are evaluated
//     in a sandboxed JavaScript runtime by pkg/rules.
//   - It does NOT manage LLM prompt budgets or provider routing; prompt assembly and
//     model abstractions belong to pkg/harness.
//   - It does NOT serve HTTP/WebSocket APIs or desktop UI; presentation and routing
//     live in pkg/gui.
package engine
