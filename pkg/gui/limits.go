package gui

import (
	"errors"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
)

// providerKeyForRole resolves the canonical key a role's traffic belongs to, so
// a backoff follows the provider and is lifted by a configuration change. A role
// whose configuration names no registered adapter falls back to its own name,
// which no canonical key can collide with.
func (s *Service) providerKeyForRole(role string) string {
	key, ok := s.providerKey(role)
	if !ok || key == "" {
		return role
	}
	return string(key)
}

// providerKey resolves the canonical key a role's provider records under.
func (s *Service) providerKey(role string) (provider.Key, bool) {
	cfg := s.configMgr.Get()
	switch role {
	case "tts":
		return media.TTSKeyFor(cfg.Media.TTS)
	case "stt":
		return media.STTKeyFor(cfg.Media.STT)
	case "image":
		return media.ImageKeyFor(cfg.Media.Image)
	default:
		return llmKeyForRole(cfg, role)
	}
}

// llmKeyForRole resolves the LLM key a role uses, following an inherit chain so
// a backoff lands on the provider that actually serves it.
func llmKeyForRole(cfg *config.Config, role string) (provider.Key, bool) {
	if cfg == nil {
		return "", false
	}
	roleCfg, ok := cfg.Agents.Roles[role]
	if !ok {
		return "", false
	}
	seen := map[string]bool{role: true}
	for roleCfg.Type == "inherit" && roleCfg.InheritFrom != "" && !seen[roleCfg.InheritFrom] {
		seen[roleCfg.InheritFrom] = true
		next, ok := cfg.Agents.Roles[roleCfg.InheritFrom]
		if !ok {
			break
		}
		roleCfg = next
	}
	return harness.KeyFor(harness.ProviderConfig{
		Type:        roleCfg.Type,
		BuiltinName: roleCfg.BuiltinName,
		Command:     roleCfg.Command,
		Endpoint:    roleCfg.Endpoint,
	})
}

// guardRole refuses work while the provider behind a role is backed off.
func (s *Service) guardRole(role string) error {
	key := s.providerKeyForRole(role)
	if until, ok := s.limits.Blocked(key, role); ok {
		return &harness.ErrRateLimitedUntil{Provider: key, Role: role, Until: until}
	}
	return nil
}

// noteFailure records a rate-limit backoff or a funds failure from a provider
// error, and clears a stale funds flag after any other success.
func (s *Service) noteFailure(role string, err error) {
	if err == nil {
		return
	}
	key := s.providerKeyForRole(role)
	switch harness.ClassifyProviderError(err) {
	case harness.FailureRateLimited:
		retryAfter := retryAfterFor(err)
		if retryAfter <= 0 {
			retryAfter = 30 * time.Second
		}
		s.limits.Block(key, role, time.Now().Add(retryAfter))
	case harness.FailureInsufficientFunds:
		s.limits.RecordFundsFailure(key, role, err.Error())
	default:
		s.limits.ClearFundsFailure(key, role)
	}
}

// noteSuccess clears a funds failure: a later call to the same provider proves
// the account is funded again.
func (s *Service) noteSuccess(role string) {
	s.limits.ClearFundsFailure(s.providerKeyForRole(role), role)
}

// retryAfterFor extracts the backoff a provider advertised, from either the
// typed provider error or the failure wrapper around it.
func retryAfterFor(err error) time.Duration {
	var limited *provider.RateLimitedError
	if errors.As(err, &limited) && limited.RetryAfter > 0 {
		return limited.RetryAfter
	}
	var failure *harness.GenerationFailure
	if errors.As(err, &failure) && failure.RetryAfterMS > 0 {
		return time.Duration(failure.RetryAfterMS) * time.Millisecond
	}
	return 0
}

// Limits reports the live blocks and funds failures, for the API and UI.
func (s *Service) Limits() []harness.LimitState {
	return s.limits.Snapshot()
}
