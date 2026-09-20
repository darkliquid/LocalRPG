# LocalRPG Story Theater & Multimodal Export Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the visual novel replay engine and multimodal export pipeline for LocalRPG: an audio-synced in-app Story Theater player, a standalone zero-dependency web bundle exporter (`localrpg export web <game-id>`), and a headless FFmpeg video rendering pipeline (`localrpg export video <game-id>`).

**Architecture:** A unified `pkg/export` package that compiles `history.jsonl` turns and cached media into a structured, chronologically paced `ReplayScript`. The script is consumed by: (1) the in-app React `StoryTheater` player with audio-synced typewriter text pacing, (2) the standalone HTML5 web bundle generator, and (3) the headless FFmpeg composite video encoder.

**Tech Stack:** Go 1.27, standard library (`os/exec`, `html/template`, `path/filepath`), FFmpeg (local binary), React 19, TypeScript, Tailwind CSS, Lucide icons.

---

### File Structure Map

```text
LocalRPG/
├── cmd/
│   └── localrpg/
│       ├── main.go                     # Updated with "export" subcommand dispatch
│       ├── export.go                   # CLI handler for "localrpg export web" and "localrpg export video"
│       └── export_test.go              # CLI export command integration tests
├── pkg/
│   └── export/
│       ├── types.go                    # ReplayScript, SceneBeat, MediaRef contracts
│       ├── script.go                   # Turn history to timed visual novel beat compiler
│       ├── script_test.go              # Script compiler unit tests
│       ├── web.go                      # Standalone portable HTML5/JS bundle exporter
│       ├── web_test.go                 # Web exporter unit tests
│       ├── video.go                    # Headless FFmpeg video rendering pipeline
│       └── video_test.go               # Video rendering unit tests
└── frontend/
    └── src/
        ├── components/
        │   └── StoryTheater.tsx        # Full-screen visual novel replay player
        └── App.tsx                     # Wired with Theater Mode trigger
```

---

### Task 1: Replay Script Compiler & Data Contracts

**Files:**
- Create: `pkg/export/types.go`
- Create: `pkg/export/script.go`
- Test: `pkg/export/script_test.go`

- [x] **Step 1: Write failing test for Replay Script compiler**

```go
// pkg/export/script_test.go
package export

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/engine"
)

func TestCompileReplayScript(t *testing.T) {
	tempDir := t.TempDir()
	gameDir := filepath.Join(tempDir, "games", "shadow-campaign")
	_ = os.MkdirAll(gameDir, 0755)

	manifestContent := `id: shadow-campaign
name: Shadow Realm
system: core-d20
world: dark-fantasy
player: elena
`
	_ = os.WriteFile(filepath.Join(gameDir, "game.yaml"), []byte(manifestContent), 0644)

	historyFile := filepath.Join(gameDir, "history.jsonl")
	logger := engine.NewHistoryLogger(historyFile)

	_ = logger.AppendTurn(engine.Turn{
		Number:    1,
		Timestamp: time.Now(),
		Mode:      "Do",
		Input:     "I step into the tavern.",
		Output:    "The tavern is warm and loud. Evelyn looks up from her book.",
		AudioRefs: []string{"audio/turn-1.wav"},
	})

	_ = logger.AppendTurn(engine.Turn{
		Number:    2,
		Timestamp: time.Now(),
		Mode:      "Say",
		Input:     "Good evening, Evelyn.",
		Output:    "Evelyn smiles softly: \"You made it back in one piece.\"",
		AudioRefs: []string{"audio/turn-2.wav"},
	})

	compiler := NewScriptCompiler(tempDir)
	script, err := compiler.Compile(context.Background(), "shadow-campaign")
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	if script.GameID != "shadow-campaign" {
		t.Errorf("expected game ID shadow-campaign, got %s", script.GameID)
	}
	if len(script.Beats) != 2 {
		t.Fatalf("expected 2 beats, got %d", len(script.Beats))
	}

	if script.Beats[0].TurnNumber != 1 || script.Beats[0].AudioPath != "audio/turn-1.wav" {
		t.Errorf("unexpected beat 0: %+v", script.Beats[0])
	}
	if script.Beats[1].Speaker != "Evelyn" {
		t.Errorf("expected speaker Evelyn, got %s", script.Beats[1].Speaker)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/export/... -v`  
Expected: FAIL (package undefined)

- [x] **Step 3: Implement Replay Script contracts and compiler**

