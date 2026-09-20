package gui

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

// DefaultSocketPath returns the standard Unix domain socket path for LocalRPG.
// Priority:
// 1. $XDG_RUNTIME_DIR/localrpg.sock
// 2. ~/.local/state/localrpg/gui.sock
// 3. /tmp/localrpg-<uid>.sock
func DefaultSocketPath() string {
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		return filepath.Join(xdg, "localrpg.sock")
	}

	if home, err := os.UserHomeDir(); err == nil && home != "" {
		dir := filepath.Join(home, ".local", "state", "localrpg")
		_ = os.MkdirAll(dir, 0700)
		return filepath.Join(dir, "gui.sock")
	}

	return filepath.Join(os.TempDir(), fmt.Sprintf("localrpg-%d.sock", os.Getuid()))
}

// ListenUnix creates a net.Listener on a Unix domain socket.
// It handles stale socket cleanup and enforces 0600 permissions.
func ListenUnix(socketPath string) (net.Listener, error) {
	// Ensure parent directory exists
	dir := filepath.Dir(socketPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("creating socket directory %s: %w", dir, err)
	}

	// Check if socket file exists
	if _, err := os.Stat(socketPath); err == nil {
		// Attempt a quick connection to check if an active instance is alive
		conn, dialErr := net.DialTimeout("unix", socketPath, 100*time.Millisecond)
		if dialErr == nil {
			_ = conn.Close()
			return nil, fmt.Errorf("another LocalRPG instance is already running on socket %s", socketPath)
		}
		// Connection refused -> stale socket file, remove it
		if err := os.Remove(socketPath); err != nil {
			return nil, fmt.Errorf("removing stale socket %s: %w", socketPath, err)
		}
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("listening on socket %s: %w", socketPath, err)
	}

	// Restrict permissions to owner only
	if err := os.Chmod(socketPath, 0600); err != nil {
		_ = listener.Close()
		_ = os.Remove(socketPath)
		return nil, fmt.Errorf("setting socket permissions %s: %w", socketPath, err)
	}

	return listener, nil
}
