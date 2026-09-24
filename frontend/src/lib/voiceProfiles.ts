import { VoiceProfile } from '../types';

// KOKORO_VOICE_PROFILES are the eleven Kokoro voices as archetypes. They mirror
// the profiles the catalogue's Kokoro presets carry, and seed the "Load Kokoro
// Voices" button.
export const KOKORO_VOICE_PROFILES: VoiceProfile[] = [
  { id: 'af', name: 'Default (American Female)', voice_id: 'af', pitch: 1.0, speech_rate: 1.0, tags: ['american', 'female', 'default', 'neutral'], description: "The model's stock American female voice." },
  { id: 'af_bella', name: 'Bella (American Female)', voice_id: 'af_bella', pitch: 1.0, speech_rate: 1.0, tags: ['american', 'female', 'bella', 'warm', 'friendly'], description: 'American female voice, warm, approachable, and pleasant.' },
  { id: 'af_nicole', name: 'Nicole (American Female)', voice_id: 'af_nicole', pitch: 1.0, speech_rate: 1.0, tags: ['american', 'female', 'nicole', 'youthful', 'energetic'], description: 'American female voice, brisk, youthful, and direct.' },
  { id: 'af_sarah', name: 'Sarah (American Female)', voice_id: 'af_sarah', pitch: 1.0, speech_rate: 1.0, tags: ['american', 'female', 'sarah', 'poised', 'narrative'], description: 'American female voice, polished, measured, and story-oriented.' },
  { id: 'af_sky', name: 'Sky (American Female)', voice_id: 'af_sky', pitch: 1.0, speech_rate: 1.0, tags: ['american', 'female', 'sky', 'light', 'airy'], description: 'American female voice, light, gentle, and breathy.' },
  { id: 'am_adam', name: 'Adam (American Male)', voice_id: 'am_adam', pitch: 1.0, speech_rate: 1.0, tags: ['american', 'male', 'adam', 'deep', 'authoritative'], description: 'American male voice, deep, steady, and commanding.' },
  { id: 'am_michael', name: 'Michael (American Male)', voice_id: 'am_michael', pitch: 1.0, speech_rate: 1.0, tags: ['american', 'male', 'michael', 'commanding', 'formal'], description: 'American male voice, disciplined, authoritative, and formal.' },
  { id: 'bf_emma', name: 'Emma (British Female)', voice_id: 'bf_emma', pitch: 1.0, speech_rate: 1.0, tags: ['british', 'female', 'emma', 'gentle', 'poised'], description: 'British female voice, elegant, gentle, and softly spoken.' },
  { id: 'bf_isabella', name: 'Isabella (British Female)', voice_id: 'bf_isabella', pitch: 1.0, speech_rate: 1.0, tags: ['british', 'female', 'isabella', 'noble', 'melodic'], description: 'British female voice, aristocratic, melodic, and graceful.' },
  { id: 'bm_george', name: 'George (British Male)', voice_id: 'bm_george', pitch: 1.0, speech_rate: 1.0, tags: ['british', 'male', 'george', 'mature', 'distinguished'], description: 'British male voice, mature, distinguished, and resonant.' },
  { id: 'bm_lewis', name: 'Lewis (British Male)', voice_id: 'bm_lewis', pitch: 1.0, speech_rate: 1.0, tags: ['british', 'male', 'lewis', 'thoughtful', 'refined'], description: 'British male voice, measured, polite, and reflective.' },
];

// DEFAULT_VOICE_PROFILES are the stock fantasy archetypes the "Load Fantasy
// Defaults" button restores.
export const DEFAULT_VOICE_PROFILES: VoiceProfile[] = [
  {
    id: 'elder_sage',
    name: 'Elder Sage / Veteran',
    voice_id: 'bm_george',
    pitch: 0.85,
    speech_rate: 0.9,
    tags: ['elder', 'male', 'wise', 'gravelly', 'veteran'],
    description: 'Ancient wizards, battle-weary commanders, village elders.',
  },
  {
    id: 'young_scout',
    name: 'Young Scout / Rogue',
    voice_id: 'af_bella',
    pitch: 1.05,
    speech_rate: 1.1,
    tags: ['young', 'female', 'quick', 'eager', 'rogue'],
    description: 'Nimble rangers, streetwise thieves, eager apprentices.',
  },
  {
    id: 'gruff_blacksmith',
    name: 'Gruff Dwarf / Guard',
    voice_id: 'am_adam',
    pitch: 0.75,
    speech_rate: 0.95,
    tags: ['stout', 'male', 'deep', 'authoritative', 'guard'],
    description: 'Dwarven smiths, tavern bouncers, fortress wardens.',
  },
  {
    id: 'sinister_cultist',
    name: 'Hushed Mystic / Villain',
    voice_id: 'bf_emma',
    pitch: 0.9,
    speech_rate: 0.85,
    tags: ['eerie', 'whisper', 'sinister', 'cultist'],
    description: 'Shadow mages, deceptive nobles, oracle priestesses.',
  },
];
