package content

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/darkliquid/localrpg/pkg/core"
)

// BuildManifest scans dir and constructs a Manifest from its files and descriptor.
// If typ is empty, it attempts to detect "world" or "system" based on world.yaml or system.yaml.
// BuildManifest scans dir and constructs a Manifest from its files and descriptor.
// If typ is empty, it attempts to detect "world" or "system" based on world.yaml or system.yaml.
func BuildManifest(dir, typ string, meta ManifestMeta) (Manifest, error) {
	typ = detectPackageType(dir, typ)

	relPaths, err := packageFilePaths(dir)
	if err != nil {
		return Manifest{}, err
	}

	files, err := hashedFileEntries(dir, relPaths)
	if err != nil {
		return Manifest{}, err
	}

	m := Manifest{
		Type:  typ,
		Files: files,
	}
	applyDescriptor(&m, dir)
	applyMeta(&m, meta)

	if m.Version == "" {
		m.Version = "1.0.0"
	}
	if err := core.ValidateSemver(m.Version); err != nil {
		return Manifest{}, fmt.Errorf("package version: %w", err)
	}
	return m, nil
}

// detectPackageType returns typ, or the type the directory's descriptor names. A
// world is preferred when a directory somehow carries both.
func detectPackageType(dir, typ string) string {
	if typ != "" {
		return typ
	}
	if _, err := os.Stat(filepath.Join(dir, "world.yaml")); err == nil {
		return "world"
	}
	if _, err := os.Stat(filepath.Join(dir, "system.yaml")); err == nil {
		return "system"
	}
	return typ
}

// packageFilePaths lists the files a package carries, relative to dir and sorted,
// leaving out the hidden, legacy and reserved ones.
func packageFilePaths(dir string) ([]string, error) {
	var relPaths []string

	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return nil
			}
			return fmt.Errorf("walk %q: %w", path, walkErr)
		}
		if entry.IsDir() {
			if path == dir {
				return nil
			}
			name := entry.Name()
			if strings.HasPrefix(name, ".") || name == "cache" {
				return fs.SkipDir
			}
			return nil
		}
		if !isPackagedFile(entry) {
			return nil
		}

		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		relPaths = append(relPaths, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan content directory: %w", err)
	}

	sort.Strings(relPaths)
	return relPaths, nil
}

// isPackagedFile reports whether a directory entry becomes a package member.
func isPackagedFile(entry fs.DirEntry) bool {
	name := entry.Name()
	if strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".legacy") || name == "package.yaml" || name == "package.sig" {
		return false
	}
	return entry.Type().IsRegular()
}

// hashedFileEntries reads each file and records the size and SHA-256 the manifest
// declares for it.
func hashedFileEntries(dir string, relPaths []string) ([]FileEntry, error) {
	fileEntries := make([]FileEntry, 0, len(relPaths))
	for _, rel := range relPaths {
		diskPath := filepath.Join(dir, filepath.FromSlash(rel))
		b, err := os.ReadFile(diskPath)
		if err != nil {
			return nil, fmt.Errorf("read file %q: %w", rel, err)
		}
		h := sha256.Sum256(b)
		fileEntries = append(fileEntries, FileEntry{
			Path:   rel,
			SHA256: hex.EncodeToString(h[:]),
			Size:   int64(len(b)),
		})
	}
	return fileEntries, nil
}

// applyDescriptor fills in the identity a package's own descriptor declares.
func applyDescriptor(m *Manifest, dir string) {
	switch m.Type {
	case "world":
		worldPath := filepath.Join(dir, "world.yaml")
		if wm, err := core.LoadWorldManifest(worldPath); err == nil && wm != nil {
			m.ID = wm.ID
			m.Name = wm.Name
			m.Description = wm.Description
			if wm.Version != "" {
				m.Version = wm.Version
			} else {
				m.Version = "1.0.0"
			}
		}
	case "system":
		sysPath := filepath.Join(dir, "system.yaml")
		if sm, err := core.LoadSystemManifest(sysPath); err == nil && sm != nil {
			m.ID = sm.ID
			m.Name = sm.Name
			m.Description = sm.Description
			m.Version = sm.Version
		}
	}
}

