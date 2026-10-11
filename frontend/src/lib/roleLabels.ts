// ROLE_LABELS names the roles the engine knows by default. A role that is not
// listed falls back to its own identifier, so a custom role still renders.
export const ROLE_LABELS: Record<string, string> = {
  gm: 'Game Master (GM / Storyteller)',
  narrator: 'Atmospheric Narrator',
  extractor: 'Entity Extractor (per-turn world state)',
};
