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

// settingsTabEntry names one settings tab.
type settingsTabEntry struct {
	key   string
	label string
	icon  IconGlyph
}

// settingsTabs mirrors the original studio's navigation strip.
var settingsTabs = []settingsTabEntry{
	{"paths", "Paths", TypFolder},
	{"providers", "Providers", TypCloudStorage},
	{"agents", "AI Agents", TypGroup},
	{"media", "Media Engines", TypVolume},
	{"preferences", "Preferences", TypAdjustContrast},
	{"debug", "Debug", TypBeaker},
}

func settingsView() {
	Container(Attrs(Viewport, BackgroundVec(ui.CanvasBG), Pad(28), Gap(16)), func() {
		ScrollOnInput()
		ScrollBars()
		settingsHeader()
		if appState.SettingsSaved {
			settingsFeedback()
		}
		if appState.Config == nil {
			Label("Loading system configuration…", Fonts(Monospace...), FontSize(12), TextColorVec(ui.TextMuted))
			return
		}
		p := ui.DefaultPalette()
		switch settingsTab() {
		case "paths":
			settingsCard(func() { settingsPaths(p) })
		case "providers":
			settingsCard(func() { settingsProviders(p) })
		case "agents":
			settingsCard(func() { settingsAgents(p) })
		case "media":
			settingsCard(func() { settingsMedia(p) })
		case "preferences":
			settingsCard(func() { settingsPreferences(p) })
		case "debug":
			settingsCard(func() { settingsDebug(p) })
		default:
			Label("This tab is provided by a later task.", FontSize(12), TextColorVec(ui.TextMuted))
		}
	})
}

// settingsHeader is the tab strip, config path pill and Save button.
func settingsHeader() {
	Container(Attrs(Expand, Gap(12)), func() {
		Container(Attrs(Row, CrossMid, Gap(12), Expand), func() {
			iconButton("settings.home", SymHome, func() {
				appState.Screen = ScreenLauncher
			})
			settingsTabStrip()
			Filler(1)
			settingsSaveButton()
		})
		Element(Attrs(Expand, FixHeight(1), BackgroundVec(ui.Hairline)))
		settingsConfigPill()
	})
}

func settingsTabStrip() {
	Container(Attrs(Row, CrossMid, Gap(4), Corners(12), Pad(4), BackgroundVec(ui.DockBG),
		BorderWidth(1), BorderColorVec(ui.Hairline)), func() {
		for _, tab := range settingsTabs {
			tab := tab
			active := settingsTab() == tab.key
			Container(Attrs(Row, CrossMid, Gap(6), Corners(8), Pad2(6, 12)), func() {
				switch {
				case active:
					ModAttrs(BackgroundVec(ui.AccentBtn), BoxShadow(14))
				case IsHovered():
					ModAttrs(BackgroundVec(ui.HoverFill))
				}
				NextAccessName("settings.tab." + tab.key)
				if PressAction() {
					appState.SettingsTab = tab.key
				}
				AssignAccess()
				weight := WeightNormal
				if active {
					weight = WeightBold
				}
				Icon(tab.icon, FontSize(14), TextColorVec(ui.TextMain))
				Label(tab.label, Fonts(ui.SansStack...), FontSize(12),
					TextColorVec(ui.TextMain), FontWeight(weight))
			})
		}
	})
}

func settingsConfigPill() {
	prefix := "Global User Config"
	if appState.ConfigIsOverride {
		prefix = "Workspace Override"
	}
	Container(Attrs(Row, CrossMid, Expand, Gap(0), Clip), func() {
		Container(Attrs(Corners(12), Pad2(7, 12), BackgroundVec(ui.PillBG),
			BorderWidth(1), BorderColorVec(ui.Hairline)), func() {
			Label(prefix+": "+abbreviatePath(appState.ConfigPath, 72),
				Fonts(Monospace...), FontSize(11), TextColorVec(ui.TextMuted))
		})
	})
}

// abbreviatePath shortens a long path with a middle ellipsis.
func abbreviatePath(path string, max int) string {
	runes := []rune(path)
	if len(runes) <= max {
		return path
	}
	keep := max - 1
	head := keep / 2
	tail := keep - head
	return string(runes[:head]) + "…" + string(runes[len(runes)-tail:])
}

func settingsSaveButton() {
	Container(Attrs(FixHeight(34), Corners(12), Pad2(0, 18), Center, BackgroundVec(ui.AccentBtn),
		BorderWidth(1), BorderColorVec(ui.AccentBorder), BoxShadow(18)), func() {
		if IsHovered() {
			ModAttrs(BackgroundVec(ui.AccentBtnHover))
		}
		NextAccessName("settings.global.save")
		if PressAction() {
			saveSettingsNow()
		}
		AssignAccess()
		Container(Attrs(Row, CrossMid, Gap(8)), func() {
			Icon(TypInputChecked, FontSize(14), TextColorVec(ui.TextMain))
			Label("Save Settings", Fonts(ui.SansStack...), FontSize(12), FontWeight(WeightBold), TextColorVec(ui.TextMain))
		})
	})
}

