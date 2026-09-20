# Zero-Default Systems & Worlds with Comprehensive Reference Examples Specification

## 1. Overview & Goals

LocalRPG currently ships with zero pre-installed systems or worlds on disk, but contained hardcoded fallbacks to `"daggerheart"` and `"solitary_defiance"`. Furthermore, authoring environments lacked a first-class prompt layer (`prompts/rules.md` and `prompts/lore.md`) teaching the AI storytelling agent what mechanics hooks exist, when to invoke them, and how to adjudicate outcomes.

### Goals:
1. **Zero Built-In / Hardcoded Defaults:** Remove all hardcoded fallbacks in backend and frontend. The game starts with completely empty `systems/`, `worlds/`, and `games/` directories, requiring no phantom default assets.
2. **Comprehensive Placeholder / Reference Examples:** Provide a production-grade reference implementation pre-populated in the Studio authoring environments and restorable via a "Reset to Reference Template" button:
   - **System:** *Narrative 2d6 Engine* with complete `system.yaml`, rich `prompts/rules.md` (explaining resolution thresholds, action modes, and result interpretation to the agent), and `mechanics.js` implementing action hooks (`do`, `attack`), dice rolling (`roll("2d6")`), entity state access (`getStat`, `setStat`), and GM directives (`injectGMDirection`).
   - **World:** *The Ashen Reach* with complete `world.yaml`, atmospheric `prompts/lore.md`, art style guide prompts, and 3 starter entity templates (`location`, `character`, `arc` with progress clocks).
3. **Engine Context Assembly Integration:** Update `pkg/engine/orchestrator.go` and `pkg/harness/context.go` to inject the active system's `rules.md` and world's `lore.md` into the agent's context prompt, ensuring the LLM understands and respects mechanics results.
4. **Intuitive Zero-State Wizard:** Update `LauncherHub.tsx` to handle zero systems/worlds gracefully with clear prompts and one-click shortcuts to the Studio tabs.

---

## 2. Architecture & File Structure

### 2.1 File Layout on Disk
```text
systems/
└── <system-id>/
    ├── system.yaml         # Manifest (id, name, version, description)
    ├── mechanics.js        # JavaScript hooks & resolution logic
    └── prompts/
        └── rules.md        # GM instructions on rules, action modes, and result handling

worlds/
└── <world-id>/
    ├── world.yaml          # Manifest (id, name, genre, default_system, art_style, tags)
    ├── prompts/
    │   └── lore.md         # GM setting lore, sensory guidelines, atmosphere
    └── entities/           # Markdown entity templates copied into new games
        ├── the_ashen_bastion.md
        ├── wardens_of_the_ember.md
        └── the_creeping_miasma.md
```

### 2.2 Backend DTOs & Service (`pkg/gui`)
- `SystemDetailDTO`:
  - `ID`: `string`
  - `Name`: `string`
  - `Version`: `string`
  - `Description`: `string`
  - `Script`: `string` (`mechanics.js`)
  - `RulesPrompt`: `string` (`prompts/rules.md`)
- `CreateSystemRequestDTO`:
  - Same fields as above, optional for creation/update.
- `WorldDetailDTO`:
  - `ID`: `string`
  - `Name`: `string`
  - `Description`: `string`
  - `Genre`: `string`
  - `DefaultSystem`: `string`
  - `ArtStyle`: `string`
  - `Tags`: `[]string`
  - `LorePrompt`: `string` (`prompts/lore.md`)
  - `Entities`: `[]WorldEntitySummaryDTO`
- `CreateWorldRequestDTO`:
  - Same fields as above, optional for creation/update.

### 2.3 Service Layer Implementation (`pkg/gui/service.go`)
- `GetSystem(ctx, id)`: Loads `system.yaml`, `mechanics.js`, and `prompts/rules.md` (if existing).
- `SaveSystem(ctx, req)`: Writes `system.yaml`, `mechanics.js`, and creates `prompts/rules.md`.
- `GetWorld(ctx, id)`: Loads `world.yaml`, `prompts/lore.md`, and lists entities.
- `SaveWorld(ctx, req)`: Writes `world.yaml`, creates `prompts/lore.md`, and saves metadata.

