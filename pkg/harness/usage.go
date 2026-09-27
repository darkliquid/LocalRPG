package harness

// Usage is one provider call's consumption. Estimated marks a value derived from
// request shape rather than reported by the provider.
type Usage struct {
	Provider     string `json:"provider,omitempty"`
	Model        string `json:"model,omitempty"`
	InputTokens  int    `json:"input_tokens,omitempty"`
	OutputTokens int    `json:"output_tokens,omitempty"`
	Characters   int    `json:"characters,omitempty"`
	Requests     int    `json:"requests,omitempty"`
	Estimated    bool   `json:"estimated,omitempty"`
}
