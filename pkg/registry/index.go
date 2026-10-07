package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/pathutil"
)

// Package describes one content package available in a registry index.
type Package struct {
	Type        string                    `json:"type"`
	ID          string                    `json:"id"`
	Name        string                    `json:"name"`
	Version     string                    `json:"version"`
	Description string                    `json:"description,omitempty"`
	Author      string                    `json:"author,omitempty"`
	License     string                    `json:"license,omitempty"`
	Download    string                    `json:"download"`
	SHA256      string                    `json:"sha256"`
	Publisher   string                    `json:"publisher,omitempty"` // public key fingerprint
	Requires    []core.ContentRequirement `json:"requires,omitempty"`
}

// Index is a static package catalogue served by a registry.
type Index struct {
	Name     string    `json:"name"`
	Homepage string    `json:"homepage,omitempty"`
	Packages []Package `json:"packages"`
}

// PackageRef pairs a Package with its source registry metadata.
type PackageRef struct {
	RegistryName string  `json:"registry_name"`
	RegistryURL  string  `json:"registry_url"`
	Package      Package `json:"package"`
}

// ParseIndex parses and validates registry index JSON bytes.
func ParseIndex(data []byte) (Index, error) {
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return Index{}, fmt.Errorf("unmarshal index: %w", err)
	}

	if strings.TrimSpace(idx.Name) == "" {
		return Index{}, errors.New("registry index name is required")
	}

	for i, p := range idx.Packages {
		if err := pathutil.ValidateID(p.ID); err != nil {
			return Index{}, fmt.Errorf("package [%d] has invalid id %q: %w", i, p.ID, err)
		}
		if p.Type != "world" && p.Type != "system" {
			return Index{}, fmt.Errorf("package %q has invalid type %q (must be world or system)", p.ID, p.Type)
		}
		if err := core.ValidateSemver(p.Version); err != nil {
			return Index{}, fmt.Errorf("package %q has invalid version %q: %w", p.ID, p.Version, err)
		}
		if strings.TrimSpace(p.Download) == "" {
			return Index{}, fmt.Errorf("package %q is missing download URL/path", p.ID)
		}
		if strings.TrimSpace(p.SHA256) == "" {
			return Index{}, fmt.Errorf("package %q is missing sha256 checksum", p.ID)
		}
	}

	return idx, nil
}
