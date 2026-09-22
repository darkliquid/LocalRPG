package models

import (
	"archive/tar"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

var (
	ErrModelNotFound    = errors.New("model not found in registry")
	ErrDownloadActive   = errors.New("model download is already active")
	ErrChecksumMismatch = errors.New("downloaded archive failed checksum verification")
)

type ModelSpec struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	URL           string   `json:"url"`
	SHA256        string   `json:"sha256"`
	SizeBytes     int64    `json:"size_bytes"`
	ArchiveType   string   `json:"archive_type"` // "tar", "tar.gz", "tar.bz2"
	Subdir        string   `json:"subdir"`
	RequiredFiles []string `json:"required_files"`
}

type ModelStatus struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Installed       bool    `json:"installed"`
	Downloading     bool    `json:"downloading"`
	Progress        float64 `json:"progress"`
	BytesDownloaded int64   `json:"bytes_downloaded"`
	TotalBytes      int64   `json:"total_bytes"`
	Error           string  `json:"error,omitempty"`
}

type Manager struct {
	cacheDir  string
	mu        sync.RWMutex
	specs     map[string]ModelSpec
	active    map[string]*downloadSession
	listeners map[chan ModelStatus]struct{}
}

type downloadSession struct {
	status   ModelStatus
	cancel   context.CancelFunc
	channels []chan ModelStatus
}

func NewManager(cacheDir string) *Manager {
	m := &Manager{
		cacheDir:  cacheDir,
		specs:     make(map[string]ModelSpec),
		active:    make(map[string]*downloadSession),
		listeners: make(map[chan ModelStatus]struct{}),
	}
	m.registerDefaultSpecs()
	return m
}

func (m *Manager) registerDefaultSpecs() {
	m.specs["kokoro-tts"] = ModelSpec{
		ID:          "kokoro-tts",
		Name:        "Kokoro Voice Pack",
		URL:         "https://github.com/k2-fsa/sherpa-onnx/releases/download/tts-models/kokoro-en-v0_19.tar.bz2",
		SHA256:      "a3d3c82e666c0d0a2dbe5429399432d67786440dbd06b539bf58778f654b50c0",
		SizeBytes:   90177536,
		ArchiveType: "tar.bz2",
		Subdir:      filepath.Join("tts", "kokoro"),
		RequiredFiles: []string{
			"model.onnx",
			"voices.bin",
			"tokens.txt",
			"espeak-ng-data",
		},
	}
}

func (m *Manager) RegisterSpec(spec ModelSpec) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.specs[spec.ID] = spec
}

func (m *Manager) ModelDir(modelID string) string {
	m.mu.RLock()
	spec, ok := m.specs[modelID]
	m.mu.RUnlock()
	if !ok {
		return ""
	}
	return filepath.Join(m.cacheDir, "models", spec.Subdir)
}

func (m *Manager) Status(modelID string) ModelStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	spec, ok := m.specs[modelID]
	if !ok {
		return ModelStatus{ID: modelID, Error: "unknown model"}
	}

	if sess, ok := m.active[modelID]; ok {
		return sess.status
	}

	installed := m.isInstalledLocked(spec)
	return ModelStatus{
		ID:         spec.ID,
		Name:       spec.Name,
		Installed:  installed,
		TotalBytes: spec.SizeBytes,
	}
}

func (m *Manager) ListStatuses() []ModelStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	statuses := make([]ModelStatus, 0, len(m.specs))
	for _, spec := range m.specs {
		if sess, ok := m.active[spec.ID]; ok {
			statuses = append(statuses, sess.status)
		} else {
			statuses = append(statuses, ModelStatus{
				ID:         spec.ID,
				Name:       spec.Name,
				Installed:  m.isInstalledLocked(spec),
				TotalBytes: spec.SizeBytes,
			})
		}
	}
	return statuses
}

func (m *Manager) isInstalledLocked(spec ModelSpec) bool {
	destDir := filepath.Join(m.cacheDir, "models", spec.Subdir)
	for _, req := range spec.RequiredFiles {
		if _, err := os.Stat(filepath.Join(destDir, req)); err != nil {
			return false
		}
	}
	return true
}

func (m *Manager) Subscribe() chan ModelStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch := make(chan ModelStatus, 16)
	m.listeners[ch] = struct{}{}
	return ch
}

func (m *Manager) Unsubscribe(ch chan ModelStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.listeners, ch)
	close(ch)
}

func (m *Manager) broadcast(status ModelStatus) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for ch := range m.listeners {
		select {
		case ch <- status:
		default:
		}
	}
}

func (m *Manager) Download(ctx context.Context, modelID string) (<-chan ModelStatus, error) {
	m.mu.Lock()
	spec, ok := m.specs[modelID]
	if !ok {
		m.mu.Unlock()
		return nil, ErrModelNotFound
	}

	if _, ok := m.active[modelID]; ok {
		m.mu.Unlock()
		return nil, ErrDownloadActive
	}

	downloadCtx, cancel := context.WithCancel(ctx)
	statusCh := make(chan ModelStatus, 16)
	session := &downloadSession{
		status: ModelStatus{
			ID:          modelID,
			Name:        spec.Name,
			Downloading: true,
			TotalBytes:  spec.SizeBytes,
		},
		cancel:   cancel,
		channels: []chan ModelStatus{statusCh},
	}
	m.active[modelID] = session
	m.mu.Unlock()

	go m.runDownload(downloadCtx, spec, session)

	return statusCh, nil
}