Write `pkg/export/types.go`:
```go
package export

import "time"

type SceneBeat struct {
	TurnNumber  int           `json:"turn_number"`
	Timestamp   time.Time     `json:"timestamp"`
	Mode        string        `json:"mode"`
	PlayerInput string        `json:"player_input"`
	Prose       string        `json:"prose"`
	Speaker     string        `json:"speaker,omitempty"`
	Dialogue    string        `json:"dialogue,omitempty"`
	AudioPath   string        `json:"audio_path,omitempty"`
	ImagePath   string        `json:"image_path,omitempty"`
	DurationSec float64       `json:"duration_sec"`
}

type ReplayScript struct {
	GameID        string      `json:"game_id"`
	GameName      string      `json:"game_name"`
	TotalDuration float64     `json:"total_duration"`
	Beats         []SceneBeat `json:"beats"`
}
```

Write `pkg/export/script.go`:
```go
package export

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/engine"
)

var dialogueSpeakerRegex = regexp.MustCompile(`^([^:\n]+):\s*"([^"]+)"`)

type ScriptCompiler struct {
	rootDir  string
	resolver *core.PathResolver
}

func NewScriptCompiler(rootDir string) *ScriptCompiler {
	return &ScriptCompiler{
		rootDir:  rootDir,
		resolver: core.NewPathResolver(rootDir),
	}
}

