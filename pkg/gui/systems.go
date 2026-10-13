// Systems: the rules packages, their scenarios, and the working copies a user edits.

package gui

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/pathutil"
	"github.com/darkliquid/localrpg/pkg/refsystems"
	"github.com/darkliquid/localrpg/pkg/sysgen"
	"github.com/darkliquid/localrpg/pkg/systemtest"
	"gopkg.in/yaml.v3"
)

// systemDraftsDir is where system drafts live: a dot-directory under systems/.
func (s *Service) systemDraftsDir() string {
	return filepath.Join(s.resolver.SystemsDir(), sysgen.DraftsDirName)
}

func (s *Service) ListSystems(ctx context.Context) ([]SystemSummaryDTO, error) {
	sysDir := s.resolver.SystemsDir()
	entries, err := os.ReadDir(sysDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []SystemSummaryDTO{}, nil
		}
		return nil, fmt.Errorf("read systems dir: %w", err)
	}

	summaries := make([]SystemSummaryDTO, 0)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		manifestPath := filepath.Join(sysDir, e.Name(), "system.yaml")
		m, err := core.LoadSystemManifest(manifestPath)
		if err != nil {
			continue
		}
		summaries = append(summaries, SystemSummaryDTO{
			ID:          m.ID,
			Name:        m.Name,
			Description: m.Description,
			Version:     m.Version,
		})
	}
	return summaries, nil
}

const defaultMechanicsScript = `// LocalRPG Rule System Engine
// Globals available: roll(notation), state, log(msg)

function evaluateRoll(stats, diceExpr) {
  const result = roll(diceExpr || "2d6");
  return {
    total: result.total,
    success: result.total >= 10,
    rolls: result.rolls
  };
}
`

func (s *Service) GetSystem(ctx context.Context, id string) (*SystemDetailDTO, error) {
	if err := pathutil.ValidateID(id); err != nil {
		return nil, fmt.Errorf("invalid system id: %w", err)
	}

	sysDir := s.resolver.SystemDir(id)
	m, err := core.LoadSystemManifest(filepath.Join(sysDir, "system.yaml"))
	if err != nil {
		return nil, fmt.Errorf("load system manifest: %w", err)
	}

	script := ""
	if data, err := os.ReadFile(filepath.Join(sysDir, "mechanics.js")); err == nil {
		script = string(data)
	}

	rulesPrompt := ""
	if data, err := os.ReadFile(filepath.Join(sysDir, "prompts", "rules.md")); err == nil {
		rulesPrompt = string(data)
	}

	return &SystemDetailDTO{
		ID:                m.ID,
		Name:              m.Name,
		Version:           m.Version,
		Description:       m.Description,
		Script:            script,
		RulesPrompt:       rulesPrompt,
		CharacterCreation: m.CharacterCreation,
		Mechanics:         m.Mechanics,
	}, nil
}

func (s *Service) SaveSystem(ctx context.Context, req CreateSystemRequestDTO) (*SystemDetailDTO, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("system name is required")
	}
	id := req.ID
	if id == "" {
		id = slugify(req.Name)
	}
	if err := pathutil.ValidateID(id); err != nil {
		return nil, fmt.Errorf("invalid system id %q: %w", id, err)
	}
	if req.Version == "" {
		req.Version = "1.0.0"
	}
	script := req.Script
	if script == "" {
		script = defaultMechanicsScript
	}

	gate := sysgen.Gate(sysgen.System{ID: id, Script: script, Mechanics: req.Mechanics})
	if req.Strict && !gate.OK {
		return nil, fmt.Errorf("system failed the smoke test: %s", gate.FailureText())
	}

	sysDir := s.resolver.SystemDir(id)
	if err := os.MkdirAll(sysDir, 0755); err != nil {
		return nil, fmt.Errorf("create system dir: %w", err)
	}

	manifest := core.SystemManifest{
		ID:                id,
		Name:              req.Name,
		Version:           req.Version,
		Description:       req.Description,
		CharacterCreation: req.CharacterCreation,
		Mechanics:         req.Mechanics,
	}
	data, err := yaml.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("marshal system manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(sysDir, "system.yaml"), data, 0644); err != nil {
		return nil, fmt.Errorf("write system.yaml: %w", err)
	}

	if err := os.WriteFile(filepath.Join(sysDir, "mechanics.js"), []byte(script), 0644); err != nil {
		return nil, fmt.Errorf("write mechanics.js: %w", err)
	}

	if req.RulesPrompt != "" {
		promptDir := filepath.Join(sysDir, "prompts")
		if err := os.MkdirAll(promptDir, 0755); err != nil {
			return nil, fmt.Errorf("create system prompts dir: %w", err)
		}
		if err := os.WriteFile(filepath.Join(promptDir, "rules.md"), []byte(req.RulesPrompt), 0644); err != nil {
			return nil, fmt.Errorf("write rules.md: %w", err)
		}
	}

	detail, err := s.GetSystem(ctx, id)
	if err != nil {
		return nil, err
	}
	warnings := validateMechanics(req.Mechanics)
	if !gate.OK {
		warnings = append(warnings, "smoke test: "+gate.FailureText())
	}
	detail.Warnings = warnings
	return detail, nil
}

// ErrSystemNotFound reports an attempt to operate on a system that does not exist.
var ErrSystemNotFound = errors.New("system not found")

