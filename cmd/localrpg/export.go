package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/darkliquid/localrpg/pkg/export"
)

func handleExportCommand(args []string) {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: localrpg export <web|video> <game-id> [flags]")
		fmt.Fprintln(os.Stderr, "Export story replay to web bundle or video")
		fs.PrintDefaults()
	}
	out := fs.String("out", "", "Output path for export")
	dir := fs.String("dir", ".", "Root directory")

	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fs.Usage()
		os.Exit(0)
	}

	format := args[0]
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "Error: missing game ID")
		fs.Usage()
		os.Exit(1)
	}
	gameID := args[1]
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
