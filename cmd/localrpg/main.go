package main

import (
	"flag"
	"fmt"
	"os"
)

const Version = "0.1.0"

func main() {
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
	fmt.Println("  gui                Launch desktop application (Wails v3)")
	fmt.Println("  export <format>    Export story replay (web, video)")
	fmt.Println("  version            Print version information")
}
