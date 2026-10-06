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
