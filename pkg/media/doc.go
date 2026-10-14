// Package media manages audio synthesis, procedural graphics, and visual asset
// generation for campaigns.
//
// This package orchestrates text-to-speech (TTS) synthesis (tts.go), voice catalog
// resolution, speech cue segmentation (speakable.go, sentence.go), and speech-to-text
// (STT) transcription (stt.go). It also houses the procedural art pipeline
// (procedural_art.go, procedural_portrait.go, procedural_scene.go), image generation
// request coordination, caching (cache.go), and style packs (style_pack.go).
//
// What media is NOT:
//   - It does NOT assemble timeline scenes or pacing models; compiling beats and
//     visual sequences belongs to pkg/scene.
//   - It does NOT encode or mux finished video containers (MP4/WebM); that is the
//     responsibility of pkg/export and subpackages pkg/media/webm and pkg/media/opus.
//   - It does NOT implement direct network provider protocols; concrete HTTP/CLI
//     adapters live in pkg/provider/*.
//   - It does NOT manage UI presentation or audio playback in the desktop webview;
//     desktop playback belongs to pkg/media/playback and pkg/gui.
package media
