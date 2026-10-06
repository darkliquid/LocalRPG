import { describe, expect, it } from 'vitest';
import { isCharacterType } from './entityTypes';

describe('isCharacterType', () => {
  it('accepts the speaking types', () => {
    for (const type of ['character', 'Character', 'npc', 'NPC', 'person', ' creature ']) {
      expect(isCharacterType(type)).toBe(true);
    }
  });

  it('rejects the rest', () => {
    for (const type of ['location', 'item', 'faction', 'arc', '', undefined]) {
      expect(isCharacterType(type)).toBe(false);
    }
  });
});
