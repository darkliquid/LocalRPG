package desktop

import (
	"context"
	"fmt"

	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

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
	Label("STT Engine", FontSize(14), FontWeight(WeightBold), TextColorVec(p.Text))
	Label("Type", FontSize(12), TextColorVec(p.Muted))
	OptionGroup(&cfg.Media.STT.Type, func() {
		OptionButton("Disabled", "disabled")
		OptionButton("HTTP", "http")
		OptionButton("CLI", "cli")
	})
	Label("Endpoint", FontSize(12), TextColorVec(p.Muted))
	TextInput(&cfg.Media.STT.Endpoint)
	Label("Model", FontSize(12), TextColorVec(p.Muted))
	TextInput(&cfg.Media.STT.Model)
	Label("Command", FontSize(12), TextColorVec(p.Muted))
	TextInput(&cfg.Media.STT.Command)
}

func settingsImage(p ui.Palette) {
	cfg := appState.Config
	Label("Image Engine", FontSize(14), FontWeight(WeightBold), TextColorVec(p.Text))
	CheckBox(&cfg.Media.Image.AutoGenerate, "Auto-generate scene imagery")
	CheckBox(&cfg.Media.Image.BuiltinFallback, "Use built-in procedural art as a fallback")
	Label("Type", FontSize(12), TextColorVec(p.Muted))
	OptionGroup(&cfg.Media.Image.Type, func() {
		OptionButton("Disabled", "disabled")
		OptionButton("Built-in", "builtin")
		OptionButton("HTTP", "http")
	})
	Label("Built-in name", FontSize(12), TextColorVec(p.Muted))
	TextInput(&cfg.Media.Image.BuiltinName)
	Label("Endpoint", FontSize(12), TextColorVec(p.Muted))
	TextInput(&cfg.Media.Image.Endpoint)
	Label("Model", FontSize(12), TextColorVec(p.Muted))
	TextInput(&cfg.Media.Image.Model)
	Label("Aspect ratio", FontSize(12), TextColorVec(p.Muted))
	TextInput(&cfg.Media.Image.AspectRatio)
	Label("Person generation", FontSize(12), TextColorVec(p.Muted))
	TextInput(&cfg.Media.Image.PersonGeneration)
	Label("API key", FontSize(12), TextColorVec(p.Muted))
	PasswordInput(&cfg.Media.Image.APIKey)
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
	Label("Models", FontSize(14), FontWeight(WeightBold), TextColorVec(p.Text))
	if len(appState.Models) == 0 {
		Label("No models reported.", FontSize(11), TextColorVec(p.Muted))
		return
	}
	for _, model := range appState.Models {
		Label(model.Name, FontSize(12), TextColorVec(p.Text))
		if model.Installed {
			Label("installed", FontSize(11), TextColorVec(p.Accent))
			continue
		}
		ProgressBar(float32(model.Progress))
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
		if model.Error != "" {
			Label(model.Error, FontSize(11), TextColorVec(p.Danger))
		}
		Label(fmt.Sprintf("%d/%d bytes", model.BytesDownloaded, model.TotalBytes), FontSize(10), TextColorVec(p.Muted))
	}
}
