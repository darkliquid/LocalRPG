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
