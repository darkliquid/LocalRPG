package models

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// createMockTar generates an in-memory tar containing test model files.
func createMockTar(t *testing.T) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	files := map[string]string{
		"model.onnx":             "fake onnx weights",
		"voices.bin":             "fake voice styles",
		"tokens.txt":             "fake tokens",
		"espeak-ng-data/phontab": "fake phontab data",
	}

	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0644,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("write tar body: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}

	tarBytes := buf.Bytes()
	sum := sha256.Sum256(tarBytes)
	return tarBytes, hex.EncodeToString(sum[:])
}

func TestManagerDownloadAndVerify(t *testing.T) {
	cacheDir := t.TempDir()
	tarData, checksum := createMockTar(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(tarData)))
		_, _ = w.Write(tarData)
	}))
	defer server.Close()

	mgr := NewManager(cacheDir)
	spec := ModelSpec{
		ID:            "test-tts",
		Name:          "Test TTS Model",
		URL:           server.URL,
		SHA256:        checksum,
		SizeBytes:     int64(len(tarData)),
		ArchiveType:   "tar",
		Subdir:        filepath.Join("tts", "test"),
		RequiredFiles: []string{"model.onnx", "voices.bin", "tokens.txt"},
	}
	mgr.RegisterSpec(spec)

	status := mgr.Status("test-tts")
	if status.Installed {
		t.Fatalf("expected model to not be installed initially")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	events, err := mgr.Download(ctx, "test-tts")
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}

	var lastStatus ModelStatus
	for s := range events {
		lastStatus = s
	}

	if !lastStatus.Installed {
		t.Fatalf("expected model to be installed after download, got error: %s", lastStatus.Error)
	}

	// Verify required files exist on disk
	destDir := filepath.Join(cacheDir, "models", "tts", "test")
	if _, err := os.Stat(filepath.Join(destDir, "model.onnx")); err != nil {
		t.Errorf("model.onnx missing: %v", err)
	}
}

func TestManagerChecksumMismatchRejectsDownload(t *testing.T) {
	cacheDir := t.TempDir()
	tarData, _ := createMockTar(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(tarData)
	}))
	defer server.Close()

	mgr := NewManager(cacheDir)
	spec := ModelSpec{
		ID:            "bad-checksum-model",
		Name:          "Bad Checksum Model",
		URL:           server.URL,
		SHA256:        "0000000000000000000000000000000000000000000000000000000000000000",
		SizeBytes:     int64(len(tarData)),
		ArchiveType:   "tar",
		Subdir:        filepath.Join("tts", "bad"),
		RequiredFiles: []string{"model.onnx"},
	}
	mgr.RegisterSpec(spec)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	events, err := mgr.Download(ctx, "bad-checksum-model")
	if err != nil {
		t.Fatalf("unexpected call error: %v", err)
	}

	var lastStatus ModelStatus
	for s := range events {
		lastStatus = s
	}

	if lastStatus.Installed {
		t.Errorf("expected model to NOT be installed with bad checksum")
	}
	if lastStatus.Error == "" {
		t.Errorf("expected error message on bad checksum, got empty string")
	}

	// Ensure destination directory is not populated
	destDir := filepath.Join(cacheDir, "models", "tts", "bad")
	if _, err := os.Stat(destDir); !os.IsNotExist(err) {
		t.Errorf("expected destDir to not exist, err: %v", err)
	}
}

func TestExtractArchiveRejectsZipSlip(t *testing.T) {
	destDir := t.TempDir()

	// Create a tar archive with a malicious traversal entry
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	hdr := &tar.Header{
		Name: "../evil.txt",
		Mode: 0600,
		Size: int64(len("malicious")),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("malicious")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	tarPath := filepath.Join(destDir, "test.tar")
	if err := os.WriteFile(tarPath, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}

	mgr := NewManager(t.TempDir())
	outDir := filepath.Join(destDir, "out")
	err := mgr.extractArchive(tarPath, "tar", outDir)
	if err == nil {
		t.Error("extractArchive expected error for Zip Slip traversal entry, got nil")
	}

	// Verify evil.txt was NOT written outside destDir/out
	if _, err := os.Stat(filepath.Join(destDir, "evil.txt")); err == nil {
		t.Error("evil.txt was written outside target directory!")
	}
}

