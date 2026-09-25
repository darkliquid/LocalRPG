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
	Notation   string           `yaml:"notation,omitempty"`
	Outcome    []string         `yaml:"outcome,omitempty"`
	Difficulty []DifficultySpec `yaml:"difficulty,omitempty"`
}

// DifficultySpec is one named difficulty target.
type DifficultySpec struct {
	ID     string `yaml:"id"`
	Label  string `yaml:"label,omitempty"`
	Target int    `yaml:"target"`
}
