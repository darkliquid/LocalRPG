package gui

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/engine"
	"github.com/darkliquid/localrpg/pkg/harness"
)

// maxNegotiationsPerCheck bounds the argument: each counter replaces the terms,
// and an unbounded loop would let a player renegotiate forever.
const maxNegotiationsPerCheck = 3

// ErrInvalidCounter means the counter-proposal says nothing.
var ErrInvalidCounter = errors.New("the counter-proposal says nothing")

// ErrCheckResolved means the pending check has already been rolled.
var ErrCheckResolved = errors.New("the check has already been resolved")

// ErrNegotiationLimit means the check has been argued as often as it may be.
var ErrNegotiationLimit = errors.New("the check has been renegotiated as often as it may be")

// RenegotiateRequestDTO is a player's counter-proposal to a pending check.
type RenegotiateRequestDTO struct {
	PendingRef string                  `json:"pending_check_ref"`
	Counter    harness.CounterProposal `json:"counter"`
}

// RenegotiateResultDTO is the GM's ruling and the terms now in force.
type RenegotiateResultDTO struct {
	Ruling       harness.Adjudication  `json:"ruling"`
	PendingCheck *PendingCheckDTO      `json:"pending_check,omitempty"`
	Negotiations []harness.Negotiation `json:"negotiations,omitempty"`
}

// Renegotiate asks the GM to rule on a counter-proposal and updates the pending
// check in place, so the player then rolls the agreed terms. A hold leaves the
// check as it was, and the reason explains why.
func (s *Service) Renegotiate(ctx context.Context, gameID string, turnNumber int, req RenegotiateRequestDTO) (*RenegotiateResultDTO, error) {
	if strings.TrimSpace(req.Counter.Approach) == "" && strings.TrimSpace(req.Counter.Stakes) == "" && strings.TrimSpace(req.Counter.Difficulty) == "" {
		return nil, ErrInvalidCounter
	}

	gameDir := s.resolver.GameDir(gameID)
	manifest, err := core.LoadGameManifest(fmt.Sprintf("%s/game.yaml", gameDir))
	if err != nil {
		return nil, fmt.Errorf("load manifest: %w", err)
	}

	turns, err := s.cachedHistory(gameID)
	if err != nil {
		return nil, err
	}
	index := -1
	for i := range turns {
		if turns[i].Number == turnNumber {
			index = i
			break
		}
	}
	if index < 0 {
		return nil, fmt.Errorf("%w: turn %d", fs.ErrNotExist, turnNumber)
	}
	turn := turns[index]
	if turn.PendingCheck == nil {
		// A turn that resolved its check carries the check's ref, which is how a
		// counter after the roll is refused rather than silently accepted.
		if turn.ResolvesCheckRef != "" {
			return nil, ErrCheckResolved
		}
		return nil, ErrNoPendingCheck
	}
	if req.PendingRef != "" && req.PendingRef != turn.PendingCheck.Ref {
		return nil, ErrPendingCheckMismatch
	}
	if count := negotiationsFor(turn.Negotiations, turn.PendingCheck.Ref); count >= maxNegotiationsPerCheck {
		return nil, ErrNegotiationLimit
	}

	runtime, err := s.runtimeFor(gameID, manifest)
	if err != nil {
		return nil, err
	}
	provider, err := runtime.router.GetProviderForRole("gm")
	if err != nil {
		return nil, &harness.GenerationFailure{
			Code:    harness.FailureProviderUnavailable,
			Message: fmt.Sprintf("no GM provider to adjudicate with: %v", err),
		}
	}

	ruling, err := engine.Adjudicate(ctx, provider, *turn.PendingCheck, req.Counter)
	if err != nil {
		return nil, err
	}

	if ruling.Agreed() {
		applyAdjudication(turn.PendingCheck, ruling)
	}
	turn.Negotiations = append(turn.Negotiations, harness.Negotiation{Counter: req.Counter, Ruling: ruling})

	store, err := s.store(gameID)
	if err != nil {
		return nil, err
	}
	timeline := engine.NewTimeline(s.resolver, store, engine.NewHistoryLogger(fmt.Sprintf("%s/history.jsonl", gameDir)), gameID)
	if err := timeline.UpdateTurn(ctx, &turn); err != nil {
		return nil, fmt.Errorf("update turn: %w", err)
	}
	// The history cache is keyed on size and mtime, so an in-place rewrite is
	// picked up on the next read; dropping it here keeps the very next request from
	// racing a filesystem timestamp.
	s.invalidateHistoryCache(gameID)

	return &RenegotiateResultDTO{
		Ruling:       ruling,
		PendingCheck: s.pendingCheckDTO(turn.PendingCheck, store),
		Negotiations: turn.Negotiations,
	}, nil
}

// applyAdjudication writes the agreed terms onto the pending check, leaving any
// field the GM did not restate as it was.
func applyAdjudication(pending *harness.PendingCheck, ruling harness.Adjudication) {
	if stakes := strings.TrimSpace(ruling.Stakes); stakes != "" {
		pending.Request.Stakes = stakes
	}
	if difficulty := strings.TrimSpace(ruling.Difficulty); difficulty != "" {
		pending.Request.Difficulty = difficulty
	}
	if notation := strings.TrimSpace(ruling.Notation); notation != "" {
		pending.Request.Notation = notation
	}
	if profile := strings.TrimSpace(ruling.Profile); profile != "" {
		pending.Request.Profile = profile
	}
}

// negotiationsFor counts the counters recorded against one pending check.
func negotiationsFor(negotiations []harness.Negotiation, ref string) int {
	// The check keeps its ref across a renegotiation, so every recorded counter
	// belongs to it.
	if ref == "" {
		return len(negotiations)
	}
	return len(negotiations)
}
