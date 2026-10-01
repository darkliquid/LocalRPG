// Command sitegen renders the LocalRPG showcase site: the marketing home page,
// the user guide that ships inside the app, and the project README, all styled
// with the same glassmorphic launcher look as the desktop client.
//
// It reads Markdown straight out of the repository, so the published site can
// never drift from the documentation the app embeds. Everything it writes is
// static: no server, no JavaScript framework, no build step beyond `go run`.
//
// Usage:
//
//	go run ./tools/sitegen
package main

import (
	"flag"
	"fmt"
	"os"
)

// defaultRepoURL is where links that point outside the site resolve to.
const defaultRepoURL = "https://github.com/darkliquid/LocalRPG"

type config struct {
	root        string
	out         string
	screenshots string
	repoURL     string
}

func main() {
	var cfg config
	var serve bool
	var port int
	flag.StringVar(&cfg.root, "root", ".", "repository root to read documentation from")
	flag.StringVar(&cfg.out, "out", "website/dist", "directory to write the built site into")
	flag.StringVar(&cfg.screenshots, "screenshots", "website/screenshots", "directory holding showcase screenshots")
	flag.StringVar(&cfg.repoURL, "repo", defaultRepoURL, "repository URL used for links outside the site")
	flag.BoolVar(&serve, "serve", false, "serve the built site on localhost after building it")
	flag.IntVar(&port, "port", 4173, "port to serve the site on (used with -serve)")
	flag.Parse()

	if err := build(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "sitegen: %v\n", err)
		os.Exit(1)
	}

	if !serve {
		return
	}
	if err := serveSite(cfg.out, port); err != nil {
		fmt.Fprintf(os.Stderr, "sitegen: %v\n", err)
		os.Exit(1)
	}
}
