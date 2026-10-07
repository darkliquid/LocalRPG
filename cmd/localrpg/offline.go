package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/darkliquid/localrpg/pkg/config"
)

func handleConfigCommand(args []string) {
	code := runConfigCommand(args, os.Stdout, os.Stderr)
	if code != 0 {
		os.Exit(code)
	}
}

func runConfigCommand(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printConfigUsage(stderr)
		return 1
	}

	switch args[0] {
	case "offline-preset":
		return runConfigOfflinePreset(args[1:], stdout, stderr)
	case "check-offline":
		return runConfigCheckOffline(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printConfigUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "Unknown config subcommand: %s\n", args[0])
		printConfigUsage(stderr)
		return 1
	}
}

func printConfigUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: localrpg config <subcommand> [flags]")
	fmt.Fprintln(w, "\nSubcommands:")
	fmt.Fprintln(w, "  offline-preset [--tts native-os|sherpa-onnx]   Apply the fully offline preset bundle")
	fmt.Fprintln(w, "  check-offline                                   Verify all configured providers are offline")
}

func runConfigOfflinePreset(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("offline-preset", flag.ContinueOnError)
	fs.SetOutput(stderr)
	tts := fs.String("tts", "native-os", "TTS provider for offline preset (native-os or sherpa-onnx)")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	if fs.NArg() > 0 && *tts == "native-os" {
		*tts = fs.Arg(0)
	}

	cfgMgr := config.NewConfigManager()
	cfg, err := cfgMgr.Load()
	if err != nil {
		fmt.Fprintf(stderr, "Error loading configuration: %v\n", err)
		return 1
	}

	changes := config.ApplyOfflinePreset(cfg, *tts)
	if err := cfgMgr.Save(cfg); err != nil {
		fmt.Fprintf(stderr, "Error saving configuration: %v\n", err)
		return 1
	}

	if len(changes) == 0 {
		fmt.Fprintln(stdout, "No changes needed; configuration is already offline.")
	} else {
		fmt.Fprintln(stdout, "Applied offline preset:")
		for _, change := range changes {
			fmt.Fprintf(stdout, "  - %s\n", change)
		}
	}

	if *tts == "sherpa-onnx" {
		fmt.Fprintln(stdout, "\nNote: sherpa-onnx requires local model weights before first synthesis.")
	}

	return 0
}

func runConfigCheckOffline(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check-offline", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return 1
	}

	cfgMgr := config.NewConfigManager()
	cfg, err := cfgMgr.Load()
	if err != nil {
		fmt.Fprintf(stderr, "Error loading configuration: %v\n", err)
		return 1
	}

	report := config.VerifyOffline(cfg)
	if report.Offline {
		fmt.Fprintln(stdout, "All configured providers are offline.")
		return 0
	}

	fmt.Fprintln(stdout, "Provider configuration is not fully offline:")
	for _, issue := range report.Issues {
		tier := issue.Tier
		if tier == "" {
			tier = "unknown"
		}
		fmt.Fprintf(stdout, "  - [%s] %s (%s): %s\n", issue.Role, issue.ProviderKey, tier, issue.Reason)
	}
	return 1
}
