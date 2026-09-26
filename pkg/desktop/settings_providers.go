package desktop

import (
	"context"

	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/provider"
	"github.com/darkliquid/localrpg/pkg/ui"
)

// testProvider is injected by Run for a live service.
var testProvider func(ctx context.Context, svc *gui.Service, req gui.TestProviderRequestDTO) (*gui.TestProviderResponseDTO, error)

// providersForFamily filters the catalogue by provider family.
func providersForFamily(family provider.Family) []provider.Descriptor {
	var out []provider.Descriptor
	for _, p := range appState.Providers {
		if p.Family == family {
			out = append(out, p)
		}
	}
	return out
}

func settingsProviders(p ui.Palette) {
	cfg := appState.Config
	Label("Cloud & Ecosystem Providers", FontSize(14), FontWeight(WeightBold), TextColorVec(p.Text))
	Label("Google Gemini API key (shared default)", FontSize(12), TextColorVec(p.Muted))
	PasswordInput(&cfg.Providers.Gemini.APIKey)
	Label("Registered providers", FontSize(12), FontWeight(WeightBold), TextColorVec(p.Muted))
	if len(appState.Providers) == 0 {
		Label("No providers registered.", FontSize(11), TextColorVec(p.Muted))
		return
	}
	for _, desc := range appState.Providers {
		Label(desc.Label+" · "+string(desc.Family)+" · "+desc.Source, FontSize(11), TextColorVec(p.Muted))
	}
}

func settingsAgents(p ui.Palette) {
	cfg := appState.Config
	Label("AI Agents & Role Routing", FontSize(14), FontWeight(WeightBold), TextColorVec(p.Text))

	if cfg.Agents.ActionEcho != nil {
		CheckBox(cfg.Agents.ActionEcho, "Echo the player's action in narration")
	}

	roles := make([]string, 0, len(cfg.Agents.Roles))
	for name := range cfg.Agents.Roles {
		roles = append(roles, name)
	}
	if len(roles) == 0 {
		Label("No roles configured.", FontSize(12), TextColorVec(p.Muted))
		return
	}
	if appState.SelectedRole == "" {
		appState.SelectedRole = roles[0]
	}
	role := cfg.Agents.Roles[appState.SelectedRole]

	Label("Role", FontSize(12), TextColorVec(p.Muted))
	MenuButton(NoIcon, appState.SelectedRole, func() {
		for _, name := range roles {
			name := name
			if MenuItem(NoIcon, name) {
				appState.SelectedRole = name
				appState.TestResult = nil
			}
		}
	})

	Label("Type", FontSize(12), TextColorVec(p.Muted))
	OptionGroup(&role.Type, func() {
		OptionButton("Disabled", "disabled")
		OptionButton("HTTP", "http")
		OptionButton("CLI", "cli")
		OptionButton("Built-in", "builtin")
		OptionButton("Inherit", "inherit")
	})

	Label("Endpoint", FontSize(12), TextColorVec(p.Muted))
	TextInput(&role.Endpoint)
	Label("Model", FontSize(12), TextColorVec(p.Muted))
	TextInput(&role.Model)
	Label("API key", FontSize(12), TextColorVec(p.Muted))
	PasswordInput(&role.APIKey)
	Label("Command", FontSize(12), TextColorVec(p.Muted))
	TextInput(&role.Command)
	Label("Built-in name", FontSize(12), TextColorVec(p.Muted))
	TextInput(&role.BuiltinName)
	Label("Inherit from", FontSize(12), TextColorVec(p.Muted))
	TextInput(&role.InheritFrom)

	temp := float32(role.Temperature)
	Label("Temperature", FontSize(12), TextColorVec(p.Muted))
	Slider(&temp, SliderAttrs{Min: 0, Max: 2, Step: 0.05, Width: 320})
	role.Temperature = float64(temp)

	intSlider(&role.MaxTokens, 0, 200000, 256)

	cfg.Agents.Roles[appState.SelectedRole] = role

	Container(Attrs(Row, CrossMid, Gap(8)), func() {
		NextAccessName("settings.agents.test")
		if Button(NoIcon, "Test") {
			svc := liveService
			current := role
			if testProvider != nil && svc != nil {
				go func() {
					res, err := testProvider(context.Background(), svc, gui.TestProviderRequestDTO{
						Category: "llm", Provider: current,
					})
					if err != nil {
						res = &gui.TestProviderResponseDTO{Message: err.Error()}
					}
					WithFrameLock(func() { appState.TestResult = res })
					RequestNextFrame()
				}()
			}
		}
		AssignAccess()
		if appState.TestResult != nil {
			Label(appState.TestResult.Message, FontSize(11), TextColorVec(p.Muted))
		}
	})
}

// applyPreset decodes a provider preset's config map into a role config.
func applyPreset(role *config.AgentRoleConfig, preset provider.Preset) {
	if preset.Config == nil {
		return
	}
	if v, ok := preset.Config["type"].(string); ok {
		role.Type = v
	}
	if v, ok := preset.Config["builtin_name"].(string); ok {
		role.BuiltinName = v
	}
	if v, ok := preset.Config["endpoint"].(string); ok {
		role.Endpoint = v
	}
	if v, ok := preset.Config["model"].(string); ok {
		role.Model = v
	}
}