func (m *Manager) runDownload(ctx context.Context, spec ModelSpec, session *downloadSession) {
	updateStatus := func(update func(s *ModelStatus)) {
		m.mu.Lock()
		update(&session.status)
		current := session.status
		for _, ch := range session.channels {
			select {
			case ch <- current:
			default:
			}
		}
		m.mu.Unlock()
		m.broadcast(current)
	}

	cleanup := func(err error) {
		m.mu.Lock()
		if err != nil {
			session.status.Downloading = false
			session.status.Error = err.Error()
		} else {
			session.status.Downloading = false
			session.status.Installed = true
			session.status.Progress = 1.0
			session.status.Error = ""
		}
		finalStatus := session.status
		for _, ch := range session.channels {
			select {
			case ch <- finalStatus:
			default:
			}
			close(ch)
		}
		delete(m.active, spec.ID)
		m.mu.Unlock()
		m.broadcast(finalStatus)
	}

	tmpDir := filepath.Join(m.cacheDir, "models", "tmp")
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		cleanup(fmt.Errorf("create tmp dir: %w", err))
		return
	}

	partPath := filepath.Join(tmpDir, spec.ID+".part")
	defer os.Remove(partPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, spec.URL, nil)
	if err != nil {
		cleanup(fmt.Errorf("prepare request: %w", err))
		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cleanup(fmt.Errorf("download request: %w", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		cleanup(fmt.Errorf("download HTTP error %d: %s", resp.StatusCode, resp.Status))
		return
	}

	outFile, err := os.Create(partPath)
	if err != nil {
		cleanup(fmt.Errorf("create part file: %w", err))
		return
	}

	hasher := sha256.New()
	writer := io.MultiWriter(outFile, hasher)

	buf := make([]byte, 64*1024)
	var downloaded int64
	total := spec.SizeBytes
	if resp.ContentLength > 0 {
		total = resp.ContentLength
	}

	for {
		select {
		case <-ctx.Done():
			_ = outFile.Close()
			cleanup(ctx.Err())
			return
		default:
		}

		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := writer.Write(buf[:n]); werr != nil {
				_ = outFile.Close()
				cleanup(fmt.Errorf("write part file: %w", werr))
				return
			}
			downloaded += int64(n)
			prog := 0.0
			if total > 0 {
				prog = float64(downloaded) / float64(total)
			}
			updateStatus(func(s *ModelStatus) {
				s.BytesDownloaded = downloaded
				s.TotalBytes = total
				s.Progress = prog
			})
		}
		if rerr != nil {
			if rerr == io.EOF {
				break
			}
			_ = outFile.Close()
			cleanup(fmt.Errorf("download stream read: %w", rerr))
			return
		}
	}

	_ = outFile.Close()

	// Verify checksum
	calculatedSum := hex.EncodeToString(hasher.Sum(nil))
	if spec.SHA256 != "" && calculatedSum != spec.SHA256 {
		cleanup(fmt.Errorf("%w: expected %s, got %s", ErrChecksumMismatch, spec.SHA256, calculatedSum))
		return
	}

	// Extract to staging directory
	stagingDir := filepath.Join(tmpDir, spec.ID+"_extracted")
	_ = os.RemoveAll(stagingDir)
	if err := os.MkdirAll(stagingDir, 0755); err != nil {
		cleanup(fmt.Errorf("create staging dir: %w", err))
		return
	}
	defer os.RemoveAll(stagingDir)

	if err := m.extractArchive(partPath, spec.ArchiveType, stagingDir); err != nil {
		cleanup(fmt.Errorf("extract archive: %w", err))
		return
	}

	// Atomically move to target directory
	targetDir := filepath.Join(m.cacheDir, "models", spec.Subdir)
	_ = os.RemoveAll(targetDir)
	if err := os.MkdirAll(filepath.Dir(targetDir), 0755); err != nil {
		cleanup(fmt.Errorf("create parent dir: %w", err))
		return
	}

	// Check if extracted contents are nested in a single root directory
	entries, err := os.ReadDir(stagingDir)
	if err == nil && len(entries) == 1 && entries[0].IsDir() {
		nested := filepath.Join(stagingDir, entries[0].Name())
		if err := os.Rename(nested, targetDir); err != nil {
			cleanup(fmt.Errorf("move extracted folder: %w", err))
			return
		}
	} else {
		if err := os.Rename(stagingDir, targetDir); err != nil {
			cleanup(fmt.Errorf("move extracted files: %w", err))
			return
		}
	}

	cleanup(nil)
}

func (m *Manager) extractArchive(archivePath, archiveType, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	var tr *tar.Reader
	switch archiveType {
	case "tar":
		tr = tar.NewReader(f)
	case "tar.gz":
		gz, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer gz.Close()
		tr = tar.NewReader(gz)
	case "tar.bz2":
		bz := bzip2.NewReader(f)
		tr = tar.NewReader(bz)
	default:
		return fmt.Errorf("unsupported archive format: %s", archiveType)
	}

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		cleanPath := filepath.Clean(header.Name)
		if filepath.IsAbs(cleanPath) || cleanPath == ".." || len(cleanPath) > 2 && cleanPath[:3] == "../" {
			continue // Zip Slip prevention
		}

		target := filepath.Join(destDir, cleanPath)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
	return nil
}
