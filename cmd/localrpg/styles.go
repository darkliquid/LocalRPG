package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
)

// handleStylesCommand lists the style packs the config directory holds, or
// validates one, so a pack can be checked without starting the app.
func handleStylesCommand(args []string) {
	if code := runStylesCommand(args, os.Stdout, os.Stderr); code != 0 {
		os.Exit(code)
	}
}

func runStylesCommand(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printStylesUsage(stderr)
		return 1
	}

	mgr := config.NewConfigManager()
	cfg, _ := mgr.Load()

	switch args[0] {
	case "list":
		return runStylesList(mgr, cfg, stdout, stderr)
	case "validate":
		if len(args) < 2 {
			printStylesUsage(stderr)
			return 1
		}
		return runStylesValidate(args[1], stdout, stderr)
	case "help", "-h", "--help":
		printStylesUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "Unknown styles command: %s\n", args[0])
		printStylesUsage(stderr)
		return 1
	}
}

func runStylesList(mgr *config.ConfigManager, cfg *config.Config, stdout, stderr io.Writer) int {
	dir := mgr.StylesDir()
	packs := media.ListStylePacks(dir)
	active := strings.TrimSpace(cfg.Styles.Pack)

	fmt.Fprintf(stdout, "style packs in %s\n", dir)
	if len(packs) == 0 {
		fmt.Fprintln(stdout, "  (none; the built-in look is in force)")
		return 0
	}
	for _, pack := range packs {
		marker := " "
		if pack.ID == active {
			marker = "*"
		}
		label := pack.Label
		if label == "" {
			label = pack.ID
		}
		if len(pack.Problems) > 0 {
			fmt.Fprintf(stdout, "%s %s: invalid\n", marker, pack.ID)
			for _, problem := range pack.Problems {
				fmt.Fprintf(stdout, "    %s\n", problem)
			}
			continue
		}
		fmt.Fprintf(stdout, "%s %s (%s)\n", marker, pack.ID, label)
	}
	if active != "" {
		fmt.Fprintf(stdout, "\nactive: %s\n", active)
	}
	_ = stderr
	return 0
}

func runStylesValidate(path string, stdout, stderr io.Writer) int {
	pack, err := media.LoadStylePack(path)
	if err != nil {
		fmt.Fprintf(stderr, "%s is not usable: %v\n", path, err)
		return 1
	}
	label := pack.Label
	if label == "" {
		label = pack.ID
	}
	fmt.Fprintf(stdout, "%s (%s) is valid\n", pack.ID, label)
	return 0
}

func printStylesUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: localrpg styles <cmd>")
	fmt.Fprintln(w, "  list                 List the style packs in the config directory")
	fmt.Fprintln(w, "  validate <path>      Check a style pack file")
}
