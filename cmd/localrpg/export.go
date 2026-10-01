package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

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

	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fs.Usage()
		os.Exit(0)
	}

	format := args[0]
	if format == "inspect" {
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Error: missing bundle path")
			os.Exit(1)
		}
		handleExportInspect(args[1])
		return
	}
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "Error: missing game ID")
		fs.Usage()
		os.Exit(1)
	}
	gameID := args[1]
	fs.Parse(args[2:])

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

	switch format {
	case "web":
		target := *out
		if target == "" {
			target = fmt.Sprintf("dist/%s-web.html", gameID)
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
			fmt.Printf("Exported web replay bundle to %s (%d KB)\n", path, info.Size()/1024)
			break
		}
		fmt.Printf("Exported web replay bundle to %s\n", path)

	case "video":
		target := *out
		if target == "" {
			target = fmt.Sprintf("dist/%s.webm", gameID)
		}
		pipeline := export.NewVideoPipeline(*dir)
		pipeline.SetStill(*still)
		pipeline.SetFPS(*fps)
		pipeline.SetQuality(*quality)
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

		if err := pipeline.RenderVideo(context.Background(), script, target); err != nil {
			fmt.Fprintf(os.Stderr, "Video render failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Rendered video replay to %s\n", target)

	default:
		fmt.Fprintf(os.Stderr, "Unknown export format: %s (supported: web, video)\n", format)
		os.Exit(1)
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
