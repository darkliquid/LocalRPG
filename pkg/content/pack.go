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
	"strings"
	"time"

	"github.com/darkliquid/localrpg/pkg/core"
)

// BuildManifest scans dir and constructs a Manifest from its files and descriptor.
// If typ is empty, it attempts to detect "world" or "system" based on world.yaml or system.yaml.
func BuildManifest(dir, typ string, meta ManifestMeta) (Manifest, error) {
	if typ == "" {
		if _, err := os.Stat(filepath.Join(dir, "world.yaml")); err == nil {
			typ = "world"
		} else if _, err := os.Stat(filepath.Join(dir, "system.yaml")); err == nil {
			typ = "system"
		}
	}

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

		name := entry.Name()
		if strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".legacy") || name == "package.yaml" || name == "package.sig" {
			return nil
		}
		if !entry.Type().IsRegular() {
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
		return Manifest{}, fmt.Errorf("scan content directory: %w", err)
	}

	sort.Strings(relPaths)

	fileEntries := make([]FileEntry, 0, len(relPaths))
	for _, rel := range relPaths {
		diskPath := filepath.Join(dir, filepath.FromSlash(rel))
		b, err := os.ReadFile(diskPath)
		if err != nil {
			return Manifest{}, fmt.Errorf("read file %q: %w", rel, err)
		}
		h := sha256.Sum256(b)
		hexSum := hex.EncodeToString(h[:])
		size := int64(len(b))

		fileEntries = append(fileEntries, FileEntry{
			Path:   rel,
			SHA256: hexSum,
			Size:   size,
		})
	}

	m := Manifest{
		Type:  typ,
		Files: fileEntries,
	}

	if typ == "world" {
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
	} else if typ == "system" {
		sysPath := filepath.Join(dir, "system.yaml")
		if sm, err := core.LoadSystemManifest(sysPath); err == nil && sm != nil {
			m.ID = sm.ID
			m.Name = sm.Name
			m.Description = sm.Description
			m.Version = sm.Version
		}
	}

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
	if m.Version == "" {
		m.Version = "1.0.0"
	}

	if err := core.ValidateSemver(m.Version); err != nil {
		return Manifest{}, fmt.Errorf("package version: %w", err)
	}

	return m, nil
}

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

	gzw := gzip.NewWriter(w)
	gzw.ModTime = time.Unix(0, 0)
	gzw.OS = 255

	tw := tar.NewWriter(gzw)

	pkgHdr := &tar.Header{
		Name:     "package.yaml",
		Mode:     0644,
		Size:     int64(len(manifestBytes)),
		ModTime:  time.Unix(0, 0),
		Format:   tar.FormatPAX,
	}
	if err := tw.WriteHeader(pkgHdr); err != nil {
		_ = gzw.Close()
		return Manifest{}, fmt.Errorf("write package.yaml header: %w", err)
	}
	if _, err := tw.Write(manifestBytes); err != nil {
		_ = gzw.Close()
		return Manifest{}, fmt.Errorf("write package.yaml content: %w", err)
	}

	sigPath := filepath.Join(dir, "package.sig")
	if sigBytes, err := os.ReadFile(sigPath); err == nil && len(sigBytes) > 0 {
		sigHdr := &tar.Header{
			Name:     "package.sig",
			Mode:     0644,
			Size:     int64(len(sigBytes)),
			ModTime:  time.Unix(0, 0),
			Format:   tar.FormatPAX,
		}
		if err := tw.WriteHeader(sigHdr); err != nil {
			_ = gzw.Close()
			return Manifest{}, fmt.Errorf("write package.sig header: %w", err)
		}
		if _, err := tw.Write(sigBytes); err != nil {
			_ = gzw.Close()
			return Manifest{}, fmt.Errorf("write package.sig content: %w", err)
		}
	}

	for _, fe := range m.Files {
		diskPath := filepath.Join(dir, filepath.FromSlash(fe.Path))
		b, err := os.ReadFile(diskPath)
		if err != nil {
			_ = gzw.Close()
			return Manifest{}, fmt.Errorf("read file %q: %w", fe.Path, err)
		}

		hdr := &tar.Header{
			Name:     fe.Path,
			Mode:     0644,
			Size:     fe.Size,
			ModTime:  time.Unix(0, 0),
			Format:   tar.FormatPAX,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			_ = gzw.Close()
			return Manifest{}, fmt.Errorf("write %q header: %w", fe.Path, err)
		}
		if _, err := tw.Write(b); err != nil {
			_ = gzw.Close()
			return Manifest{}, fmt.Errorf("write %q content: %w", fe.Path, err)
		}
	}

	if err := tw.Close(); err != nil {
		_ = gzw.Close()
		return Manifest{}, fmt.Errorf("close tar writer: %w", err)
	}
	if err := gzw.Close(); err != nil {
		return Manifest{}, fmt.Errorf("close gzip writer: %w", err)
	}

	return m, nil
}
