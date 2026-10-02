package harness

import "sync"

// Usage is one provider call's consumption. Estimated marks a value derived from
// request shape rather than reported by the provider. Batch marks a call made
// through a provider's batch API, which is priced at a discount.
type Usage struct {
	Provider     string `json:"provider,omitempty"`
	Model        string `json:"model,omitempty"`
	InputTokens  int    `json:"input_tokens,omitempty"`
	OutputTokens int    `json:"output_tokens,omitempty"`
	Characters   int    `json:"characters,omitempty"`
	Requests     int    `json:"requests,omitempty"`
	Estimated    bool   `json:"estimated,omitempty"`
	Batch        bool   `json:"batch,omitempty"`
}

// UsageRecorder is the sink a Router or Extractor reports to. It knows the role
// because the caller does.
type UsageRecorder interface {
	RecordUsage(role string, u Usage)
}

// UsageSink is where a UsageContext ultimately writes: a campaign database,
// stamped with the turn.
type UsageSink interface {
	RecordUsage(gameID string, turn int, role string, u Usage)
}

// UsageContext stamps every record with the campaign and the current turn. One
// is created per turn and shared by the Router and Extractor, so a concurrent
// extraction is attributed to the same turn.
type UsageContext struct {
	sink   UsageSink
	gameID string

	mu   sync.Mutex
	turn int
}

func NewUsageContext(sink UsageSink, gameID string) *UsageContext {
	return &UsageContext{sink: sink, gameID: gameID}
}

func (c *UsageContext) SetTurn(turn int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.turn = turn
}

// RecordUsage stamps the record with the context's campaign and turn.
func (c *UsageContext) RecordUsage(role string, u Usage) {
	if c == nil || c.sink == nil {
		return
	}
	c.mu.Lock()
	turn := c.turn
	gameID := c.gameID
	c.mu.Unlock()
	c.sink.RecordUsage(gameID, turn, role, u)
}
