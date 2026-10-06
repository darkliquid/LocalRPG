package core

// MechanicsSpec is a system's optional declarative mechanics: the stats it
// declares, its skills, its health convention, and how checks resolve. Every
// part is optional; a nil MechanicsSpec means the system is schema-agnostic.
type MechanicsSpec struct {
	Stats  []StatSpec       `yaml:"stats,omitempty"`
	Skills []SkillSpec      `yaml:"skills,omitempty"`
	Health *HealthSpec      `yaml:"health,omitempty"`
	Checks CheckConventions `yaml:"checks,omitempty"`
	// AllowFreeformState permits state changes to undeclared paths even when the
	// system declares stats.
	AllowFreeformState bool `yaml:"allow_freeform_state,omitempty"`
	// Engagement is the system's default mechanics policy: "off", "auto", or
	// "ask". Empty means the configured default.
	Engagement string `yaml:"engagement,omitempty"`
	// Advancement is the optional progression schema. A nil value means the
	// system has no advancement.
	Advancement *AdvancementSpec `yaml:"advancement,omitempty"`
}

// AdvancementSpec declares a system's progression: an earned currency, how it is
// earned, and what it buys.
type AdvancementSpec struct {
	Currency  CurrencySpec `yaml:"currency"`
	Mode      string       `yaml:"mode,omitempty"` // spend | track | threshold
	Earn      []EarnRule   `yaml:"earn,omitempty"`
	TrackSize int          `yaml:"track_size,omitempty"`
	Gate      string       `yaml:"gate,omitempty"` // "" | downtime
	Unlocks   []UnlockSpec `yaml:"unlocks,omitempty"`
	Levels    []LevelSpec  `yaml:"levels,omitempty"`
}

// CurrencySpec names the stat that holds earned advancement points.
type CurrencySpec struct {
	Stat  string `yaml:"stat"`
	Label string `yaml:"label,omitempty"`
}

// EarnRule awards the currency when an engine-recognised event occurs.
type EarnRule struct {
	On      string `yaml:"on"`                // miss | check_outcome | turn_end | hook
	Outcome string `yaml:"outcome,omitempty"` // for on: check_outcome
	Rank    string `yaml:"rank,omitempty"`
	Amount  int    `yaml:"amount"`
}

// UnlockSpec is one thing the currency can buy.
type UnlockSpec struct {
	ID          string       `yaml:"id"`
	Label       string       `yaml:"label"`
	Description string       `yaml:"description,omitempty"`
	Cost        int          `yaml:"cost"`
	Requires    []string     `yaml:"requires,omitempty"`
	Effects     []EffectSpec `yaml:"effects,omitempty"`
}

// EffectSpec is one change an unlock applies.
type EffectSpec struct {
	Type   string `yaml:"type"` // stat_increase | set_stat | grant_tag | hook
	Stat   string `yaml:"stat,omitempty"`
	Amount int    `yaml:"amount,omitempty"`
	Max    int    `yaml:"max,omitempty"`
	Tag    string `yaml:"tag,omitempty"`
	Hook   string `yaml:"hook,omitempty"`
}

// LevelSpec is a threshold-mode level.
type LevelSpec struct {
	At      int          `yaml:"at"`
	Label   string       `yaml:"label,omitempty"`
	Effects []EffectSpec `yaml:"effects,omitempty"`
}

// StatSpec declares one stat.
type StatSpec struct {
	ID      string      `yaml:"id"`
	Label   string      `yaml:"label,omitempty"`
	Type    string      `yaml:"type,omitempty"` // number | string | bool
	Default interface{} `yaml:"default,omitempty"`
	Min     *int        `yaml:"min,omitempty"`
	Max     *int        `yaml:"max,omitempty"`
}

// SkillSpec declares one skill and the stat that governs it.
type SkillSpec struct {
	ID    string `yaml:"id"`
	Label string `yaml:"label,omitempty"`
	Stat  string `yaml:"stat,omitempty"`
}

// HealthSpec declares which stats represent health and what happens at zero.
type HealthSpec struct {
	Stat       string `yaml:"stat"`
	MaxStat    string `yaml:"max_stat,omitempty"`
	ZeroEffect string `yaml:"zero_effect,omitempty"`
}

// CheckConventions describe how a check resolves by default.
type CheckConventions struct {
	Notation   string                       `yaml:"notation,omitempty"`
	Outcome    []string                     `yaml:"outcome,omitempty"`
	Difficulty []DifficultySpec             `yaml:"difficulty,omitempty"`
	Profiles   map[string]ResolutionProfile `yaml:"profiles,omitempty"`
}

// DifficultySpec is one named difficulty target.
type DifficultySpec struct {
	ID     string `yaml:"id"`
	Label  string `yaml:"label,omitempty"`
	Target int    `yaml:"target"`
}
