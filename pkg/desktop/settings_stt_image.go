package desktop

import (
	"context"
	"fmt"

	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/models"
	"github.com/darkliquid/localrpg/pkg/provider"
	"github.com/darkliquid/localrpg/pkg/ui"
)

// downloadModel is injected by Run for a live service.
var downloadModel func(ctx context.Context, svc *gui.Service, id string) error

// applyModelStatus replaces a status by id, appending when new.
func applyModelStatus(status models.ModelStatus) {
	for i := range appState.Models {
		if appState.Models[i].ID == status.ID {
			appState.Models[i] = status
			return
		}
	}
	appState.Models = append(appState.Models, status)
}

// startModelEvents subscribes to model download progress for the process.
func startModelEvents(svc *gui.Service) {
	ch := svc.SubscribeModelEvents()
	go func() {
		for status := range ch {
			WithFrameLock(func() { applyModelStatus(status) })
			RequestNextFrame()
		}
	}()
}

// refreshModels loads the current model statuses.
func refreshModels(svc *gui.Service) {
	go func() {
		statuses := svc.GetModelsStatus()
		WithFrameLock(func() { appState.Models = statuses })
		RequestNextFrame()
	}()
}

func settingsSTT(p ui.Palette) {
	cfg := appState.Config
	settingsSubTitle(TypMicrophone, "Speech-to-Text")
	if presets := providerPresets(provider.FamilySTT); len(presets) > 0 {
		MenuButton(NoIcon, "Load STT Preset", func() {
			for _, preset := range presets {
				preset := preset
				if MenuItem(NoIcon, preset.Label) {
					applySTTPreset(&cfg.Media.STT, preset)
				}
			}
		})
	}
	Label("Type", FontSize(12), Fonts(ui.SansStack...), TextColorVec(ui.TextMuted))
	OptionGroup(&cfg.Media.STT.Type, func() {
		OptionButton("Disabled", "disabled")
		OptionButton("HTTP", "http")
		OptionButton("CLI", "cli")
	})
	Label("Endpoint", FontSize(12), Fonts(ui.SansStack...), TextColorVec(ui.TextMuted))
	FieldInput(&cfg.Media.STT.Endpoint)
	Label("Model", FontSize(12), Fonts(ui.SansStack...), TextColorVec(ui.TextMuted))
	FieldInput(&cfg.Media.STT.Model)
	Label("Command", FontSize(12), Fonts(ui.SansStack...), TextColorVec(ui.TextMuted))
	FieldInput(&cfg.Media.STT.Command)
	providerTestRow("settings.stt.test", "Test Transcription Engine", "stt", cfg.Media.STT)
}

func settingsImage(p ui.Palette) {
	cfg := appState.Config
	settingsSubTitle(TypImage, "Image Generation")
	if presets := providerPresets(provider.FamilyImage); len(presets) > 0 {
		MenuButton(NoIcon, "Load Image Preset", func() {
			for _, preset := range presets {
				preset := preset
				if MenuItem(NoIcon, preset.Label) {
					applyImagePreset(&cfg.Media.Image, preset)
				}
			}
		})
	}
	CheckBox(&cfg.Media.Image.AutoGenerate, "Auto-generate scene imagery")
	CheckBox(&cfg.Media.Image.BuiltinFallback, "Use built-in procedural art as a fallback")
	Label("Type", FontSize(12), Fonts(ui.SansStack...), TextColorVec(ui.TextMuted))
	OptionGroup(&cfg.Media.Image.Type, func() {
		OptionButton("Disabled", "disabled")
		OptionButton("Built-in", "builtin")
		OptionButton("HTTP", "http")
	})
	Label("Built-in name", FontSize(12), Fonts(ui.SansStack...), TextColorVec(ui.TextMuted))
	FieldInput(&cfg.Media.Image.BuiltinName)
	Label("Endpoint", FontSize(12), Fonts(ui.SansStack...), TextColorVec(ui.TextMuted))
	FieldInput(&cfg.Media.Image.Endpoint)
	Label("Model", FontSize(12), Fonts(ui.SansStack...), TextColorVec(ui.TextMuted))
	FieldInput(&cfg.Media.Image.Model)
	Label("Aspect ratio", FontSize(12), Fonts(ui.SansStack...), TextColorVec(ui.TextMuted))
	FieldInput(&cfg.Media.Image.AspectRatio)
	Label("Person generation", FontSize(12), Fonts(ui.SansStack...), TextColorVec(ui.TextMuted))
	FieldInput(&cfg.Media.Image.PersonGeneration)
	Label("API key", FontSize(12), Fonts(ui.SansStack...), TextColorVec(ui.TextMuted))
	FieldPassword(&cfg.Media.Image.APIKey)
	providerTestRow("settings.image.test", "Test Image Engine", "image", cfg.Media.Image)
}