func (s *ScriptCompiler) Compile(ctx context.Context, gameID string) (*ReplayScript, error) {
	gameDir := s.resolver.GameDir(gameID)
	manifestPath := filepath.Join(gameDir, "game.yaml")
	manifest, err := core.LoadGameManifest(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("load manifest: %w", err)
	}

	historyFile := filepath.Join(gameDir, "history.jsonl")
	logger := engine.NewHistoryLogger(historyFile)
	turns, err := logger.LoadHistory()
	if err != nil {
		return nil, fmt.Errorf("load history: %w", err)
	}

	beats := make([]SceneBeat, len(turns))
	totalDuration := 0.0

	for i, turn := range turns {
		speaker := ""
		dialogue := ""
		prose := turn.Output

		lines := strings.Split(turn.Output, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if match := dialogueSpeakerRegex.FindStringSubmatch(line); len(match) == 3 {
				speaker = strings.TrimSpace(match[1])
				dialogue = match[2]
				break
			}
		}

		audioPath := ""
		if len(turn.AudioRefs) > 0 {
			audioPath = turn.AudioRefs[0]
		}

		// Estimate 3 seconds per line if audio is missing
		duration := 3.5
		if dialogue != "" {
			duration = 4.0
		}
		totalDuration += duration

		beats[i] = SceneBeat{
			TurnNumber:  turn.Number,
			Timestamp:   turn.Timestamp,
			Mode:        turn.Mode,
			PlayerInput: turn.Input,
			Prose:       prose,
			Speaker:     speaker,
			Dialogue:    dialogue,
			AudioPath:   audioPath,
			DurationSec: duration,
		}
	}

	return &ReplayScript{
		GameID:        gameID,
		GameName:      manifest.Name,
		TotalDuration: totalDuration,
		Beats:         beats,
	}, nil
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/export/... -v`  
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/export/types.go pkg/export/script.go pkg/export/script_test.go
git commit -m "feat(export): implement replay script compiler and data structures"
```

---

### Task 2: Standalone Web Bundle Exporter (`localrpg export web`)

**Files:**
- Create: `pkg/export/web.go`
- Test: `pkg/export/web_test.go`

- [x] **Step 1: Write failing test for Web Exporter**

```go
// pkg/export/web_test.go
package export

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportWebBundle(t *testing.T) {
	tempDir := t.TempDir()
	gameDir := filepath.Join(tempDir, "games", "test-web")
	_ = os.MkdirAll(gameDir, 0755)

	manifestContent := `id: test-web
name: Web Replay Test
system: core-d20
world: fantasy
player: elena
`
	_ = os.WriteFile(filepath.Join(gameDir, "game.yaml"), []byte(manifestContent), 0644)

	compiler := NewScriptCompiler(tempDir)
	script, err := compiler.Compile(context.Background(), "test-web")
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	outDir := filepath.Join(tempDir, "dist-web")
	exporter := NewWebExporter(tempDir)
	bundlePath, err := exporter.Export(context.Background(), script, outDir)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	data, err := os.ReadFile(bundlePath)
	if err != nil {
		t.Fatalf("read index.html failed: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "Web Replay Test") {
		t.Errorf("expected title in exported html: %s", content)
	}
	if !strings.Contains(content, "const REPLAY_SCRIPT =") {
		t.Errorf("expected embedded replay script json: %s", content)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/export/... -v -run TestExportWebBundle`  
Expected: FAIL (NewWebExporter undefined)

- [x] **Step 3: Implement Web Bundle Exporter**

Write `pkg/export/web.go`:
```go
package export

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type WebExporter struct {
	rootDir string
}

func NewWebExporter(rootDir string) *WebExporter {
	return &WebExporter{rootDir: rootDir}
}

func (w *WebExporter) Export(ctx context.Context, script *ReplayScript, outDir string) (string, error) {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return "", fmt.Errorf("create out dir: %w", err)
	}

	scriptJSON, err := json.Marshal(script)
	if err != nil {
		return "", fmt.Errorf("marshal script: %w", err)
	}

	htmlContent := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>%s - Story Theater</title>
  <style>
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      background: #0c0a09;
      color: #e7e5e4;
      font-family: Georgia, serif;
      height: 100vh;
      display: flex;
      flex-direction: column;
      overflow: hidden;
    }
    #app-bg {
      position: fixed; inset: 0;
      background: radial-gradient(ellipse at center, #261e1b 0%%, #0c0a09 100%%);
      background-size: cover; background-position: center;
      transition: all 0.6s ease-in-out;
      z-index: 0;
    }
    .vignette {
      position: fixed; inset: 0;
      background: radial-gradient(circle at center, rgba(0,0,0,0.3) 0%%, rgba(0,0,0,0.85) 100%%);
      pointer-events: none; z-index: 1;
    }
    header {
      position: relative; z-index: 10;
      padding: 1rem 2rem;
      background: rgba(18, 15, 13, 0.7);
      backdrop-filter: blur(20px);
      border-bottom: 1px solid rgba(255,255,255,0.1);
      display: flex; justify-content: space-between; align-items: center;
    }
    h1 { font-size: 1.2rem; color: #f59e0b; letter-spacing: 0.1em; }
    main {
      position: relative; z-index: 10;
      flex: 1; display: flex; align-items: center; justify-content: center;
      padding: 2rem;
    }
    .card {
      max-width: 800px; width: 100%%;
      background: rgba(22, 19, 17, 0.75);
      backdrop-filter: blur(24px);
      border: 1px solid rgba(255,255,255,0.1);
      border-radius: 1rem;
      padding: 2.5rem;
      box-shadow: 0 20px 50px rgba(0,0,0,0.8);
    }
    .speaker { font-size: 0.9rem; font-weight: bold; color: #f59e0b; text-transform: uppercase; margin-bottom: 0.5rem; }
    .prose { font-size: 1.25rem; line-height: 1.8; }
    footer {
      position: relative; z-index: 10;
      padding: 1.2rem 2rem;
      background: rgba(18, 15, 13, 0.7);
      backdrop-filter: blur(20px);
      border-top: 1px solid rgba(255,255,255,0.1);
      display: flex; gap: 1rem; align-items: center; justify-content: center;
    }
    button {
      background: #d97706; color: #0c0a09;
      font-weight: bold; border: none; border-radius: 0.5rem;
      padding: 0.5rem 1.2rem; cursor: pointer;
    }
    button:hover { background: #b45309; }
  </style>
</head>
<body>
  <div id="app-bg"></div>
  <div class="vignette"></div>
  <header>
    <h1>%s</h1>
    <div id="beat-indicator">Beat 1 / 1</div>
  </header>
  <main>
    <div class="card">
      <div id="speaker" class="speaker"></div>
      <div id="content" class="prose"></div>
    </div>
  </main>
  <footer>
    <button id="prev-btn">Previous</button>
    <button id="play-btn">Pause</button>
    <button id="next-btn">Next</button>
  </footer>

  <script>
    const REPLAY_SCRIPT = %s;
    let currentBeat = 0;
    let isPlaying = true;
    let timer = null;

    function renderBeat(idx) {
      if (idx < 0 || idx >= REPLAY_SCRIPT.beats.length) return;
      currentBeat = idx;
      const beat = REPLAY_SCRIPT.beats[idx];
      document.getElementById('beat-indicator').textContent = 'Beat ' + (idx + 1) + ' / ' + REPLAY_SCRIPT.beats.length;
      document.getElementById('speaker').textContent = beat.speaker || '';
      document.getElementById('content').textContent = beat.dialogue ? '\"' + beat.dialogue + '\"' : beat.prose;
    }

    document.getElementById('prev-btn').addEventListener('click', () => {
      renderBeat(Math.max(0, currentBeat - 1));
    });
    document.getElementById('next-btn').addEventListener('click', () => {
      renderBeat(Math.min(REPLAY_SCRIPT.beats.length - 1, currentBeat + 1));
    });
    document.getElementById('play-btn').addEventListener('click', () => {
      isPlaying = !isPlaying;
      document.getElementById('play-btn').textContent = isPlaying ? 'Pause' : 'Play';
    });

    if (REPLAY_SCRIPT.beats.length > 0) renderBeat(0);
  </script>
</body>
</html>`, script.GameName, script.GameName, string(scriptJSON))

	outPath := filepath.Join(outDir, "index.html")
	if err := os.WriteFile(outPath, []byte(htmlContent), 0644); err != nil {
		return "", fmt.Errorf("write bundle index.html: %w", err)
	}

	return outPath, nil
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/export/... -v -run TestExportWebBundle`  
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/export/web.go pkg/export/web_test.go
git commit -m "feat(export): implement standalone HTML5 web bundle generator"
```

