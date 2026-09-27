package harness

import (
	"sync"
	"testing"
)

type captureSink struct {
	mu   sync.Mutex
	rows []struct {
		game string
		turn int
		role string
		u    Usage
	}
}

func (c *captureSink) RecordUsage(gameID string, turn int, role string, u Usage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rows = append(c.rows, struct {
		game string
		turn int
		role string
		u    Usage
	}{gameID, turn, role, u})
}

func TestUsageContextStampsTheTurn(t *testing.T) {
	sink := &captureSink{}
	ctx := NewUsageContext(sink, "campaign-01")
	ctx.SetTurn(3)
	ctx.RecordUsage("gm", Usage{Provider: "gemini", InputTokens: 10})
	ctx.RecordUsage("tts", Usage{Provider: "elevenlabs", Characters: 20})

	if len(sink.rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(sink.rows))
	}
	if sink.rows[0].turn != 3 || sink.rows[0].game != "campaign-01" || sink.rows[0].role != "gm" {
		t.Fatalf("first row = %+v", sink.rows[0])
	}
	if sink.rows[1].u.Characters != 20 {
		t.Fatalf("second row = %+v", sink.rows[1])
	}
}
