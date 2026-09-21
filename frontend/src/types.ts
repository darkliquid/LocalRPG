export interface PlayerState {
  id: string;
  name: string;
  type: string;
  state: Record<string, any>;
}

export interface GameState {
  game_id: string;
  game_name: string;
  player: PlayerState;
  arcs: Array<{ id: string; name: string; progress: number; max_progress: number; status: string }>;
  clocks: Array<{ faction: string; name: string; ticks: number; max_ticks: number }>;
  locations: string[];
}

export interface TurnSegment {
  kind: 'narration' | 'speech';
  speaker?: string;
  speaker_id?: string;
  text: string;
  audio_url?: string;
  // Seconds the backend estimates this line takes to read, which is the same
  // estimate the exports pace with.
  duration?: number;
}

export interface Turn {
  turn_number: number;
  input_text: string;
  mode: string;
  prose: string;
  segments?: TurnSegment[];
  image_url?: string;
  entities_hit?: string[];
  location_id?: string;
  location_name?: string;
  location_art_url?: string;
  outcome?: string;
}

export interface TurnEvent {
  type: 'chunk' | 'turn' | 'error';
  text?: string;
  turn?: Turn;
  message?: string;
}

export interface EntityNote {
  id: string;
  name: string;
  type: string;
  markdown: string;
  state: Record<string, any>;
  backlinks: string[];
  history?: number[];
}

export interface GraphNode {
  id: string;
  label: string;
  type: string;
}

export interface GraphLink {
  source: string;
  target: string;
}

export interface GraphData {
  nodes: GraphNode[];
  links: GraphLink[];
}

export interface GameSummary {
  id: string;
  name: string;
  system_id: string;
  world_id: string;
  player_name: string;
  turn_count: number;
  last_played: string;
  thumbnail_url?: string;
}

export interface SystemInfo {
  id: string;
  name: string;
  description: string;
  version: string;
}

export interface WorldInfo {
  id: string;
  name: string;
  description: string;
  genre: string;
  compatible_systems: string[];
}

export interface CreateGameRequest {
  id?: string;
  name: string;
  system_id: string;
  world_id: string;
  player_name: string;
}

export interface SystemDetail {
  id: string;
  name: string;
  version: string;
  description: string;
  script: string;
  rules_prompt?: string;
}

export interface CreateSystemRequest {
  id?: string;
  name: string;
  version?: string;
  description?: string;
  script?: string;
  rules_prompt?: string;
}

export interface WorldEntitySummary {
  id: string;
  name: string;
  type: string;
}

export interface WorldDetail {
  id: string;
  name: string;
  description: string;
  genre: string;
  default_system: string;
  art_style: string;
  tags: string[];
  lore_prompt?: string;
  entities: WorldEntitySummary[];
}

export interface CreateWorldRequest {
  id?: string;
  name: string;
  description?: string;
  genre?: string;
  default_system?: string;
  art_style?: string;
  tags?: string[];
  lore_prompt?: string;
}

export interface WorldEntityDetail {
  id: string;
  markdown: string;
}

export interface PathsConfig {
  systems: string;
  worlds: string;
  games: string;
  cache: string;
}

export interface AgentRoleConfig {
  type: 'builtin' | 'http' | 'cli' | 'inherit' | 'disabled';
  inherit_from?: string;
  builtin_name?: string;
  command?: string;
  args?: string[];
  endpoint?: string;
  model?: string;
  api_key?: string;
  temperature?: number;
  max_tokens?: number;
}

export interface AgentsConfig {
  default_role: string;
  roles: Record<string, AgentRoleConfig>;
  fallbacks?: Record<string, string>;
}

export interface VoiceProfile {
  id: string;
  name: string;
  voice_id: string;
  pitch: number;
  speech_rate: number;
  tags?: string[];
  description?: string;
}

export interface TTSConfig {
  type: 'builtin' | 'http' | 'cli' | 'disabled';
  builtin_name?: string;
  command?: string;
  args?: string[];
  endpoint?: string;
  model?: string;
  api_key?: string;
  default_voice?: string;
  pitch?: number;
  speech_rate?: number;
  auto_play: boolean;
  master_volume: number;
  voice_profiles?: VoiceProfile[];
}

export interface STTConfig {
  type: 'builtin' | 'http' | 'cli' | 'disabled';
  builtin_name?: string;
  command?: string;
  args?: string[];
  endpoint?: string;
  model?: string;
  api_key?: string;
}

export interface ImageConfig {
  type: 'builtin' | 'http' | 'cli' | 'disabled';
  builtin_name?: string;
  command?: string;
  args?: string[];
  endpoint?: string;
  model?: string;
  api_key?: string;
  auto_generate: boolean;
}

export interface MediaConfig {
  tts: TTSConfig;
  stt: STTConfig;
  image: ImageConfig;
}

export interface PreferencesConfig {
  streaming: boolean;
  typing_speed_ms: number;
  cinematic_effects: boolean;
  font_scale: 'small' | 'medium' | 'large';
}

export interface AppConfig {
  version: string;
  paths: PathsConfig;
  agents: AgentsConfig;
  media: MediaConfig;
  preferences: PreferencesConfig;
}

export interface SettingsResponse {
  config: AppConfig;
  config_file_path: string;
  is_local_override: boolean;
}

export interface TestProviderRequest {
  category: 'llm' | 'tts' | 'stt' | 'image';
  provider: AgentRoleConfig | TTSConfig | STTConfig | ImageConfig;
  test_prompt?: string;
}

export interface TestProviderResponse {
  success: boolean;
  latency_ms: number;
  message: string;
  preview?: string;
}


