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
//
// The work happens in a staging directory beside destDir and is promoted by a
// rename once every member has verified, so a failure leaves no half-extracted
// package behind.
func Unpack(r io.Reader, destDir string) (Manifest, []byte, error) {
	parentDir, err := prepareDestination(destDir)
	if err != nil {
		return Manifest{}, nil, err
	}

	stagingDir, err := os.MkdirTemp(parentDir, ".unpack-tmp-*")
	if err != nil {
		return Manifest{}, nil, fmt.Errorf("create staging directory: %w", err)
	}
	promoted := false
	defer func() {
		if !promoted {
			_ = os.RemoveAll(stagingDir)
		}
	}()

	gzr, err := gzip.NewReader(r)
	if err != nil {
		return Manifest{}, nil, fmt.Errorf("open gzip stream: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	m, err := readManifestMember(tr)
	if err != nil {
		return Manifest{}, nil, err
	}

	declared, err := declaredFiles(m)
	if err != nil {
		return Manifest{}, nil, err
	}

	extraction, err := extractMembers(tr, stagingDir, declared)
	if err != nil {
		return Manifest{}, nil, err
	}
	if err := extraction.checkComplete(declared); err != nil {
		return Manifest{}, nil, err
	}

	// The destination may exist as an empty directory; the rename needs it gone.
	_ = os.Remove(destDir)

	if err := os.Rename(stagingDir, destDir); err != nil {
		return Manifest{}, nil, fmt.Errorf("finalize unpack: %w", err)
	}
	promoted = true
	return m, extraction.signature, nil
}

// prepareDestination checks that destDir can be replaced by an extraction, and
// returns the directory the staging directory belongs in.
func prepareDestination(destDir string) (string, error) {
	if destDir == "" {
		return "", errors.New("destination directory cannot be empty")
	}

	if fi, err := os.Stat(destDir); err == nil {
		if fi.IsDir() {
			entries, readErr := os.ReadDir(destDir)
			if readErr == nil && len(entries) > 0 {
				return "", fmt.Errorf("destination directory %q already exists and is not empty", destDir)
			}
		} else {
			return "", fmt.Errorf("destination path %q exists and is a file", destDir)
		}
	}

	parentDir := filepath.Dir(destDir)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return "", fmt.Errorf("create parent directory %q: %w", parentDir, err)
	}
	return parentDir, nil
}

// readManifestMember reads the archive's first member, which must be package.yaml,
// and validates the manifest it carries.
func readManifestMember(tr *tar.Reader) (Manifest, error) {
	firstHdr, err := tr.Next()
	if err != nil {
		return Manifest{}, fmt.Errorf("read package archive: %w", err)
	}
	if firstHdr.Name != "package.yaml" {
		return Manifest{}, fmt.Errorf("first archive member must be package.yaml, got %q", firstHdr.Name)
	}
	if firstHdr.Size > maxSingleFileSize {
		return Manifest{}, fmt.Errorf("package.yaml exceeds maximum size (%d bytes)", firstHdr.Size)
	}

	manifestBytes, err := readMember(tr, "package.yaml")
	if err != nil {
		return Manifest{}, err
	}

	m, err := ParseManifest(manifestBytes)
	if err != nil {
		return Manifest{}, fmt.Errorf("parse package.yaml: %w", err)
	}

	if err := pathutil.ValidateID(m.ID); err != nil {
		return Manifest{}, fmt.Errorf("invalid package ID %q: %w", m.ID, err)
	}
	if m.Type != "system" && m.Type != "world" {
		return Manifest{}, fmt.Errorf("invalid package type %q, must be 'system' or 'world'", m.Type)
	}
	if len(m.Files) == 0 {
		return Manifest{}, errors.New("package manifest contains no files")
	}
	return m, nil
}

// readMember reads one archive member under the single-file cap.
func readMember(tr *tar.Reader, name string) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(tr, maxSingleFileSize+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	if int64(len(b)) > maxSingleFileSize {
		return nil, fmt.Errorf("%s exceeds maximum size", name)
	}
	return b, nil
}

// declaredFiles indexes the manifest's files by their cleaned, slash-separated
// path, rejecting one that escapes the package and one declared twice.
func declaredFiles(m Manifest) (map[string]FileEntry, error) {
	declared := make(map[string]FileEntry, len(m.Files))
	for _, fe := range m.Files {
		cleanPath := filepath.ToSlash(filepath.Clean(fe.Path))
		if isUnsafePath(cleanPath) {
			return nil, fmt.Errorf("manifest contains unsafe path %q", fe.Path)
		}
		if _, exists := declared[cleanPath]; exists {
			return nil, fmt.Errorf("manifest contains duplicate entry for %q", cleanPath)
		}
		declared[cleanPath] = fe
	}
	return declared, nil
}

// isUnsafePath reports whether a cleaned, slash-separated archive path escapes the
// directory it would be extracted into.
func isUnsafePath(cleanPath string) bool {
	return strings.HasPrefix(cleanPath, "../") || cleanPath == ".." || filepath.IsAbs(cleanPath)
}

// unpackState is the tally an extraction keeps as it walks the archive.
type unpackState struct {
	signature   []byte
	extracted   map[string]bool
	totalSize   int64
	memberCount int
}

// checkComplete names the declared files the archive did not carry.
func (s *unpackState) checkComplete(declared map[string]FileEntry) error {
	if len(s.extracted) == len(declared) {
		return nil
	}
	var missing []string
	for path := range declared {
		if !s.extracted[path] {
			missing = append(missing, path)
		}
	}
	return fmt.Errorf("package missing declared files: %s", strings.Join(missing, ", "))
}

// extractMembers writes every member of the archive into stagingDir, verifying each
// against the size and checksum the manifest declares for it. package.sig is the
// one reserved member, read and written alongside them.
func extractMembers(tr *tar.Reader, stagingDir string, declared map[string]FileEntry) (*unpackState, error) {
	state := &unpackState{extracted: make(map[string]bool, len(declared))}

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return state, nil
		}
		if err != nil {
			return nil, fmt.Errorf("read archive entry: %w", err)
		}

		state.memberCount++
		if state.memberCount > maxMemberCount {
			return nil, fmt.Errorf("archive exceeds maximum member count (%d)", maxMemberCount)
		}

		// tar.Reader normalises the legacy TypeRegA ('\x00') to TypeReg, or to
		// TypeDir when the name ends in a slash, so only TypeReg reaches here.
		if hdr.Typeflag != tar.TypeReg {
			return nil, fmt.Errorf("unsupported member type for %q (only regular files permitted)", hdr.Name)
		}

		cleanRel := filepath.ToSlash(filepath.Clean(hdr.Name))
		if isUnsafePath(cleanRel) {
			return nil, fmt.Errorf("unsafe path in archive %q", hdr.Name)
		}

		// package.sig is a reserved member excluded from manifest.Files
		if cleanRel == "package.sig" {
			sig, err := readSignatureMember(tr, hdr, stagingDir)
			if err != nil {
				return nil, err
			}
			state.signature = sig
			continue
		}

		fe, isDeclared := declared[cleanRel]
		if !isDeclared {
			return nil, fmt.Errorf("archive member %q not declared in manifest", cleanRel)
		}
		if state.extracted[cleanRel] {
			return nil, fmt.Errorf("duplicate archive member %q", cleanRel)
		}
		if hdr.Size != fe.Size {
			return nil, fmt.Errorf("archive member %q size %d does not match manifest size %d", cleanRel, hdr.Size, fe.Size)
		}

		state.totalSize += fe.Size
		if state.totalSize > maxPackageSize {
			return nil, fmt.Errorf("archive exceeds maximum package size (%d bytes)", maxPackageSize)
		}

		if err := writeVerifiedMember(tr, stagingDir, cleanRel, fe); err != nil {
			return nil, err
		}
		state.extracted[cleanRel] = true
	}
}