### 2.4 Agent Context Assembly (`pkg/harness/context.go` & `pkg/engine/orchestrator.go`)
`ContextAssembler.AssembleContext` receives `rulesPrompt` and `lorePrompt`. The generated prompt to the LLM agent is structured as:
```markdown
## SYSTEM RULES & RESOLUTION MECHANICS
<contents of prompts/rules.md>

## WORLD LORE & ATMOSPHERE
<contents of prompts/lore.md>

[MECHANICS RESULT: <result from action hook if applicable>]

## IMMEDIATE SCENE
**Current Location:** <Name>
<Location Body>
**Player Character:** <Name>
State: <State Map>

## LIVING WORLD & BACKGROUND ARCS
### Arc: <Name>
<Arc Body>

## PRESENT CHARACTERS & NOTABLE BEINGS
- **<Name>**: <Body>

## PLAYER ACTION
<actionInput>
```

---

## 3. Comprehensive Reference Templates

### 3.1 Reference System: `Narrative 2d6 Engine` (`narrative_2d6`)
- **Manifest (`system.yaml`)**:
  ```yaml
  id: narrative_2d6
  name: Narrative 2d6 Engine
  version: 1.0.0
  description: Versatile 2d6 resolution with partial success, dynamic stress, and event hooks.
  ```
- **Rules Prompt (`prompts/rules.md`)**:
  ```markdown
  You are the Game Master adjudicating a campaign governed by the **Narrative 2d6 Engine**.

  ### 1. Resolution Ladder
  When actions involve risk, uncertainty, or opposition, evaluate outcomes based on 2d6 rolls:
  - **10+ (Strong Hit / Full Success)**: The protagonist achieves their goal cleanly without complication, cost, or collateral harm.
  - **7–9 (Weak Hit / Mixed Success)**: The protagonist achieves their goal, but at a distinct cost, minor injury/stress, complication, or diminished effect.
  - **6- (Miss / Hard Move)**: The action fails or triggers immediate escalation, enemy counterattack, loss of a resource, or a worsening situation.

  ### 2. Action Modes & Mechanics Hooks
  The system engine executes JavaScript hooks before your turn response:
  - `do`: General active intent (physical tasks, maneuvering, infiltration). Invokes `onAction("do", ctx)`.
  - `attack`: Direct violent conflict. Invokes `onAction("attack", ctx)`. Calculates damage and tracks health.
  - `say`: Social interaction, negotiation, or deception.
  - `roll`: Direct arbitrary dice expressions.

  ### 3. Handling Mechanics Results
  When the turn prompt contains a `[MECHANICS RESULT: ...]` tag:
  - You **must** honor the mechanical outcome described in the message.
  - Weave damage dealt, health lost, or tactical setbacks directly into the narrative prose.
  - Never contradict the numerical roll total or state changes.
  ```
- **Mechanics Script (`mechanics.js`)**:
  ```javascript
  // Handle active "do" actions
  onAction("do", function(ctx) {
    var r = roll("2d6");
    var message = "";
    if (r.total >= 10) {
      message = "Strong Hit (Total: " + r.total + ") - Clean success without setback.";
    } else if (r.total >= 7) {
      message = "Weak Hit (Total: " + r.total + ") - Success, but with a complication or cost.";
    } else {
      message = "Miss (Total: " + r.total + ") - The action falters; danger looms.";
      injectGMDirection("The player's action failed. Introduce an immediate complication or escalate threat.");
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
      msg = "Glancing Hit (Total: " + r.total + ")! Target takes 2 damage, but counterattacks for 1 damage.";
      var hp = getStat(ctx.player, "health");
      if (hp === null || hp === undefined) hp = 10;
      setStat(ctx.player, "health", Math.max(0, hp - 1));
    } else {
      msg = "Attack Deflected (Total: " + r.total + ")! The enemy seizes the initiative.";
      injectGMDirection("The enemy retaliates swiftly. Put the player on the defensive.");
    }
    return {
      success: r.total >= 7,
      message: msg,
      roll: r
    };
  });

  // Turn lifecycle hook
  onTurnEnd(function(ctx) {
    log("Turn " + ctx.turn + " completed in Narrative 2d6 Engine.");
  });
  ```

### 3.2 Reference World: `The Ashen Reach` (`the_ashen_reach`)
- **Manifest (`world.yaml`)**:
  ```yaml
  id: the_ashen_reach
  name: The Ashen Reach
  description: A mist-veiled frontier of shattered cathedral keeps, lingering ember magic, and peat bogs.
  genre: Dark Fantasy / Gothic Exploration
  default_system: narrative_2d6
  art_style: Oil on textured canvas, chiaroscuro lighting, deep umber and lantern gold, atmospheric fog, classical dark fantasy illustration
  tags:
    - gothic
    - dark_fantasy
    - mystery
    - ruins
  ```
