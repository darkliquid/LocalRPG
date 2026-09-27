package engine

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/core"
)

func TestResolveEngagementOrder(t *testing.T) {
	sys := &core.SystemManifest{Mechanics: &core.MechanicsSpec{Engagement: "ask"}}
	cfg := &config.Config{Mechanics: config.MechanicsConfig{Engagement: "off"}}

	if got := ResolveEngagement(&core.GameManifest{}, sys, cfg); got != "ask" {
		t.Errorf("system default = %q, want ask", got)
	}
	campaign := &core.GameManifest{Settings: map[string]interface{}{"mechanics_engagement": "off"}}
	if got := ResolveEngagement(campaign, sys, cfg); got != "off" {
		t.Errorf("campaign override = %q, want off", got)
	}
	if got := ResolveEngagement(&core.GameManifest{}, nil, cfg); got != "off" {
		t.Errorf("config default = %q, want off", got)
	}
	if got := ResolveEngagement(&core.GameManifest{}, nil, &config.Config{}); got != "auto" {
		t.Errorf("built-in default = %q, want auto", got)
	}
}
