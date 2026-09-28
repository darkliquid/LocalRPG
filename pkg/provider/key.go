package provider

import (
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

// Key is a canonical provider identity: "<family>:<adapter>" or
// "<family>:<adapter>@<discriminator>". The adapter segment is the registry
// descriptor ID; the discriminator names one configuration of it.
type Key string

var (
	adapterPattern       = regexp.MustCompile(`^[a-z0-9-]+$`)
	discriminatorPattern = regexp.MustCompile(`^[a-z0-9.:-]+$`)
)

func validFamily(f Family) bool {
	switch f {
	case FamilyLLM, FamilyTTS, FamilySTT, FamilyImage, FamilyEmbedding:
		return true
	default:
		return false
	}
}

// NewKey builds an adapter key, validating the family and adapter segment.
func NewKey(f Family, adapter string) (Key, error) {
	if !validFamily(f) {
		return "", fmt.Errorf("provider: unknown family %q", f)
	}
	if !adapterPattern.MatchString(adapter) {
		return "", fmt.Errorf("provider: adapter %q must match %s", adapter, adapterPattern)
	}
	return Key(string(f) + ":" + adapter), nil
}

// ParseKey validates an adapter or instance key.
func ParseKey(s string) (Key, error) {
	family, rest, ok := strings.Cut(s, ":")
	if !ok {
		return "", fmt.Errorf("provider: key %q must be <family>:<adapter>", s)
	}
	if !validFamily(Family(family)) {
		return "", fmt.Errorf("provider: key %q has unknown family %q", s, family)
	}
	adapter, disc, hasDisc := strings.Cut(rest, "@")
	if !adapterPattern.MatchString(adapter) {
		return "", fmt.Errorf("provider: adapter %q in key %q must match %s", adapter, s, adapterPattern)
	}
	if hasDisc && !discriminatorPattern.MatchString(disc) {
		return "", fmt.Errorf("provider: discriminator %q in key %q must match %s", disc, s, discriminatorPattern)
	}
	return Key(s), nil
}

// NewInstanceKey builds an instance key from an adapter key and a discriminator.
func NewInstanceKey(k Key, discriminator string) (Key, error) {
	if _, ok := k.Instance(); ok {
		return "", fmt.Errorf("provider: %q is already an instance key", k)
	}
	if !discriminatorPattern.MatchString(discriminator) {
		return "", fmt.Errorf("provider: discriminator %q must match %s", discriminator, discriminatorPattern)
	}
	return ParseKey(string(k) + "@" + discriminator)
}

// InstanceOrSelf returns the instance key when a discriminator is present, and
// the adapter key otherwise, so resolvers need one line per family.
func InstanceOrSelf(k Key, discriminator string) Key {
	if discriminator == "" {
		return k
	}
	inst, err := NewInstanceKey(k, discriminator)
	if err != nil {
		return k
	}
	return inst
}

// Family is the segment before the colon.
func (k Key) Family() Family {
	family, _, _ := strings.Cut(string(k), ":")
	return Family(family)
}

// Adapter is the segment between the colon and any discriminator.
func (k Key) Adapter() string {
	_, rest, _ := strings.Cut(string(k), ":")
	adapter, _, _ := strings.Cut(rest, "@")
	return adapter
}

// Instance reports the discriminator and whether one is present.
func (k Key) Instance() (string, bool) {
	_, rest, _ := strings.Cut(string(k), ":")
	_, disc, ok := strings.Cut(rest, "@")
	return disc, ok
}

// Parent drops the instance discriminator.
func (k Key) Parent() Key {
	disc, ok := k.Instance()
	if !ok {
		return k
	}
	return Key(strings.TrimSuffix(string(k), "@"+disc))
}

// HostDiscriminator is the host and port of an endpoint, lowercased and
// sanitised, or empty when no endpoint is given. An odd value that does not
// parse still names a distinct provider rather than collapsing to nothing.
func HostDiscriminator(endpoint string) string {
	trimmed := strings.TrimSpace(endpoint)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err == nil && parsed.Host != "" {
		return sanitiseDiscriminator(parsed.Host)
	}
	stripped := strings.TrimPrefix(strings.TrimPrefix(trimmed, "http://"), "https://")
	if idx := strings.IndexByte(stripped, '/'); idx >= 0 {
		stripped = stripped[:idx]
	}
	return sanitiseDiscriminator(stripped)
}

// CommandDiscriminator is the basename of a command, lowercased.
func CommandDiscriminator(command string) string {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return ""
	}
	return sanitiseDiscriminator(filepath.Base(trimmed))
}

func sanitiseDiscriminator(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '.', r == ':':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
