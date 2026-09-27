package harness

import (
	"errors"
	"testing"
	"time"
)

func TestRegistryBlocksAndExpiresOnRead(t *testing.T) {
	r := NewLimitRegistry()
	r.Block("gemini", "gm", time.Now().Add(50*time.Millisecond))
	if _, ok := r.Blocked("gemini", "gm"); !ok {
		t.Fatal("expected a block")
	}
	if _, ok := r.Blocked("gemini", "tts"); ok {
		t.Fatal("a different role must be unaffected")
	}
	time.Sleep(80 * time.Millisecond)
	if _, ok := r.Blocked("gemini", "gm"); ok {
		t.Fatal("block should have expired")
	}
}

func TestRegistryClearLiftsABlock(t *testing.T) {
	r := NewLimitRegistry()
	r.Block("gemini", "gm", time.Now().Add(time.Hour))
	r.Clear("gemini", "gm")
	if _, ok := r.Blocked("gemini", "gm"); ok {
		t.Fatal("Clear must lift the block")
	}
}

func TestRegistryKeepsTheLatestDeadline(t *testing.T) {
	r := NewLimitRegistry()
	soon := time.Now().Add(time.Minute)
	later := time.Now().Add(time.Hour)
	r.Block("gemini", "gm", later)
	r.Block("gemini", "gm", soon)
	until, ok := r.Blocked("gemini", "gm")
	if !ok || !until.Equal(later) {
		t.Fatalf("a shorter block must not shorten a longer one: %v, %v", until, ok)
	}
}

func TestRegistryRecordsFundsFailureWithoutBlocking(t *testing.T) {
	r := NewLimitRegistry()
	r.RecordFundsFailure("elevenlabs", "tts", "insufficient_credits")
	if _, ok := r.Blocked("elevenlabs", "tts"); ok {
		t.Fatal("insufficient funds must not block")
	}
	states := r.Snapshot()
	if len(states) != 1 || states[0].FundsFailure == "" {
		t.Fatalf("funds failure not recorded: %+v", states)
	}
	r.ClearFundsFailure("elevenlabs", "tts")
	if len(r.Snapshot()) != 0 {
		t.Fatal("funds failure should clear")
	}
}

func TestErrRateLimitedUntilCarriesDeadline(t *testing.T) {
	until := time.Now().Add(time.Minute)
	err := &ErrRateLimitedUntil{Provider: "gemini", Role: "gm", Until: until}
	var target *ErrRateLimitedUntil
	if !errors.As(err, &target) || target.Until != until {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.RetryAfter() <= 0 {
		t.Fatal("RetryAfter should be positive before the deadline")
	}
}
