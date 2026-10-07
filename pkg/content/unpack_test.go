package content_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/content"
)

func TestUnpackRoundTrip(t *testing.T) {
	dir := writeFixtureWorld(t)
	var buf bytes.Buffer
	origManifest, err := content.Pack(dir, "world", content.ManifestMeta{}, &buf)
	if err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(t.TempDir(), "unpacked")
	unpackedManifest, err := content.Unpack(&buf, dest)
	if err != nil {
		t.Fatal(err)
	}

	if unpackedManifest.ID != origManifest.ID {
		t.Fatalf("unpacked ID = %q, want %q", unpackedManifest.ID, origManifest.ID)
	}

	worldPath := filepath.Join(dest, "world.yaml")
	if _, err := os.Stat(worldPath); err != nil {
		t.Fatalf("world.yaml missing after unpack: %v", err)
	}
	gatehousePath := filepath.Join(dest, "entities", "gatehouse.md")
	if _, err := os.Stat(gatehousePath); err != nil {
		t.Fatalf("entities/gatehouse.md missing after unpack: %v", err)
	}
}

func TestUnpackRejectsTraversal(t *testing.T) {
	// Construct an archive with "../evil.txt"
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	m := content.Manifest{
		ID:      "bad_pack",
		Name:    "Bad Pack",
		Version: "1.0.0",
		Type:    "world",
		Files: []content.FileEntry{
			{Path: "../evil.txt", SHA256: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", Size: 0},
		},
	}
	mb, err := m.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	_ = tw.WriteHeader(&tar.Header{
		Name:    "package.yaml",
		Mode:    0644,
		Size:    int64(len(mb)),
		ModTime: time.Unix(0, 0),
	})
	_, _ = tw.Write(mb)

	_ = tw.WriteHeader(&tar.Header{
		Name:    "../evil.txt",
		Mode:    0644,
		Size:    0,
		ModTime: time.Unix(0, 0),
	})
	_ = tw.Close()
	_ = gzw.Close()

	dest := filepath.Join(t.TempDir(), "dest")
	if _, err := content.Unpack(&buf, dest); err == nil {
		t.Fatal("expected error unpacking path traversal, got nil")
	}

	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("expected dest %q to not exist after failed unpack, err=%v", dest, err)
	}
}

func TestUnpackRejectsTamper(t *testing.T) {
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	realData := []byte("legit content")
	realSum := sha256.Sum256(realData)

	m := content.Manifest{
		ID:      "tampered_pack",
		Name:    "Tampered Pack",
		Version: "1.0.0",
		Type:    "world",
		Files: []content.FileEntry{
			{Path: "world.yaml", SHA256: hex.EncodeToString(realSum[:]), Size: int64(len(realData))},
		},
	}
	mb, err := m.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	_ = tw.WriteHeader(&tar.Header{
		Name:    "package.yaml",
		Mode:    0644,
		Size:    int64(len(mb)),
		ModTime: time.Unix(0, 0),
	})
	_, _ = tw.Write(mb)

	tamperedData := []byte("hacked content")
	_ = tw.WriteHeader(&tar.Header{
		Name:    "world.yaml",
		Mode:    0644,
		Size:    int64(len(tamperedData)),
		ModTime: time.Unix(0, 0),
	})
	_, _ = tw.Write(tamperedData)
	_ = tw.Close()
	_ = gzw.Close()

	dest := filepath.Join(t.TempDir(), "dest")
	if _, err := content.Unpack(&buf, dest); err == nil {
		t.Fatal("expected error unpacking tampered content, got nil")
	}

	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("expected dest %q to not exist after failed unpack, err=%v", dest, err)
	}
}
