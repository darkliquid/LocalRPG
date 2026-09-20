package gui

import (
	"embed"
	"io/fs"
	"net/http"
)

// FallbackAssetHandler returns a fallback HTML handler when assets are not bundled
func FallbackAssetHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<!DOCTYPE html><html><body><h1>LocalRPG GUI Backend Running</h1><p>Vite dev server or frontend bundle active.</p></body></html>"))
	})
}

// FSAssetHandler returns an http.Handler serving an embedded filesystem
func FSAssetHandler(embeddedFS embed.FS, subDir string) http.Handler {
	sub, err := fs.Sub(embeddedFS, subDir)
	if err != nil {
		return FallbackAssetHandler()
	}
	return http.FileServer(http.FS(sub))
}