// applySTTPreset copies a preset's config values into an STT config.
func applySTTPreset(cfg *config.STTConfig, preset provider.Preset) {
	if preset.Config == nil {
		return
	}
	if v, ok := preset.Config["type"].(string); ok {
		cfg.Type = v
	}
	if v, ok := preset.Config["builtin_name"].(string); ok {
		cfg.BuiltinName = v
	}
	if v, ok := preset.Config["endpoint"].(string); ok {
		cfg.Endpoint = v
	}
	if v, ok := preset.Config["model"].(string); ok {
		cfg.Model = v
	}
	if v, ok := preset.Config["command"].(string); ok {
		cfg.Command = v
	}
}

// applyImagePreset copies a preset's config values into an image config.
func applyImagePreset(cfg *config.ImageConfig, preset provider.Preset) {
	if preset.Config == nil {
		return
	}
	if v, ok := preset.Config["type"].(string); ok {
		cfg.Type = v
	}
	if v, ok := preset.Config["builtin_name"].(string); ok {
		cfg.BuiltinName = v
	}
	if v, ok := preset.Config["endpoint"].(string); ok {
		cfg.Endpoint = v
	}
	if v, ok := preset.Config["model"].(string); ok {
		cfg.Model = v
	}
	if v, ok := preset.Config["aspect_ratio"].(string); ok {
		cfg.AspectRatio = v
	}
	if v, ok := preset.Config["auto_generate"].(bool); ok {
		cfg.AutoGenerate = v
	}
	if v, ok := preset.Config["builtin_fallback"].(bool); ok {
		cfg.BuiltinFallback = v
	}
}

// settingsModelPresets offers provider presets for a family.
func settingsModelPresets(p ui.Palette, family provider.Family, apply func(provider.Preset)) {
	for _, desc := range providersForFamily(family) {
		for _, preset := range desc.Presets {
			preset := preset
			if Button(NoIcon, preset.Label) {
				apply(preset)
			}
		}
	}
}

func modelsSection(p ui.Palette) {
	settingsSubTitle(TypDownload, "Models")
	if len(appState.Models) == 0 {
		settingsHint("No models reported.")
		return
	}
	for _, model := range appState.Models {
		Container(Attrs(Expand, Gap(6), Corners(10), Pad(12), BackgroundVec(ui.PillBG),
			BorderWidth(1), BorderColorVec(ui.Hairline)), func() {
			Container(Attrs(Row, CrossMid, Gap(8)), func() {
				Label(model.Name, Fonts(ui.SansStack...), FontSize(12), FontWeight(WeightBold), TextColorVec(ui.TextMain))
				Filler(1)
				if model.Installed {
					Label("installed", Fonts(Monospace...), FontSize(11), TextColorVec(ui.Success))
				}
			})
			if model.Installed {
				return
			}
			ProgressBar(float32(model.Progress))
			Container(Attrs(Row, CrossMid, Gap(8)), func() {
				if model.Downloading {
					BusyDots()
				}
				NextAccessName("models.download." + model.ID)
				if Button(NoIcon, "Download") {
					if downloadModel != nil && liveService != nil {
						id := model.ID
						go func() { _ = downloadModel(context.Background(), liveService, id) }()
					}
				}
				AssignAccess()
				Label(fmt.Sprintf("%d/%d bytes", model.BytesDownloaded, model.TotalBytes), Fonts(Monospace...), FontSize(10), TextColorVec(ui.TextFaint))
			})
			if model.Error != "" {
				Label(model.Error, FontSize(11), TextColorVec(ui.Danger))
			}
		})
	}
}
