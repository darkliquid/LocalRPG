---
id: 09-systems-studio
title: Building Custom Systems Guide
category: Studio Guides
order: 9
description: Authoring system rules prompts, coding mechanics.js hooks, and testing dice expressions.
---

# Building Custom Systems: Studio Guide

The **Systems Studio** enables you to craft tabletop mechanics from scratch or recreate your favorite roleplaying games.

## System Configuration (`system.yaml`)

```yaml
id: grim-survival
name: Grim Survival d6
description: A gritty ruleset focusing on stamina depletion, scarcity, and dangerous skill checks.
version: "1.0.0"
action_modes:
  - id: do
    label: Action
  - id: say
    label: Dialogue
  - id: roll
    label: Skill Check
```

## Crafting the Rules Prompt (`prompts/rules.md`)

The rules prompt guides the GM agent when calling for checks:

```markdown
# Rules System: Grim Survival

## Core Resolution
When the player attempts a risky or uncertain action, call for a d6 check:
- **1-2**: Failure with severe consequence or injury.
- **3-4**: Partial success with a complication or stamina loss.
- **5-6**: Clean success.

## Difficulty Modifiers
- Difficult tasks impose a -1 modifier.
- Prepared equipment or relevant traits grant +1.
```

## Writing `mechanics.js` Hooks

Create custom resolution logic in JavaScript. The sandbox includes helper methods like `rollDice(notation)`:

```javascript
/**
 * resolveAction is invoked whenever a turn requires mechanics evaluation.
 * @param {Object} ctx - The execution context
 * @returns {Object} Resolution outcome and state mutations
 */
function resolveAction(ctx) {
  if (ctx.action.mode === 'roll') {
    const roll = rollDice('1d6');
    let outcome = 'failure';
    let cue = 'Things go terribly wrong.';

    if (roll.total >= 5) {
      outcome = 'success';
      cue = 'You achieve your goal cleanly.';
    } else if (roll.total >= 3) {
      outcome = 'mixed';
      cue = 'You succeed, but pay a price.';
    }

    return {
      success: outcome !== 'failure',
      outcome: outcome,
      roll: roll,
      narrative_cue: cue,
      state_patch: {
        last_roll: roll.total
      }
    };
  }

  return { pass_to_gm: true };
}
```

## Live Studio Testing

Use the built-in **Dice & Rules Tester** at the bottom of the Systems Studio to execute trial actions, verify dice formulas, and inspect returned state patches before deploying your system to a campaign.
