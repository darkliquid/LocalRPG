package export

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"

	"github.com/darkliquid/localrpg/pkg/scene"
)

// WebExporter writes a self-contained animated player bundle.
type WebExporter struct {
	rootDir string
}

// NewWebExporter builds an exporter rooted at a campaign directory.
func NewWebExporter(rootDir string) *WebExporter {
	return &WebExporter{rootDir: rootDir}
}

// webBeat is one beat as the browser sees it: durations in seconds and paths
// relative to the bundle root. Audio is the beat's clips in play order, one per
// sentence of reduced text.
type webBeat struct {
	Kind     string   `json:"kind"`
	Speaker  string   `json:"speaker,omitempty"`
	Text     string   `json:"text"`
	Art      string   `json:"art,omitempty"`
	Audio    []string `json:"audio,omitempty"`
	Duration float64  `json:"duration"`
}

type webScene struct {
	Location string    `json:"location,omitempty"`
	Art      string    `json:"art,omitempty"`
	Beats    []webBeat `json:"beats"`
}

type webPayload struct {
	GameName string     `json:"game_name"`
	Scenes   []webScene `json:"scenes"`
	Total    float64    `json:"total_duration"`
}

// Export writes a self-contained animated player and returns its directory.
func (w *WebExporter) Export(ctx context.Context, script *scene.Script, outDir string) (string, error) {
	if script == nil || len(script.Scenes) == 0 {
		return "", fmt.Errorf("script has no scenes to export")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Join(outDir, "assets"), 0755); err != nil {
		return "", fmt.Errorf("create assets dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(outDir, "audio"), 0755); err != nil {
		return "", fmt.Errorf("create audio dir: %w", err)
	}

	payload := &webPayload{GameName: script.GameName, Total: script.TotalDuration.Seconds()}
	beatNumber := 0

	for i := range script.Scenes {
		sc := script.Scenes[i]
		entry := webScene{Location: sc.LocationName}

		if sc.ArtPath != "" {
			name := fmt.Sprintf("scene-%03d%s", i+1, filepath.Ext(sc.ArtPath))
			if err := copyFile(sc.ArtPath, filepath.Join(outDir, "assets", name)); err == nil {
				entry.Art = "assets/" + name
			}
		}

		for j := range sc.Beats {
			beat := sc.Beats[j]

			jsBeat := webBeat{
				Kind:     string(beat.Kind),
				Speaker:  beat.Speaker,
				Text:     beat.Text,
				Art:      entry.Art,
				Duration: beat.Duration.Seconds(),
			}

			for _, clip := range beat.AudioPaths {
				// Clips are numbered in the order they appear, so a bundle's audio
				// directory is a readable running order rather than beat numbers
				// with holes in them.
				beatNumber++
				name := fmt.Sprintf("beat-%04d%s", beatNumber, filepath.Ext(clip))
				if err := copyFile(clip, filepath.Join(outDir, "audio", name)); err == nil {
					jsBeat.Audio = append(jsBeat.Audio, "audio/"+name)
				}
			}

			entry.Beats = append(entry.Beats, jsBeat)
		}

		payload.Scenes = append(payload.Scenes, entry)
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}

	page := fmt.Sprintf(playerHTML, html.EscapeString(script.GameName), payloadJSON)
	if err := os.WriteFile(filepath.Join(outDir, "index.html"), []byte(page), 0644); err != nil {
		return "", fmt.Errorf("write index.html: %w", err)
	}

	return outDir, nil
}

// copyFile copies a bundle asset, so the export never depends on the original
// cache or campaign directory still existing.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

// playerHTML is the exported player. `%s` is the game name and `%s` the embedded
// script payload; it must reference nothing outside the bundle.
const playerHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>%s - Story Theater</title>
<style>
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body { background: #0c0a09; color: #e7e5e4; font-family: Georgia, serif; overflow: hidden; }
  #stage { position: fixed; inset: 0; background-size: cover; background-position: center; transition: opacity 300ms ease; }
  #scrim { position: fixed; inset: 0; background: linear-gradient(180deg, rgba(12,10,9,0.35), rgba(12,10,9,0.92)); }
  #card { position: relative; height: 100vh; display: flex; flex-direction: column; justify-content: center;
          align-items: center; padding: 6vh 8vw; text-align: center; gap: 1.5rem; }
  #speaker { font-size: 0.9rem; letter-spacing: 0.2em; text-transform: uppercase; color: #f59e0b; min-height: 1.2rem; }
  #text { font-size: clamp(1.25rem, 2.4vw, 2rem); line-height: 1.6; max-width: 46rem; }
  #scene { position: fixed; top: 1.5rem; left: 1.5rem; font-size: 0.8rem; letter-spacing: 0.2em;
           text-transform: uppercase; color: #a8a29e; }
  #controls { position: fixed; bottom: 1.25rem; left: 50%%; transform: translateX(-50%%); display: flex;
              gap: 0.75rem; align-items: center; }
  button { background: rgba(255,255,255,0.08); border: 1px solid rgba(255,255,255,0.15); color: #e7e5e4;
           border-radius: 999px; padding: 0.35rem 0.9rem; font: inherit; font-size: 0.85rem; cursor: pointer; }
  button:hover { border-color: #f59e0b; color: #fbbf24; }
  #progress { position: fixed; bottom: 0; left: 0; height: 3px; background: #f59e0b; width: 0; }
</style>
</head>
<body>
<div id="stage"></div>
<div id="scrim"></div>
<div id="scene"></div>
<div id="card">
  <div id="speaker"></div>
  <div id="text"></div>
</div>
<div id="controls">
  <button data-action="play">Play</button>
  <button data-action="prev">Previous</button>
  <button data-action="next">Next</button>
</div>
<div id="progress"></div>
<script>
const SCRIPT = %s;

const beats = [];
SCRIPT.scenes.forEach((scene) => scene.beats.forEach((beat) => beats.push({ ...beat, scene: scene.location })));

const stage = document.getElementById('stage');
const sceneLabel = document.getElementById('scene');
const speaker = document.getElementById('speaker');
const textEl = document.getElementById('text');
const progress = document.getElementById('progress');
const playButton = document.querySelector('[data-action="play"]');

const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;

let index = 0;
let timer = null;
let audio = null;
let paused = false;
// audioGeneration invalidates a beat's clip chain when the beat is left early.
let audioGeneration = 0;

// playClips plays a beat's clips in order, so a multi-sentence beat is heard in
// full rather than only its first sentence.
function playClips(clips) {
  const generation = ++audioGeneration;
  let nextClip = 0;

  const step = () => {
    if (generation !== audioGeneration || nextClip >= clips.length) return;
    const clip = new Audio(clips[nextClip++]);
    audio = clip;
    clip.onended = step;
    clip.onerror = step;
    clip.play().catch(() => { pause(); });
  };

  step();
}

function render(index) {
  const beat = beats[index];
  if (!beat) { finish(); return; }

  if (beat.art) stage.style.backgroundImage = 'url("' + beat.art + '")';
  sceneLabel.textContent = beat.scene || '';
  speaker.textContent = beat.kind === 'speech' ? (beat.speaker || 'UNKNOWN') : '';
  textEl.textContent = '';

  if (audio) { audio.pause(); audio = null; }
  if (beat.audio && beat.audio.length) playClips(beat.audio);

  startReveal(beat, performance.now());
}

function startReveal(beat, startedAt) {
  const revealMs = Math.max(1, beat.duration * 1000 * 0.6);
  const characters = Array.from(beat.text);

  const step = (now) => {
    if (paused) return;
    const elapsed = now - startedAt;
    const revealCount = reducedMotion ? characters.length : Math.ceil((elapsed / revealMs) * characters.length);
    textEl.textContent = characters.slice(0, Math.min(revealCount, characters.length)).join('');

    if (elapsed >= beat.duration * 1000) { next(); return; }
    timer = requestAnimationFrame(step);
  };

  timer = requestAnimationFrame(step);
}

function stop() {
  if (timer) cancelAnimationFrame(timer);
  timer = null;
  audioGeneration++;
  if (audio) { audio.pause(); audio = null; }
}

function schedule() {
  stop();
  const beat = beats[index];
  if (!beat) { finish(); return; }
  render(index);
  progress.style.width = ((index + 1) / beats.length * 100) + '%%';
}

function next() { index = Math.min(index + 1, beats.length); schedule(); }
function prev() { index = Math.max(index - 1, 0); schedule(); }
function finish() { playButton.textContent = 'Replay'; sceneLabel.textContent = ''; speaker.textContent = ''; }
function pause() { paused = true; stop(); playButton.textContent = 'Play'; }
function play() { paused = false; playButton.textContent = 'Pause'; schedule(); }

playButton.addEventListener('click', () => {
  if (paused) { play(); return; }
  if (index >= beats.length) { index = 0; play(); return; }
  pause();
});

document.querySelector('[data-action="next"]').addEventListener('click', next);
document.querySelector('[data-action="prev"]').addEventListener('click', prev);

// Browsers refuse to start audio without a gesture; render the first beat and
// wait for the click rather than running silently ahead.
render(0);
pause();
</script>
</body>
</html>
`
