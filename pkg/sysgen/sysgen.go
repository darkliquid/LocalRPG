// Package sysgen generates tabletop RPG systems from natural language descriptions.
package sysgen

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/jsonrepair"
	"github.com/darkliquid/localrpg/pkg/systemtest"
)

// Step is one progress report from the pipeline.
type Step struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// Pipeline steps.
const (
	StepShape  = "shape"
	StepSchema = "schema"
	StepHooks  = "hooks"
	StepRules  = "rules"
	StepVerify = "verify"
)

// Step statuses.
const (
	StatusDone  = "done"
	StatusError = "error"
)

// Brief is the starting description for generating a tabletop RPG system.
type Brief struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description"`
}

// VerifyResult records the outcome of verifying a generated system.
type VerifyResult struct {
	OK       bool     `json:"ok" yaml:"ok"`
	Failures []string `json:"failures,omitempty" yaml:"failures,omitempty"`
	Script   bool     `json:"script,omitempty" yaml:"script,omitempty"`
}

// System is a generated tabletop RPG system before it is saved.
type System struct {
	ID          string              `json:"id" yaml:"id"`
	Name        string              `json:"name" yaml:"name"`
	Version     string              `json:"version" yaml:"version"`
	Description string              `json:"description" yaml:"description"`
	Mechanics   *core.MechanicsSpec `json:"mechanics,omitempty" yaml:"mechanics,omitempty"`
	Script      string              `json:"script,omitempty" yaml:"script,omitempty"`
	RulesPrompt string              `json:"rules_prompt,omitempty" yaml:"rules_prompt,omitempty"`
	Verify      VerifyResult        `json:"verify" yaml:"verify"`
	// Notes record caveats from generation: a description that exceeded the
	// templates and escape hatches, or the hatches that produced JavaScript.
	Notes []string `json:"notes,omitempty" yaml:"notes,omitempty"`
}

// EstimateCalls reports how many model calls a generation makes for a brief:
// the choose, fill, and rules steps, plus the hooks step only when the brief
// requests an escape hatch.
func EstimateCalls(brief Brief) int {
	calls := 3
	if len(requestedHatches(brief)) > 0 {
		calls++
	}
	return calls
}

// Generator defines the structured-output seam for system generation.
type Generator interface {
	GenerateJSON(ctx context.Context, prompt, schema string) ([]byte, error)
}

// ErrMalformedReply reports a model reply that could not be read even after
// repair and one retry.
var ErrMalformedReply = errors.New("the model returned a reply that could not be read")

func decodeJSON(raw []byte, v any) error {
	payload := raw
	if res := jsonrepair.Repair(raw); res.OK {
		payload = res.Payload
	}
	if err := json.Unmarshal(payload, v); err != nil {
		// The length and the tail tell a cut-off reply from a reply that was never
		// JSON, which the unmarshal error alone does not.
		return fmt.Errorf("%w: parse model reply (%d bytes, ending %q): %v", ErrMalformedReply, len(payload), replyTail(payload), err)
	}
	return nil
}

// replyTail is the last few bytes of a reply, so a failure report shows whether
// the model stopped mid-word.
func replyTail(payload []byte) string {
	const window = 40
	if len(payload) <= window {
		return string(payload)
	}
	return "..." + string(payload[len(payload)-window:])
}

// retryHint is appended to the prompt when a reply could not be parsed, so the
// model is told exactly what to fix rather than repeating the mistake.
const retryHint = "\n\nYour previous reply could not be parsed as JSON (reason: %v). " +
	"Reply with one JSON object only, escaping every newline inside a string as \\n."

