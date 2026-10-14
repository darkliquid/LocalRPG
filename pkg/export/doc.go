// Package export produces standalone, distributable chronicle archives from
// campaign sessions, including static web pages and stitched video recordings.
//
// The exporter converts compiled scene beats into self-contained HTML chronicles
// (web.go) with embedded assets, as well as full-length video presentations (video.go).
// It synthesizes WebVTT subtitle tracks (captions.go), arranges chapter markers
// (chapters.go), manages storyboard pacing, and coordinates audio/video muxing.
//
// What export is NOT:
//   - It does NOT compile or pace the underlying scene presentation beats; that
//     model is produced by pkg/scene.
//   - It does NOT synthesize raw speech audio or render procedural character art;
//     asset generation belongs to pkg/media.
//   - It does NOT orchestrate live turns or modify campaign timelines; live session
//     management belongs to pkg/engine.
package export
