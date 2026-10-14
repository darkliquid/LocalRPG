// Package scene defines the narrative beat and scene model for campaign playback
// and story export.
//
// The package compiles turns, dialogue spans, and character actions into an ordered
// sequence of presentation beats (Beat in scene.go, compile.go). Each beat binds
// text, spoken audio paths, speaker portraits, location backgrounds, pacing durations,
// and visual effects (pan, zoom, dissolves). It serves as the single authoritative
// playback model consumed by both the interactive Story Theater (theater.go) and the
// offline video exporter.
//
// What scene is NOT:
//   - It does NOT synthesize audio or generate image assets; asset generation is
//     handled by pkg/media.
//   - It does NOT encode video files or generate static HTML export pages; final media
//     export belongs to pkg/export.
//   - It does NOT advance campaign turn state or record history; that belongs to
//     pkg/engine.
package scene