// generateJSON calls the generator and decodes the reply, retrying once with the
// parse failure appended to the prompt before giving up. A generate error is
// returned as-is: there is no reply to re-ask about.
func generateJSON(ctx context.Context, gen Generator, prompt, schema string, v any) error {
	raw, err := gen.GenerateJSON(ctx, prompt, schema)
	if err != nil {
		return err
	}
	parseErr := decodeJSON(raw, v)
	if parseErr == nil {
		return nil
	}

	raw, err = gen.GenerateJSON(ctx, prompt+fmt.Sprintf(retryHint, parseErr), schema)
	if err != nil {
		return parseErr
	}
	if retryErr := decodeJSON(raw, v); retryErr != nil {
		return retryErr
	}
	return nil
}

// Generate creates a System from a Brief using the provided Generator.
// stepReporter calls a caller's step callback when it has one. The callback is optional
// because a caller that only wants the finished system does not care how it was built, and
// every stage reports through here so no stage can be forgotten.
type stepReporter struct{ report func(Step) }

func (r stepReporter) step(name, status, detail string) {
	if r.report != nil {
		r.report(Step{Name: name, Status: status, Detail: detail})
	}
}

func Generate(ctx context.Context, gen Generator, brief Brief, onStep ...func(Step)) (System, error) {
	if gen == nil {
		return System{}, fmt.Errorf("sysgen: no generator configured")
	}
	if err := ctx.Err(); err != nil {
		return System{}, err
	}

	var steps stepReporter
	if len(onStep) > 0 && onStep[0] != nil {
		steps.report = onStep[0]
	}

	tpl, choice, err := chooseShape(ctx, gen, brief, steps)
	if err != nil {
		return System{}, fmt.Errorf("sysgen: %w", err)
	}

	mech, err := buildMechanics(ctx, gen, tpl, brief, steps)
	if err != nil {
		return System{}, fmt.Errorf("sysgen: %w", err)
	}

	hatches := requestedHatches(brief)
	script, err := generateScript(ctx, gen, brief, mech, hatches, steps)
	if err != nil {
		return System{}, fmt.Errorf("sysgen: %w", err)
	}

	rules, err := generateRules(ctx, gen, brief, mech, steps)
	if err != nil {
		return System{}, fmt.Errorf("sysgen: %w", err)
	}

	id, name := systemIdentity(brief)
	s := System{
		ID:          id,
		Name:        name,
		Version:     "1.0.0",
		Description: brief.Description,
		Mechanics:   mech,
		Script:      script,
		RulesPrompt: rules,
		Verify:      VerifyResult{Script: strings.TrimSpace(script) != ""},
		Notes:       generationNotes(choice, hatches),
	}

	verifySystem(&s, steps)
	return s, nil
}

// chooseShape picks the template a system is built from and reports what it chose: the reason
// the chooser gave when it gave one, and the template's own resolution otherwise.
func chooseShape(ctx context.Context, gen Generator, brief Brief, steps stepReporter) (Template, templateChoice, error) {
	tpl, choice, err := chooseTemplate(ctx, gen, brief)
	if err != nil {
		steps.step(StepShape, StatusError, err.Error())
		return Template{}, templateChoice{}, err
	}

	detail := tpl.Resolution
	if reason := strings.TrimSpace(choice.Reason); reason != "" {
		detail = reason
	}
	steps.step(StepShape, StatusDone, detail)
	return tpl, choice, nil
}

// buildMechanics fills the chosen template's parameters and builds the mechanics from them.
// Both halves report as the schema step, because they are one stage of the generation: a
// caller watching does not need to know which half failed.
func buildMechanics(ctx context.Context, gen Generator, tpl Template, brief Brief, steps stepReporter) (*core.MechanicsSpec, error) {
	params, err := fillParams(ctx, gen, tpl, brief)
	if err != nil {
		steps.step(StepSchema, StatusError, err.Error())
		return nil, err
	}
	mech, err := tpl.Build(params)
	if err != nil {
		steps.step(StepSchema, StatusError, err.Error())
		return nil, err
	}
	steps.step(StepSchema, StatusDone, tpl.ID)
	return mech, nil
}

