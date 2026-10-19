import { describe, expect, it } from 'vitest';
import {
  DEFAULT_GEMINI_AGENT_MODELS,
  DEFAULT_KOKORO_MODEL_ID,
  DEFAULT_KOKORO_SIZE_BYTES,
  DEFAULT_STAGE_DIRECTIONS_DISPLAY_MODE,
  PACKAGE_ACCEPT,
  PACKAGE_EXTENSIONS,
  STORAGE_KEYS,
} from './constants';

describe('constants', () => {
  it('defines expected storage keys', () => {
    expect(STORAGE_KEYS.ACTIVE_GAME).toBe('localrpg_active_game');
    expect(STORAGE_KEYS.LAST_PLAYED_GAME).toBe('localrpg_last_played_game');
    expect(STORAGE_KEYS.SETTINGS_ADVANCED).toBe('localrpg.settings.advanced');
  });

  it('defines valid package extensions', () => {
    expect(PACKAGE_EXTENSIONS.SYSTEM).toBe('.lrpgsystem');
    expect(PACKAGE_EXTENSIONS.WORLD).toBe('.lrpgworld');
    expect(PACKAGE_EXTENSIONS.PACK).toBe('.lrpgpack');
  });

  it('defines valid package accept filter strings', () => {
    expect(PACKAGE_ACCEPT.SYSTEM).toBe('.lrpgsystem,.lrpgpack');
    expect(PACKAGE_ACCEPT.WORLD).toBe('.lrpgworld,.lrpgpack');
    expect(PACKAGE_ACCEPT.ALL).toBe('.lrpgworld,.lrpgsystem,.lrpgpack');
  });

  it('defines media defaults', () => {
    expect(DEFAULT_KOKORO_MODEL_ID).toBe('kokoro-tts');
    expect(DEFAULT_KOKORO_SIZE_BYTES).toBe(90_177_536);
  });

  it('defines default stage directions display mode', () => {
    expect(DEFAULT_STAGE_DIRECTIONS_DISPLAY_MODE).toBe('stage_directions');
  });

  it('defines non-empty fallback Gemini agent models', () => {
    expect(DEFAULT_GEMINI_AGENT_MODELS.length).toBeGreaterThan(0);
    expect(DEFAULT_GEMINI_AGENT_MODELS).toContain('gemini-2.5-flash');
  });
});
