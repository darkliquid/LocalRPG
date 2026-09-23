package engine

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/rules"
)

type Turn struct {
	Number    int                  `json:"number"`
	Timestamp time.Time            `json:"timestamp"`
	Mode      string               `json:"mode"` // "Do", "Say", "Story", "Roll", "GM", "System"
	Input     string               `json:"input"`
	Narration string               `json:"narration"`
	Segments  []entity.TurnSegment `json:"segments,omitempty"`
	Roll      *rules.RollResult    `json:"roll,omitempty"`
	Entities  []entity.Mention     `json:"entities,omitempty"`
	Location  string               `json:"location,omitempty"`
	Outcome   string               `json:"outcome,omitempty"`
	// Truncated records that the model hit its token limit mid-reply, so the
	// client can say so instead of presenting a cut-off scene as a complete one.
	Truncated bool `json:"truncated,omitempty"`
	// Recovery records how a reply that stopped mid-thought was repaired:
	// "continued" (a second call finished it), "trimmed" (the unfinished tail
	// was dropped), "kept" (recovery was skipped or failed), or empty (nothing
	// was wrong). Truncated is true only when the recorded prose is still
	// incomplete.
	Recovery string `json:"recovery,omitempty"`
	// ContextNotes records anything the prompt budget left out, so a thinner reply
	// can be explained rather than looking like drift.
	ContextNotes []string `json:"context_notes,omitempty"`
	// ContinuityNotes record prose that contradicts what the campaign knows. They are
	// advisory: nothing is rewritten, and a correction is the player's to send.
	ContinuityNotes []string `json:"continuity_notes,omitempty"`

	// LegacyOutput is only populated when reading records written before the
	// narration rename. New records must not set it.
	LegacyOutput string `json:"output,omitempty"`
}

// Prose returns the narrator-visible text regardless of which field holds it.
func (t Turn) Prose() string {
	if strings.TrimSpace(t.Narration) != "" {
		return t.Narration
	}
	return t.LegacyOutput
}

type HistoryLogger struct {
	mu   sync.RWMutex
	path string
}

func NewHistoryLogger(path string) *HistoryLogger {
	return &HistoryLogger{path: path}
}

func (h *HistoryLogger) AppendTurn(t Turn) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	f, err := os.OpenFile(h.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("open history file: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("marshal turn: %w", err)
	}

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write turn: %w", err)
	}
	return nil
}

func (h *HistoryLogger) LoadHistory() ([]Turn, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.loadHistoryUnlocked()
}

func (h *HistoryLogger) RewindToTurn(targetTurnNumber int) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	turns, err := h.loadHistoryUnlocked()
	if err != nil {
		return err
	}

	var kept []Turn
	for _, t := range turns {
		if t.Number <= targetTurnNumber {
			kept = append(kept, t)
		}
	}

	f, err := os.Create(h.path)
	if err != nil {
		return fmt.Errorf("rewrite history file: %w", err)
	}
	defer f.Close()

	for _, t := range kept {
		data, err := json.Marshal(t)
		if err != nil {
			return err
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			return err
		}
	}
	return nil
}

func (h *HistoryLogger) loadHistoryUnlocked() ([]Turn, error) {
	f, err := os.Open(h.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Turn{}, nil
		}
		return nil, err
	}
	defer f.Close()

	var turns []Turn
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var t Turn
		if err := json.Unmarshal(line, &t); err == nil {
			if strings.TrimSpace(t.Narration) == "" && t.LegacyOutput != "" {
				t.Narration = t.LegacyOutput
				t.LegacyOutput = ""
			}
			turns = append(turns, t)
		}
	}
	return turns, scanner.Err()
}
