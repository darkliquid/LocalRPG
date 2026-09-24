package harness

// RefKind defines the type of reference tracked in prompt provenance.
type RefKind string

const (
	RefEntity   RefKind = "entity"
	RefTurn     RefKind = "turn"
	RefSummary  RefKind = "summary"
	RefThread   RefKind = "thread"
	RefWorld    RefKind = "world"
	RefSystem   RefKind = "system"
	RefLocation RefKind = "location"
	RefSession  RefKind = "session"
)

// Ref is one thing a section read. Relation names why: present, mention,
// retrieved, arc, voice, recall, action.
type Ref struct {
	Kind     RefKind `json:"kind"`
	ID       string  `json:"id"`
	Relation string  `json:"relation,omitempty"`
}

// SectionReport describes one prompt section and what it referenced.
type SectionReport struct {
	Name     string `json:"name"`
	Tokens   int    `json:"tokens"`
	Included bool   `json:"included"`
	Source   string `json:"source,omitempty"`
	Refs     []Ref  `json:"refs,omitempty"`
}

// ContextStrategy is how the assembled context reaches the provider.
type ContextStrategy string

const (
	StrategyFullPrompt    ContextStrategy = "full_prompt"
	StrategyCachedPrefix  ContextStrategy = "cached_prefix"
	StrategyServerSession ContextStrategy = "server_session"
)

// ProviderSession identifies a server-held conversation. It is a cache keyed by
// the local tip it covers, never the source of truth.
type ProviderSession struct {
	Provider    string `json:"provider"`
	ID          string `json:"id"`
	ThroughTurn int    `json:"through_turn"`
	Model       string `json:"model,omitempty"`
	PrefixHash  string `json:"prefix_hash,omitempty"`
}

// TurnContext is the durable description of one turn's context.
type TurnContext struct {
	TurnNumber      int              `json:"turn_number"`
	Mode            string           `json:"mode"`
	Budget          int              `json:"budget"`
	EstimatedTokens int              `json:"estimated_tokens"`
	Sections        []SectionReport  `json:"sections"`
	Refs            []Ref            `json:"refs"`
	WorkingSet      []Ref            `json:"working_set"`
	Threads         []string         `json:"threads,omitempty"`
	SummaryVersion  int              `json:"summary_version"`
	WorldHash       string           `json:"world_hash,omitempty"`
	SystemHash      string           `json:"system_hash,omitempty"`
	PromptHash      string           `json:"prompt_hash"`
	Strategy        ContextStrategy  `json:"strategy"`
	PrefixHash      string           `json:"prefix_hash,omitempty"`
	Session         *ProviderSession `json:"session,omitempty"`
	CachedTokens    int              `json:"cached_tokens,omitempty"`
}