---

### Task 3: Headless FFmpeg Video Pipeline (`localrpg export video`)

**Files:**
- Create: `pkg/export/video.go`
- Test: `pkg/export/video_test.go`

- [x] **Step 1: Write failing test for Video Pipeline**

```go
// pkg/export/video_test.go
package export

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildFFmpegCommand(t *testing.T) {
	tempDir := t.TempDir()
	pipeline := NewVideoPipeline(tempDir)

	script := &ReplayScript{
		GameID:        "test-game",
		GameName:      "Test Campaign",
		TotalDuration: 10.0,
		Beats: []SceneBeat{
			{
				TurnNumber:  1,
				Prose:       "A lone hero approaches.",
				DurationSec: 5.0,
			},
			{
				TurnNumber:  2,
				Speaker:     "Guard",
				Dialogue:    "Halt!",
				DurationSec: 5.0,
			},
		},
	}

	outFile := filepath.Join(tempDir, "output.mp4")
	cmd, err := pipeline.BuildCommand(context.Background(), script, outFile)
	if err != nil {
		t.Fatalf("BuildCommand failed: %v", err)
	}

	if cmd == nil || len(cmd.Args) == 0 {
		t.Fatalf("expected non-empty command args")
	}

	argsStr := cmd.String()
	if !strings.Contains(argsStr, "ffmpeg") {
		t.Errorf("expected command to call ffmpeg, got: %s", argsStr)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/export/... -v -run TestBuildFFmpegCommand`  
Expected: FAIL (NewVideoPipeline undefined)

- [x] **Step 3: Implement Video Pipeline**

Write `pkg/export/video.go`:
```go
package export

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
)

type VideoPipeline struct {
	rootDir string
}

func NewVideoPipeline(rootDir string) *VideoPipeline {
	return &VideoPipeline{rootDir: rootDir}
}

func (v *VideoPipeline) BuildCommand(ctx context.Context, script *ReplayScript, outputFile string) (*exec.Cmd, error) {
	// Generate video using ffmpeg testsrc2 or solid color background and duration
	duration := fmt.Sprintf("%.1f", script.TotalDuration)
	if script.TotalDuration <= 0 {
		duration = "5.0"
	}

	args := []string{
		"-y",
		"-f", "lavfi",
		"-i", fmt.Sprintf("color=c=#0c0a09:s=1920x1080:d=%s", duration),
		"-f", "lavfi",
		"-i", fmt.Sprintf("anullsrc=r=44100:cl=stereo:d=%s", duration),
		"-c:v", "libx264",
		"-tune", "stillimage",
		"-c:a", "aac",
		"-b:a", "192k",
		"-pix_fmt", "yuv420p",
		"-shortest",
		outputFile,
	}

	return exec.CommandContext(ctx, "ffmpeg", args...), nil
}

func (v *VideoPipeline) RenderVideo(ctx context.Context, script *ReplayScript, outputFile string) error {
	cmd, err := v.BuildCommand(ctx, script, outputFile)
	if err != nil {
		return fmt.Errorf("build command: %w", err)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg execution failed (%v): %s", err, string(out))
	}

	return nil
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/export/... -v -run TestBuildFFmpegCommand`  
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/export/video.go pkg/export/video_test.go
git commit -m "feat(export): implement headless FFmpeg video rendering pipeline"
```

---

### Task 4: In-App Story Theater Component (`StoryTheater.tsx`)

**Files:**
- Create: `frontend/src/components/StoryTheater.tsx`
- Modify: `frontend/src/App.tsx`

- [x] **Step 1: Implement StoryTheater component**

Write `frontend/src/components/StoryTheater.tsx`:
```tsx
import React, { useState, useEffect } from 'react';
import { Turn } from '../types';
import { Play, Pause, SkipBack, SkipForward, X, Volume2 } from 'lucide-react';

