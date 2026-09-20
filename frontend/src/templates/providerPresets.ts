import { AgentRoleConfig, TTSConfig, STTConfig, ImageConfig, VoiceProfile } from '../types';

export interface PresetItem<T> {
  label: string;
  description: string;
  config: T;
}

export const AGENT_PRESETS: Record<string, PresetItem<AgentRoleConfig>> = {
  ollama: {
    label: 'Ollama (Local HTTP)',
    description: 'Connects to local Ollama server running on port 11434 with llama3.2.',
    config: {
      type: 'http',
      endpoint: 'http://localhost:11434/v1',
      model: 'llama3.2',
      temperature: 0.7,
      max_tokens: 1024,
    },
  },
  'lm-studio': {
    label: 'LM Studio (Local HTTP)',
    description: 'Connects to LM Studio local server on port 1234.',
    config: {
      type: 'http',
      endpoint: 'http://localhost:1234/v1',
      model: 'default',
      temperature: 0.7,
      max_tokens: 1024,
    },
  },
  localai: {
    label: 'LocalAI (Local HTTP)',
    description: 'Connects to LocalAI server on port 8080.',
    config: {
      type: 'http',
      endpoint: 'http://localhost:8080/v1',
      model: 'gpt-4',
      temperature: 0.7,
      max_tokens: 1024,
    },
  },
  vllm: {
    label: 'vLLM (Local HTTP)',
    description: 'Connects to high-throughput vLLM instance on port 8000.',
    config: {
      type: 'http',
      endpoint: 'http://localhost:8000/v1',
      model: 'default',
      temperature: 0.7,
      max_tokens: 1024,
    },
  },
  'llama-cli': {
    label: 'llama-cli (Local Executable)',
    description: 'Direct llama.cpp command execution without a background server.',
    config: {
      type: 'cli',
      command: 'llama-cli',
      args: ['-m', 'models/model.gguf', '-p'],
      temperature: 0.7,
      max_tokens: 1024,
    },
  },
  'claude-cli': {
    label: 'Claude Code CLI',
    description: 'Executes Anthropic Claude CLI directly from command line.',
    config: {
      type: 'cli',
      command: 'claude',
      args: ['-p'],
    },
  },
  'narrative-oracle': {
    label: 'Narrative Oracle (Built-in Zero-GPU)',
    description: 'Deterministic pure-Go procedural storyteller with rule-based narrative outcomes.',
    config: {
      type: 'builtin',
      builtin_name: 'narrative-oracle',
    },
  },
};

export const TTS_PRESETS: Record<string, PresetItem<TTSConfig>> = {
  'kokoro-fastapi': {
    label: 'Kokoro-FastAPI (Local HTTP)',
    description: 'High quality 82M open-weights TTS running via local FastAPI server on port 8880.',
    config: {
      type: 'http',
      endpoint: 'http://localhost:8880/v1/audio/speech',
      model: 'kokoro',
      default_voice: 'af_bella',
      pitch: 1.0,
      speech_rate: 1.0,
      auto_play: true,
      master_volume: 1.0,
    },
  },
  alltalk: {
    label: 'AllTalk TTS (Local HTTP)',
    description: 'Coqui XTTSv2 / AllTalk web UI API running on port 7851.',
    config: {
      type: 'http',
      endpoint: 'http://localhost:7851/api/tts-generate',
      default_voice: 'default',
      pitch: 1.0,
      speech_rate: 1.0,
      auto_play: true,
      master_volume: 1.0,
    },
  },
  piper: {
    label: 'Piper TTS (Local CLI)',
    description: 'Fast, lightweight neural TTS running directly via the piper binary.',
    config: {
      type: 'cli',
      command: 'piper',
      args: ['--model', 'en_US-lessac-medium.onnx', '--output_file', '-'],
      pitch: 1.0,
      speech_rate: 1.0,
      auto_play: true,
      master_volume: 1.0,
    },
  },
  'native-os': {
    label: 'Native OS Speech (Built-in Zero-GPU)',
    description: 'Uses spd-say (Linux), say (macOS), or PowerShell (Windows) with procedural audio fallback.',
    config: {
      type: 'builtin',
      builtin_name: 'native-os',
      pitch: 1.0,
      speech_rate: 1.0,
      auto_play: true,
      master_volume: 1.0,
    },
  },
  'openai-speech': {
    label: 'OpenAI Audio Speech (Cloud API)',
    description: 'Cloud synthesis with OpenAI tts-1 model.',
    config: {
      type: 'http',
      endpoint: 'https://api.openai.com/v1/audio/speech',
      model: 'tts-1',
      default_voice: 'alloy',
      pitch: 1.0,
      speech_rate: 1.0,
      auto_play: true,
      master_volume: 1.0,
    },
  },
};

