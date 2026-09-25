export interface PlayerState {
  id: string;
  name: string;
  type: string;
  state: Record<string, any>;
  appearance?: string;
  voice?: VoiceProfile;
}

export interface GameState {
  game_id: string;
  game_name: string;
  player: PlayerState;
  arcs: Array<{ id: string; name: string; progress: number; max_progress: number; status: string }>;
  clocks: Array<{ faction: string; name: string; ticks: number; max_ticks: number }>;
  locations: string[];
  // The player's own instruction for the campaign's opening scene, when set.
  opening_prompt?: string;
  narrator_voice?: string;
  start_location?: string;
  banner_url?: string;
}

export interface TurnSegment {
  kind: 'narration' | 'speech';
  speaker?: string;
  speaker_id?: string;
  text: string;
  audio_url?: string;
  // Version token for the clip, which changes when the speaker's voice changes.
  audio_key?: string;
  // True for the protagonist's own line, which renders as speech but suppresses
  // the duplicate action block for the turn.
  player?: boolean;
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
  // Set when the model hit its token limit mid-reply.
  truncated?: boolean;
  // How a reply that stopped mid-thought was repaired: 'continued', 'trimmed',
  // or 'kept'. Absent when nothing was wrong.
  recovery?: string;
  // Anything the prompt budget left out, so a thinner reply can be explained.
  context_notes?: string[];
  continuity_notes?: string[];
  // What the turn looked up: the name and result size of each tool call.
  tool_calls?: ToolCall[];
  // The GM's verdict on the player's action, whether it was rejected as
  // impossible, and the checks it resolved.
  verdict?: { feasibility: 'automatic' | 'uncertain' | 'impossible'; reason?: string };
  rejected?: boolean;
  checks?: TurnCheck[];
}

export interface TurnCheck {
  check_id: string;
  outcome: string;
  roll?: { notation: string; total: number };
}

export interface EntityMemory {
  turn: number;
  kind: string;
  text: string;
  importance: number;
  tags?: string[];
}

export interface ToolCall {
  name: string;
  result_chars: number;
}

export interface TurnEvent {
  type: 'chunk' | 'turn' | 'tool' | 'error' | 'model_missing';
  text?: string;
  turn?: Turn;
  message?: string;
  model_id?: string;
  name?: string;
  size_bytes?: number;
  tool_name?: string;
  tool_status?: 'running' | 'done';
  tool_summary?: string;
  code?: string;
  detail?: string;
  failure?: GenerationFailure;
}

export interface ModelStatus {
  id: string;
  name: string;
  installed: boolean;
  downloading: boolean;
  progress: number;
  bytes_downloaded: number;
  total_bytes: number;
  error?: string;
}

export interface Thread {
  id: string;
  name: string;
  status: string;
  last_advanced: number;
  idle: number;
}

export interface Recap {
  summary?: string;
  through_turn: number;
  enabled: boolean;
  threads?: Thread[];
}

export interface EntitySummary {
  id: string;
  name: string;
  type: string;
  location?: string;
  tags?: string[];
  parse_error?: boolean;
}

export interface EntityNote {
  id: string;
  name: string;
  type: string;
  markdown: string;
  state: Record<string, any>;
  backlinks: string[];
  history?: number[];
  parse_error?: boolean;
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
  banner_url?: string;
  icon_url?: string;
  play_time_seconds?: number;
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
  art_style?: string;
  tags?: string[];
  compatible_systems: string[];
  banner_url?: string;
  icon_url?: string;
}

export interface CreateGameRequest {
  id?: string;
  name: string;
  system_id: string;
  world_id: string;
  player_name: string;
  player?: PlayerCharacter;
  opening_prompt?: string;
  narrator_voice?: string;
  start_location?: string;
}

export interface GameSettingsPatch {
  opening_prompt?: string;
  narrator_voice?: string;
  start_location?: string;
}

export interface CharacterCreationField {
  id: string;
  label: string;
  prompt?: string;
  kind?: 'text' | 'long' | 'number' | 'select' | 'voice';
  required?: boolean;
  generatable?: boolean;
  options?: string[];
  default?: string;
}

export interface CharacterCreationSpec {
  preamble?: string;
  fields?: CharacterCreationField[];
}

export interface PlayerCharacter {
  appearance?: string;
  age?: string;
  gender?: string;
  pronouns?: string;
  background?: string;
  voice?: VoiceProfile;
  extra?: Record<string, string>;
}

export type GenerationFailureCode =
  | 'provider_unavailable' | 'provider_error' | 'empty_response'
  | 'parse_error' | 'timeout' | 'context_too_large' | 'invalid_request';

export interface GenerationAttempt {
  role: string;
  provider: string;
  code: GenerationFailureCode;
  detail?: string;
  duration_ms: number;
}

