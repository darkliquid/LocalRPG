package gui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// findingsFile is where addressed continuity findings are recorded. It is a sidecar
// rather than an edit to history.jsonl, because that log is append-only and a
// dismissal is a player's decision about a turn, not a change to it.
const findingsFile = "findings.json"

// AddressedFinding names one finding a player has dealt with.
type AddressedFinding struct {
	Turn int    `json:"turn"`
	Rule string `json:"rule"`
}

// FindingsDTO is the whole sidecar as a client reads it.
type FindingsDTO struct {
	Addressed []AddressedFinding `json:"addressed"`
}

func (s *Service) findingsPath(gameID string) string {
	return filepath.Join(s.resolver.GameDir(gameID), findingsFile)
}

// Findings returns what has been addressed for a campaign.
func (s *Service) Findings(gameID string) (FindingsDTO, error) {
	result := FindingsDTO{Addressed: []AddressedFinding{}}

	data, err := os.ReadFile(s.findingsPath(gameID))
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return result, fmt.Errorf("read findings: %w", err)
	}

	if err := json.Unmarshal(data, &result); err != nil {
		return result, fmt.Errorf("parse findings: %w", err)
	}
	if result.Addressed == nil {
		result.Addressed = []AddressedFinding{}
	}
	return result, nil
}

// AddressFinding records that a player has dealt with one finding. Recording it twice
// is a no-op rather than an error, because a client may retry.
func (s *Service) AddressFinding(gameID string, turn int, rule string) error {
	current, err := s.Findings(gameID)
	if err != nil {
		return err
	}

	for _, existing := range current.Addressed {
		if existing.Turn == turn && existing.Rule == rule {
			return nil
		}
	}

	current.Addressed = append(current.Addressed, AddressedFinding{Turn: turn, Rule: rule})

	data, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return fmt.Errorf("encode findings: %w", err)
	}
	if err := os.WriteFile(s.findingsPath(gameID), data, 0644); err != nil {
		return fmt.Errorf("write findings: %w", err)
	}
	return nil
}
