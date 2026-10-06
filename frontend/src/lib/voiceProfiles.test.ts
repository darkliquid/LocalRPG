import { describe, expect, it } from 'vitest';
import { DEFAULT_VOICE_PROFILES, KOKORO_VOICE_PROFILES } from './voiceProfiles';

const everyProfile = [...KOKORO_VOICE_PROFILES, ...DEFAULT_VOICE_PROFILES];

describe('the voice profile catalogues', () => {
  it('carries the eleven Kokoro voices', () => {
    expect(KOKORO_VOICE_PROFILES).toHaveLength(11);
  });

  it('gives every profile a unique id', () => {
    const ids = everyProfile.map((profile) => profile.id);
    expect(new Set(ids).size).toBe(ids.length);
  });

  it('fills in the fields a selection needs', () => {
    for (const profile of everyProfile) {
      expect(profile.name.length).toBeGreaterThan(0);
      expect(profile.voice_id.length).toBeGreaterThan(0);
      expect(profile.pitch).toBeGreaterThan(0);
      expect(profile.speech_rate).toBeGreaterThan(0);
    }
  });

  it('tags every profile so a lookup can match one', () => {
    for (const profile of everyProfile) {
      expect(profile.tags?.length ?? 0).toBeGreaterThan(0);
    }
  });
});