export interface GenerationFailure {
  code: GenerationFailureCode;
  message: string;
  attempts?: GenerationAttempt[];
  finish_reason?: string;
  prompt_chars?: number;
  context_chars?: number;
  elapsed_ms?: number;
}

export interface GenerateCharacterRequest {
  system_id?: string;
  world_id?: string;
  name?: string;
  fields: CharacterCreationField[];
  seed?: Record<string, string>;
}

export interface GenerateCharacterResponse {
  values: Record<string, string>;
  generated_by: string;
  warning?: GenerationFailure;
}

export interface GenerateTextRequest {
  form_type: 'character' | 'world' | 'system' | 'campaign';
  field_name: string;
  context: Record<string, string>;
  world_id?: string;
  system_id?: string;
  seed?: string;
}

export interface GenerateTextResponse {
  fields: Record<string, string>;
  generated_by: string;
  warning?: GenerationFailure;
}

export interface SystemDetail {
  id: string;
  name: string;
  version: string;
  description: string;
  script: string;
  rules_prompt?: string;
  character_creation?: CharacterCreationSpec;
}

export interface CreateSystemRequest {
  id?: string;
  name: string;
  version?: string;
  description?: string;
  script?: string;
  rules_prompt?: string;
  character_creation?: CharacterCreationSpec;
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

// WorldSelection models what the Worlds Studio editor is showing: a saved
// world, a local unsaved draft, or nothing.
export type WorldSelection =
  | { kind: 'saved'; id: string }
  | { kind: 'draft' }
  | null;

export interface WorldDraft {
  localId: string;
  dirty: boolean;
}

export interface PathsConfig {
  systems: string;
  worlds: string;
  games: string;
  cache: string;
}

export interface AgentRoleConfig {
  type: 'builtin' | 'http' | 'cli' | 'inherit' | 'disabled' | 'gemini';
  inherit_from?: string;
  builtin_name?: string;
  command?: string;
  args?: string[];
  endpoint?: string;
  model?: string;
  api_key?: string;
  temperature?: number;
  max_tokens?: number;
  // "auto" (HTTP providers only), "yes", or "no".
  supports_tools?: 'auto' | 'yes' | 'no';
  thinking_budget?: number;
  top_p?: number;
  top_k?: number;
}

export interface AgentsConfig {
  default_role: string;
  roles: Record<string, AgentRoleConfig>;
  fallbacks?: Record<string, string>;
  // A turn's wall clock and the silence tolerated between narration deltas.
  turn_timeout_seconds?: number;
  chunk_timeout_seconds?: number;
  // The assembled prompt's estimated token ceiling. 0 means unbounded.
  context_token_budget?: number;
  // Tool rounds per turn, and the cap on one tool result.
  tool_rounds?: number;
  tool_result_chars?: number;
  // How far back the narrator is reminded, and how much of each turn.
  recent_turn_window?: number;
  recent_turn_char_limit?: number;
  // Recall bounds.
  scene_recall_turns?: number;
  scene_recall_chars?: number;
  retrieval_turns?: number;
  retrieval_chars?: number;
  retrieval_halflife_turns?: number;
  // Tracing is opt-in, so these bound a debug session rather than normal play.
  trace_payload_chars?: number;
  trace_max_bytes?: number;
  trace_max_files?: number;
  trace_rotate_check?: number;
  trace_chunk_limit?: number;
  thread_idle_turns?: number;
  threads_max?: number;
  continuity_checks?: boolean;
}

export interface VoiceProfile {
  id: string;
  name: string;
  voice_id: string;
  provider?: string;
  pitch: number;
  speech_rate: number;
  tags?: string[];
  description?: string;
  // Provider-declared tunables keyed by VoiceOption.key. Omitted means the
  // provider's own defaults.
  options?: Record<string, unknown>;
}

export interface VoiceOption {
  key: string;
  label: string;
  kind: 'float' | 'int' | 'bool' | 'string' | 'enum';
  min?: number;
  max?: number;
  step?: number;
  options?: string[];
  default?: unknown;
  help?: string;
}

export interface ProviderVoice {
  id: string;
  name: string;
  language?: string;
  gender?: string;
  accent?: string;
  categories?: string[];
  tags?: string[];
  description?: string;
  preview_url?: string;
  defaults?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
}

export interface VoiceCatalog {
  available: boolean;
  fetched_at?: string;
  stale: boolean;
  voices: ProviderVoice[];
}

export interface SpeechCueCapabilities {
  audio_tags: boolean;
  markdown_emphasis: boolean;
  supported_tags?: string[];
  prompt_guidance?: string;
}

export interface SpeechCuesConfig {
  enabled: boolean;
  audio_tags?: boolean;
  markdown_emphasis?: boolean;
  display_mode?: 'stage_directions' | 'hidden' | 'raw';
}

export interface TTSInspectRequest {
  config: TTSConfig;
  refresh?: boolean;
}

export interface TTSInspectResponse {
  provider_key: string;
  metered: boolean;
  options?: VoiceOption[];
  catalog: VoiceCatalog;
  key_present: boolean;
  key_required: boolean;
  error?: string;
  speech_cues?: SpeechCueCapabilities;
}

export interface TTSConfig {
  type: 'builtin' | 'http' | 'cli' | 'disabled' | 'gemini';
  builtin_name?: string;
  model_path?: string;
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
  // How narration Markdown is treated before synthesis. Omitted means auto.
  markdown?: 'auto' | 'strip' | 'keep';
  // Provider-declared tunables for the default voice.
  options?: Record<string, unknown>;
  // Overrides a provider's own metered declaration when set.
  metered?: boolean;
  // Vocal performance steering tags and transcript display.
  speech_cues?: SpeechCuesConfig;
}

export interface STTConfig {
  type: 'builtin' | 'http' | 'cli' | 'disabled' | 'web-speech';
  builtin_name?: string;
  command?: string;
  args?: string[];
  endpoint?: string;
  model?: string;
  api_key?: string;
}

export interface ImageConfig {
  type: 'builtin' | 'http' | 'cli' | 'disabled' | 'comfyui' | 'gemini';
  builtin_name?: string;
  command?: string;
  args?: string[];
  endpoint?: string;
  model?: string;
  api_key?: string;
  auto_generate: boolean;
  builtin_fallback?: boolean;
  aspect_ratio?: string;
  person_generation?: string;
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
  // "off", "summary", or "full". Off writes nothing, so normal play costs nothing.
  trace_level?: string;
}

export interface TraceEvent {
  ts: string;
  event: string;
  level: string;
  fields?: Record<string, unknown>;
}

export interface ProvidersConfig {
  gemini?: {
    api_key?: string;
  };
}

export interface AppConfig {
  version: string;
  paths: PathsConfig;
  providers?: ProvidersConfig;
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
  voice_id?: string;
}

export interface TestProviderResponse {
  success: boolean;
  latency_ms: number;
  message: string;
  preview?: string;
  audio_data_uri?: string;
  model_missing?: boolean;
  model_id?: string;
}

export interface GeminiModel {
  id: string;
  display_name?: string;
  description?: string;
  supported_actions?: string[];
  input_token_limit?: number;
  output_token_limit?: number;
  thinking?: boolean;
}

export interface ModelCatalogueResponse {
  models: GeminiModel[];
  error?: string;
}

export interface VoiceSearchRequest {
  config: TTSConfig;
  query?: string;
  type?: string;
  language_code?: string;
  gender?: string;
  accent?: string;
  persona?: string;
}

export interface VoiceSearchResponse {
  voices: ProviderVoice[];
  error?: string;
}

export type ProviderFamily = 'llm' | 'tts' | 'stt' | 'image';

export type ProviderFeature =
  | 'streaming'
  | 'tools'
  | 'thinking'
  | 'vision'
  | 'voice_catalog'
  | 'voice_options'
  | 'speech_cues'
  | 'markdown_emphasis'
  | 'metered'
  | 'key_required'
  | 'model_catalogue'
  | 'extended_voices'
  | 'offline'
  | 'auto_generate'
  | 'sessions'
  | 'context_cache';

export interface ProviderTunable {
  key: string;
  label: string;
  kind: 'float' | 'int' | 'bool' | 'string' | 'enum';
  min?: number;
  max?: number;
  step?: number;
  options?: string[];
  default?: unknown;
  help?: string;
}

export interface ProviderPreset {
  id: string;
  label: string;
  description: string;
  config: Record<string, unknown>;
  order: number;
}

export interface ProviderDescriptor {
  id: string;
  family: ProviderFamily;
  label: string;
  description: string;
  source: string;
  features: ProviderFeature[];
  tunables?: ProviderTunable[];
  presets?: ProviderPreset[];
}

export interface ProviderCatalog {
  providers: ProviderDescriptor[];
}

export interface AddressedFinding {
  turn: number;
  rule: string;
}

export interface Ref {
  kind: string;
  id: string;
  relation?: string;
}

export interface SectionReport {
  name: string;
  tokens: number;
  included: boolean;
  source?: string;
  refs?: Ref[];
}

export interface ProviderSession {
  provider: string;
  id: string;
  through_turn: number;
  model?: string;
  prefix_hash?: string;
}

export interface TurnContext {
  turn_number: number;
  mode: string;
  budget: number;
  estimated_tokens: number;
  sections: SectionReport[];
  refs: Ref[];
  working_set: Ref[];
  threads?: string[];
  summary_version: number;
  world_hash?: string;
  system_hash?: string;
  prompt_hash: string;
  strategy: string;
  prefix_hash?: string;
  session?: ProviderSession;
  cached_tokens?: number;
  prompt?: string;
}

export interface WorkingEntry {
  kind: string;
  id: string;
  name?: string;
  weight: number;
  last_turn: number;
  role?: string;
}



