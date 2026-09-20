package gui

import (
	"embed"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

//go:embed all:dist
var embeddedDist embed.FS

// FallbackAssetHandler returns a fallback HTML handler when assets are not bundled
func FallbackAssetHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte("<!DOCTYPE html><html><body><h1>LocalRPG GUI Backend Running</h1><p>Run 'mise run build:frontend' to compile the React frontend.</p></body></html>"))
	})
}

// AssetHandler returns an http.Handler that serves the embedded React application
// with SPA fallback to index.html for client-side navigation.
func AssetHandler() http.Handler {
	distFS, err := fs.Sub(embeddedDist, "dist")
	if err != nil {
		return FallbackAssetHandler()
	}

	// Verify if dist has index.html
	if _, err := fs.Stat(distFS, "index.html"); err != nil {
		// Check local disk fallbacks (useful during development)
		if _, err := os.Stat("frontend/dist/index.html"); err == nil {
			return spaHandler(os.DirFS("frontend/dist"))
		}
		if _, err := os.Stat("pkg/gui/dist/index.html"); err == nil {
			return spaHandler(os.DirFS("pkg/gui/dist"))
		}
		return FallbackAssetHandler()
	}

	return spaHandler(distFS)
}

func spaHandler(fileSystem fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(fileSystem))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(filepath.Clean(r.URL.Path), "/")
		if path == "" || path == "." {
			path = "index.html"
		}

		// Try opening the requested file
		f, err := fileSystem.Open(path)
		if err == nil {
			_ = f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		// If the path contains an extension, it's a missing asset -> 404
		if strings.Contains(filepath.Base(path), ".") {
			http.NotFound(w, r)
			return
		}

		// Otherwise, it's a client-side SPA route -> serve index.html
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}
