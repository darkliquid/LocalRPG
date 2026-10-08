package sysgen

import (
	"context"
	"strings"
)

// OracleGenerator is the deterministic fallback generator for sysgen when no model provider is available.
type OracleGenerator struct{}

// NewOracleGenerator returns a deterministic fallback generator.
func NewOracleGenerator() *OracleGenerator {
	return &OracleGenerator{}
}

// GenerateJSON returns fixed, valid JSON responses for each sysgen step.
func (o *OracleGenerator) GenerateJSON(ctx context.Context, prompt, schema string) ([]byte, error) {
	if strings.Contains(schema, `"resolution"`) {
		return []byte(`{"resolution":"d20","stats":["strength","dexterity","mind"],"skills":["athletics","stealth","lore"],"health":"points","advancement":true}`), nil
	}
	if strings.Contains(schema, `"checks"`) {
		return []byte(`{"stats":[{"id":"strength","label":"Strength"},{"id":"dexterity","label":"Dexterity"},{"id":"mind","label":"Mind"}],"skills":[{"id":"athletics","label":"Athletics","stat":"strength"},{"id":"stealth","label":"Stealth","stat":"dexterity"},{"id":"lore","label":"Lore","stat":"mind"}],"health":{"type":"points","max":10,"current":10},"checks":{"notation":"1d20","outcome":["failure","success"],"profiles":{"check":{"notation":"1d20","dc":10}}}}`), nil
	}
	if strings.Contains(schema, `"hooks"`) {
		return []byte(`{"hooks":[]}`), nil
	}
	if strings.Contains(schema, `"rules"`) {
		return []byte(`{"rules":"# Rules Guide\n\nRoll 1d20 against a target of 10 for standard checks."}`), nil
	}
	return []byte(`{}`), nil
}

// Oracle reports whether a generator is the deterministic template generator.
func Oracle(g Generator) bool {
	_, ok := g.(*OracleGenerator)
	return ok
}
