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
	if strings.Contains(schema, `"proposals"`) {
		return []byte(`{"proposals":[]}`), nil
	}
	if strings.Contains(schema, `"explanation"`) {
		return []byte(`{"explanation":"No model provider is configured, so this is a placeholder explanation. Assign one in Settings to explain the system's mechanics."}`), nil
	}
	if strings.Contains(schema, `"success_on"`) {
		return []byte(`{"stats":[{"id":"strength","label":"Strength"},{"id":"dexterity","label":"Dexterity"},{"id":"mind","label":"Mind"}],"skills":[{"id":"athletics","label":"Athletics","stat":"strength"},{"id":"stealth","label":"Stealth","stat":"dexterity"},{"id":"lore","label":"Lore","stat":"mind"}],"notation":"1d20","ladder":[{"min":10,"outcome":"strong"},{"min":7,"outcome":"weak"},{"min":0,"outcome":"miss"}],"dc":10,"success_on":">=8","outcomes":[{"min":3,"max":-1,"outcome":"strong"},{"min":1,"max":2,"outcome":"weak"},{"min":0,"max":0,"outcome":"miss"}],"health_stat":"strength","advancement_stat":"mind"}`), nil
	}
	if strings.Contains(schema, `"resolution"`) {
		return []byte(`{"resolution":"dc","health":"single","advancement":"none","reason":"a d20 system with a single health track","gaps":""}`), nil
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