// readSignatureMember reads the reserved package.sig member and writes it beside
// the extracted files, returning what it read for the caller to verify.
func readSignatureMember(tr *tar.Reader, hdr *tar.Header, stagingDir string) ([]byte, error) {
	if hdr.Size > maxSingleFileSize {
		return nil, fmt.Errorf("package.sig exceeds maximum size (%d bytes)", hdr.Size)
	}

	sig, err := readMember(tr, "package.sig")
	if err != nil {
		return nil, err
	}

	targetPath, err := pathutil.ResolveSafeChild(stagingDir, "package.sig")
	if err != nil {
		return nil, fmt.Errorf("resolve safe path for package.sig: %w", err)
	}
	if err := os.WriteFile(targetPath, sig, 0644); err != nil {
		return nil, fmt.Errorf("write package.sig: %w", err)
	}
	return sig, nil
}

// writeVerifiedMember writes one declared member under stagingDir and verifies it
// against the size and checksum the manifest declared for it.
func writeVerifiedMember(tr *tar.Reader, stagingDir, cleanRel string, fe FileEntry) error {
	targetPath, err := pathutil.ResolveSafeChild(stagingDir, filepath.FromSlash(cleanRel))
	if err != nil {
		return fmt.Errorf("resolve safe path for %q: %w", cleanRel, err)
	}
	// Containment, in the form the scanner models rather than the form it does not:
	// resolve the destination, join, and require the result to stay under it.
	// filepath.Join cleans, so a ".." in the entry name resolves out of the
	// destination and the prefix check rejects it. ResolveSafeChild above does the
	// same job, and the check is repeated here because CodeQL cannot see through a
	// helper; the lgtm comments that used to sit on the two calls below are the
	// pre-GitHub-Actions syntax and were suppressing nothing.
	absStaging, err := filepath.Abs(stagingDir)
	if err != nil {
		return fmt.Errorf("resolve staging directory: %w", err)
	}
	if !strings.HasPrefix(targetPath, absStaging+string(os.PathSeparator)) {
		return fmt.Errorf("archive member %q escapes staging directory", cleanRel)
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return fmt.Errorf("create directory for %q: %w", cleanRel, err)
	}

	targetFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("create file %q: %w", cleanRel, err)
	}

	hasher := sha256.New()
	written, copyErr := io.CopyN(io.MultiWriter(targetFile, hasher), tr, fe.Size)
	closeErr := targetFile.Close()

	if copyErr != nil {
		return fmt.Errorf("extract %q: %w", cleanRel, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close %q: %w", cleanRel, closeErr)
	}
	if written != fe.Size {
		return fmt.Errorf("extracted %q: wrote %d bytes, expected %d", cleanRel, written, fe.Size)
	}

	computedSum := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(computedSum, fe.SHA256) {
		return fmt.Errorf("checksum mismatch for %q: got %s, want %s", cleanRel, computedSum, fe.SHA256)
	}
	return nil
}
