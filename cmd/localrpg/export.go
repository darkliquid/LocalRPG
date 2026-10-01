package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/export"
	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/scene"
)

func handleExportCommand(args []string) {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: localrpg export <web|video> <game-id> [flags]")
		fmt.Fprintln(os.Stderr, "Export the campaign as an animated web bundle or a video")
		fs.PrintDefaults()
	}
	out := fs.String("out", "", "Output path for export")
	dir := fs.String("dir", ".", "Root directory")
	noArt := fs.Bool("no-art", false, "Skip scene imagery")
	noAudio := fs.Bool("no-audio", false, "Skip speech clips, using only cached audio")
	still := fs.Bool("still", false, "Render one frame per beat instead of animating")
	fps := fs.Int("fps", scene.DefaultFPS, "Video frame rate")
	size := fs.String("size", "1920x1080", "Video size as WxH")
	quality := fs.Int("quality", 80, "VP8 quality, 0-100")
	effort := fs.Int("effort", export.DefaultEffort, "Encoder effort, 0-6 (higher is slower and cleaner)")
	intro := fs.Duration("intro", 1500*time.Millisecond, "Hold the opening picture this long before the story starts")
	outro := fs.Duration("outro", 1500*time.Millisecond, "Hold the closing picture this long after the story ends")
	gap := fs.Duration("gap", 300*time.Millisecond, "Hold every segment this much longer than the script paces it")
	showProgress := fs.Bool("progress", false, "Show a live progress bar while rendering")

	if len(args) == 0 {
		fs.Usage()
		os.Exit(0)
	}

	// Go's flag package stops at the first positional argument, which would read
	// `export video --dir x <game>` as the game id "--dir". Separate the two so a
	// flag works wherever it is written.
	flags, positional := splitExportArgs(fs, args)
	_ = fs.Parse(flags)

	if len(positional) == 0 {
		fs.Usage()
		os.Exit(0)
	}

	format := positional[0]
	if format == "inspect" {
		if len(positional) < 2 {
			fmt.Fprintln(os.Stderr, "Error: missing bundle path")
			os.Exit(1)
		}
		handleExportInspect(positional[1])
		return
	}
	if len(positional) < 2 {
		fmt.Fprintln(os.Stderr, "Error: missing game ID")
		fs.Usage()
		os.Exit(1)
	}
	gameID := positional[1]

	compiler := export.NewScriptCompiler(*dir)
	compiler.SetMedia(!*noArt, !*noAudio)

	script, err := compiler.Compile(context.Background(), gameID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to compile replay script: %v\n", err)
		os.Exit(1)
	}
	for _, miss := range compiler.SpeechMisses() {
		fmt.Fprintf(os.Stderr, "No clip for a beat: %s\n", miss)
	}
	if len(compiler.SpeechMisses()) > 0 {
		fmt.Fprintf(os.Stderr, "Looked for clips in %s; pass --dir if the app keeps its campaigns elsewhere.\n", compiler.CacheDir())
	}

	switch format {
	case "web":
		target := resolveOutPath(*out, gameID, "-web.html")
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Web export failed: %v\n", err)
			os.Exit(1)
		}
		exporter := export.NewWebExporter(*dir)
		// A bundle is the theatre's own player, so it ships the frontend build and
		// renders performance tags the way the campaign's settings ask for.
		assets, assetErr := gui.AssetFS()
		if assetErr != nil {
			fmt.Fprintf(os.Stderr, "Web export failed: %v\n", assetErr)
			os.Exit(1)
		}
		exporter.SetAssets(assets)
		if cfg, cfgErr := config.NewConfigManager().Load(); cfgErr == nil && cfg != nil {
			exporter.SetDisplayMode(cfg.Media.TTS.SpeechCues.DisplayMode)
		}
		path, err := exporter.Export(context.Background(), script, target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Web export failed: %v\n", err)
			os.Exit(1)
		}
		if info, statErr := os.Stat(path); statErr == nil {
			fmt.Printf("Exported web replay bundle to %s (%d KB)\n", absPath(path), info.Size()/1024)
			break
		}
		fmt.Printf("Exported web replay bundle to %s\n", absPath(path))

	case "video":
		target := resolveOutPath(*out, gameID, ".webm")
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Video export failed: %v\n", err)
			os.Exit(1)
		}
		pipeline := export.NewVideoPipeline(*dir)
		pipeline.SetStill(*still)
		pipeline.SetFPS(*fps)
		pipeline.SetQuality(*quality)
		pipeline.SetEffort(*effort)
		pipeline.SetIntro(*intro)
		pipeline.SetOutro(*outro)
		pipeline.SetGap(*gap)
		if cfg, cfgErr := config.NewConfigManager().Load(); cfgErr == nil && cfg != nil {
			pipeline.SetDisplayMode(scene.DisplayMode(cfg.Media.TTS.SpeechCues.DisplayMode))
		}
		if width, height, err := parseSize(*size); err == nil {
			pipeline.SetSize(width, height)
		} else {
			// A bad geometry is not worth abandoning a long render for, so the
			// default resolution stands and the reason is reported.
			fmt.Fprintf(os.Stderr, "export: ignoring --size %q: %v\n", *size, err)
		}

		// Progress is opt-in. A render can take a while, but a quiet default is
		// what a script wants, so --progress draws a live bar instead.
		if *showProgress {
			pipeline.SetProgress(renderProgressBar)
		}

		if err := pipeline.RenderVideo(context.Background(), script, target); err != nil {
			fmt.Fprintf(os.Stderr, "Video render failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Rendered video replay to %s\n", absPath(target))

	default:
		fmt.Fprintf(os.Stderr, "Unknown export format: %s (supported: web, video)\n", format)
		os.Exit(1)
	}
}

