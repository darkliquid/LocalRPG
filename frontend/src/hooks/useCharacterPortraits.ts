import { useCallback, useEffect, useState } from 'react';
import { APIClient } from '../api/client';
import { slugify } from '../lib/slug';
import { EntitySummary, Turn } from '../types';

/** Portrait is the image a character is shown with, and whether it is their own. */
export interface Portrait {
  url: string;
  hasCustom: boolean;
}

export type Portraits = Record<string, Portrait>;

export interface UseCharacterPortraitsOptions {
  chronicle: Turn[];
  entities: EntitySummary[];
  activeGameID: string | null;
}

// mergePortraits folds a freshly derived set into what is already shown. A custom
// portrait wins over a generated one and is never downgraded by a later generated
// one, which is the rule both sources depend on; a set that changes nothing returns
// the same object, so a poll that finds nothing new does not re-render.
function mergePortraits(prev: Portraits, incoming: Portraits): Portraits {
  let next: Portraits | null = null;
  for (const [id, portrait] of Object.entries(incoming)) {
    const current = prev[id];
    if (!current || (!current.hasCustom && portrait.hasCustom)) {
      next = { ...(next ?? prev) };
      next[id] = portrait;
    }
  }
  return next ?? prev;
}

// useCharacterPortraits derives every character's portrait from the two places one
// can arrive: the segments of a turn, which carry the portrait the server resolved
// while narrating, and the entity list, which carries one once a character has their
// own. The app derived this twice before, with the same merge written out twice.
export interface CharacterPortraits {
  portraits: Portraits;
  /** notePortrait is the mid-turn case: the stream reports one as it is generated. */
  notePortrait: (characterId: string, portrait: Portrait) => void;
}

export function useCharacterPortraits({
  chronicle,
  entities,
  activeGameID,
}: UseCharacterPortraitsOptions): CharacterPortraits {
  const [portraits, setPortraits] = useState<Portraits>({});

  // A different campaign has its own characters, so nothing carries over. This
  // effect is declared first so it clears before the two below refill it.
  useEffect(() => {
    setPortraits({});
  }, [activeGameID]);

  const notePortrait = useCallback((characterId: string, portrait: Portrait) => {
    setPortraits((prev) => mergePortraits(prev, { [characterId]: portrait }));
  }, []);

  // A turn's segments are where a generated portrait first appears.
  useEffect(() => {
    if (chronicle.length === 0) return;

    const fromChronicle: Portraits = {};
    for (const turn of chronicle) {
      for (const segment of turn.segments || []) {
        const characterId = segment.speaker_id || (segment.speaker ? slugify(segment.speaker) : undefined);
        if (!characterId || !segment.portrait_url) continue;
        // A custom portrait is fetched with a timestamp so a regenerated one is
        // not served from the browser's cache; a generated one is already unique.
        const needsBust = segment.has_custom_portrait && !segment.portrait_url.includes('?') ? `?t=${Date.now()}` : '';
        fromChronicle[characterId] = {
          url: `${segment.portrait_url}${needsBust}`,
          hasCustom: !!segment.has_custom_portrait,
        };
      }
    }
    setPortraits((prev) => mergePortraits(prev, fromChronicle));
  }, [chronicle]);

  // The entity list is where a character's own portrait appears, whether it was
  // uploaded or generated, so it is always the custom one.
  useEffect(() => {
    if (entities.length === 0) return;

    const fromEntities: Portraits = {};
    for (const entity of entities) {
      if (entity.type !== 'character' || !entity.has_portrait) continue;
      const base =
        entity.portrait_url ||
        APIClient.characterPortraitURL(activeGameID || '', entity.id);
      fromEntities[entity.id] = { url: `${base}?t=${Date.now()}`, hasCustom: true };
    }
    if (Object.keys(fromEntities).length > 0) {
      setPortraits((prev) => mergePortraits(prev, fromEntities));
    }
  }, [entities, activeGameID]);

  return { portraits, notePortrait };
}
