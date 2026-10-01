package driver

// ActionType enumerates the supported declarative scenario actions.
type ActionType string

const (
	ActionNavigate          ActionType = "navigate"
	ActionClick             ActionType = "click"
	ActionTypeInput         ActionType = "type"
	ActionWaitVisible       ActionType = "wait_visible"
	ActionAssertVisible     ActionType = "assert_visible"
	ActionAssertTurnOutcome ActionType = "assert_turn_outcome"
	ActionFaultInjection    ActionType = "fault_injection"
	ActionSleep             ActionType = "sleep"
	ActionScreenshot        ActionType = "screenshot"
)

// Step defines a single declarative driver action.
type Step struct {
	Action       ActionType `yaml:"action" json:"action"`
	URL          string     `yaml:"url,omitempty" json:"url,omitempty"`
	Selector     string     `yaml:"selector,omitempty" json:"selector,omitempty"`
	Text         string     `yaml:"text,omitempty" json:"text,omitempty"`
	TimeoutMs    int        `yaml:"timeout_ms,omitempty" json:"timeout_ms,omitempty"`
	TextContains string     `yaml:"text_contains,omitempty" json:"text_contains,omitempty"`
	Expected     string     `yaml:"expected,omitempty" json:"expected,omitempty"`
	Fault        string     `yaml:"fault,omitempty" json:"fault,omitempty"`
	Path         string     `yaml:"path,omitempty" json:"path,omitempty"`
	Width        int        `yaml:"width,omitempty" json:"width,omitempty"`
	Height       int        `yaml:"height,omitempty" json:"height,omitempty"`
	Format       string     `yaml:"format,omitempty" json:"format,omitempty"`
	Quality      int        `yaml:"quality,omitempty" json:"quality,omitempty"`
}

// Setup defines initial environment requirements for the scenario.
type Setup struct {
	World        string            `yaml:"world,omitempty" json:"world,omitempty"`
	System       string            `yaml:"system,omitempty" json:"system,omitempty"`
	MockProvider map[string]string `yaml:"mock_provider,omitempty" json:"mock_provider,omitempty"`
}

// Scenario encapsulates an entire test routine.
type Scenario struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Setup       Setup  `yaml:"setup,omitempty" json:"setup,omitempty"`
	Steps       []Step `yaml:"steps" json:"steps"`
}
