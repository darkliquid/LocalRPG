import type { APIClient } from '../../api/client';
import type { EntitySummary } from '../../types';

interface CachedIndex {
  gameID: string;
  entities: EntitySummary[];
  loadedAt: number;
}

let cache: CachedIndex | null = null;

// The index is refreshed on a timer rather than per keystroke: a campaign holds
// tens to low hundreds of notes, so one fetch filters locally and completion stays
// instant and offline.
const maxAgeMs = 30_000;

export async function loadEntityIndex(
  client: APIClient,
  gameID: string,
  force = false,
): Promise<EntitySummary[]> {
  const fresh = cache !== null && cache.gameID === gameID && Date.now() - cache.loadedAt < maxAgeMs;
  if (fresh && !force && cache) return cache.entities;

  try {
    const entities = await client.listEntities();
    cache = { gameID, entities, loadedAt: Date.now() };
    return entities;
  } catch {
    // A failed refresh keeps the last good list rather than emptying completion.
    return cache?.entities ?? [];
  }
}

// invalidateEntityIndex drops the cache after a write, so a note saved in this
// session is immediately linkable.
export function invalidateEntityIndex(): void {
  cache = null;
}
