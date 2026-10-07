export interface ReferenceEntityTemplate {
  id: string;
  name: string;
  type: string;
  markdown: string;
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
