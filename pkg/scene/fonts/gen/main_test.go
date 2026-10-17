package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceEntries(t *testing.T) {
	if len(source) != 8 {
		t.Fatalf("expected 8 font source entries, got %d", len(source))
	}
	for _, entry := range source {
		if entry.dest == "" {
			t.Errorf("empty destination in entry %+v", entry)
		}
		if !strings.HasSuffix(entry.dest, ".ttf") && !strings.HasSuffix(entry.dest, ".txt") {
			t.Errorf("dest %q must end with .ttf or .txt", entry.dest)
		}
		if !strings.HasPrefix(entry.src, base) {
			t.Errorf("src %q must start with base %q", entry.src, base)
		}
	}
}

func TestFetchSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("mock-font-bytes"))
	}))
	defer server.Close()

	dest := filepath.Join(t.TempDir(), "font.ttf")
	if err := fetch(server.URL, dest); err != nil {
		t.Fatalf("fetch failed: %v", err)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("os.ReadFile: %v", err)
	}
	if string(data) != "mock-font-bytes" {
		t.Errorf("got %q, want 'mock-font-bytes'", string(data))
	}
}

func TestFetchErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	dest := filepath.Join(t.TempDir(), "font.ttf")

	// 1. HTTP error status
	err := fetch(server.URL, dest)
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("expected 404 error, got: %v", err)
	}

	// 2. Network error
	err = fetch("http://127.0.0.1:0/invalid", dest)
	if err == nil {
		t.Error("expected network error, got nil")
	}

	// 3. Write failure (destination inside a non-directory)
	badDest := filepath.Join(t.TempDir(), "not-a-dir", "sub", "font.ttf")
	okServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer okServer.Close()

	err = fetch(okServer.URL, badDest)
	if err == nil {
		t.Error("expected write error for nonexistent directory, got nil")
	}
}

func TestRun(t *testing.T) {
	tempDir := t.TempDir()
	fetched := make(map[string]string)

	mockFetcher := func(url, dest string) error {
		fetched[dest] = url
		return nil
	}

	// 1. Success case
	outDir := filepath.Join(tempDir, "out")
	if err := run(outDir, mockFetcher); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(fetched) != len(source) {
		t.Errorf("fetched %d files, want %d", len(fetched), len(source))
	}

	// 2. Fetcher error
	failFetcher := func(_, _ string) error {
		return errors.New("simulated fetch error")
	}
	err := run(filepath.Join(tempDir, "out2"), failFetcher)
	if err == nil || !strings.Contains(err.Error(), "simulated fetch error") {
		t.Errorf("expected simulated fetch error, got: %v", err)
	}

	// 3. Mkdir failure
	filePath := filepath.Join(tempDir, "file-blocking-dir")
	if err := os.WriteFile(filePath, []byte("x"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	err = run(filepath.Join(filePath, "cannot-create-subdir"), mockFetcher)
	if err == nil {
		t.Error("expected mkdir error when path traverses a file, got nil")
	}
}
