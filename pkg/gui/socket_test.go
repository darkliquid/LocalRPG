package gui

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultSocketPath(t *testing.T) {
	path := DefaultSocketPath()
	if path == "" {
		t.Fatal("expected non-empty default socket path")
	}
	if filepath.Ext(path) != ".sock" {
		t.Errorf("expected socket extension .sock, got %s", path)
	}
}

func TestListenUnix(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "test.sock")

	listener, err := ListenUnix(sockPath)
	if err != nil {
		t.Fatalf("ListenUnix failed: %v", err)
	}
	defer listener.Close()

	// Verify file was created
	info, err := os.Stat(sockPath)
	if err != nil {
		t.Fatalf("stat socket file failed: %v", err)
	}

	// Verify file mode is 0600
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("expected socket permissions 0600, got %o", perm)
	}

	// Test stale socket cleanup: close listener without removing file
	_ = listener.Close()
	// Re-listen on same path should detect dead socket and succeed
	listener2, err := ListenUnix(sockPath)
	if err != nil {
		t.Fatalf("re-listening on dead socket failed: %v", err)
	}
	defer listener2.Close()
}

func TestUnixSocketHTTPCommunication(t *testing.T) {
	gameID, svc := setupTestGame(t)
	sockPath := filepath.Join(t.TempDir(), "http_test.sock")

	server := NewServer(svc, AssetHandler())

	listener, err := ListenUnix(sockPath)
	if err != nil {
		t.Fatalf("ListenUnix failed: %v", err)
	}
	defer listener.Close()

	httpServer := &http.Server{Handler: server}
	go func() {
		_ = httpServer.Serve(listener)
	}()
	defer httpServer.Close()

	// Client connecting over unix domain socket
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", sockPath)
			},
		},
		Timeout: 2 * time.Second,
	}

	resp, err := client.Get("http://unix/api/game/" + gameID + "/state")
	if err != nil {
		t.Fatalf("HTTP request over unix socket failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 OK, got %d: %s", resp.StatusCode, string(body))
	}
}
