export interface ReferenceEntityTemplate {
  id: string;
  name: string;
  type: string;
  markdown: string;
}

export interface ReferenceSystemTemplate {
  id: string;
  name: string;
  version: string;
  description: string;
  rules_prompt: string;
  script: string;
}

export interface ReferenceWorldTemplate {
  id: string;
  name: string;
  description: string;
  genre: string;
  default_system: string;
  art_style: string;
  tags: string[];
  lore_prompt: string;
  entities: ReferenceEntityTemplate[];
}

export const REFERENCE_SYSTEM_TEMPLATE: ReferenceSystemTemplate = {
  id: 'narrative_2d6',
  name: 'Narrative 2d6 Engine',
  version: '1.0.0',
  description: 'Versatile 2d6 resolution with partial success, dynamic stress, and event hooks.',
  rules_prompt: `You are the Game Master adjudicating a campaign governed by the **Narrative 2d6 Engine**.

### 1. Core Resolution Ladder
When player actions face meaningful risk, adversity, or uncertainty, outcomes are adjudicated using two six-sided dice (2d6):
- **10+ (Strong Hit / Full Success)**: The protagonist accomplishes their goal cleanly without complication, resource expenditure, or collateral harm.
- **7–9 (Weak Hit / Partial Success)**: The protagonist accomplishes their goal, but at a tangible cost: minor damage or stress, a dangerous complication, a trade-off, or diminished effect.
- **6- (Miss / Hard Move)**: The action falters. Escalate the immediate threat, introduce an ambush or sudden turn of fortune, deplete a vital resource, or put the protagonist in immediate peril.

### 2. Action Modes & Mechanics Hooks
The engine runs JavaScript hooks before generating your GM narrative:
- \`do\`: General active intent (physical feats, athletics, stealth, lockpicking). Invokes the \`onAction("do", ctx)\` hook.
- \`attack\`: Direct violent conflict against adversaries. Invokes \`onAction("attack", ctx)\`, dealing damage and modifying character health stats.
- \`say\`: Social dialogue, persuasion, interrogation, or intimidation.
- \`roll\`: Direct arbitrary dice expressions (e.g., 1d20, 2d6+2).

### 3. Handling Mechanics Results
When the prompt contains a \`[MECHANICS RESULT: ...]\` tag:
- You **must** honor the outcome described in the message.
- Weave any damage dealt, wounds suffered, or tactical shifts directly into the narrative prose.
- Never contradict the numerical roll total, damage numbers, or state changes reported by the mechanics engine.
`,
  script: `// ==========================================
// Narrative 2d6 Engine - Mechanics Script
// ==========================================

// Handle active "do" actions (physical feats, infiltration, survival)
onAction("do", function(ctx) {
  var r = roll("2d6");
  var message = "";

  if (r.total >= 10) {
    message = "Strong Hit (Total: " + r.total + ") - Complete triumph with no complications.";
  } else if (r.total >= 7) {
    message = "Weak Hit (Total: " + r.total + ") - Success achieved, but at a cost or complication.";
  } else {
    message = "Miss (Total: " + r.total + ") - The attempt falters; danger escalates.";
    injectGMDirection("The action failed. Introduce an immediate complication or escalate danger.");
  }

  return {
    success: r.total >= 7,
    message: message,
    roll: r
  };
});

// Handle combat "attack" actions
onAction("attack", function(ctx) {
  var r = roll("2d6");
  var msg = "";

  if (r.total >= 10) {
    msg = "Critical Strike (Total: " + r.total + ")! Target takes 4 damage.";
  } else if (r.total >= 7) {
    msg = "Glancing Hit (Total: " + r.total + ")! Target takes 2 damage, but counters for 1 damage.";
    var hp = getStat(ctx.player, "health");
    if (hp === null || hp === undefined) hp = 10;
    setStat(ctx.player, "health", Math.max(0, hp - 1));
  } else {
    msg = "Attack Deflected (Total: " + r.total + ")! The adversary seizes the upper hand.";
    injectGMDirection("The enemy retaliates swiftly. Put the protagonist on the defensive.");
  }

  return {
    success: r.total >= 7,
    message: msg,
    roll: r
  };
});

// Lifecycle hook executed at the end of each turn
onTurnEnd(function(ctx) {
  log("Turn " + ctx.turn + " completed in Narrative 2d6 Engine.");
});
`,
};

export const REFERENCE_WORLD_TEMPLATE: ReferenceWorldTemplate = {
  id: 'the_ashen_reach',
  name: 'The Ashen Reach',
  description: 'A mist-veiled frontier of shattered cathedral keeps, lingering ember magic, and peat bogs.',
  genre: 'Dark Fantasy / Gothic Exploration',
  default_system: 'narrative_2d6',
  art_style: 'Oil on textured canvas, chiaroscuro lighting, deep umber and lantern gold, atmospheric fog, classical dark fantasy illustration',
  tags: ['gothic', 'dark_fantasy', 'mystery', 'ruins'],
  lore_prompt: `You are narrating an adventure in **The Ashen Reach**.

### 1. Atmosphere & Sensory Guidelines
- **Sensory Cues**: The damp chill of perpetual drizzle on worn granite. The smell of wet peat, cold iron, and guttering tallow lanterns. Distant cathedral bells whose ropes are pulled by unseen hands.
- **Visuals**: Low mist rolling across brackish marshland, silhouetting broken arches, sunken buttresses, and weathered gargoyles.
- **Theme**: Resilience amidst encroaching decay. Light is precious, warmth is fleeting, and the dark mist hungers.

### 2. Major Factions & Conflicts
- **The Ember Wardens**: Weathered knights and lantern-bearers dedicated to defending isolated havens and maintaining the brass braziers.
- **The Hollowed**: Travelers and deserters who lost their flame in the peat bogs, now wandering as sorrowful, mist-bound shades.
`,
  entities: [
    {
      id: 'the_ashen_bastion',
      name: 'The Ashen Bastion',
      type: 'location',
      markdown: `---
name: The Ashen Bastion
type: location
state:
  danger_level: 2
  brazier_lit: true
wikilinks:
  - wardens_of_the_ember
  - the_creeping_miasma
---
A fortified granite sanctuary resting upon sunken cathedral arches above the peat bogs. Its central brass brazier burns day and night, casting warm amber light that keeps the encroaching miasma at bay.
`,
    },
    {
      id: 'wardens_of_the_ember',
      name: 'Wardens of the Ember',
      type: 'character',
      markdown: `---
name: Wardens of the Ember
type: character
state:
  allegiance: friendly
  disposition: weary
  fuel_reserves: 3
wikilinks:
  - the_ashen_bastion
---
A resolute brotherhood of lantern-bearers sworn to shield the remaining settlements of the Reach. Their armor is pitted with rust and their supply of consecrated oil runs precariously low.
`,
    },
    {
      id: 'the_creeping_miasma',
      name: 'The Creeping Miasma',
      type: 'arc',
      markdown: `---
name: The Creeping Miasma
type: arc
state:
  clock_ticks: 1
  clock_max: 6
wikilinks:
  - the_ashen_bastion
---
A supernatural, freezing fog that creeps upward from the deep hollows whenever a settlement's brazier begins to gutter. At 6 ticks, the outer defenses fall to the cold mist.
`,
    },
  ],
};
