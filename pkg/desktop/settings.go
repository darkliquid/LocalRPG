package desktop

import (
	"context"

	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/ui"
)

// saveSettings is injected by Run for a live service.
var saveSettings func(ctx context.Context, svc *gui.Service, cfg config.Config) error

// settingsTab returns the active settings tab, defaulting to paths.
func settingsTab() string {
	if appState.SettingsTab == "" {
		return "paths"
	}
	return appState.SettingsTab
}

// openGlobalSettings switches to the settings screen and loads the config.
func openGlobalSettings() {
	appState.Screen = ScreenSettings
	appState.SettingsSaved = false
	svc := liveService
	if svc == nil {
		return
	}
	go func() {
		ctx := context.Background()
		loadSettings(ctx, svc)
		providers := loadProviders(ctx, svc)
		WithFrameLock(func() { appState.Providers = providers })
		RequestNextFrame()
	}()
}

// loadSettings copies the service's config into state.
func loadSettings(ctx context.Context, svc *gui.Service) {
	settings, err := svc.GetSettings(ctx)
	if err != nil || settings == nil {
		return
	}
	cfg := settings.Config
	WithFrameLock(func() {
		appState.Config = &cfg
		appState.ConfigPath = settings.ConfigFilePath
		appState.ConfigIsOverride = settings.IsLocalOverride
	})
}

func saveSettingsNow() {
	svc := liveService
	if saveSettings == nil || svc == nil || appState.Config == nil {
		return
	}
	cfg := *appState.Config
	go func() {
		if err := saveSettings(context.Background(), svc, cfg); err == nil {
			WithFrameLock(func() { appState.SettingsSaved = true })
			RequestNextFrame()
		}
	}()
}

func settingsView() {
	p := ui.DefaultPalette()
	Container(Attrs(Viewport, BackgroundVec(p.Bg), Pad(24), Gap(12)), func() {
		ScrollOnInput()
		ScrollBars()
		Container(Attrs(Row, CrossMid, Gap(10)), func() {
			Label("Settings", FontSize(24), FontWeight(WeightBold), TextColorVec(p.Text))
			if appState.SettingsSaved {
				Label("Saved", FontSize(12), TextColorVec(p.Accent))
			}
			Filler(1)
			NextAccessName("settings.global.save")
			if Button(NoIcon, "Save") {
				saveSettingsNow()
			}
			AssignAccess()
			NextAccessName("settings.global.back")
			if Button(NoIcon, "Back") {
				appState.Screen = ScreenLauncher
			}
			AssignAccess()
		})

		if appState.ConfigPath != "" {
			note := appState.ConfigPath
			if appState.ConfigIsOverride {
				note += " (local override)"
			}
			Label(note, FontSize(11), TextColorVec(p.Muted))
		}

		Container(Attrs(Row, Wrap, Gap(4)), func() {
			for _, tab := range []struct{ key, label string }{
				{"paths", "Paths"}, {"providers", "Providers"}, {"agents", "Agents"},
				{"media", "Media"}, {"preferences", "Preferences"}, {"debug", "Debug"},
			} {
				tab := tab
				selected := settingsTab() == tab.key
				Container(Attrs(Pad2(4, 10), Corners(6), BackgroundVec(p.Panel)), func() {
					if selected {
						ModAttrs(BackgroundVec(p.Accent))
					}
					NextAccessName("settings.tab." + tab.key)
					if PressAction() {
						appState.SettingsTab = tab.key
					}
					AssignAccess()
					Label(tab.label, FontSize(12), TextColorVec(p.Text))
				})
			}
		})

		if appState.Config == nil {
			Label("Loading settings…", FontSize(12), TextColorVec(p.Muted))
			return
		}
		switch settingsTab() {
		case "paths":
			settingsPaths(p)
		case "providers":
			settingsProviders(p)
		case "agents":
			settingsAgents(p)
		case "preferences":
			settingsPreferences(p)
		case "debug":
			settingsDebug(p)
		default:
			Label("This tab is provided by a later task.", FontSize(12), TextColorVec(p.Muted))
		}
	})
}

func settingsPaths(p ui.Palette) {
	cfg := appState.Config
	Label("Storage & Discovery Paths", FontSize(14), FontWeight(WeightBold), TextColorVec(p.Text))
	Label("Systems", FontSize(12), TextColorVec(p.Muted))
	DirectoryBrowse(&cfg.Paths.Systems)
	Label("Worlds", FontSize(12), TextColorVec(p.Muted))
	DirectoryBrowse(&cfg.Paths.Worlds)
	Label("Games", FontSize(12), TextColorVec(p.Muted))
	DirectoryBrowse(&cfg.Paths.Games)
	Label("Cache", FontSize(12), TextColorVec(p.Muted))
	DirectoryBrowse(&cfg.Paths.Cache)
}

func settingsPreferences(p ui.Palette) {
	cfg := appState.Config
	Label("Preferences", FontSize(14), FontWeight(WeightBold), TextColorVec(p.Text))
	CheckBox(&cfg.Preferences.Streaming, "Stream narration as it is generated")
	CheckBox(&cfg.Preferences.CinematicEffects, "Cinematic effects")
	Label("Font scale", FontSize(12), TextColorVec(p.Muted))
	OptionGroup(&cfg.Preferences.FontScale, func() {
		OptionButton("Small", "small")
		OptionButton("Medium", "medium")
		OptionButton("Large", "large")
	})
}

func settingsDebug(p ui.Palette) {
	cfg := appState.Config
	Label("Debug & Tracing", FontSize(14), FontWeight(WeightBold), TextColorVec(p.Text))
	Label("Trace level", FontSize(12), TextColorVec(p.Muted))
	OptionGroup(&cfg.Preferences.TraceLevel, func() {
		OptionButton("Off", "off")
		OptionButton("Summary", "summary")
		OptionButton("Full", "full")
	})
	Label("Trace payload chars", FontSize(12), TextColorVec(p.Muted))
	intSlider(&cfg.Agents.TracePayloadChars, 0, 20000, 100)
	Label("Trace max files", FontSize(12), TextColorVec(p.Muted))
	intSlider(&cfg.Agents.TraceMaxFiles, 0, 100, 1)
	Label("Trace rotate check", FontSize(12), TextColorVec(p.Muted))
	intSlider(&cfg.Agents.TraceRotateCheck, 0, 1000, 10)
	Label("Trace chunk limit", FontSize(12), TextColorVec(p.Muted))
	intSlider(&cfg.Agents.TraceChunkLimit, 0, 100000, 1000)
}

// intSlider edits an int through a float slider.
func intSlider(target *int, minVal, maxVal, step float32) {
	value := float32(*target)
	Slider(&value, SliderAttrs{Min: minVal, Max: maxVal, Step: step, Width: 320})
	*target = int(value)
}
