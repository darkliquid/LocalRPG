package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/darkliquid/localrpg/pkg/gui"
)

func handleContentCommand(args []string) {
	code := runContentCommand(args, os.Stdout, os.Stderr)
	if code != 0 {
		os.Exit(code)
	}
}

func runContentCommand(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printContentUsage(stderr)
		return 1
	}

	switch args[0] {
	case "export":
		return runContentExport(args[1:], stdout, stderr)
	case "import":
		return runContentImport(args[1:], stdout, stderr)
	case "sign":
		return runContentSign(args[1:], stdout, stderr)
	case "verify":
		return runContentVerify(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printContentUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "Unknown content subcommand: %s\n", args[0])
		printContentUsage(stderr)
		return 1
	}
}

func printContentUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: localrpg content <subcommand> [flags]")
	fmt.Fprintln(w, "  export <world|system> <id> [--out <file>]              Export content as a package (.lrpgworld or .lrpgsystem)")
	fmt.Fprintln(w, "  import <file> [--on-conflict refuse|rename|overwrite] [--yes] Import content from a package")
	fmt.Fprintln(w, "  sign <file|dir> --key <privkey> [--publisher <name>]   Sign a package or content directory")
	fmt.Fprintln(w, "  verify <file|dir>                                    Verify package signature and integrity")
}

func runContentExport(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("content export", flag.ContinueOnError)
	fs.SetOutput(stderr)
	outPath := fs.String("out", "", "Output package file path (defaults to stdout)")

	flags, positional := splitExportArgs(fs, args)
	if err := fs.Parse(flags); err != nil {
		return 1
	}

	if len(positional) < 2 {
		fmt.Fprintln(stderr, "Usage: localrpg content export <world|system> <id> [--out <file>]")
		return 1
	}

	typ := positional[0]
	id := positional[1]

	var outWriter io.Writer = stdout
	var fileToClose *os.File
	if *outPath != "" {
		if err := os.MkdirAll(filepath.Dir(*outPath), 0755); err != nil {
			fmt.Fprintf(stderr, "Create output directory: %v\n", err)
			return 1
		}
		f, err := os.Create(*outPath)
		if err != nil {
			fmt.Fprintf(stderr, "Create output file: %v\n", err)
			return 1
		}
		fileToClose = f
		outWriter = f
	}

	cwd, _ := os.Getwd()
	svc := gui.NewService(cwd)
	m, err := svc.ExportContent(context.Background(), typ, id, outWriter)
	if fileToClose != nil {
		_ = fileToClose.Close()
	}

	if err != nil {
		if *outPath != "" {
			_ = os.Remove(*outPath)
		}
		fmt.Fprintf(stderr, "Export failed: %v\n", err)
		return 1
	}

	if *outPath != "" {
		fmt.Fprintf(stderr, "Exported %s %q (%s, %d files) to %s\n", m.Type, m.ID, m.Version, len(m.Files), *outPath)
	}
	return 0
}

func runContentImport(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("content import", flag.ContinueOnError)
	fs.SetOutput(stderr)
	onConflict := fs.String("on-conflict", "refuse", "Conflict resolution mode: refuse, rename, or overwrite")
	yes := fs.Bool("yes", false, "Confirm import without prompting")

	flags, positional := splitExportArgs(fs, args)
	if err := fs.Parse(flags); err != nil {
		return 1
	}

	if len(positional) < 1 {
		fmt.Fprintln(stderr, "Usage: localrpg content import <file> [--on-conflict refuse|rename|overwrite] [--yes]")
		return 1
	}

	filePath := positional[0]
	f, err := os.Open(filePath)
	if err != nil {
		fmt.Fprintf(stderr, "Open package file: %v\n", err)
		return 1
	}
	defer f.Close()

	if !*yes {
		fmt.Fprintln(stderr, "Import requires confirmation. Re-run with --yes to confirm.")
		return 1
	}

	cwd, _ := os.Getwd()
	svc := gui.NewService(cwd)
	res, err := svc.ImportContentWithOptions(context.Background(), f, gui.ImportContentOptions{
		ConflictMode: *onConflict,
		Filename:     filepath.Base(filePath),
	})
	if err != nil {
		fmt.Fprintf(stderr, "Import failed: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "Imported %s %q (%s) as %s (%s, %d files)\n", res.Type, res.Name, res.Version, res.ID, res.Action, res.FileCount)
	return 0
}
