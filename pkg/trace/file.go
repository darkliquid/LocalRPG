package trace

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileOptions configures the JSONL sink.
type FileOptions struct {
	Level        Level
	MaxBytes     int64
	MaxFiles     int
	RotateCheck  int
	PayloadChars int
}

// FileSink appends one JSON object per line to a single file. It is the canonical
// trace: a per-session file would make `tail -f` a hunt for timestamps, and an
// appended file has no natural age, so size is the only honest bound.
type FileSink struct {
	mu       sync.Mutex
	path     string
	opts     FileOptions
	game     string
	file     *os.File
	writer   *bufio.Writer
	written  int64
	since    int
	failures int
}

func NewFileSink(path string, opts FileOptions) (*FileSink, error) {
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = 268435456
	}
	if opts.MaxFiles <= 0 {
		opts.MaxFiles = 3
	}
	if opts.RotateCheck <= 0 {
		opts.RotateCheck = 200
	}
	if opts.PayloadChars <= 0 {
		opts.PayloadChars = 20000
	}

	sink := &FileSink{path: path, opts: opts}
	if err := sink.open(); err != nil {
		return nil, err
	}
	return sink, nil
}

func (s *FileSink) Path() string { return s.path }

func (s *FileSink) Enabled(level Level) bool {
	return s != nil && s.opts.Level != LevelOff && level <= s.opts.Level
}

// Failures reports how many events could not be written. Tracing never fails a
// turn, so a count is the only signal available.
func (s *FileSink) Failures() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.failures
}

// SetGame stamps later events with a campaign, so one appended file can still be
// filtered per campaign.
func (s *FileSink) SetGame(gameID string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.game = gameID
}

func (s *FileSink) Event(name string, fields map[string]interface{}) {
	if s == nil || s.opts.Level == LevelOff {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	line := map[string]interface{}{
		"ts":    time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		"event": name,
		"level": s.opts.Level.String(),
	}
	if s.game != "" {
		line["game"] = s.game
	}
	for key, value := range Sanitize(fields, s.opts.Level, s.opts.PayloadChars) {
		line[key] = value
	}

	encoded, err := json.Marshal(line)
	if err != nil {
		s.failures++
		return
	}
	if s.writer == nil {
		s.failures++
		return
	}

	encoded = append(encoded, '\n')
	written, err := s.writer.Write(encoded)
	if err == nil {
		// Flushed per event, so a crash keeps what was seen.
		err = s.writer.Flush()
	}
	if err != nil {
		s.failures++
		return
	}

	s.written += int64(written)
	s.since++
	if s.since >= s.opts.RotateCheck {
		s.rotateLocked()
	}
}

func (s *FileSink) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closeLocked()
}

func (s *FileSink) closeLocked() error {
	if s.writer != nil {
		_ = s.writer.Flush()
		s.writer = nil
	}
	if s.file != nil {
		err := s.file.Close()
		s.file = nil
		return err
	}
	return nil
}

func (s *FileSink) open() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return fmt.Errorf("create trace dir: %w", err)
	}
	file, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("open trace file: %w", err)
	}
	if info, err := file.Stat(); err == nil {
		s.written = info.Size()
	}
	s.file = file
	s.writer = bufio.NewWriter(file)
	return nil
}

// rotateLocked shifts the numbered files up and starts a fresh live file. It runs
// only when the ceiling has been crossed, so the rotate-check cadence bounds how
// often the size is considered without making the hot path stat the file.
func (s *FileSink) rotateLocked() {
	if s.written < s.opts.MaxBytes {
		return
	}

	_ = s.closeLocked()

	for index := s.opts.MaxFiles - 1; index >= 1; index-- {
		from := fmt.Sprintf("%s.%d", s.path, index)
		to := fmt.Sprintf("%s.%d", s.path, index+1)
		if _, err := os.Stat(from); err == nil {
			_ = os.Rename(from, to)
		}
	}
	_ = os.Rename(s.path, s.path+".1")
	_ = os.Remove(fmt.Sprintf("%s.%d", s.path, s.opts.MaxFiles+1))

	s.written = 0
	s.since = 0
	if err := s.open(); err != nil {
		s.writer = nil
		s.file = nil
	}
}

// Multi fans one event out to several loggers, so a CLI run can print to stderr
// and write the file from the same call sites.
func Multi(loggers ...Logger) Logger {
	active := make([]Logger, 0, len(loggers))
	for _, logger := range loggers {
		if logger != nil {
			active = append(active, logger)
		}
	}
	if len(active) == 0 {
		return Nop()
	}
	return multiLogger(active)
}

type multiLogger []Logger

func (m multiLogger) Enabled(level Level) bool {
	for _, logger := range m {
		if logger.Enabled(level) {
			return true
		}
	}
	return false
}

func (m multiLogger) Event(name string, fields map[string]interface{}) {
	for _, logger := range m {
		logger.Event(name, fields)
	}
}

func (m multiLogger) SetGame(gameID string) {
	for _, logger := range m {
		logger.SetGame(gameID)
	}
}
