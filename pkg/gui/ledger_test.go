package gui

import (
	"sync"
	"testing"
)

func TestLedgerRecordAndMerge(t *testing.T) {
	l := NewLedger()
	l.Record("a", 500, 2000, false)
	l.Merge(map[string]Entry{"a": {PlayedMS: 900, TotalMS: 2000}})
	e, _ := l.Entry("a")
	if e.PlayedMS != 900 {
		t.Fatalf("played = %d, want the max 900", e.PlayedMS)
	}
}

func TestLedgerMergeNeverRewinds(t *testing.T) {
	l := NewLedger()
	l.Record("a", 900, 2000, false)
	l.Merge(map[string]Entry{"a": {PlayedMS: 100}})
	e, _ := l.Entry("a")
	if e.PlayedMS != 900 || e.TotalMS != 2000 {
		t.Fatalf("entry = %+v, want the later offset kept", e)
	}
}

func TestLedgerCompleteWins(t *testing.T) {
	l := NewLedger()
	l.Record("a", 2000, 2000, true)
	l.Merge(map[string]Entry{"a": {PlayedMS: 100}})
	e, _ := l.Entry("a")
	if !e.Complete {
		t.Fatal("a complete entry must not be downgraded")
	}
}

func TestLedgerMergeIsIdempotent(t *testing.T) {
	l := NewLedger()
	incoming := map[string]Entry{"a": {PlayedMS: 700, TotalMS: 1500}, "b": {Complete: true}}
	l.Merge(incoming)
	first := l.Snapshot()
	l.Merge(incoming)
	second := l.Snapshot()
	if len(first) != len(second) || first["a"] != second["a"] || first["b"] != second["b"] {
		t.Fatalf("a repeated merge changed the ledger: %+v then %+v", first, second)
	}
}

func TestLedgerIgnoresEmptyKeysAndNegativeOffsets(t *testing.T) {
	l := NewLedger()
	l.Record("", 100, 100, true)
	l.Merge(map[string]Entry{"": {Complete: true}, "a": {PlayedMS: -5, TotalMS: -1}})
	if _, ok := l.Entry(""); ok {
		t.Fatal("an empty key was recorded")
	}
	e, ok := l.Entry("a")
	if !ok || e.PlayedMS != 0 || e.TotalMS != 0 {
		t.Fatalf("entry = %+v, %v, want negative values clamped to zero", e, ok)
	}
}

func TestLedgerHeardState(t *testing.T) {
	l := NewLedger()
	l.Record("done", 1000, 1000, true)
	l.Record("half", 400, 1000, false)
	if !l.Complete("done") || l.Complete("half") || l.Complete("missing") {
		t.Fatal("complete reports the wrong clips")
	}
	if !l.Partial("half") || l.Partial("done") || l.Partial("missing") {
		t.Fatal("partial reports the wrong clips")
	}
}

func TestNilLedgerIsEmpty(t *testing.T) {
	var l *Ledger
	l.Record("a", 1, 1, true)
	l.Merge(map[string]Entry{"a": {Complete: true}})
	if _, ok := l.Entry("a"); ok {
		t.Fatal("a nil ledger reported an entry")
	}
	if l.Complete("a") || l.Partial("a") || len(l.Snapshot()) != 0 {
		t.Fatal("a nil ledger is not empty")
	}
}

func TestLedgerIsSafeForConcurrentUse(t *testing.T) {
	l := NewLedger()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			l.Record("a", i*10, 1000, false)
			l.Merge(map[string]Entry{"a": {PlayedMS: i * 20}})
		}(i)
	}
	wg.Wait()
	if e, _ := l.Entry("a"); e.PlayedMS != 15*20 {
		t.Fatalf("played = %d, want the max %d", e.PlayedMS, 15*20)
	}
}

func TestOwnerFlips(t *testing.T) {
	svc := &Service{}
	ch, cancel := svc.SubscribeAudioStatus()
	defer cancel()

	svc.setPlaybackOwner(ownerDevice)
	if got := svc.PlaybackOwner(); got != ownerDevice {
		t.Fatalf("owner = %q, want %q", got, ownerDevice)
	}

	svc.broadcastAudioStatus(AudioStatusDTO{Playing: true})
	select {
	case status := <-ch:
		if status.Owner != ownerDevice {
			t.Fatalf("broadcast owner = %q, want %q", status.Owner, ownerDevice)
		}
	default:
		t.Fatal("expected status broadcast")
	}

	svc.setPlaybackOwner(ownerBrowser)
	if got := svc.PlaybackOwner(); got != ownerBrowser {
		t.Fatalf("owner = %q, want %q", got, ownerBrowser)
	}
}