// applyMeta overlays the metadata a caller passed over the descriptor's.
func applyMeta(m *Manifest, meta ManifestMeta) {
	if meta.ID != "" {
		m.ID = meta.ID
	}
	if meta.Name != "" {
		m.Name = meta.Name
	}
	if meta.Version != "" {
		m.Version = meta.Version
	}
	if meta.Description != "" {
		m.Description = meta.Description
	}
	if meta.Author != "" {
		m.Author = meta.Author
	}
	if meta.License != "" {
		m.License = meta.License
	}
	if meta.MinApp != "" {
		m.MinApp = meta.MinApp
	}
	if len(meta.Dependencies) > 0 {
		m.Dependencies = meta.Dependencies
	}
}

// Pack writes dir as a .lrpgpack (gzip-compressed tar) to w.
// typ is "system" or "world".
// Pack writes dir as a .lrpgpack (gzip-compressed tar) to w.
// typ is "system" or "world".
func Pack(dir, typ string, meta ManifestMeta, w io.Writer) (Manifest, error) {
	m, err := BuildManifest(dir, typ, meta)
	if err != nil {
		return Manifest{}, err
	}

	manifestBytes, err := m.Marshal()
	if err != nil {
		return Manifest{}, fmt.Errorf("marshal package manifest: %w", err)
	}

	if err := writePackageArchive(w, dir, manifestBytes, m.Files); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// writePackageArchive writes a package: the manifest first, the signature when the
// directory carries one, then every file the manifest lists. Members are written
// with a fixed mode and timestamp, so packing the same directory twice produces the
// same bytes.
func writePackageArchive(w io.Writer, dir string, manifestBytes []byte, files []FileEntry) error {
	gzw := gzip.NewWriter(w)
	gzw.ModTime = time.Unix(0, 0)
	gzw.OS = 255

	tw := tar.NewWriter(gzw)

	if err := writeArchiveMember(tw, gzw, "package.yaml", "package.yaml", manifestBytes); err != nil {
		return err
	}

	sigPath := filepath.Join(dir, "package.sig")
	if sigBytes, err := os.ReadFile(sigPath); err == nil && len(sigBytes) > 0 {
		if err := writeArchiveMember(tw, gzw, "package.sig", "package.sig", sigBytes); err != nil {
			return err
		}
	}

	for _, fe := range files {
		diskPath := filepath.Join(dir, filepath.FromSlash(fe.Path))
		b, err := os.ReadFile(diskPath)
		if err != nil {
			_ = gzw.Close()
			return fmt.Errorf("read file %q: %w", fe.Path, err)
		}
		if err := writeArchiveMember(tw, gzw, fe.Path, strconv.Quote(fe.Path), b); err != nil {
			return err
		}
	}

	if err := tw.Close(); err != nil {
		_ = gzw.Close()
		return fmt.Errorf("close tar writer: %w", err)
	}
	if err := gzw.Close(); err != nil {
		return fmt.Errorf("close gzip writer: %w", err)
	}
	return nil
}

// writeArchiveMember writes one regular file member with the archive's fixed mode
// and timestamp, closing the gzip writer when the write fails. label is how the
// error names the member: the reserved members are named bare and a file member is
// named quoted, which is the difference the two callers have always made.
func writeArchiveMember(tw *tar.Writer, gzw *gzip.Writer, name, label string, data []byte) error {
	hdr := &tar.Header{
		Name:    name,
		Mode:    0644,
		Size:    int64(len(data)),
		ModTime: time.Unix(0, 0),
		Format:  tar.FormatPAX,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		_ = gzw.Close()
		return fmt.Errorf("write %s header: %w", label, err)
	}
	if _, err := tw.Write(data); err != nil {
		_ = gzw.Close()
		return fmt.Errorf("write %s content: %w", label, err)
	}
	return nil
}
