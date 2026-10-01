package main

import "embed"

// templateFS holds the page shells the generator renders.
//
//go:embed templates/*.tmpl
var templateFS embed.FS

// assetFS holds the stylesheet and script the generator copies verbatim into
// the built site, so the site carries no build step of its own.
//
//go:embed assets
var assetFS embed.FS
