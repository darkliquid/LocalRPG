import type { StageDirectionsDisplayMode } from '../types';

// Storage keys used across local session and settings persistence.
export const STORAGE_KEYS = {
  ACTIVE_GAME: 'localrpg_active_game',
  LAST_PLAYED_GAME: 'localrpg_last_played_game',
  SETTINGS_ADVANCED: 'localrpg.settings.advanced',
} as const;

// Package file extensions and file-picker filter strings.
export const PACKAGE_EXTENSIONS = {
  SYSTEM: '.lrpgsystem',
  WORLD: '.lrpgworld',
  PACK: '.lrpgpack',
} as const;

export const PACKAGE_ACCEPT = {
  SYSTEM: '.lrpgsystem,.lrpgpack',
  WORLD: '.lrpgworld,.lrpgpack',
  ALL: '.lrpgworld,.lrpgsystem,.lrpgpack',
} as const;

// Default media model configurations.
export const DEFAULT_KOKORO_MODEL_ID = 'kokoro-tts';
export const DEFAULT_KOKORO_SIZE_BYTES = 90_177_536;

// Offline fallback model suggestions for Gemini agents.
export const DEFAULT_GEMINI_AGENT_MODELS: readonly string[] = [
  'gemini-3.8-flash',
  'gemini-3.7-flash',
  'gemini-3.5-flash',
  'gemini-3.5-flash-lite',
  'gemini-3.1-flash-lite',
  'gemini-3.1-pro-preview',
  'gemini-3-flash-preview',
  'gemini-2.5-flash',
  'gemini-2.5-pro',
  'gemini-2.5-flash-lite',
  'gemma-4-31b-it',
  'gemma-4-26b-it',
  'gemma-4-12b-it',
  'gemma-3-27b-it',
];

export const DEFAULT_STAGE_DIRECTIONS_DISPLAY_MODE: StageDirectionsDisplayMode = 'stage_directions';