export const STT_PRESETS: Record<string, PresetItem<STTConfig>> = {
  'faster-whisper': {
    label: 'Faster-Whisper (Local HTTP)',
    description: 'Local OpenAI-compatible transcription server running on port 8000.',
    config: {
      type: 'http',
      endpoint: 'http://localhost:8000/v1/audio/transcriptions',
      model: 'whisper-1',
    },
  },
  'whisper-cli': {
    label: 'Whisper.cpp (Local CLI)',
    description: 'Whisper.cpp command-line tool with GGML model.',
    config: {
      type: 'cli',
      command: 'whisper-cli',
      args: ['-m', 'models/ggml-base.bin', '-f', '%INPUT%', '-nt'],
    },
  },
  'openai-whisper': {
    label: 'OpenAI Whisper (Cloud API)',
    description: 'Cloud transcription via OpenAI Whisper API.',
    config: {
      type: 'http',
      endpoint: 'https://api.openai.com/v1/audio/transcriptions',
      model: 'whisper-1',
    },
  },
};

export const IMAGE_PRESETS: Record<string, PresetItem<ImageConfig>> = {
  comfyui: {
    label: 'ComfyUI (Local HTTP)',
    description: 'Connects to local ComfyUI graph execution server on port 8188.',
    config: {
      type: 'http',
      endpoint: 'http://127.0.0.1:8188',
      auto_generate: false,
    },
  },
  automatic1111: {
    label: 'Stable Diffusion WebUI / A1111 (Local HTTP)',
    description: 'Connects to AUTOMATIC1111 txt2img API on port 7860.',
    config: {
      type: 'http',
      endpoint: 'http://127.0.0.1:7860/sdapi/v1/txt2img',
      auto_generate: false,
    },
  },
  'localai-image': {
    label: 'LocalAI Image (Local HTTP)',
    description: 'LocalAI image generation endpoint on port 8080.',
    config: {
      type: 'http',
      endpoint: 'http://127.0.0.1:8080/v1/images/generations',
      model: 'stablediffusion',
      auto_generate: false,
    },
  },
  'sd-cli': {
    label: 'stable-diffusion.cpp (Local CLI)',
    description: 'Direct SD inference binary using quantized GGUF weights.',
    config: {
      type: 'cli',
      command: 'sd',
      args: ['-m', 'models/sd-v1-5.gguf', '-p'],
      auto_generate: false,
    },
  },
  'procedural-art': {
    label: 'Procedural Dark Fantasy (Built-in Zero-GPU)',
    description: 'Pure-Go vector landscape and fortress generator creating atmospheric SVG illustrations.',
    config: {
      type: 'builtin',
      builtin_name: 'procedural-art',
      auto_generate: false,
    },
  },
  'dall-e-3': {
    label: 'OpenAI DALL-E 3 (Cloud API)',
    description: 'Cloud generation using OpenAI DALL-E 3 endpoint.',
    config: {
      type: 'http',
      endpoint: 'https://api.openai.com/v1/images/generations',
      model: 'dall-e-3',
      auto_generate: false,
    },
  },
};

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
