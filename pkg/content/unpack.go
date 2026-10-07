package content

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/darkliquid/localrpg/pkg/pathutil"
)

const (
	maxPackageSize    int64 = 500 * 1024 * 1024 // 500 MB
	maxSingleFileSize int64 = 100 * 1024 * 1024 // 100 MB
	maxMemberCount          = 10000
)

// Unpack extracts a .lrpgpack from r into destDir, verifying every file
// against the manifest's SHA-256 and size, and rejecting path traversal.
// It returns the package manifest and the signature bytes (if package.sig was present).
func Unpack(r io.Reader, destDir string) (Manifest, []byte, error) {
	if destDir == "" {
		return Manifest{}, nil, errors.New("destination directory cannot be empty")
	}

	if fi, err := os.Stat(destDir); err == nil {
		if fi.IsDir() {
			entries, readErr := os.ReadDir(destDir)
			if readErr == nil && len(entries) > 0 {
				return Manifest{}, nil, fmt.Errorf("destination directory %q already exists and is not empty", destDir)
			}
		} else {
			return Manifest{}, nil, fmt.Errorf("destination path %q exists and is a file", destDir)
		}
	}

	parentDir := filepath.Dir(destDir)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return Manifest{}, nil, fmt.Errorf("create parent directory %q: %w", parentDir, err)
	}

	stagingDir, err := os.MkdirTemp(parentDir, ".unpack-tmp-*")
	if err != nil {
		return Manifest{}, nil, fmt.Errorf("create staging directory: %w", err)
	}

	success := false
	defer func() {
		if !success {
			_ = os.RemoveAll(stagingDir)
		}
	}()

	gzr, err := gzip.NewReader(r)
	if err != nil {
		return Manifest{}, nil, fmt.Errorf("open gzip stream: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	// First entry must be package.yaml
	firstHdr, err := tr.Next()
	if err != nil {
		return Manifest{}, nil, fmt.Errorf("read package archive: %w", err)
	}
	if firstHdr.Name != "package.yaml" {
		return Manifest{}, nil, fmt.Errorf("first archive member must be package.yaml, got %q", firstHdr.Name)
	}
	if firstHdr.Size > maxSingleFileSize {
		return Manifest{}, nil, fmt.Errorf("package.yaml exceeds maximum size (%d bytes)", firstHdr.Size)
	}

	manifestBytes, err := io.ReadAll(io.LimitReader(tr, maxSingleFileSize+1))
	if err != nil {
		return Manifest{}, nil, fmt.Errorf("read package.yaml: %w", err)
	}
	if int64(len(manifestBytes)) > maxSingleFileSize {
		return Manifest{}, nil, errors.New("package.yaml exceeds maximum size")
	}

	m, err := ParseManifest(manifestBytes)
	if err != nil {
		return Manifest{}, nil, fmt.Errorf("parse package.yaml: %w", err)
	}

	if err := pathutil.ValidateID(m.ID); err != nil {
		return Manifest{}, nil, fmt.Errorf("invalid package ID %q: %w", m.ID, err)
	}
	if m.Type != "system" && m.Type != "world" {
		return Manifest{}, nil, fmt.Errorf("invalid package type %q, must be 'system' or 'world'", m.Type)
	}
	if len(m.Files) == 0 {
		return Manifest{}, nil, errors.New("package manifest contains no files")
	}

	declared := make(map[string]FileEntry, len(m.Files))
	for _, fe := range m.Files {
		cleanPath := filepath.ToSlash(filepath.Clean(fe.Path))
		if strings.HasPrefix(cleanPath, "../") || cleanPath == ".." || filepath.IsAbs(cleanPath) {
			return Manifest{}, nil, fmt.Errorf("manifest contains unsafe path %q", fe.Path)
		}
		if _, exists := declared[cleanPath]; exists {
			return Manifest{}, nil, fmt.Errorf("manifest contains duplicate entry for %q", cleanPath)
		}
		declared[cleanPath] = fe
	}

	var sigBytes []byte
	extracted := make(map[string]bool, len(m.Files))
	var totalSize int64
	memberCount := 0

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Manifest{}, nil, fmt.Errorf("read archive entry: %w", err)
		}

		memberCount++
		if memberCount > maxMemberCount {
			return Manifest{}, nil, fmt.Errorf("archive exceeds maximum member count (%d)", maxMemberCount)
		}

		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeRegA {
			return Manifest{}, nil, fmt.Errorf("unsupported member type for %q (only regular files permitted)", hdr.Name)
		}

		cleanRel := filepath.ToSlash(filepath.Clean(hdr.Name))
		if strings.HasPrefix(cleanRel, "../") || cleanRel == ".." || filepath.IsAbs(cleanRel) {
			return Manifest{}, nil, fmt.Errorf("unsafe path in archive %q", hdr.Name)
		}

		// package.sig is a reserved member excluded from manifest.Files
		if cleanRel == "package.sig" {
			if hdr.Size > maxSingleFileSize {
				return Manifest{}, nil, fmt.Errorf("package.sig exceeds maximum size (%d bytes)", hdr.Size)
			}
			b, err := io.ReadAll(io.LimitReader(tr, maxSingleFileSize+1))
			if err != nil {
				return Manifest{}, nil, fmt.Errorf("read package.sig: %w", err)
			}
			if int64(len(b)) > maxSingleFileSize {
				return Manifest{}, nil, errors.New("package.sig exceeds maximum size")
			}
			sigBytes = b

			targetPath, err := pathutil.ResolveSafeChild(stagingDir, "package.sig")
			if err != nil {
				return Manifest{}, nil, fmt.Errorf("resolve safe path for package.sig: %w", err)
			}
			if err := os.WriteFile(targetPath, sigBytes, 0644); err != nil {
				return Manifest{}, nil, fmt.Errorf("write package.sig: %w", err)
			}
			continue
		}

		fe, isDeclared := declared[cleanRel]
		if !isDeclared {
			return Manifest{}, nil, fmt.Errorf("archive member %q not declared in manifest", cleanRel)
		}
		if extracted[cleanRel] {
			return Manifest{}, nil, fmt.Errorf("duplicate archive member %q", cleanRel)
		}
		if hdr.Size != fe.Size {
			return Manifest{}, nil, fmt.Errorf("archive member %q size %d does not match manifest size %d", cleanRel, hdr.Size, fe.Size)
		}

		totalSize += fe.Size
		if totalSize > maxPackageSize {
			return Manifest{}, nil, fmt.Errorf("archive exceeds maximum package size (%d bytes)", maxPackageSize)
		}

		targetPath, err := pathutil.ResolveSafeChild(stagingDir, filepath.FromSlash(cleanRel))
		if err != nil {
			return Manifest{}, nil, fmt.Errorf("resolve safe path for %q: %w", cleanRel, err)
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return Manifest{}, nil, fmt.Errorf("create directory for %q: %w", cleanRel, err)
		}

		targetFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return Manifest{}, nil, fmt.Errorf("create file %q: %w", cleanRel, err)
		}

		hasher := sha256.New()
		writer := io.MultiWriter(targetFile, hasher)

		written, copyErr := io.CopyN(writer, tr, fe.Size)
		closeErr := targetFile.Close()

		if copyErr != nil {
			return Manifest{}, nil, fmt.Errorf("extract %q: %w", cleanRel, copyErr)
		}
		if closeErr != nil {
			return Manifest{}, nil, fmt.Errorf("close %q: %w", cleanRel, closeErr)
		}
		if written != fe.Size {
			return Manifest{}, nil, fmt.Errorf("extracted %q: wrote %d bytes, expected %d", cleanRel, written, fe.Size)
		}

		computedSum := hex.EncodeToString(hasher.Sum(nil))
		if !strings.EqualFold(computedSum, fe.SHA256) {
			return Manifest{}, nil, fmt.Errorf("checksum mismatch for %q: got %s, want %s", cleanRel, computedSum, fe.SHA256)
		}

		extracted[cleanRel] = true
	}

	if len(extracted) != len(declared) {
		var missing []string
		for path := range declared {
			if !extracted[path] {
				missing = append(missing, path)
			}
		}
		return Manifest{}, nil, fmt.Errorf("package missing declared files: %s", strings.Join(missing, ", "))
	}

	// Remove empty destDir if it existed beforehand
	_ = os.Remove(destDir)

	if err := os.Rename(stagingDir, destDir); err != nil {
		return Manifest{}, nil, fmt.Errorf("finalize unpack: %w", err)
	}

	success = true
	return m, sigBytes, nil
}
