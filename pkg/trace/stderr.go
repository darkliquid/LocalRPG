package trace

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
)

// StderrLogger prints events as one line each. It is what makes `--trace` useful
// from a terminal without tailing a file, and it is only constructed when someone
// asked for it, so it never adds noise to normal play.
type StderrLogger struct {
	mu       sync.Mutex
	level    Level
	game     string
	out      io.Writer
	payloads int
}

// NewStderr returns a logger that writes to standard error.
func NewStderr(level Level) *StderrLogger {
	return &StderrLogger{level: level, out: os.Stderr, payloads: defaultPayloadChars}
}

// SetWriter redirects the output, which is what a test needs to read it.
func (l *StderrLogger) SetWriter(out io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.out = out
}

// SetLevel changes the detail recorded.
func (l *StderrLogger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

func (l *StderrLogger) Enabled(level Level) bool {
	return l.level != LevelOff && level <= l.level
}

func (l *StderrLogger) Event(name string, fields map[string]interface{}) {
	if !l.Enabled(LevelSummary) {
		return
	}

	stamped := fields
	if l.game != "" {
		stamped = make(map[string]interface{}, len(fields)+1)
		for key, value := range fields {
			stamped[key] = value
		}
		stamped["game"] = l.game
	}

	encoded, err := json.Marshal(Sanitize(stamped, l.level, l.payloads))
	if err != nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintf(l.out, "trace %s %s\n", name, encoded)
}

// SetGame stamps later events with a campaign, matching the file sink.
func (l *StderrLogger) SetGame(gameID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.game = gameID
}
