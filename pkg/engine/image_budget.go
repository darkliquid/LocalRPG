package engine

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
)

// ImageBudget bounds a campaign's image generation by count, by spend, or by
// both. The zero value is unlimited, so a campaign that never sets one is
// unchanged.
type ImageBudget struct {
	MaxImages int   `json:"max_images,omitempty" yaml:"max_images,omitempty"`
	MaxMicros int64 `json:"max_micros,omitempty" yaml:"max_micros,omitempty"`
	Used      int   `json:"used_images,omitempty" yaml:"used_images,omitempty"`
	Spent     int64 `json:"spent_micros,omitempty" yaml:"spent_micros,omitempty"`
}

// Image approval modes.
const (
	// ImageApprovalAuto generates without asking, bounded by the budget.
	ImageApprovalAuto = "auto"
	// ImageApprovalAsk asks before each metered generation.
	ImageApprovalAsk = "ask"
)

// ImageBudgetSetting and ImageApprovalSetting are the campaign settings keys.
const (
	ImageBudgetSetting   = "image_budget"
	ImageApprovalSetting = "image_approval"
)

// Exhausted reports whether the budget has no room for another image. An
// unlimited budget is never exhausted.
func (b ImageBudget) Exhausted() bool {
	if b.MaxImages > 0 && b.Used >= b.MaxImages {
		return true
	}
	if b.MaxMicros > 0 && b.Spent >= b.MaxMicros {
		return true
	}
	return false
}

// Charge records one generated image and its cost, in micros. An unpriced
// generation records the image and adds nothing to the spend.
func (b *ImageBudget) Charge(micros int64) {
	b.Used++
	if micros > 0 {
		b.Spent += micros
	}
}

// Remaining reports the images the budget still allows, or -1 when unlimited.
func (b ImageBudget) Remaining() int {
	if b.MaxImages <= 0 {
		return -1
	}
	if left := b.MaxImages - b.Used; left > 0 {
		return left
	}
	return 0
}

// Unlimited reports whether the budget bounds nothing.
func (b ImageBudget) Unlimited() bool {
	return b.MaxImages <= 0 && b.MaxMicros <= 0
}

// ImageBudgetFromManifest reads a campaign's image budget. An absent or malformed
// entry is the unlimited default, so a campaign without one behaves as it did.
func ImageBudgetFromManifest(manifest *core.GameManifest) ImageBudget {
	if manifest == nil || manifest.Settings == nil {
		return ImageBudget{}
	}
	raw, ok := manifest.Settings[ImageBudgetSetting]
	if !ok || raw == nil {
		return ImageBudget{}
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return ImageBudget{}
	}
	var budget ImageBudget
	if err := json.Unmarshal(data, &budget); err != nil {
		return ImageBudget{}
	}
	return budget
}

// SetImageBudget writes a campaign's budget back onto its manifest.
func SetImageBudget(manifest *core.GameManifest, budget ImageBudget) {
	if manifest == nil {
		return
	}
	if manifest.Settings == nil {
		manifest.Settings = map[string]interface{}{}
	}
	manifest.Settings[ImageBudgetSetting] = map[string]interface{}{
		"max_images":   budget.MaxImages,
		"max_micros":   budget.MaxMicros,
		"used_images":  budget.Used,
		"spent_micros": budget.Spent,
	}
}

// ImageApprovalMode reads a campaign's image approval mode, defaulting to auto so
// a campaign without one generates without asking.
func ImageApprovalMode(manifest *core.GameManifest) string {
	if manifest == nil || manifest.Settings == nil {
		return ImageApprovalAuto
	}
	mode, _ := manifest.Settings[ImageApprovalSetting].(string)
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case ImageApprovalAsk:
		return ImageApprovalAsk
	default:
		return ImageApprovalAuto
	}
}

// SetImageApprovalMode writes a campaign's image approval mode.
func SetImageApprovalMode(manifest *core.GameManifest, mode string) {
	if manifest == nil {
		return
	}
	if manifest.Settings == nil {
		manifest.Settings = map[string]interface{}{}
	}
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case ImageApprovalAsk:
		manifest.Settings[ImageApprovalSetting] = ImageApprovalAsk
	default:
		manifest.Settings[ImageApprovalSetting] = ImageApprovalAuto
	}
}

// LoadImageBudget reads a campaign's budget from its manifest on disk.
func LoadImageBudget(path string) (ImageBudget, error) {
	manifest, err := core.LoadGameManifest(path)
	if err != nil {
		return ImageBudget{}, fmt.Errorf("load manifest: %w", err)
	}
	return ImageBudgetFromManifest(manifest), nil
}