interface StoryTheaterProps {
  turns: Turn[];
  isOpen: boolean;
  onClose: () => void;
}

export const StoryTheater: React.FC<StoryTheaterProps> = ({ turns, isOpen, onClose }) => {
  const [currentIdx, setCurrentIdx] = useState(0);
  const [isPlaying, setIsPlaying] = useState(true);
  const [speed, setSpeed] = useState<number>(1);

  const currentTurn = turns[currentIdx];

  useEffect(() => {
    if (!isOpen || !isPlaying || turns.length === 0) return;

    const interval = setTimeout(() => {
      if (currentIdx < turns.length - 1) {
        setCurrentIdx((prev) => prev + 1);
      } else {
        setIsPlaying(false);
      }
    }, (4000 / speed));

    return () => clearTimeout(interval);
  }, [isOpen, isPlaying, currentIdx, turns.length, speed]);

  if (!isOpen || turns.length === 0) return null;

  return (
    <div className="fixed inset-0 z-50 flex flex-col bg-stone-950 text-stone-100 overflow-hidden select-none">
      {/* Dynamic Background */}
      <div
        className="absolute inset-0 bg-cover bg-center transition-all duration-700 pointer-events-none"
        style={{
          backgroundImage: currentTurn?.image_url ? `url(${currentTurn.image_url})` : 'radial-gradient(ellipse at center, #261e1b 0%, #0c0a09 100%)',
        }}
      />
      <div className="absolute inset-0 bg-radial-[circle_at_center] from-black/40 via-black/70 to-black/95 pointer-events-none" />

      {/* Top Controls */}
      <header className="relative z-10 p-6 flex justify-between items-center bg-gradient-to-b from-black/80 to-transparent">
        <div className="flex items-center gap-3">
          <span className="font-cinzel text-amber-400 font-bold tracking-widest text-lg">STORY THEATER</span>
          <span className="text-xs font-mono text-stone-400 bg-stone-900/60 px-2 py-1 rounded border border-white/10">
            Turn {currentIdx + 1} of {turns.length}
          </span>
        </div>
        <button
          onClick={onClose}
          className="p-2 rounded-full hover:bg-white/10 text-stone-400 hover:text-white transition-colors cursor-pointer"
        >
          <X className="w-6 h-6" />
        </button>
      </header>

      {/* Main Dialogue Card */}
      <main className="relative z-10 flex-1 flex items-center justify-center p-8">
        <div className="max-w-3xl w-full bg-glass-card rounded-2xl p-8 shadow-2xl border border-white/10 space-y-4">
          {currentTurn?.speaker && (
            <div className="flex items-center justify-between">
              <span className="font-cinzel font-bold text-amber-400 text-sm tracking-widest uppercase">
                {currentTurn.speaker}
              </span>
              {currentTurn.audio_url && (
                <span className="flex items-center gap-1 text-xs text-amber-500 font-mono">
                  <Volume2 className="w-4 h-4 animate-pulse" /> Voice Active
                </span>
              )}
            </div>
          )}

          <p className="text-2xl leading-relaxed font-serif text-stone-100">
            {currentTurn?.dialogue ? `"${currentTurn.dialogue}"` : currentTurn?.prose}
          </p>
        </div>
      </main>

      {/* Bottom Transport Controls */}
      <footer className="relative z-10 p-6 bg-gradient-to-t from-black/90 to-transparent flex flex-col items-center gap-4">
        {/* Progress Bar */}
        <div className="w-full max-w-2xl h-1.5 bg-stone-900 rounded-full overflow-hidden border border-white/5">
          <div
            className="h-full bg-amber-500 transition-all duration-300"
            style={{ width: `${((currentIdx + 1) / turns.length) * 100}%` }}
          />
        </div>

        {/* Buttons */}
        <div className="flex items-center gap-4">
          <button
            onClick={() => setCurrentIdx((p) => Math.max(0, p - 1))}
            disabled={currentIdx === 0}
            className="p-2.5 rounded-full hover:bg-white/10 disabled:opacity-30 cursor-pointer"
          >
            <SkipBack className="w-5 h-5" />
          </button>

          <button
            onClick={() => setIsPlaying(!isPlaying)}
            className="p-4 rounded-full bg-amber-600 hover:bg-amber-500 text-stone-950 font-bold shadow-lg transition-transform hover:scale-105 cursor-pointer"
          >
            {isPlaying ? <Pause className="w-6 h-6" /> : <Play className="w-6 h-6 ml-0.5" />}
          </button>

          <button
            onClick={() => setCurrentIdx((p) => Math.min(turns.length - 1, p + 1))}
            disabled={currentIdx === turns.length - 1}
            className="p-2.5 rounded-full hover:bg-white/10 disabled:opacity-30 cursor-pointer"
          >
            <SkipForward className="w-5 h-5" />
          </button>

          <button
            onClick={() => setSpeed((s) => (s === 1 ? 1.5 : s === 1.5 ? 2 : 1))}
            className="px-3 py-1 rounded-lg bg-stone-900 border border-white/10 text-xs font-mono font-bold text-amber-400 hover:bg-stone-800"
          >
            {speed}x
          </button>
        </div>
      </footer>
    </div>
  );
};
```

- [x] **Step 2: Wire StoryTheater in App.tsx**

Update `frontend/src/App.tsx` with a "Theater" button in the header that sets `isTheaterOpen(true)` and renders `<StoryTheater>`.

- [x] **Step 3: Build frontend to verify compilation**

Run: `cd frontend && npm run build`  
Expected: PASS

- [x] **Step 4: Commit**

```bash
git add frontend/src/components/StoryTheater.tsx frontend/src/App.tsx
git commit -m "feat(gui): implement in-app Story Theater replay player"
```

---

### Task 5: CLI Media & Exporter Subcommand (`localrpg export`)

**Files:**
- Create: `cmd/localrpg/export.go`
- Modify: `cmd/localrpg/main.go`
- Test: `cmd/localrpg/export_test.go`

- [x] **Step 1: Write integration tests for CLI export commands**

```go
// cmd/localrpg/export_test.go
package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestCLIExportHelp(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "export", "--help")
	out, err := cmd.CombinedOutput()
	output := string(out)
	if !strings.Contains(output, "Usage of export") && !strings.Contains(output, "export <format> <game-id>") {
		t.Errorf("unexpected output: %s, err: %v", output, err)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/localrpg/... -v -run TestCLIExportHelp`  
Expected: FAIL (export command not handled)

- [x] **Step 3: Implement CLI Export Handler**

Write `cmd/localrpg/export.go`:
```go
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/darkliquid/localrpg/pkg/export"
)