func settingsFeedback() {
	Container(Attrs(Row, CrossMid, Gap(10), Corners(12), Pad(12), BackgroundVec(ui.SuccessSoft),
		BorderWidth(1), BorderColorVec(ui.Success)), func() {
		Icon(TypTick, FontSize(16), TextColorVec(ui.Success))
		Label("Settings saved.", FontSize(12), TextColorVec(ui.TextMain))
	})
}

// settingsCard is the translucent panel wrapping a tab's content.
func settingsCard(body func()) {
	Container(Attrs(Expand, ui.Card(Gap(14), Pad(20))), body)
}

// settingsSectionTitle is the card's heading: a purple sans title with an icon.
func settingsSectionTitle(icon IconGlyph, text string) {
	Container(Attrs(Row, CrossMid, Gap(8)), func() {
		Icon(icon, FontSize(16), TextColorVec(ui.Accent))
		Label(text, Fonts(ui.SansStack...), FontSize(14), FontWeight(WeightBold), TextColorVec(ui.Accent))
	})
}

// settingsSubTitle heads a subsection inside a card.
func settingsSubTitle(icon IconGlyph, text string) {
	Container(Attrs(Row, CrossMid, Gap(8), Pad2(6, 0)), func() {
		Icon(icon, FontSize(14), TextColorVec(ui.TextMuted))
		Label(text, Fonts(ui.SansStack...), FontSize(13), FontWeight(WeightBold), TextColorVec(ui.TextMain))
	})
}

// settingsHint is secondary explanatory copy.
func settingsHint(text string) {
	Label(text, FontSize(12), TextColorVec(ui.TextMuted))
}

func settingsPaths(p ui.Palette) {
	cfg := appState.Config
	settingsSectionTitle(TypFolder, "Storage & Discovery Paths")
	settingsHint("Configure directories where LocalRPG looks for rule systems, world lore, saved campaigns, and generated media caches.")
	Label("Rule Systems Directory", FontSize(12), Fonts(ui.SansStack...), TextColorVec(ui.TextMuted))
	FieldDirectory(&cfg.Paths.Systems)
	Label("World Lore Directory", FontSize(12), Fonts(ui.SansStack...), TextColorVec(ui.TextMuted))
	FieldDirectory(&cfg.Paths.Worlds)
	Label("Campaigns Directory", FontSize(12), Fonts(ui.SansStack...), TextColorVec(ui.TextMuted))
	FieldDirectory(&cfg.Paths.Games)
	Label("Cache Directory", FontSize(12), Fonts(ui.SansStack...), TextColorVec(ui.TextMuted))
	FieldDirectory(&cfg.Paths.Cache)
}

func settingsPreferences(p ui.Palette) {
	cfg := appState.Config
	settingsSectionTitle(TypAdjustContrast, "Preferences & Appearance")
	settingsSubTitle(TypAdjustBrightness, "Experience")
	CheckBox(&cfg.Preferences.Streaming, "Stream narration as it is generated")
	CheckBox(&cfg.Preferences.CinematicEffects, "Cinematic effects")
	settingsSubTitle(TypZoom, "Font scale")
	OptionGroup(&cfg.Preferences.FontScale, func() {
		OptionButton("Small", "small")
		OptionButton("Medium", "medium")
		OptionButton("Large", "large")
	})
}

func settingsDebug(p ui.Palette) {
	cfg := appState.Config
	settingsSectionTitle(TypBeaker, "Debug & Tracing")
	settingsHint("Developer controls over the agent trace log.")
	settingsSubTitle(TypFilter, "Trace level")
	OptionGroup(&cfg.Preferences.TraceLevel, func() {
		OptionButton("Off", "off")
		OptionButton("Summary", "summary")
		OptionButton("Full", "full")
	})
	Label("Trace payload chars", FontSize(12), Fonts(ui.SansStack...), TextColorVec(ui.TextMuted))
	intSlider(&cfg.Agents.TracePayloadChars, 0, 20000, 100)
	Label("Trace max files", FontSize(12), Fonts(ui.SansStack...), TextColorVec(ui.TextMuted))
	intSlider(&cfg.Agents.TraceMaxFiles, 0, 100, 1)
	Label("Trace rotate check", FontSize(12), Fonts(ui.SansStack...), TextColorVec(ui.TextMuted))
	intSlider(&cfg.Agents.TraceRotateCheck, 0, 1000, 10)
	Label("Trace chunk limit", FontSize(12), Fonts(ui.SansStack...), TextColorVec(ui.TextMuted))
	intSlider(&cfg.Agents.TraceChunkLimit, 0, 100000, 1000)
}

// intSlider edits an int through a float slider.
func intSlider(target *int, minVal, maxVal, step float32) {
	value := float32(*target)
	Slider(&value, SliderAttrs{Min: minVal, Max: maxVal, Step: step, Width: 320})
	*target = int(value)
}
