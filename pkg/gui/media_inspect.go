package gui

import (
	"context"
	"strings"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
)

// InspectMedia lists every named entry of one media family with its provider
// identity, key state, metered flag, and capability tier, so the provider
// manager can describe a whole family in one call.
func (s *Service) InspectMedia(ctx context.Context, req MediaInspectRequestDTO) (*MediaInspectResponseDTO, error) {
	resp := &MediaInspectResponseDTO{Entries: []MediaInspectEntryDTO{}}
	cfg := s.configMgr.Get()
	if cfg == nil {
		return resp, nil
	}
	switch req.Family {
	case "tts":
		for _, name := range cfg.Media.TTSNames() {
			entry := cfg.Media.TTSFor(name)
			key, ok := media.TTSKeyFor(entry)
			resp.Entries = append(resp.Entries, s.mediaInspectEntry(cfg, name, key, ok, entry.APIKey))
		}
	case "stt":
		for _, name := range cfg.Media.STTNames() {
			entry := cfg.Media.STTFor(name)
			key, ok := media.STTKeyFor(entry)
			resp.Entries = append(resp.Entries, s.mediaInspectEntry(cfg, name, key, ok, entry.APIKey))
		}
	case "image":
		for _, name := range cfg.Media.ImageNames() {
			entry := cfg.Media.ImageFor(name)
			key, ok := media.ImageKeyFor(entry)
			resp.Entries = append(resp.Entries, s.mediaInspectEntry(cfg, name, key, ok, entry.APIKey))
		}
	}
	return resp, nil
}

// mediaInspectEntry describes one configuration from its canonical key and the
// shared credential it may inherit.
func (s *Service) mediaInspectEntry(cfg *config.Config, name string, key provider.Key, ok bool, apiKey string) MediaInspectEntryDTO {
	entry := MediaInspectEntryDTO{Name: name}
	if ok {
		entry.ProviderKey = string(key)
		if reg, found := provider.Lookup(string(key.Parent())); found {
			entry.Tier = string(reg.Descriptor.Tier)
			entry.KeyRequired = hasFeature(reg.Descriptor.Features, provider.FeatureKeyRequired)
			entry.Metered = hasFeature(reg.Descriptor.Features, provider.FeatureMetered)
		}
	}
	shared := media.SharedProviderKey(cfg, key, ok)
	entry.KeyPresent = strings.TrimSpace(apiKey) != "" || strings.TrimSpace(shared) != ""
	return entry
}

// hasFeature reports whether a descriptor advertises a feature.
func hasFeature(features []provider.Feature, want provider.Feature) bool {
	for _, feature := range features {
		if feature == want {
			return true
		}
	}
	return false
}