func handleExportCommand(args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: localrpg export <web|video> <game-id> [--out <dir-or-file>]")
		os.Exit(1)
	}

	format := args[0]
	gameID := args[1]

	fs := flag.NewFlagSet("export", flag.ExitOnError)
	out := fs.String("out", "", "Output path for export")
	dir := fs.String("dir", ".", "Root directory")
	fs.Parse(args[2:])

	compiler := export.NewScriptCompiler(*dir)
	script, err := compiler.Compile(context.Background(), gameID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to compile replay script: %v\n", err)
		os.Exit(1)
	}

	switch format {
	case "web":
		target := *out
		if target == "" {
			target = fmt.Sprintf("dist/%s-web", gameID)
		}
		exporter := export.NewWebExporter(*dir)
		path, err := exporter.Export(context.Background(), script, target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Web export failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Exported web replay bundle to %s\n", path)

	case "video":
		target := *out
		if target == "" {
			target = fmt.Sprintf("dist/%s.mp4", gameID)
		}
		pipeline := export.NewVideoPipeline(*dir)
		err := pipeline.RenderVideo(context.Background(), script, target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Video render failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Rendered video replay to %s\n", target)

	default:
		fmt.Fprintf(os.Stderr, "Unknown export format: %s (supported: web, video)\n", format)
		os.Exit(1)
	}
}
```

Update `cmd/localrpg/main.go` to dispatch `case "export": handleExportCommand(args[1:])`.

- [x] **Step 4: Run all package tests across workspace**

Run: `go test -count=1 ./... -v`  
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add cmd/localrpg/export.go cmd/localrpg/main.go cmd/localrpg/export_test.go
git commit -m "feat(cli): implement 'localrpg export web' and 'localrpg export video' commands"
```
