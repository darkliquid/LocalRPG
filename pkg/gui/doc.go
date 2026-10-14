// Package gui implements the desktop GUI server, HTTP/WebSocket API daemon, and
// embedded web application distribution.
//
// The Server (server.go) and Service (service.go) expose the complete LocalRPG
// backend over HTTP REST routes (routes.go) and WebSocket streams for turn streaming,
// audio playback events, and asset generation. It enforces Cross-Origin / CSRF
// protections (middleware.go), serves the embedded Vite frontend single-page
// application (dist/), and hosts the embedded documentation catalog (docs.go).
//
// What gui is NOT:
//   - It is NOT the frontend user interface; the UI is a React/TypeScript application
//     located in the frontend/ directory.
//   - It does NOT implement game mechanics or campaign turn progression; core logic
//     is delegated to pkg/engine, pkg/rules, and pkg/storage.
//   - It does NOT implement LLM or media generation algorithms; these belong to
//     pkg/harness and pkg/media.
package gui