- **Lore Prompt (`prompts/lore.md`)**:
  ```markdown
  You are narrating an adventure in **The Ashen Reach**.

  ### Setting Atmosphere & Tone
  - **Sensory Cues**: The smell of wet peat, cold iron, and lingering incense. The damp chill clinging to cobblestones. Distant mournful bell chimes filtering through gray fog.
  - **The Sunken Bastions**: Centuries ago, the cathedrals and fortresses of the Silver Lantern sank into the marshes. Now only spires and arched hallways breach the gray mist.
  - **Factions in Conflict**:
    - *The Ember Wardens*: Weathered knights and lantern-bearers guarding survivors in fortified sanctuaries.
    - *The Hollowed*: Wanderers consumed by the mist, drawn toward any open flame.
  ```
- **Starter Entity Templates**:
  - `the_ashen_bastion.md`:
    ```markdown
    ---
    name: The Ashen Bastion
    type: location
    state:
      danger_level: 2
      lantern_lit: true
    wikilinks:
      - wardens_of_the_ember
    ---
    A fortified granite refuge perched upon sunken cathedral arches. Its great brass brazier burns night and day, warding off the creeping mist.
    ```
  - `wardens_of_the_ember.md`:
    ```markdown
    ---
    name: Wardens of the Ember
    type: character
    state:
      allegiance: friendly
      disposition: weary
    wikilinks:
      - the_ashen_bastion
    ---
    Veteran lantern-bearers sworn to defend the survivors of the Reach. Their armor is tarnished and their fuel supplies run low.
    ```
  - `the_creeping_miasma.md`:
    ```markdown
    ---
    name: The Creeping Miasma
    type: arc
    state:
      clock_ticks: 1
      clock_max: 6
    wikilinks:
      - the_ashen_bastion
    ---
    A supernatural fog that slowly engulfs the lower valleys as ancient braziers gutter and die.
    ```

---

## 4. UI & Studio Integration

### 4.1 Systems Studio (`SystemsStudio.tsx`)
- Tab 1: **Manifest Info** (Name, ID, Version, Description).
- Tab 2: **Agent Rules Prompt (`prompts/rules.md`)** (Markdown editor for rules instruction to the agent).
- Tab 3: **Mechanics Script (`mechanics.js`)** (JavaScript code editor).
- Actions:
  - `+ New System`: Pre-fills editor with *Narrative 2d6 Engine* reference template.
  - `Reset to Reference Template`: Restores the reference template at any time.
  - `Save System`: Writes manifest, mechanics script, and `prompts/rules.md`.

### 4.2 Worlds Studio (`WorldsStudio.tsx`)
- Tab 1: **Setting Lore & Atmosphere** (Name, ID, Genre, System dropdown, Art Style, Tags, Synopsis).
- Tab 2: **Agent Lore Prompt (`prompts/lore.md`)** (Markdown editor for GM setting guidelines).
- Tab 3: **Starter Entities (`entities/*.md`)** (Entity template list + Markdown editor).
- Actions:
  - `+ New World`: Pre-fills with *The Ashen Reach* reference template and 3 entity templates.
  - `Reset to Reference Template`: Restores the reference template at any time.
  - `Save World`: Writes manifest, lore prompt, and starter entities.

### 4.3 Launcher Hub Clean-Up (`LauncherHub.tsx`)
- Strip `'daggerheart'` and `'solitary_defiance'` hardcoded defaults.
- If `systems.length === 0 || worlds.length === 0`:
  - Show clear guidance in the New Campaign Wizard explaining that at least one Rule System and World Setting are needed.
  - Provide direct buttons `[Create Rule System]` and `[Create World Setting]` taking the user directly to the studio tabs pre-loaded with reference templates.

---

## 5. Verification & Testing

1. **Backend Tests**:
   - Update `TestSystemAndWorldStudioCRUD` in `pkg/gui/server_test.go` to test saving and retrieving `prompts/rules.md` and `prompts/lore.md`.
   - Update `TestContextAssembler` in `pkg/harness/context_test.go` to verify rules prompt and lore prompt injection.
2. **Frontend Tests**:
   - `mise run test:frontend` (`tsc --noEmit`) passes with 0 errors.
   - `mise run build:frontend` builds cleanly into `pkg/gui/dist`.
3. **End-to-End Verification**:
   - Start daemon over Unix socket with clean temporary directory (no systems/worlds).
   - Verify empty state in Launcher Hub.
   - Create system from reference template, world from reference template with entities.
   - Launch new campaign and verify turn generation includes rules prompt and lore prompt in context.
