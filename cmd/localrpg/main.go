package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/darkliquid/localrpg/pkg/provider"
	_ "github.com/darkliquid/localrpg/pkg/provider/all"
)

// Version is stamped at release time with
// -ldflags "-X main.Version=...", so it stays a variable rather than a
// constant. The literal is what a plain `go build` reports.
var Version = "0.4.1"

func main() {
	if err := provider.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Provider registry: %v\n", err)
		os.Exit(1)
	}

	versionFlag := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("LocalRPG v%s\n", Version)
		os.Exit(0)
	}

	args := flag.Args()
	if len(args) == 0 {
		printUsage()
		os.Exit(0)
	}

	switch args[0] {
	case "roll":
		handleRollCommand(args[1:])
	case "prompt":
		handlePromptCommand(args[1:])
	case "play":
		handlePlayCommand(args[1:])
	case "tts":
		handleTTSCommand(args[1:])
	case "image":
		handleImageCommand(args[1:])
	case "gui":
		handleGUICommand(args[1:])
	case "export":
		handleExportCommand(args[1:])
	case "content":
		handleContentCommand(args[1:])
	case "debug":
		handleDebugCommand(args[1:])
	case "config":
		handleConfigCommand(args[1:])
	case "version":
		fmt.Printf("LocalRPG v%s\n", Version)
	case "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", args[0])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: localrpg <command> [arguments]")
	fmt.Println("\nCommands:")
	fmt.Println("  roll <notation>    Evaluate dice notation (e.g. 1d20+5, 4d6kh3, 4dF)")
	fmt.Println("  prompt [flags]     Test model execution via CLI harness or HTTP")
	fmt.Println("  play <game-id>     Launch terminal TUI play mode")
	fmt.Println("  tts <text>         Synthesize text to speech")
	fmt.Println("  image <prompt>     Generate scene or character image")
	fmt.Println("  gui                Launch desktop application (Wails v3)")
	fmt.Println("  export <format>    Export story replay (web, video)")
	fmt.Println("  content <cmd>      Export or import content packages (.lrpgpack)")
	fmt.Println("  debug <cmd>        Run automated scenario tests or debug server")
	fmt.Println("  config <cmd>       Manage configuration (offline-preset, check-offline)")
	fmt.Println("  version            Print version information")
}
