package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// serveSite hosts the built site for local review. It lives here so that
// previewing needs no extra tooling: the same `go run` that builds the site can
// serve it, with the same relative links and directory indexes it will have in
// production.
func serveSite(out string, port int) error {
	if _, err := os.Stat(filepath.Join(out, "index.html")); err != nil {
		return fmt.Errorf("nothing to serve: %w", err)
	}

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	server := &http.Server{
		Addr:              addr,
		Handler:           http.FileServer(http.Dir(out)),
		ReadHeaderTimeout: 5 * time.Second,
	}

	fmt.Printf("sitegen: preview at http://%s (press ctrl-c to stop)\n", addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve %s: %w", out, err)
	}
	return nil
}
