package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/darkliquid/localrpg/pkg/gui"
)

func handleGUICommand(args []string) {
	fs := flag.NewFlagSet("gui", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: localrpg gui [flags]")
		fmt.Fprintln(os.Stderr, "Launch desktop GUI or browser app")
		fs.PrintDefaults()
	}
	port := fs.Int("port", 8080, "Port for GUI web server")
	dir := fs.String("dir", ".", "Project root directory")
	fs.Parse(args)

	svc := gui.NewService(*dir)
	server := gui.NewServer(svc, gui.FallbackAssetHandler())

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	fmt.Printf("Starting LocalRPG GUI on http://%s\n", addr)
	if err := http.ListenAndServe(addr, server); err != nil {
		fmt.Fprintf(os.Stderr, "Server failed: %v\n", err)
		os.Exit(1)
	}
}
