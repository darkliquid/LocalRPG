// Package trace records what the system actually did: the prompt it assembled,
// the request it sent, what came back, and how the reply was parsed. It exists
// because every diagnosis of a bad turn previously meant inferring from the
// timeline, and every remedy was therefore a guess.
package trace

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// Level is how much detail a trace records.
type Level int

const (
	// LevelOff writes nothing. It is the default, so normal play costs nothing.
	LevelOff Level = iota
	// LevelSummary records decisions, sizes, timings, and errors, but no payloads.
	LevelSummary
	// LevelFull additionally records prompts, generated text, and raw wire lines.
	LevelFull
)

func (l Level) String() string {
	switch l {
	case LevelSummary:
		return "summary"
	case LevelFull:
		return "full"
	default:
		return "off"
	}
}

// ParseLevel reads a configured level. An empty value means off, so configuration
// written before tracing existed behaves as it did.
func ParseLevel(value string) (Level, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "off", "none", "false":
		return LevelOff, nil
	case "summary":
		return LevelSummary, nil
	case "full", "true", "debug":
		return LevelFull, nil
	default:
		return LevelOff, fmt.Errorf("unknown trace level %q", value)
	}
}

// Logger records one structured event. Implementations must never block a turn
// and must never fail it: a sink that cannot write counts the failure instead.
type Logger interface {
	// Enabled reports whether events at level would be recorded.
	Enabled(level Level) bool
	// Event records a named event. Fields are flat values, nested maps, or lists.
	Event(name string, fields map[string]any)
	// SetGame stamps later events with a campaign, so one appended trace can be
	// filtered back down to a campaign without a second file.
	SetGame(gameID string)
}

// ContextLogger is an optional extension of Logger for callers that hold a
// context: an event can then be attached to the active span. Event remains the
// fallback, so packages with no context keep working unchanged.
type ContextLogger interface {
	Logger
	EventCtx(ctx context.Context, name string, fields map[string]any)
}

// LogEvent records an event through the context-aware variant when the logger
// supports it, so span-attached events are used wherever a context exists and
// nothing changes where it does not.
func LogEvent(ctx context.Context, logger Logger, name string, fields map[string]any) {
	if contextual, ok := logger.(ContextLogger); ok {
		contextual.EventCtx(ctx, name, fields)
		return
	}
	logger.Event(name, fields)
}

type nopLogger struct{}

// Nop is the logger used when tracing is off, so no caller branches on nil.
func Nop() Logger { return nopLogger{} }

func (nopLogger) Enabled(Level) bool           { return false }
func (nopLogger) Event(string, map[string]any) {}
func (nopLogger) SetGame(string)               {}

// OrNil makes a possibly-nil logger safe to use.
func OrNil(logger Logger) Logger {
	if logger == nil {
		return Nop()
	}
	return logger
}

// Event is one recorded event, as it was handed to the sink.
type Event struct {
	Name   string
	Fields map[string]any
}

// Memory keeps events in memory. It is what tests assert against, so tracing is
// testable without touching a filesystem.
type Memory struct {
	mu     sync.Mutex
	level  Level
	game   string
	events []Event
}

func NewMemory(level Level) *Memory {
	return &Memory{level: level}
}

func (m *Memory) Enabled(level Level) bool {
	return m.level != LevelOff && level <= m.level
}

// Event behaves like a real sink apart from the disk: nothing is recorded when
// tracing is off, and payloads are dropped below full. Tests therefore assert the
// same filtering a written trace would have.
func (m *Memory) Event(name string, fields map[string]any) {
	if m == nil || m.level == LevelOff {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	stamped := Sanitize(fields, m.level, defaultPayloadChars)
	if m.game != "" {
		stamped = make(map[string]any, len(fields)+1)
		for key, value := range fields {
			stamped[key] = value
		}
		stamped["game"] = m.game
	}
	m.events = append(m.events, Event{Name: name, Fields: stamped})
}

// SetGame stamps later events with a campaign.
func (m *Memory) SetGame(gameID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.game = gameID
}

func (m *Memory) Events() []Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Event(nil), m.events...)
}

func (m *Memory) Names() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	names := make([]string, 0, len(m.events))
	for _, event := range m.events {
		names = append(names, event.Name)
	}
	return names
}

func (m *Memory) Find(name string) (Event, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, event := range m.events {
		if event.Name == name {
			return event, true
		}
	}
	return Event{}, false
}
