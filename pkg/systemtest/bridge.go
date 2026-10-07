package systemtest

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/rules"
)

// PlayerID is the entity id every scenario's player is known by.
const PlayerID = "player"

// Bridge is a deterministic GameHostAPI for a scenario: dice come from a seeded
// source, and state lives in one in-memory entity. It is the only randomness a
// scenario sees, so the same seed always produces the same run.
type Bridge struct {
	rand       *rand.Rand
	mechanics  *core.MechanicsSpec
	player     *entity.Entity
	location   string
	directives []string
	logs       []string
}

// NewBridge builds a deterministic bridge seeded from the scenario.
func NewBridge(mechanics *core.MechanicsSpec, seed int64, player SetupEntity) *Bridge {
	ent := &entity.Entity{ID: PlayerID, Name: "Player", Type: "character", Tags: player.Tags}
	ent.InitState(player.Stats)
	return &Bridge{
		rand:      rand.New(rand.NewSource(seed)),
		mechanics: mechanics,
		player:    ent,
	}
}

// Roll evaluates the notation subset the harness supports with the seeded source:
// NdM, an optional >=K success target, and an optional +k/-k modifier. A target
// makes Total the success count, matching the dice library the engine uses.
func (b *Bridge) Roll(notation string) (*rules.RollResult, error) {
	original := strings.TrimSpace(notation)
	rest := original

	target := 0
	if idx := strings.Index(rest, ">="); idx >= 0 {
		t, err := strconv.Atoi(strings.TrimSpace(rest[idx+2:]))
		if err != nil {
			return nil, fmt.Errorf("roll %q: bad success target", original)
		}
		target = t
		rest = strings.TrimSpace(rest[:idx])
	}

	mod := 0
	if idx := strings.LastIndexAny(rest, "+-"); idx > 0 {
		m, err := strconv.Atoi(rest[idx:])
		if err != nil {
			return nil, fmt.Errorf("roll %q: bad modifier", original)
		}
		mod = m
		rest = strings.TrimSpace(rest[:idx])
	}

	count, sides := 1, 0
	lower := strings.ToLower(rest)
	if d := strings.IndexByte(lower, 'd'); d >= 0 {
		if d > 0 {
			n, err := strconv.Atoi(rest[:d])
			if err != nil {
				return nil, fmt.Errorf("roll %q: bad dice count", original)
			}
			count = n
		}
		m, err := strconv.Atoi(rest[d+1:])
		if err != nil {
			return nil, fmt.Errorf("roll %q: bad die size", original)
		}
		sides = m
	} else if n, err := strconv.Atoi(rest); err == nil {
		// A bare number is a constant: total n, no dice.
		return &rules.RollResult{Notation: original, Total: n + mod}, nil
	}
	if sides <= 0 || count <= 0 {
		return nil, fmt.Errorf("roll %q: unsupported notation", original)
	}

	faces := make([]harness.DieFace, 0, count)
	sum := 0
	successes := 0
	for i := 0; i < count; i++ {
		value := b.rand.Intn(sides) + 1
		faces = append(faces, harness.DieFace{Value: value, Symbol: strconv.Itoa(value)})
		sum += value
		if target > 0 && value >= target {
			successes++
		}
	}

	total := sum + mod
	if target > 0 {
		total = successes
	}
	return &rules.RollResult{
		Notation:  original,
		Total:     total,
		Successes: successes,
		RollCount: count,
		Dice:      faces,
	}, nil
}

// GetStat reads a numeric or string stat from the player.
func (b *Bridge) GetStat(entityID string, path string) (interface{}, error) {
	ent, err := b.resolve(entityID)
	if err != nil {
		return nil, err
	}
	if ent.State == nil {
		return nil, nil
	}
	value, _ := ent.State.Get(path)
	return value, nil
}

// SetStat writes a stat on the player.
func (b *Bridge) SetStat(entityID string, path string, value interface{}) error {
	ent, err := b.resolve(entityID)
	if err != nil {
		return err
	}
	if ent.State == nil {
		ent.InitState(make(map[string]interface{}))
	}
	return ent.State.Set(path, value)
}

// GetEntity returns the player, and nothing else, so a scenario cannot reach
// entities it never set up.
func (b *Bridge) GetEntity(entityID string) (*entity.Entity, error) {
	return b.resolve(entityID)
}

// SaveEntity is a no-op: the bridge is the store.
func (b *Bridge) SaveEntity(*entity.Entity) error { return nil }

// InjectGMDirection records a directive a scenario may inspect.
func (b *Bridge) InjectGMDirection(directive string) { b.directives = append(b.directives, directive) }

// GetDirectives returns every injected directive.
func (b *Bridge) GetDirectives() []string { return b.directives }

// Log records a script log line.
func (b *Bridge) Log(message string) { b.logs = append(b.logs, message) }

// GetLogs returns every logged line.
func (b *Bridge) GetLogs() []string { return b.logs }

// SetLocation records the player's location.
func (b *Bridge) SetLocation(locationID string) error {
	b.location = locationID
	return nil
}

// GetLocation returns the player's location.
func (b *Bridge) GetLocation() (string, error) { return b.location, nil }

// ListStats returns the declared stats.
func (b *Bridge) ListStats() []core.StatSpec {
	if b.mechanics == nil {
		return nil
	}
	return b.mechanics.Stats
}

// ListSkills returns the declared skills.
func (b *Bridge) ListSkills() []core.SkillSpec {
	if b.mechanics == nil {
		return nil
	}
	return b.mechanics.Skills
}

// CheckConventions returns the declared check conventions.
func (b *Bridge) CheckConventions() core.CheckConventions {
	if b.mechanics == nil {
		return core.CheckConventions{}
	}
	return b.mechanics.Checks
}

// StateValue reads a stat from the bridge's player, for a scenario's state
// assertions.
func (b *Bridge) StateValue(path string) (interface{}, bool) {
	if b.player.State == nil {
		return nil, false
	}
	return b.player.State.Get(path)
}

func (b *Bridge) resolve(entityID string) (*entity.Entity, error) {
	if entityID == "" || entityID == PlayerID {
		return b.player, nil
	}
	return nil, fmt.Errorf("entity %q not found", entityID)
}