// generateScript generates the escape hatches a brief asked for. A brief that asked for none
// reports that rather than skipping the step, so a caller sees every stage of the generation
// whether or not there was work in it.
func generateScript(ctx context.Context, gen Generator, brief Brief, mech *core.MechanicsSpec, hatches []string, steps stepReporter) (string, error) {
	if len(hatches) == 0 {
		steps.step(StepHooks, StatusDone, "no escape hatch requested")
		return "", nil
	}

	script, err := runHooks(ctx, gen, brief, mech, hatches)
	if err != nil {
		steps.step(StepHooks, StatusError, err.Error())
		return "", err
	}
	steps.step(StepHooks, StatusDone, strings.Join(hatches, ", "))
	return script, nil
}

// generateRules writes the rules prompt the system ships with.
func generateRules(ctx context.Context, gen Generator, brief Brief, mech *core.MechanicsSpec, steps stepReporter) (string, error) {
	rules, err := runRules(ctx, gen, brief, mech)
	if err != nil {
		steps.step(StepRules, StatusError, err.Error())
		return "", err
	}
	steps.step(StepRules, StatusDone, "")
	return rules, nil
}

// systemIdentity is the id and the name a generated system is saved under. A brief with no
// name gets one, because a system has to be called something.
func systemIdentity(brief Brief) (id, name string) {
	name = brief.Name
	if name == "" {
		name = "Custom System"
	}
	id = entity.Slugify(name)
	if id == "" {
		id = "custom-system"
	}
	return id, name
}

// generationNotes is what a caller is told about how the system was built: the gaps the
// template choice admitted to, and the escape hatches that got generated code.
func generationNotes(choice templateChoice, hatches []string) []string {
	var notes []string
	if gaps := strings.TrimSpace(choice.Gaps); gaps != "" {
		notes = append(notes, gaps)
	}
	if len(hatches) > 0 {
		notes = append(notes, "Generated JavaScript for: "+strings.Join(hatches, ", "))
	}
	return notes
}

// verifySystem runs a generated system through a smoke scenario and records what it found.
// The scenario exercises one check and one action, which is enough to catch a system whose
// mechanics do not load or whose script does not run; whether the system is any *good* is the
// player's judgement rather than this one's.
func verifySystem(s *System, steps stepReporter) {
	sys := systemtest.System{
		ID:        s.ID,
		Script:    s.Script,
		Mechanics: s.Mechanics,
	}

	failures := systemtest.Run(sys, smokeScenario(s.Mechanics))
	if len(failures) == 0 {
		s.Verify.OK = true
		steps.step(StepVerify, StatusDone, "passed")
		return
	}

	s.Verify.OK = false
	for _, f := range failures {
		detail := f.Detail
		if f.Scenario != "" && f.Step > 0 {
			detail = fmt.Sprintf("[%s step %d] %s", f.Scenario, f.Step, f.Detail)
		} else if f.Scenario != "" {
			detail = fmt.Sprintf("[%s] %s", f.Scenario, f.Detail)
		}
		s.Verify.Failures = append(s.Verify.Failures, detail)
	}
	steps.step(StepVerify, StatusDone, fmt.Sprintf("%d failures", len(failures)))
}

// smokeScenario is the test a generated system is put through: one check against the first
// profile it declares, and one action against its first stat. A system that declares neither
// gets an empty scenario, which passes without proving anything.
func smokeScenario(mech *core.MechanicsSpec) systemtest.Scenario {
	scenario := systemtest.Scenario{Name: "smoke"}
	if mech == nil {
		return scenario
	}

	if len(mech.Checks.Profiles) > 0 {
		keys := make([]string, 0, len(mech.Checks.Profiles))
		for k := range mech.Checks.Profiles {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		scenario.Steps = append(scenario.Steps, systemtest.Step{
			Action: "check",
			Input:  keys[0],
		})
	}
	if len(mech.Stats) > 0 {
		scenario.Steps = append(scenario.Steps, systemtest.Step{
			Action: "do",
			Input:  mech.Stats[0].ID,
		})
	}
	return scenario
}