// splitExportArgs separates flags from positional arguments, so a flag may be
// written before or after the format and game id. Go's flag package stops at the
// first positional argument, which would otherwise read `export video --dir x
// <game>` as the game id "--dir".
func splitExportArgs(fs *flag.FlagSet, args []string) (flags, positional []string) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positional = append(positional, arg)
			continue
		}
		flags = append(flags, arg)

		name := strings.TrimLeft(arg, "-")
		if strings.Contains(name, "=") {
			continue // the value is attached
		}
		f := fs.Lookup(name)
		if f == nil {
			continue // let flag.Parse report the unknown flag
		}
		if boolFlag, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && boolFlag.IsBoolFlag() {
			continue // a bool flag takes no value
		}
		if i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	return flags, positional
}

// absPath reports where a file landed, so a relative default is not a mystery.
func absPath(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return absolute
}

// resolveOutPath turns --out into a file path. An empty value names the file
// after the game in the working directory; a value that is an existing directory
// (or ends with a separator) gets the same name inside it.
func resolveOutPath(out, gameID, ext string) string {
	out = strings.TrimSpace(out)
	if out == "" {
		return gameID + ext
	}
	if info, err := os.Stat(out); err == nil && info.IsDir() {
		return filepath.Join(out, gameID+ext)
	}
	if strings.HasSuffix(out, string(os.PathSeparator)) {
		return filepath.Join(out, gameID+ext)
	}
	return out
}

// renderProgressBar draws a single line that overwrites itself, so an export
// says where it is without filling a terminal. It is only installed when
// --progress is passed.
func renderProgressBar(p scene.Progress) {
	switch p.Phase {
	case "encode":
		// Transient: the done line follows immediately.
		return
	case "done":
		fmt.Fprintf(os.Stderr, "\r%s\n", formatProgress(p, progressBarWidth))
	default:
		fmt.Fprintf(os.Stderr, "\r%s", formatProgress(p, progressBarWidth))
	}
}

// progressBarWidth is the number of cells the bar itself occupies.
const progressBarWidth = 20

// formatProgress is the bar's line: how far through, how many frames were drawn
// against how many repeated the one before, how much audio was muxed, and how
// long it has run.
func formatProgress(p scene.Progress, barWidth int) string {
	percent := 0
	if p.Total > 0 {
		percent = p.Done * 100 / p.Total
	}
	if percent > 100 {
		percent = 100
	}
	filled := percent * barWidth / 100
	bar := strings.Repeat("=", filled)
	if filled < barWidth {
		bar += ">" + strings.Repeat(" ", barWidth-filled-1)
	}

	line := fmt.Sprintf("export %3d%% [%s] %d/%d frames (%d new, %d repeat)",
		percent, bar, p.Frames, p.Total, p.ImageFrames, p.RepeatFrames)
	if p.TotalAudioPackets > 0 {
		line += fmt.Sprintf(" %d/%d audio %s/%s",
			p.AudioPackets, p.TotalAudioPackets,
			humanSize(p.AudioBytes), humanSize(p.TotalAudioBytes))
	}
	if p.Elapsed > 0 {
		line += fmt.Sprintf(" %s", p.Elapsed.Round(time.Second))
	}
	return line
}

// humanSize reports a byte count the way a person reads it.
func humanSize(size int64) string {
	switch {
	case size >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(size)/(1<<20))
	case size >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(size)/(1<<10))
	default:
		return fmt.Sprintf("%d B", size)
	}
}

// parseSize reads a WxH geometry.
func parseSize(value string) (int, int, error) {
	parts := strings.SplitN(strings.ToLower(strings.TrimSpace(value)), "x", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("expected WxH")
	}

	width, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, err
	}
	height, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, err
	}
	if width < 16 || height < 16 {
		return 0, 0, fmt.Errorf("dimensions must be at least 16x16")
	}
	return width, height, nil
}

// handleExportInspect reports what a bundle's beats can play, so a bundle a browser refuses
// can be diagnosed without the app that made it.
func handleExportInspect(path string) {
	beats, err := export.InspectBundle(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Inspect failed: %v\n", err)
		os.Exit(1)
	}

	var clips, playable int
	var problems int
	for index, beat := range beats {
		speaker := beat.Speaker
		if speaker == "" {
			speaker = "-"
		}
		line := fmt.Sprintf("%3d %-10s %-16s clips %d", index+1, beat.Kind, speaker, len(beat.Clips))
		for _, clip := range beat.Clips {
			clips++
			if clip.Complete && clip.Decodable {
				playable++
				line += fmt.Sprintf(" [%d bytes, plays]", clip.Bytes)
				continue
			}
			problems++
			line += fmt.Sprintf(" [%d bytes, UNPLAYABLE: %s]", clip.Bytes, clip.Problem)
		}
		fmt.Println(line)
	}

	fmt.Printf("\n%d beats, %d clips, %d playable, %d unplayable\n", len(beats), clips, playable, problems)
	if problems > 0 {
		os.Exit(1)
	}
}