// ErrSystemInUse reports an attempt to delete a system that is still referenced by a campaign or world.
var ErrSystemInUse = errors.New("system is in use")

// DeleteSystem removes a system directory.
// When force is false, it refuses to delete if any campaign or world references this system.
func (s *Service) DeleteSystem(ctx context.Context, systemID string, force bool) error {
	if err := pathutil.ValidateID(systemID); err != nil {
		return fmt.Errorf("invalid system id: %w", err)
	}

	sysDir := s.resolver.SystemDir(systemID)
	if _, err := os.Stat(filepath.Join(sysDir, "system.yaml")); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s", ErrSystemNotFound, systemID)
		}
		return fmt.Errorf("system %q: %w", systemID, err)
	}

	if !force {
		games, err := s.ListGames(ctx)
		if err == nil {
			for _, g := range games {
				if g.SystemID == systemID {
					return fmt.Errorf("%w: campaign %q (%s)", ErrSystemInUse, g.Name, g.ID)
				}
			}
		}

		worlds, err := s.ListWorlds(ctx)
		if err == nil {
			for _, w := range worlds {
				for _, comp := range w.CompatibleSystems {
					if comp == systemID {
						return fmt.Errorf("%w: world %q (%s)", ErrSystemInUse, w.Name, w.ID)
					}
				}
			}
		}
	}

	if err := os.RemoveAll(sysDir); err != nil {
		return fmt.Errorf("remove system: %w", err)
	}
	return nil
}

// ListReferenceSystems returns the shipped starting systems, so the studio offers
// the same corpus the tests exercise.
func (s *Service) ListReferenceSystems(_ context.Context) (*ReferenceSystemsDTO, error) {
	systems := refsystems.List()
	out := &ReferenceSystemsDTO{Systems: make([]ReferenceSystemDTO, 0, len(systems))}
	for _, sys := range systems {
		out.Systems = append(out.Systems, ReferenceSystemDTO{
			ID:          sys.ID,
			Name:        sys.Name,
			Version:     sys.Version,
			Description: sys.Description,
			RulesPrompt: sys.RulesPrompt,
			Script:      sys.Script,
			Mechanics:   sys.Mechanics,
		})
	}
	return out, nil
}

// TestSystem runs scenarios against a system and returns the expectations that
// failed, so an authored system can be verified without a store or a provider.
func (s *Service) TestSystem(_ context.Context, req SystemTestRequestDTO) (*SystemTestResponseDTO, error) {
	sys := systemtest.System{ID: req.System.ID, Script: req.System.Script, Mechanics: req.System.Mechanics}
	failures := systemtest.RunAll(sys, req.Scenarios)
	out := &SystemTestResponseDTO{}
	for _, f := range failures {
		out.Failures = append(out.Failures, SystemTestFailureDTO{Scenario: f.Scenario, Step: f.Step, Detail: f.Detail})
	}
	return out, nil
}

// SystemScenarios reads a system's stored scenarios from its tests directory.
func (s *Service) SystemScenarios(_ context.Context, id string) (*SystemScenariosDTO, error) {
	if err := pathutil.ValidateID(id); err != nil {
		return nil, fmt.Errorf("invalid system id: %w", err)
	}
	dir := filepath.Join(s.resolver.SystemDir(id), "tests")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return &SystemScenariosDTO{}, nil
	}
	out := &SystemScenariosDTO{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		scenario, err := systemtest.LoadScenario(data)
		if err != nil {
			return nil, fmt.Errorf("load scenario %q: %w", entry.Name(), err)
		}
		out.Scenarios = append(out.Scenarios, scenario)
	}
	return out, nil
}

// SaveSystemScenario writes one scenario under systems/<id>/tests/<slug>.yaml.
// The scenario is re-parsed before the write lands, so a malformed or step-less
// scenario is refused rather than stored.
func (s *Service) SaveSystemScenario(_ context.Context, id string, scenario systemtest.Scenario) error {
	if err := pathutil.ValidateID(id); err != nil {
		return fmt.Errorf("invalid system id: %w", err)
	}
	name := strings.TrimSpace(scenario.Name)
	if name == "" {
		return fmt.Errorf("a scenario needs a name")
	}
	data, err := systemtest.EncodeScenario(scenario)
	if err != nil {
		return err
	}
	dir := filepath.Join(s.resolver.SystemDir(id), "tests")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create tests dir: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, entity.Slugify(name)+".yaml"), data, 0644)
}

// DeleteSystemScenario removes a stored scenario by the name it declares, so a
// scenario hand-renamed on disk is still reachable.
func (s *Service) DeleteSystemScenario(_ context.Context, id, name string) error {
	if err := pathutil.ValidateID(id); err != nil {
		return fmt.Errorf("invalid system id: %w", err)
	}
	want := strings.TrimSpace(name)
	dir := filepath.Join(s.resolver.SystemDir(id), "tests")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("delete scenario %q: %w", want, fs.ErrNotExist)
		}
		return fmt.Errorf("read tests dir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			continue
		}
		scenario, loadErr := systemtest.LoadScenario(data)
		if loadErr != nil {
			continue
		}
		if scenario.Name == want || entity.Slugify(scenario.Name) == entity.Slugify(want) {
			if removeErr := os.Remove(path); removeErr != nil {
				return fmt.Errorf("remove scenario: %w", removeErr)
			}
			return nil
		}
	}
	return fmt.Errorf("delete scenario %q: %w", want, fs.ErrNotExist)
}
