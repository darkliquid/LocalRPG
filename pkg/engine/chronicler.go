package engine

import (
	"context"
	"fmt"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/storage"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// Chronicler owns a campaign's long memory: whether a regeneration is due, the
// regeneration itself, and the note it writes. It is a separate type from the
// orchestrator because its work is detached from any turn.
type Chronicler struct {
	// The timeline is the campaign's log, so the chronicler reads the same one the
	// turn pipeline writes rather than deriving a path of its own.
	timeline   *Timeline
	store      *storage.Store
	summariser *harness.Summariser
	every      int
	logger     trace.Logger
}

func NewChronicler(timeline *Timeline, store *storage.Store, summariser *harness.Summariser) *Chronicler {
	return &Chronicler{timeline: timeline, store: store, summariser: summariser}
}

// SetEvery sets the cadence. Zero disables summarisation.
func (c *Chronicler) SetEvery(every int) {
	c.every = every
}

func (c *Chronicler) SetLogger(logger trace.Logger) {
	c.logger = trace.OrNil(logger)
}

// Recap returns the campaign's summary, or an empty one when it has none.
func (c *Chronicler) Recap(gameID string) (Chronicle, error) {
	return ReadChronicle(c.store)
}

// Due reports whether the campaign has turned far enough past its summary to need
// another. It is false when summarisation is disabled or has no provider, so a
// caller never has to check both.
func (c *Chronicler) Due(gameID string) (bool, error) {
	if c.every <= 0 || c.summariser == nil {
		return false, nil
	}

	chronicle, err := ReadChronicle(c.store)
	if err != nil {
		return false, err
	}

	turns, err := c.loadHistory(gameID)
	if err != nil {
		return false, err
	}
	if len(turns) == 0 {
		return false, nil
	}

	latest := turns[len(turns)-1].Number
	if chronicle.ThroughTurn >= latest {
		return false, nil
	}
	return latest-chronicle.ThroughTurn >= c.every, nil
}

// Regenerate summarises everything since the last one and writes the note. It
// reports whether the chronicle changed, and it leaves through_turn alone when the
// provider fails so the next trigger retries the same range.
func (c *Chronicler) Regenerate(ctx context.Context, gameID string) (bool, error) {
	if c.every <= 0 || c.summariser == nil {
		return false, nil
	}

	chronicle, err := ReadChronicle(c.store)
	if err != nil {
		return false, err
	}

	turns, err := c.loadHistory(gameID)
	if err != nil {
		return false, err
	}

	pending := make([]harness.SummaryTurn, 0, len(turns))
	for _, turn := range turns {
		if turn.Number <= chronicle.ThroughTurn {
			continue
		}
		pending = append(pending, harness.SummaryTurn{
			Number:    turn.Number,
			Mode:      turn.Mode,
			Input:     turn.Input,
			Narration: turn.Narration,
		})
	}
	if len(pending) == 0 {
		return false, nil
	}

	summary, err := c.summariser.Summarise(ctx, chronicle.Summary, pending)
	if err != nil {
		return false, err
	}

	through := pending[len(pending)-1].Number
	if err := WriteChronicle(c.store, c.entitiesDir(gameID), Chronicle{
		Summary:     summary,
		ThroughTurn: through,
	}); err != nil {
		return false, err
	}

	c.logger = trace.OrNil(c.logger)
	c.logger.Event("summary.written", map[string]any{
		"from_turn": chronicle.ThroughTurn + 1,
		"to_turn":   through,
		"chars":     len([]rune(summary)),
	})

	return true, nil
}

func (c *Chronicler) loadHistory(gameID string) ([]Turn, error) {
	if c.timeline == nil {
		return nil, fmt.Errorf("load history: no timeline")
	}

	turns, err := c.timeline.history.LoadHistory()
	if err != nil {
		return nil, fmt.Errorf("load history: %w", err)
	}
	return turns, nil
}

func (c *Chronicler) entitiesDir(gameID string) string {
	if c.timeline == nil {
		return ""
	}
	return c.timeline.EntitiesDir()
}
