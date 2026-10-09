---
id: 03-systems
title: Systems & Mechanics
category: Core Concepts
order: 3
description: Rules prompts, action modes, dice expressions, and the sandboxed JavaScript mechanics engine.
---

# Systems & Mechanics

A **System** defines how actions are resolved, what dice are rolled, how characters progress, and how rules are enforced.

## System Structure

```text
systems/classic-d20/
├── system.yaml         # System metadata, modes, and dice definitions
├── mechanics.js        # Sandboxed JavaScript mechanics engine
└── prompts/
    └── rules.md        # Natural language instructions for the GM agent
```

## Action Modes

LocalRPG supports four fundamental action modes configured in `system.yaml`:

- **`do`**: Physical or active interventions ("I leap across the chasm").
- **`say`**: Direct dialogue or social interactions ("I ask the merchant about the lost amulet").
- **`story`**: Narrative establishment or background declarations ("Ten years ago, my guild swore an oath…").
- **`roll`**: Explicit rules checks evaluated against the mechanics engine ("Roll Athletics DC 15").

## Dice Expressions

LocalRPG features a built-in dice evaluation engine supporting standard tabletop notations:

- `1d20 + 5`: Roll a 20-sided die and add 5.
- `2d6`: Roll two six-sided dice and sum the results.
- `4dF`: Fate/Fudge dice (values -1, 0, +1).
- `1d100` / `d%`: Percentile dice.
- `3d6kh2`: Keep highest 2 of 3 six-sided dice.

## Resolution Profiles

A system can declare named **resolution profiles** under `mechanics.checks.profiles`, so the GM can name how a check resolves. A profile maps a roll to an outcome. The supported kinds are threshold ladders, difficulty classes, success-count pools, and position/effect pairs.

```yaml
mechanics:
  checks:
    notation: 2d6
    profiles:
      pbta:
        ladder:
          - { min: 10, outcome: strong }
          - { min: 7, outcome: weak }
          - { min: 0, outcome: miss }
      d20:
        notation: 1d20
        dc: 15
      pool:
        notation: 5d10
        success_on: ">=8"
        outcomes:
          - { min: 3, max: -1, outcome: strong }
          - { min: 1, max: 2, outcome: weak }
          - { min: 0, max: 0, outcome: miss }
```

Ladders resolve highest-first. Difficulty classes pass when the total meets them. Pools count the dice that meet `success_on`. A check without a profile resolves through the system's default conventions, exactly as before.

### Opposed Checks

A profile can declare that a check is **opposed**. The opponent rolls too, and the
higher total decides:

```yaml
mechanics:
  checks:
    profiles:
      grapple:
        notation: 2d6
        dc: 10
        opposed: might    # the opponent rolls Might
        ties: opponent    # an equal total goes to the opponent; the default is actor
```

A check is opposed when the GM sets `target` to the opponent and `opposed` to a
stat. When the profile declares `opposed`, it supplies the stat and the GM sets
only the target. The opponent rolls the same notation and adds its `opposed` stat
to the roll. An unknown opponent rolls flat, and a mistyped target resolves
against that flat roll. The profile's best outcome applies when the actor's total
is higher, and its worst when the opponent's is.

## Sandboxed JavaScript Mechanics Engine (`mechanics.js`)

Custom systems export JavaScript functions that execute inside an isolated Goja runtime. The engine passes a small context and receives a structured resolution:

```javascript
// Example mechanics.js
onAction('roll', function (ctx) {
  // ctx.player is the player's entity id, and ctx.action is the player's input.
  const athletics = getStat(ctx.player, 'athletics') || 0;
  const result = roll('1d20 + ' + athletics);
  const targetDC = 12;
  const success = result.total >= targetDC;

  return {
    success: success,
    roll: result,
    message: success ? 'Feat succeeded with style.' : 'Complication arises.',
  };
});
```

The mechanics engine exposes hooks including `onAction`, `onTurnBegin`, `onTurnEnd`, `onWorldTick`, `onCheck`, and `onHealthZero`, plus helpers such as `roll`, `getStat`, `setStat`, and `injectGMDirection`.
