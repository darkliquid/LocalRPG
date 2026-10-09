package gui

import (
	"context"
	"fmt"

	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// StylePacksDTO is the style packs a client can choose from, with the one in
// force and any reason a pack could not be applied.
type StylePacksDTO struct {
	Active   string                  `json:"active,omitempty"`
	Packs    []media.StylePackStatus `json:"packs"`
	Warnings []string                `json:"warnings,omitempty"`
}

// applyStylePack puts the configured style pack in force, or restores the built-in
// look. A pack that does not load is reported and ignored rather than failing, so a
// render never breaks because of a pack.
func (s *Service) applyStylePack() []string {
	cfg := s.configMgr.Get()
	name := cfg.StylePackName()
	if name == "" {
		media.SetActiveTables(media.Tables{})
		s.setStyleWarnings(nil)
		return nil
	}

	path := media.StylePackPath(s.configMgr.StylesDir(), name)
	pack, err := media.LoadStylePack(path)
	if err != nil {
		warning := fmt.Sprintf("style pack %q was ignored: %v", name, err)
		trace.OrNil(s.logger).Event("style.pack_invalid", map[string]interface{}{"pack": name, "error": err.Error()})
		media.SetActiveTables(media.Tables{})
		s.setStyleWarnings([]string{warning})
		return []string{warning}
	}

	media.SetActiveTables(pack.Merge())
	s.setStyleWarnings(nil)
	trace.OrNil(s.logger).Event("style.pack_active", map[string]interface{}{"pack": pack.ID})
	return nil
}

// ListStylePacks reports the packs under the config directory's styles folder,
// with the one in force and any warnings from applying it.
func (s *Service) ListStylePacks(_ context.Context) (*StylePacksDTO, error) {
	packs := media.ListStylePacks(s.configMgr.StylesDir())
	if packs == nil {
		packs = []media.StylePackStatus{}
	}
	return &StylePacksDTO{
		Active:   media.ActivePackID(),
		Packs:    packs,
		Warnings: s.styleWarnings(),
	}, nil
}
