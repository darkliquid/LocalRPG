package export

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/scene"
)

// playerPage is the built player the bundle is made from, relative to the frontend
// build's root.
const playerPage = "player/player.html"

// WebExporter writes a self-contained player bundle: the theatre's own page plus the
// campaign's art, portraits, and clips, with nothing left to fetch.
type WebExporter struct {
	rootDir string
	assets  fs.FS
	// displayMode is the campaign's speech-cue display setting, baked into the bundle
	// so an export shows performance tags and stage directions the way the app does.
	displayMode string
}

// NewWebExporter builds an exporter rooted at a campaign directory.
func NewWebExporter(rootDir string) *WebExporter {
	return &WebExporter{rootDir: rootDir}
}

// SetAssets supplies the built player (the frontend's dist directory). Without it an
// export cannot produce a page anyone can watch, so Export refuses rather than
// writing a bundle that opens to nothing.
func (w *WebExporter) SetAssets(assets fs.FS) { w.assets = assets }

// SetDisplayMode records how the campaign renders performance tags.
func (w *WebExporter) SetDisplayMode(mode string) { w.displayMode = mode }

// webBeat is one beat as the player sees it: durations in seconds and paths relative
// to the bundle root. Audio is the beat's clips in play order, one per sentence.
type webBeat struct {
	Kind     string   `json:"kind"`
	Speaker  string   `json:"speaker,omitempty"`
	Text     string   `json:"text"`
	Art      string   `json:"art,omitempty"`
	Portrait string   `json:"portrait,omitempty"`
	Audio    []string `json:"audio,omitempty"`
	Duration float64  `json:"duration"`
	Player   bool     `json:"player,omitempty"`
}

type webScene struct {
	Location string    `json:"location,omitempty"`
	Art      string    `json:"art,omitempty"`
	Beats    []webBeat `json:"beats"`
}

type webPayload struct {
	GameName       string     `json:"game_name"`
	DisplayMode    string     `json:"display_mode,omitempty"`
	PlayerPortrait string     `json:"player_portrait,omitempty"`
	Scenes         []webScene `json:"scenes"`
	// Total is the script's own pacing, kept for a reader of the payload; the player
	// paces itself per beat.
	Total float64 `json:"total_duration"`
}

// Export writes a self-contained player bundle and returns its directory.
func (w *WebExporter) Export(ctx context.Context, script *scene.Script, outDir string) (string, error) {
	if script == nil || len(script.Scenes) == 0 {
		return "", fmt.Errorf("script has no scenes to export")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if w.assets == nil {
		return "", fmt.Errorf("web export needs the built player: run `mise run build:frontend`")
	}

	if err := os.MkdirAll(filepath.Join(outDir, "assets"), 0755); err != nil {
		return "", fmt.Errorf("create assets dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(outDir, "audio"), 0755); err != nil {
		return "", fmt.Errorf("create audio dir: %w", err)
	}

	// The player comes first: the bundle's index.html is that page with the story in
	// it, so a bundle and the app cannot drift apart.
	if err := w.copyPlayer(outDir); err != nil {
		return "", err
	}

	payload := &webPayload{
		GameName:    script.GameName,
		DisplayMode: w.displayMode,
		Total:       script.TotalDuration.Seconds(),
	}
	if script.PlayerPortrait != "" {
		if name, err := copyInto(outDir, "assets", "portrait-player", script.PlayerPortrait); err == nil {
			payload.PlayerPortrait = name
		}
	}

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
				Player:   beat.Player,
			}

			// A face travels with the bundle: the portrait the exporter resolved, or
			// nothing, which leaves the player to show the stage without one.
			if beat.PortraitPath != "" {
				key := beat.SpeakerID
				if key == "" {
					key = entity.Slugify(beat.Speaker)
				}
				if key != "" {
					if name, err := copyInto(outDir, "assets", "portrait-"+key, beat.PortraitPath); err == nil {
						jsBeat.Portrait = name
					}
				}
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

	page, err := w.bundlePage(payload)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(outDir, "index.html"), page, 0644); err != nil {
		return "", fmt.Errorf("write index.html: %w", err)
	}

	return outDir, nil
}

// copyPlayer copies the built player into the bundle, so the page the bundle opens is
// the page the app builds. The player's own entry document becomes index.html.
func (w *WebExporter) copyPlayer(outDir string) error {
	return fs.WalkDir(w.assets, "player", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || name == playerPage {
			return nil
		}

		relative := strings.TrimPrefix(name, "player/")
		data, err := fs.ReadFile(w.assets, name)
		if err != nil {
			return fmt.Errorf("read player asset %q: %w", name, err)
		}

		target := filepath.Join(outDir, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return fmt.Errorf("create %q: %w", filepath.Dir(target), err)
		}
		if err := os.WriteFile(target, data, 0644); err != nil {
			return fmt.Errorf("write %q: %w", target, err)
		}
		return nil
	})
}

// bundlePage is the built player page with the story inlined and the title named
// after the campaign. Module scripts are deferred, so a payload anywhere before the
// document ends is in place before the player runs.
func (w *WebExporter) bundlePage(payload *webPayload) ([]byte, error) {
	raw, err := fs.ReadFile(w.assets, playerPage)
	if err != nil {
		return nil, fmt.Errorf("read player page: %w", err)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode story: %w", err)
	}

	title := strings.TrimSpace(payload.GameName)
	if title == "" {
		title = "Story Theater"
	}

	page := string(raw)
	page = strings.Replace(page, "<title>Story Theater</title>", "<title>"+html.EscapeString(title)+"</title>", 1)
	page = strings.Replace(page, "</head>", fmt.Sprintf("<script>window.__LOCALRPG_STORY__ = %s;</script>\n</head>", encoded), 1)
	return []byte(page), nil
}

// copyInto copies a source file into a bundle directory under a chosen name and
// returns its path relative to the bundle root, so nothing refers outside the bundle.
func copyInto(outDir, dir, base, src string) (string, error) {
	name := base + filepath.Ext(src)
	if err := copyFile(src, filepath.Join(outDir, dir, name)); err != nil {
		return "", err
	}
	return path.Join(dir, name), nil
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
