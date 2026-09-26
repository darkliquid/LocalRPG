package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/darkliquid/localrpg/pkg/provider"
	_ "github.com/darkliquid/localrpg/pkg/provider/all"
)

const Version = "0.1.0"

func main() {
	os.Exit(runMain(os.Args[1:]))
}

// runMain parses the root flags and dispatches. With no subcommand it boots the
// GUI directly.
func runMain(args []string) int {
	if err := provider.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Provider registry: %v\n", err)
		return 1
	}

	fs := flag.NewFlagSet("localrpg", flag.ContinueOnError)
	versionFlag := fs.Bool("version", false, "Print version and exit")
	dir := fs.String("dir", ".", "Project root directory")
	png := fs.String("png", "", "Render one GUI frame to PATH and exit")
	// Registered so the root flag set accepts it; traceFlagLevel reads the raw
	// argument list itself, because a bare --trace means "full".
	fs.String("trace", "", "Enable tracing: off, summary, or full")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	if *versionFlag {
		fmt.Printf("LocalRPG v%s\n", Version)
		return 0
	}

	rest := fs.Args()
	if len(rest) == 0 {
		return bootDesktop(*dir, *png)
	}

	switch rest[0] {
	case "roll":
		handleRollCommand(rest[1:])
	case "prompt":
		handlePromptCommand(rest[1:])
	case "tts":
		handleTTSCommand(rest[1:])
	case "image":
		handleImageCommand(rest[1:])
	case "export":
		handleExportCommand(rest[1:])
	case "version":
		fmt.Printf("LocalRPG v%s\n", Version)
	case "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", rest[0])
		printUsage()
		return 1
	}
	return 0
}

func printUsage() {
	fmt.Println("Usage: localrpg [flags] [command] [arguments]")
	fmt.Println("\nWith no command, launches the desktop GUI.")
	fmt.Println("\nFlags:")
	fmt.Println("  --dir <path>       Project root directory")
	fmt.Println("  --png <path>       Render one GUI frame and exit")
	fmt.Println("  --version          Print version information")
	fmt.Println("\nCommands:")
	fmt.Println("  roll <notation>    Evaluate dice notation (e.g. 1d20+5, 4d6kh3, 4dF)")
	fmt.Println("  prompt [flags]     Test model execution via CLI harness or HTTP")
	fmt.Println("  tts <text>         Synthesize text to speech")
	fmt.Println("  image <prompt>     Generate scene or character image")
	fmt.Println("  export <format>    Export story replay (web, video)")
	fmt.Println("  version            Print version information")
}
