package gui

import (
	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/pricing"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// routerWithChains builds a role-routed router and wires the chain price
// accessor, so a role's cheapest chain orders by the ledger. The accessor is
// installed here because pkg/pricing imports pkg/harness and cannot be imported
// back, and this is the one place the GUI builds a router.
func routerWithChains(cfg *config.Config, logger trace.Logger) (*harness.Router, error) {
	router, err := harness.RouterFromConfigWithLogger(cfg, logger)
	if err != nil {
		return nil, err
	}
	router.SetChainPrice(pricing.RouterChainPrice(cfg, router))
	return router, nil
}
