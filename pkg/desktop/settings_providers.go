package desktop

import (
	"context"
	"fmt"

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

// providerPresets flattens the preset catalogue for one family.
func providerPresets(family provider.Family) []provider.Preset {
	var out []provider.Preset
	for _, desc := range providersForFamily(family) {
		out = append(out, desc.Presets...)
	}
	return out
}

func settingsProviders(p ui.Palette) {
	cfg := appState.Config
	settingsSectionTitle(TypCloudStorage, "Cloud & Ecosystem Providers")
	settingsHint("Shared credentials and the registered provider catalogue.")
	Label("Google Gemini API key (shared default)", FontSize(12), Fonts(ui.SansStack...), TextColorVec(ui.TextMuted))
	FieldPassword(&cfg.Providers.Gemini.APIKey)

	settingsSubTitle(TypCloudStorage, "Gemini Model Catalogue")
	Container(Attrs(Row, CrossMid, Gap(10)), func() {
		settingsSmallButton("settings.providers.models", "List Gemini Models", fetchModelsNow)
		if appState.FetchingModels {
			Label("Fetching…", Fonts(Monospace...), FontSize(11), TextColorVec(ui.TextMuted))
		} else if len(appState.ModelList) > 0 {
			Label(fmt.Sprintf("%d models", len(appState.ModelList)), Fonts(Monospace...), FontSize(11), TextColorVec(ui.TextMuted))
		}
	})
	if appState.ModelListErr != "" {
		Label(appState.ModelListErr, FontSize(11), TextColorVec(ui.Danger))
	}
	Container(Attrs(Row, Wrap, Gap(6)), func() {
		for i, model := range appState.ModelList {
			if i >= 40 {
				return
			}
			model := model
			selected := model.ID == cfg.Agents.Roles[appState.SelectedRole].Model
			studioPill("settings.providers.model."+model.ID, model.ID, selected, func() {
				role := cfg.Agents.Roles[appState.SelectedRole]
				role.Model = model.ID
				cfg.Agents.Roles[appState.SelectedRole] = role
			})
		}
	})

	settingsSubTitle(TypPlug, "Registered providers")
	if len(appState.Providers) == 0 {
		settingsHint("No providers registered.")
		return
	}
	for _, desc := range appState.Providers {
		Container(Attrs(Row, CrossMid, Gap(10), Corners(10), Pad2(8, 12), BackgroundVec(ui.PillBG),
			BorderWidth(1), BorderColorVec(ui.Hairline)), func() {
			Icon(TypCloudStorage, FontSize(14), TextColorVec(ui.Accent))
			Label(desc.Label, Fonts(ui.SansStack...), FontSize(12), FontWeight(WeightBold), TextColorVec(ui.TextMain))
			Label(string(desc.Family), FontSize(11), TextColorVec(ui.TextMuted))
			Filler(1)
			Label(desc.Source, Fonts(Monospace...), FontSize(11), TextColorVec(ui.TextFaint))
		})
	}
}

func settingsAgents(p ui.Palette) {
	cfg := appState.Config
	settingsSectionTitle(TypGroup, "AI Agents & Role Routing")
	settingsHint("Route each narrative role to a model provider and tune its parameters.")

	if presets := providerPresets(provider.FamilyLLM); len(presets) > 0 {
		MenuButton(NoIcon, "Load Agent Preset", func() {
			for _, preset := range presets {
				preset := preset
				if MenuItem(NoIcon, preset.Label) {
					role := cfg.Agents.Roles[appState.SelectedRole]
					applyPreset(&role, preset)
					cfg.Agents.Roles[appState.SelectedRole] = role
				}
			}
		})
	}

	if cfg.Agents.ActionEcho != nil {
		CheckBox(cfg.Agents.ActionEcho, "Echo the player's action in narration")
	}

	roles := make([]string, 0, len(cfg.Agents.Roles))
	for name := range cfg.Agents.Roles {
		roles = append(roles, name)
	}
	if len(roles) == 0 {
		settingsHint("No roles configured.")
		return
	}
	if appState.SelectedRole == "" {
		appState.SelectedRole = roles[0]
	}
	role := cfg.Agents.Roles[appState.SelectedRole]

	settingsSubTitle(TypUser, "Role")
	MenuButton(NoIcon, appState.SelectedRole, func() {
		for _, name := range roles {
			name := name
			if MenuItem(NoIcon, name) {
				appState.SelectedRole = name
				appState.TestResult = nil
			}
		}
	})

	settingsSubTitle(TypCog, "Provider")
	Label("Type", FontSize(12), Fonts(ui.SansStack...), TextColorVec(ui.TextMuted))
	OptionGroup(&role.Type, func() {
		OptionButton("Disabled", "disabled")
		OptionButton("HTTP", "http")
		OptionButton("CLI", "cli")
		OptionButton("Built-in", "builtin")
		OptionButton("Inherit", "inherit")
	})

	Label("Endpoint", FontSize(12), Fonts(ui.SansStack...), TextColorVec(ui.TextMuted))
	FieldInput(&role.Endpoint)
	Label("Model", FontSize(12), Fonts(ui.SansStack...), TextColorVec(ui.TextMuted))
	FieldInput(&role.Model)
	Label("API key", FontSize(12), Fonts(ui.SansStack...), TextColorVec(ui.TextMuted))
	FieldPassword(&role.APIKey)
	Label("Command", FontSize(12), Fonts(ui.SansStack...), TextColorVec(ui.TextMuted))
	FieldInput(&role.Command)
	Label("Built-in name", FontSize(12), Fonts(ui.SansStack...), TextColorVec(ui.TextMuted))
	FieldInput(&role.BuiltinName)
	Label("Inherit from", FontSize(12), Fonts(ui.SansStack...), TextColorVec(ui.TextMuted))
	FieldInput(&role.InheritFrom)

	settingsSubTitle(TypAdjustContrast, "Parameters")
	temp := float32(role.Temperature)
	Label("Temperature", FontSize(12), Fonts(ui.SansStack...), TextColorVec(ui.TextMuted))
	Slider(&temp, SliderAttrs{Min: 0, Max: 2, Step: 0.05, Width: 320})
	role.Temperature = float64(temp)

	Label("Max tokens", FontSize(12), Fonts(ui.SansStack...), TextColorVec(ui.TextMuted))
	intSlider(&role.MaxTokens, 0, 200000, 256)

	cfg.Agents.Roles[appState.SelectedRole] = role

	providerTestRow("settings.agents.test", "Test Connection", "llm", role)
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
