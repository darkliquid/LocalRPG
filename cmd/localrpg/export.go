package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
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
		target := *out
		if target == "" {
			target = gameID + "-web.html"
		}
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
		target := *out
		if target == "" {
			target = gameID + ".webm"
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Video export failed: %v\n", err)
			os.Exit(1)
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

		// A render can take a while, so say where it is rather than going quiet
		// until it finishes. One line per completed beat keeps it readable.
		lastDone := -1
		pipeline.SetProgress(func(p scene.Progress) {
			if p.Phase == "frames" && p.Total > 0 {
				if p.Done == lastDone {
					return
				}
				lastDone = p.Done
				fmt.Fprintf(os.Stderr, "export: %s %d/%d\n", p.Phase, p.Done, p.Total)
				return
			}
			if p.Phase != "" {
				fmt.Fprintf(os.Stderr, "export: %s\n", p.Phase)
			}
		})

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
