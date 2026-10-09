package gui

import (
	"fmt"
	"sync"
)

// Playback owners. Exactly one of them plays a session's narration: the
// application's own audio device, or the browser's audio elements.
const (
	ownerDevice  = "device"
	ownerBrowser = "browser"
)

// ledgerTurnsKept bounds how many turns' ledgers a campaign keeps. A turn's
// ledger matters until its audio is done, which is at most a turn or two behind
// the newest, so a small window covers a slow finalise and a reconnect.
const ledgerTurnsKept = 4

// maxLedgerEntries bounds what one merge may add, so a client cannot grow the
// server's memory without limit. A turn has tens of clips, not hundreds.
const maxLedgerEntries = 512

// playbackState is a session's playback ledgers and owner. It is in memory per
// session and never persisted: a fresh session has heard nothing.
type playbackState struct {
	mu sync.Mutex
	// owner is the explicit owner, set on a handover. Empty means none was set
	// yet, and the owner is derived from whether the device can play.
	owner string
	// ledgers holds each campaign's recent turn ledgers, oldest first.
	ledgers map[string][]turnLedger
}

// turnLedger is one turn's ledger.
type turnLedger struct {
	turn   int
	ledger *Ledger
}

// PlaybackLedgerDTO is a turn's ledger on the wire, with the session's owner so
// a client learns both in one exchange.
type PlaybackLedgerDTO struct {
	Turn    int              `json:"turn"`
	Owner   string           `json:"owner"`
	Entries map[string]Entry `json:"entries"`
}

// PlaybackLedgerRequest is a client's offsets for a turn's clips, sent at the
// turn handover and again on a reconnect. Turn 0 names the newest turn.
type PlaybackLedgerRequest struct {
	Turn    int              `json:"turn"`
	Entries map[string]Entry `json:"entries"`
}

// turnLedger returns the ledger for a campaign's turn, creating it when it is
// new. Creating a turn's ledger evicts the oldest beyond ledgerTurnsKept, which
// is how a turn's ledger is discarded once its audio is long done.
func (s *Service) turnLedger(gameID string, turn int) *Ledger {
	p := &s.playback
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ledgers == nil {
		p.ledgers = map[string][]turnLedger{}
	}
	kept := p.ledgers[gameID]
	for _, tl := range kept {
		if tl.turn == turn {
			return tl.ledger
		}
	}
	ledger := NewLedger()
	kept = append(kept, turnLedger{turn: turn, ledger: ledger})
	// Keep the window ordered by turn, so eviction drops the oldest turn even
	// when a reconnect names one out of order.
	for i := len(kept) - 1; i > 0 && kept[i].turn < kept[i-1].turn; i-- {
		kept[i], kept[i-1] = kept[i-1], kept[i]
	}
	if len(kept) > ledgerTurnsKept {
		kept = append([]turnLedger(nil), kept[len(kept)-ledgerTurnsKept:]...)
	}
	p.ledgers[gameID] = kept
	return ledger
}

// newestTurn is the newest turn a campaign holds a ledger for, or 0.
func (s *Service) newestTurn(gameID string) int {
	p := &s.playback
	p.mu.Lock()
	defer p.mu.Unlock()
	kept := p.ledgers[gameID]
	if len(kept) == 0 {
		return 0
	}
	return kept[len(kept)-1].turn
}

// PlaybackLedger reports a turn's ledger and the owner. Turn 0 names the newest
// turn the session knows; a campaign with none reports an empty ledger.
func (s *Service) PlaybackLedger(gameID string, turn int) PlaybackLedgerDTO {
	if turn <= 0 {
		turn = s.newestTurn(gameID)
	}
	dto := PlaybackLedgerDTO{Turn: turn, Owner: s.PlaybackOwner(), Entries: map[string]Entry{}}
	if turn > 0 {
		dto.Entries = s.turnLedger(gameID, turn).Snapshot()
	}
	return dto
}

// MergePlaybackLedger folds a client's offsets into a turn's ledger and returns
// the merged result, so the client's mirror converges on the server's. The merge
// is a max, so a resend after a reconnect is harmless. Keys that are not clip
// keys are rejected rather than stored.
func (s *Service) MergePlaybackLedger(gameID string, req PlaybackLedgerRequest) (PlaybackLedgerDTO, error) {
	if len(req.Entries) > maxLedgerEntries {
		return PlaybackLedgerDTO{}, fmt.Errorf("too many ledger entries: %d (limit %d)", len(req.Entries), maxLedgerEntries)
	}
	for key := range req.Entries {
		if !clipKeyPattern.MatchString(key) {
			return PlaybackLedgerDTO{}, fmt.Errorf("invalid clip key %q", key)
		}
	}
	turn := req.Turn
	if turn <= 0 {
		turn = s.newestTurn(gameID)
	}
	if turn <= 0 {
		// Nothing to merge into and nothing to report: a fresh session.
		return PlaybackLedgerDTO{Owner: s.PlaybackOwner(), Entries: map[string]Entry{}}, nil
	}
	s.turnLedger(gameID, turn).Merge(req.Entries)
	return s.PlaybackLedger(gameID, turn), nil
}

// PlaybackOwner names who plays the session's narration. An explicit handover
// wins; before one, the device owns playback when it can play at all, which is
// the same rule the client used to apply on its own.
func (s *Service) PlaybackOwner() string {
	s.playback.mu.Lock()
	owner := s.playback.owner
	s.playback.mu.Unlock()
	if owner != "" {
		return owner
	}
	if player := s.audioPlayer(); player != nil && player.Available() {
		return ownerDevice
	}
	return ownerBrowser
}

// setPlaybackOwner records an explicit handover. The owner changes only here,
// never on an idle device, so the two paths cannot flap between turns.
func (s *Service) setPlaybackOwner(owner string) {
	if owner != ownerDevice && owner != ownerBrowser {
		return
	}
	s.playback.mu.Lock()
	s.playback.owner = owner
	s.playback.mu.Unlock()
}
