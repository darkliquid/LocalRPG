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
mechanics:
  stats:
    - id: stamina
      label: Stamina
      type: number
      default: 10
    - id: athletics
      label: Athletics
      type: number
      default: 1
  checks:
    notation: 1d6
    outcome: [failure, mixed, success]
```

## Reference Systems

The studio offers complete starting systems for a PbtA 2d6 ladder, a d20 difficulty class, and a d10 success pool. They are built into the binary and are the same corpus the engine tests exercise.

## Editing Mechanics in the Studio

The **Mechanics** tab edits the whole `mechanics` block: stats, skills, health, check conventions (including resolution profiles), advancement, freeform state, and the engagement default. Each list adds and removes its own rows, and the studio warns about an invalid id, a duplicate id, a skill whose stat is undeclared, or a malformed profile before it saves.

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

Create custom resolution logic in JavaScript. The sandbox exposes the helpers `roll`, `getStat`, `setStat`, `getLocation`, `setLocation`, `injectGMDirection`, `log`, and `grantXP`, and you register hooks with `onAction`, `onTurnBegin`, `onTurnEnd`, `onWorldTick`, `onCheck`, and `onHealthZero`. A handler receives the execution context and returns a resolution the engine applies:

```javascript
// onAction registers a handler for one action mode. The handler returns a
// resolution: success, an outcome word, the roll, and a message.
onAction('roll', function (ctx) {
  const result = roll('1d6');

  if (result.total >= 5) {
    return { success: true, outcome: 'success', roll: result, message: 'You achieve your goal cleanly.' };
  }
  if (result.total >= 3) {
    return { success: true, outcome: 'mixed', roll: result, message: 'You succeed, but pay a price.' };
  }
  return { success: false, outcome: 'failure', roll: result, message: 'Things go terribly wrong.' };
});
```

## Live Studio Testing

Use the built-in **Dice & Rules Tester** at the bottom of the Systems Studio to execute trial actions, verify dice formulas, and inspect returned state patches before deploying your system to a campaign.
