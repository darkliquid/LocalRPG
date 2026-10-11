export interface PlayerState {
  id: string;
  name: string;
  type: string;
  state: Record<string, any>;
  appearance?: string;
  voice?: VoiceProfile;
}

export interface AdvancementTrack {
  filled: number;
  size: number;
}

export interface AdvancementUnlock {
  id: string;
  label: string;
  description?: string;
  cost: number;
  affordable: boolean;
  requires_met: boolean;
  gate_open: boolean;
}

export interface Advancement {
  currency: string;
  label?: string;
  value: number;
  mode?: string;
  track?: AdvancementTrack;
  unlocks?: AdvancementUnlock[];
  pending: boolean;
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
  // The campaign's world genre, which tints the app's chrome when there is no banner.
  genre?: string;
  // The campaign's image allowance and what it has spent. 0 means unlimited.
  image_budget?: { max_images?: number; max_micros?: number; used_images?: number; spent_micros?: number };
  // The campaign's image approval policy: auto generates, ask waits for the player.
  image_approval?: 'auto' | 'ask';
  // The resolved mechanics policy in force: off, auto, or ask.
  mechanics_engagement?: 'off' | 'auto' | 'ask';
  // The campaign's progression summary, when the system declares advancement.
  advancement?: Advancement;
  // Content warnings (mismatch or drift detected against content lock)
  content_warnings?: string[];
}

export interface TurnSegment {
  kind: 'narration' | 'speech';
  speaker?: string;
  speaker_id?: string;
  text: string;
  // Names the check whose roll this segment narrates, so the dice render inline.
  check_ref?: string;
  // The segment's clips in play order. With grouping, a run of adjacent
  // same-speaker segments shares one clip, so this is usually one URL; each URL
  // is content-addressed, so the key in it is what identifies the audio.
  audio_urls?: string[];
  // The shared clip this segment plays, when a run of segments shares one. The
  // first segment of the group carries the play/stop/regenerate control.
  clip_group?: string;
  portrait_url?: string;
  speaker_portrait?: string;
  has_custom_portrait?: boolean;
  // True for the protagonist's own line, which renders as speech but suppresses
  // the duplicate action block for the turn.
  player?: boolean;
  // Seconds the backend estimates this line takes to read, which is the same
  // estimate the exports pace with.
  duration?: number;
}

// ClipGroupDTO is one clip that a run of segments shares, so the client renders
// a single audio control for the whole group.
export interface ClipGroupDTO {
  key: string;
  audio_urls: string[];
  segment_indexes: number[];
}

// TTSBatchJob is one offline batch synthesis job for a campaign.
export interface TTSBatchJob {
  id: string;
  game_id: string;
  game_name?: string;
  provider: string;
  model?: string;
  status: string;
  request_count: number;
  completed: number;
  failed_keys?: string[];
  // Why the job last failed to progress, empty when it is fine.
  last_error?: string;
}

// PendingCheckRequest is the check a GM proposed, as the player sees it.
export interface PendingCheckRequest {
  actor?: string;
  check_kind?: string;
  stat?: string;
  skill?: string;
  stakes?: string;
  notation?: string;
  outcomes?: Record<string, string>;
}

// PendingCheck is a GM-proposed check awaiting the player's roll, with the
// arithmetic the roll card shows.
export interface PendingCheck {
  ref: string;
  proposed_by?: string;
  request?: PendingCheckRequest;
  notation?: string;
  bonuses?: { source: string; value: number }[];
  actor_values?: Record<string, number>;
}

// CounterProposal is a player's argument about a pending check.
export interface CounterProposal {
  approach?: string;
  stakes?: string;
  difficulty?: string;
}

// Adjudication is the GM's ruling on a counter-proposal: accept, adjust, or hold.
export interface Adjudication {
  ruling: string;
  stakes?: string;
  difficulty?: string;
  notation?: string;
  profile?: string;
  reason?: string;
}

// Negotiation is one counter-proposal and the GM's ruling on it.
export interface Negotiation {
  counter: CounterProposal;
  ruling: Adjudication;
}

export interface RenegotiateResult {
  ruling: Adjudication;
  pending_check?: PendingCheck;
  negotiations?: Negotiation[];
}

export interface Turn {
  turn_number: number;
  input_text: string;
  mode: string;
  prose: string;
  segments?: TurnSegment[];
  // The shared clips a run of segments plays, so the client renders one audio
  // control per group.
  clip_groups?: ClipGroupDTO[];
  image_url?: string;
  scene_break?: boolean;
  entities_hit?: string[];
  location_id?: string;
  location_name?: string;
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
  // Whether the action was rejected as impossible, and the checks it resolved.
  rejected?: boolean;
  checks?: TurnCheck[];
  // A declared health stat reaching zero this turn, and the directive an
  // on-world-tick run injected, so the mechanical consequence is visible.
  health_effects?: { entity: string; effect: string }[];
  world_tick?: string;
  // A GM-proposed check awaiting the player's roll (ask policy).
  pending_check?: PendingCheck;
  // negotiations are the counter-proposals made to this turn's pending check and
  // the GM's ruling on each.
  negotiations?: Negotiation[];
  // The turn this one continues, when the player rolled a pending check.
  continuation_of?: number;
  // How the turn's control records fared: nil for a clean turn.
  record_report?: RecordReport;
}

// RecordIssue is one control record that needed repair or was dropped.
export interface RecordIssue {
  type: string;
  repair?: string;
  error?: string;
}

// RecordReport is a turn's control-record health summary.
export interface RecordReport {
  total: number;
  repaired: number;
  failed: number;
  issues?: RecordIssue[];
}

// DieFace is one die as it landed. Symbol is the notation's own way of showing
// that face, so a Fate die reads as a blank or a plus rather than as 0 or 1.
export interface DieFace {
  value: number;
  symbol?: string;
}

export interface TurnCheck {
  check_id: string;
  actor?: string;
  target?: string;
  check_kind?: string;
  stakes?: string;
  outcome: string;
  // outcome_text is the system's own description of the outcome, so a label such
  // as "weak" reads as fiction, and outcome_vocabulary is the declared order the
  // card uses to tone the result.
  outcome_text?: string;
  outcome_vocabulary?: string[];
  // profile names the resolution profile that decided the outcome, when one did,
  // and position/effect are the Blades-style stakes it carries.
  profile?: string;
  position?: string;
  effect?: string;
  // successes is the count of dice meeting a pool threshold, when the profile is
  // a success-count pool.
  successes?: number;
  // dice are the faces that landed, which is what a die can be drawn from: a
  // total of 4 from 2d6 says nothing about the individual dice.
  roll?: { notation: string; total: number; successes?: number; roll_count?: number; dice?: DieFace[] };
  // applied is every stat, skill, and modifier that contributed, so a player can
  // see why a 7 became a 9.
  applied?: { source: string; value: number }[];
  // source is "manual" when the total came from a player's entered dice rather
  // than a roll, so the chronicle can say so.
  source?: string;
  // An opposed check carries the opponent's roll and total, so the card shows the
  // contest rather than only the actor's side.
  opposed_roll?: { notation: string; total: number; successes?: number; roll_count?: number; dice?: DieFace[] };
  opposed_total?: number;
  opposed_actor?: string;
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

export interface AudioProgressEvent {
  turn_number: number;
  sequence: number;
  total_segments: number;
  stage: 'waiting' | 'synthesizing' | 'encoding' | 'ready' | 'failed';
  ready_count: number;
  failed_count?: number;
  audio_key?: string;
  audio_url?: string;
  owner?: string;
}

export interface PlaybackEntry {
  played_ms: number;
  total_ms?: number;
  complete?: boolean;
}

export interface PlaybackLedgerDTO {
  turn: number;
  owner: string;
  entries: Record<string, PlaybackEntry>;
}

export interface PlaybackLedgerRequest {
  turn: number;
  entries: Record<string, PlaybackEntry>;
}

export interface TurnEvent {
  type: 'chunk' | 'speech' | 'segment' | 'turn' | 'tool' | 'error' | 'model_missing' | 'audio_progress' | 'portrait' | 'scene_image' | 'step' | 'estimate' | 'draft' | 'progress' | 'batch';
  owner?: string;
  text?: string;
  turn?: Turn;
  // One parsed narration or speech unit, present when type is 'segment': it is
  // emitted while the model is still writing, before the authoritative turn.
  segment?: TurnSegment;
  // Streamed narration, present when type is 'speech': the unit's ordinal within
  // the turn, and the clip written for it.
  index?: number;
  audio_key?: string;
  audio_url?: string;
  audio_progress?: AudioProgressEvent;
  character_id?: string;
  portrait_url?: string;
  has_custom_portrait?: boolean;
  turn_number?: number;
  image_url?: string;
  version?: number;
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
  retry_after_ms?: number;
  // World generation progress, present when type is 'step'.
  step?: WorldGenStep;
  // The dry-run estimate, present when type is 'estimate'.
  estimate?: WorldEstimate;
  // The generated draft, present when type is 'draft'.
  draft?: WorldDraftInfo;
  // The generated system draft, present when type is 'draft'.
  system_draft?: SystemDraftInfo;
  // Import progress, present when type is 'progress'.
  progress?: WorldImportProgress;
  // The previewed entity batch, present when type is 'batch'.
  batch?: WorldEntityBatch;
}

// WorldImportProgress is one batch of a source import: what it read, what it
// found, and how much of the whole is done.
export interface WorldImportProgress {
  batch: number;
  batches: number;
  sources?: string[];
  found: number;
  total: number;
  names?: string[];
  // cut_off counts the batches whose reply ran out before it finished.
  cut_off?: number;
}

// WorldGenStep is one progress report from the generation pipeline.
export interface WorldGenStep {
  name: string;
  status: 'done' | 'error';
  detail?: string;
}

// WorldEstimate is a planned generation's cost, before it runs. An unpriced
// provider reports priced: false rather than a cost of zero.
export interface WorldEstimate {
  calls: number;
  chunks?: number;
  cost_micros?: number;
  priced: boolean;
}

// WorldSource names something to ingest: a folder, or a set of URLs.
export interface WorldSource {
  kind: 'folder' | 'url';
  path?: string;
  urls?: string[];
}

// WorldCounts is how many of each thing a generation should produce.
export interface WorldCounts {
  locations?: number;
  factions?: number;
  characters?: number;
}

export interface WorldGenerateRequest {
  premise: string;
  name?: string;
  genre?: string;
  themes?: string[];
  counts?: WorldCounts;
  source?: WorldSource;
  dry_run?: boolean;
  limits?: GenerationLimitsOverride;
}

export interface WorldDraftEntity {
  id: string;
  name: string;
  type: string;
  tags?: string[];
  folder?: string;
  body: string;
  source?: string;
  links?: string[];
  dropped_links?: string[];
}

export interface WorldDraftSection {
  title: string;
  body: string;
}

export interface WorldDraftInfo {
  id: string;
  name: string;
  description: string;
  genre: string;
  art_style?: string;
  tags?: string[];
  lore: string;
  sections: WorldDraftSection[];
  entities: WorldDraftEntity[];
  estimate?: WorldEstimate;
  calls?: number;
  oracle?: boolean;
}

export interface WorldEntityBatchRequest {
  instruction: string;
  kinds?: string[];
  count?: number;
  focus?: string;
  // source extracts the batch from a folder or a set of URLs instead of
  // generating it from the instruction.
  source?: WorldSource;
  limits?: GenerationLimitsOverride;
}

export interface WorldEntityBatch {
  entities: WorldDraftEntity[];
  // batch_id names the stored batch. A batch of a few hundred entities is far
  // more than a request body carries, so accepting it names the batch.
  batch_id?: string;
  // oracle marks a batch the deterministic fallback produced, so the dialog can
  // say so rather than implying a model wrote it.
  oracle?: boolean;
  // cut_off counts the batches whose reply ran out of room, so a short import is
  // not mistaken for a complete one.
  cut_off?: number;
}

export interface WorldEntityAcceptRequest {
  // The batch is named rather than posted, so an accept of hundreds of entities
  // is a small request. ids narrows it; empty means the whole batch.
  batch_id?: string;
  ids?: string[];
  // entities carries the batch itself, for a caller with no stored one.
  entities?: WorldDraftEntity[];
  rename?: boolean;
}

export interface WorldEnhanceRequest {
  instruction: string;
  kinds?: string[];
}

export interface WorldEnhancement {
  kind: 'lore' | 'entity' | 'hook';
  title: string;
  body: string;
  entity?: WorldDraftEntity;
  target?: string;
  reason?: string;
}

export interface WorldEnhanceResponse {
  proposals: WorldEnhancement[];
  oracle?: boolean;
}

export interface WorldEnhanceApplyRequest {
  proposals: WorldEnhancement[];
  rename?: boolean;
}

export interface WorldApplyResult {
  written: string[];
  renamed?: string[];
}

export interface DraftCommitRequest {
  draft_id: string;
  target_world_id?: string;
  // accept_all keeps the whole draft. Otherwise section_indexes and entity_ids
  // name what was kept; the content is read from the stored draft, because a
  // generated world can hold hundreds of entities.
  accept_all?: boolean;
  section_indexes?: number[];
  entity_ids?: string[];
  // edits carries the entities the reviewer changed, so an edit is written rather
  // than the generated text, without posting everything that was accepted.
  edits?: WorldDraftEntity[];
  meta?: CreateWorldRequest;
}

export interface DraftDiscardRequest {
  draft_id: string;
}

// DirectoryChoiceStatus is the state of a pending native folder choice.
export type DirectoryChoiceStatus = 'idle' | 'pending' | 'selected' | 'cancelled';

export interface DirectoryChoice {
  status: DirectoryChoiceStatus;
  path?: string;
}

export type SaveFileChoiceStatus = 'idle' | 'pending' | 'selected' | 'cancelled';

export interface SaveFileFilter {
  display_name: string;
  pattern: string;
}

export interface ChooseSaveFileRequest {
  title?: string;
  default_dir?: string;
  default_filename?: string;
  filters?: SaveFileFilter[];
}

export interface SaveFileChoice {
  status: SaveFileChoiceStatus;
  path?: string;
}

export interface ExportContentResult {
  path: string;
  id: string;
  version: string;
  type: string;
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
  aliases?: string[];
  folder?: string;
  filename_mismatch?: boolean;
  parse_error?: boolean;
  has_portrait?: boolean;
  portrait_url?: string;
}

export interface EntityNote {
  id: string;
  name: string;
  type: string;
  markdown: string;
  state: Record<string, any>;
  backlinks: string[];
  history?: number[];
  folder?: string;
  parse_error?: boolean;
}

export interface FolderNode {
  path: string;
  name: string;
  children?: FolderNode[];
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
  // Where the displayed artwork comes from: 'campaign', 'world', or undefined
  // when there is none. Used to offer reverting to the world's art.
  banner_source?: 'campaign' | 'world';
  icon_source?: 'campaign' | 'world';
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
  | 'parse_error' | 'timeout' | 'context_too_large' | 'invalid_request'
  | 'rate_limited' | 'insufficient_funds';

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
  retry_after_ms?: number;
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

export interface CharacterPortraitDTO {
  portrait_url: string;
  generated_at: string;
}

export interface SystemDetail {
  id: string;
  name: string;
  version: string;
  description: string;
  script: string;
  rules_prompt?: string;
  character_creation?: CharacterCreationSpec;
  mechanics?: MechanicsSpec;
  warnings?: string[];
}

export interface CreateSystemRequest {
  id?: string;
  name: string;
  version?: string;
  description?: string;
  script?: string;
  rules_prompt?: string;
  character_creation?: CharacterCreationSpec;
  mechanics?: MechanicsSpec;
}

export interface SystemVerifyResult {
  ok: boolean;
  failures?: string[];
  script?: boolean;
}

export interface SystemDraftInfo {
  id: string;
  name: string;
  version: string;
  description: string;
  mechanics?: MechanicsSpec;
  script?: string;
  rules_prompt?: string;
  verify: SystemVerifyResult;
  notes?: string[];
  estimate?: WorldEstimate;
  calls?: number;
  oracle?: boolean;
}

export interface ProfileAddition {
  name: string;
  notation?: string;
  dc?: number;
  success_on?: string;
  ladder?: LadderStep[];
  outcomes?: SuccessOutcome[];
}

// SystemProposal is one proposed addition to an existing system. valid reports
// whether the smoke gate accepts it.
export interface SystemProposal {
  kind: 'stat' | 'skill' | 'profile' | 'advancement';
  title: string;
  reason?: string;
  stat?: StatSpec;
  skill?: SkillSpec;
  profile?: ProfileAddition;
  advancement?: AdvancementSpec;
  valid: boolean;
  problems?: string[];
}

export interface SystemEnhanceResponse {
  proposals: SystemProposal[];
  oracle?: boolean;
}

export interface SystemEnhanceRequest {
  instruction: string;
  kinds?: string[];
}

export interface SystemEnhanceApplyRequest {
  proposals: SystemProposal[];
}

export interface SystemEnhanceApplyResult {
  written: string[];
  detail?: SystemDetail;
}

export interface SystemExplainResponse {
  explanation: string;
  oracle?: boolean;
}

export interface SystemDeriveRequest {
  base_id: string;
  instruction: string;
}

export interface SystemGenerateRequest {
  description: string;
  name?: string;
  dry_run?: boolean;
  limits?: GenerationLimitsOverride;
}

export interface SystemDraftCommitRequest {
  draft_id: string;
  id?: string;
  name?: string;
  version?: string;
  description?: string;
  mechanics?: MechanicsSpec;
  script?: string;
  rules_prompt?: string;
}

export interface ReferenceSystem {
  id: string;
  name: string;
  version: string;
  description: string;
  rules_prompt: string;
  script: string;
  mechanics?: MechanicsSpec;
}

export interface ReferenceSystemsResponse {
  systems: ReferenceSystem[];
}

export interface ScenarioRange {
  min: number;
  max: number;
}

export interface ScenarioExpectations {
  outcome?: string;
  outcome_one_of?: string[];
  total?: ScenarioRange;
  state?: Record<string, unknown>;
  message_contains?: string;
}

export interface ScenarioStep {
  action: string;
  input?: string;
  expect?: ScenarioExpectations;
}

export interface Scenario {
  name: string;
  seed: number;
  setup?: { player?: { stats?: Record<string, unknown>; tags?: string[] } };
  steps: ScenarioStep[];
}

export interface SystemTestRequest {
  system: { id: string; script: string; mechanics?: MechanicsSpec };
  scenarios: Scenario[];
}

export interface SystemTestFailure {
  scenario: string;
  step: number;
  detail: string;
}

export interface SystemTestResponse {
  failures?: SystemTestFailure[];
}

export interface SystemScenariosResponse {
  scenarios?: Scenario[];
}

export interface StatSpec {
  id: string;
  label?: string;
  type?: string;
  default?: unknown;
  min?: number;
  max?: number;
}

export interface SkillSpec {
  id: string;
  label?: string;
  stat?: string;
}

export interface HealthSpec {
  stat: string;
  max_stat?: string;
  zero_effect?: string;
}

export interface DifficultySpec {
  id: string;
  label?: string;
  target: number;
}

export interface LadderStep {
  min: number;
  outcome: string;
}

export interface SuccessOutcome {
  min: number;
  max: number;
  outcome: string;
}

export interface ResolutionProfile {
  label?: string;
  notation?: string;
  dc?: number;
  ladder?: LadderStep[];
  success_on?: string;
  outcomes?: SuccessOutcome[];
  position?: string[];
  effect?: string[];
  // opposed names the stat the opponent rolls when this profile is a contest.
  opposed?: string;
  // ties decides an opposed check whose totals are equal: "opponent" hands it to
  // the opponent, and empty or "actor" keeps it with the actor.
  ties?: string;
}

export interface CheckConventions {
  notation?: string;
  outcome?: string[];
  difficulty?: DifficultySpec[];
  profiles?: Record<string, ResolutionProfile>;
}

export interface CurrencySpec {
  stat: string;
  label?: string;
}

export interface EarnRule {
  on: string;
  outcome?: string;
  rank?: string;
  amount: number;
}

export interface EffectSpec {
  type: string;
  stat?: string;
  amount?: number;
  max?: number;
  tag?: string;
  hook?: string;
}

export interface UnlockSpec {
  id: string;
  label: string;
  description?: string;
  cost: number;
  requires?: string[];
  effects?: EffectSpec[];
}

export interface LevelSpec {
  at: number;
  label?: string;
  effects?: EffectSpec[];
}

export interface AdvancementSpec {
  currency: CurrencySpec;
  mode?: string;
  earn?: EarnRule[];
  track_size?: number;
  gate?: string;
  unlocks?: UnlockSpec[];
  levels?: LevelSpec[];
}

export interface MechanicsSpec {
  stats?: StatSpec[];
  skills?: SkillSpec[];
  health?: HealthSpec;
  checks?: CheckConventions;
  allow_freeform_state?: boolean;
  engagement?: string;
  advancement?: AdvancementSpec;
}

export interface WorldEntitySummary {
  id: string;
  name: string;
  type: string;
  folder?: string;
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
  folder?: string;
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
  // An optional user-chosen discriminator that keeps two configs of one adapter
  // at one endpoint distinct in usage, pricing, and caches.
  instance?: string;
  // An ordered list of provider instance names this role tries in turn, ordered
  // by select. Empty keeps the single provider and the configured fallback.
  chain?: string[];
  select?: 'first' | 'cheapest' | 'local-first' | 'by-tag';
  tag?: string;
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
  // Tool rounds per turn (0 means unbounded), and the cap on one tool result.
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
  action_echo?: boolean;
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

// MediaInspectRequest names the family to describe: "tts", "stt", or "image".
export interface MediaInspectRequest {
  family: string;
}

// MediaInspectEntry describes one named media configuration.
export interface MediaInspectEntry {
  name: string;
  provider_key: string;
  key_present: boolean;
  key_required: boolean;
  metered: boolean;
  tier: string;
  // model_id and model_installed describe a local model an entry needs. They are
  // set for an embedding encoder so the manager can offer a download.
  model_id?: string;
  model_installed?: boolean;
}

export interface MediaInspectResponse {
  entries: MediaInspectEntry[];
}

export interface TTSConfig {
  type: 'builtin' | 'http' | 'cli' | 'disabled' | 'gemini' | 'fish-audio' | 'cartesia';
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
  // An optional user-chosen discriminator that keeps two configs of one adapter
  // at one endpoint distinct in usage, pricing, and caches.
  instance?: string;
}

export interface STTConfig {
  type: 'builtin' | 'http' | 'cli' | 'disabled' | 'web-speech' | 'cartesia';
  builtin_name?: string;
  command?: string;
  args?: string[];
  endpoint?: string;
  model?: string;
  api_key?: string;
  // An optional user-chosen discriminator that keeps two configs of one adapter
  // at one endpoint distinct in usage, pricing, and caches.
  instance?: string;
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
  // An optional user-chosen discriminator that keeps two configs of one adapter
  // at one endpoint distinct in usage, pricing, and caches.
  instance?: string;
}

export interface MediaConfig {
  tts: TTSConfig;
  stt: STTConfig;
  image: ImageConfig;
  // Named configurations of each family, which coexist with the singleton
  // default above and are selected by name.
  tts_providers?: Record<string, TTSConfig>;
  stt_providers?: Record<string, STTConfig>;
  image_providers?: Record<string, ImageConfig>;
  // Maps a use name (narrator, npc, scene, portrait, placeholder) to a provider
  // name, or the family default when unset.
  purposes?: Record<string, string>;
  // Declares an ordered provider chain and selection rule per use name. A
  // purpose with no chain uses its single configured provider.
  purpose_chains?: Record<string, ProviderChain>;
}

// ProviderChain declares an ordered provider chain and the rule that orders it.
export interface ProviderChain {
  chain?: string[];
  select?: 'first' | 'cheapest' | 'local-first' | 'by-tag';
  tag?: string;
}

export type EmbeddingProviderType = 'builtin' | 'onnx' | 'http' | 'gemini' | 'disabled';

// EmbeddingProviderConfig is one named embedding provider. Unlike the media
// families there is no singleton default: the top-level provider selector names
// which entry is active.
export interface EmbeddingProviderConfig {
  type: EmbeddingProviderType;
  builtin_name?: string;
  endpoint?: string;
  url?: string;
  api_key?: string;
  model?: string;
  model_path?: string;
  instance?: string;
}

export interface EmbeddingsConfig {
  enabled: boolean;
  provider: string;
  model?: string;
  dimensions?: number;
  batch_size?: number;
  providers?: Record<string, EmbeddingProviderConfig>;
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

export interface InworldProviderConfig {
  api_key?: string;
}

export interface ProvidersConfig {
  gemini?: {
    api_key?: string;
  };
  inworld?: InworldProviderConfig;
  cartesia?: {
    api_key?: string;
  };
}

export interface AppConfig {
  version: string;
  paths: PathsConfig;
  providers?: ProvidersConfig;
  agents: AgentsConfig;
  media: MediaConfig;
  embeddings?: EmbeddingsConfig;
  preferences: PreferencesConfig;
  generation?: GenerationConfig;
  // styles.pack selects a procedural style pack; empty is the built-in look.
  styles?: StylesConfig;
}

export interface StylesConfig {
  pack?: string;
}

// StylePackStatus is one style pack the config directory holds, with any reason it
// could not be loaded.
export interface StylePackStatus {
  id: string;
  label?: string;
  path: string;
  problems?: string[];
}

export interface StylePacks {
  active?: string;
  packs: StylePackStatus[];
  warnings?: string[];
}

// GenerationConfig bounds an AI world generation. A premise, entity, or
// enhancement generation is bounded by its call count; an ingestion, whose size
// is the source's, is bounded by how many chunks it will read.
export interface GenerationConfig {
  max_calls?: number;
  max_chunks?: number;
}

// GenerationLimitsOverride raises a generation's limits for one request, so a
// user who hits a limit can carry on without leaving the flow.
export interface GenerationLimitsOverride {
  max_calls?: number;
  max_chunks?: number;
}

export interface SettingsResponse {
  config: AppConfig;
  config_file_path: string;
  is_local_override: boolean;
  warnings?: string[];
  app_version?: string;
}

export interface OfflinePresetRequest {
  tts?: string;
}

export interface OfflinePresetResponse {
  changes: string[];
}

export interface OfflineIssue {
  role: string;
  provider_key: string;
  tier: string;
  reason: string;
}

export interface OfflineReportResponse {
  offline: boolean;
  issues: OfflineIssue[];
}


export interface ExportRequest {
  game_id: string;
  format: 'web' | 'video';
  out_dir: string;
  art: boolean;
  audio: boolean;
  still?: boolean;
  fps?: number;
  size?: string;
}

export interface ExportJob {
  game_id: string;
  format: string;
  output_path?: string;
  running: boolean;
}

export interface ExportEvent {
  game_id: string;
  format: string;
  phase: string;
  done: number;
  total: number;
  message?: string;
  output_path?: string;
  error?: string;
  frames: number;
  image_frames: number;
  repeat_frames: number;
  audio_packets: number;
  total_audio_packets: number;
  audio_bytes: number;
  total_audio_bytes: number;
  elapsed_ms: number;
  length_ms: number;
}

export interface ExportCapabilities {
  default_dir?: string;
  native_dialog: boolean;
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

export type ProviderFamily = 'llm' | 'tts' | 'stt' | 'image' | 'embedding';

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
  // The honest capability tier: offline-basic, offline-neural, local-server, or
  // cloud. Caveat is the one-line "what it is not"; the UI falls back to the
  // tier's default when it is empty.
  tier?: string;
  caveat?: string;
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

export interface UsageRow {
  turn_number: number;
  role: string;
  provider: string;
  model?: string;
  input_tokens?: number;
  output_tokens?: number;
  characters?: number;
  requests?: number;
  estimated?: boolean;
  cost_micros?: number;
}

export interface CampaignUsage {
  game_id: string;
  name?: string;
  total_cost_micros: number;
}

export interface Usage {
  rows?: UsageRow[];
  by_provider?: Record<string, number>;
  by_role?: Record<string, number>;
  total_cost_micros: number;
  currency?: string;
  campaigns?: CampaignUsage[];
}

export interface LimitState {
  provider: string;
  role: string;
  until?: string;
  funds_failure?: string;
}

export interface LimitsDTO {
  blocks: LimitState[];
}

export interface DocArticleSummary {
  id: string;
  title: string;
  category: string;
  order: number;
  description: string;
}

export interface DocArticle extends DocArticleSummary {
  content: string;
}




// FrontmatterKeySchema is one accepted frontmatter key, as the server generates
// it from the Go struct. Values are suggestions, not a closed set.
export interface FrontmatterKeySchema {
  name: string;
  type: string;
  required: boolean;
  values?: string[];
  description: string;
}

export interface FrontmatterSchema {
  allowUnknown: boolean;
  keys: FrontmatterKeySchema[];
}

// EntityTypeSpec is one offered entity type, as the server describes it: how the
// picker labels it and the frontmatter keys a new note of this type is built with.
export interface EntityTypeSpec {
  id: string;
  label: string;
  description: string;
  aliases?: string[];
  keys: FrontmatterKeySchema[];
}

// EntityTypeCatalog is the served list of offered entity types. base_keys is what
// an unknown type falls back to.
export interface EntityTypeCatalog {
  base_keys: FrontmatterKeySchema[];
  types: EntityTypeSpec[];
}

// SaveErrorBody is what the entity save route returns when the frontmatter will
// not parse, so the editor can point at the offending line.
export interface SaveErrorBody {
  error: string;
  line?: number;
  column?: number;
}

export interface TrustInfo {
  state: 'verified' | 'unknown_key' | 'unsigned' | 'invalid';
  publisher?: string;
  fingerprint?: string;
}

export interface ImportResultDTO {
  id: string;
  name: string;
  version: string;
  type: string;
  author?: string;
  license?: string;
  description?: string;
  file_count: number;
  has_script: boolean;
  action: 'installed' | 'renamed' | 'overwritten';
  trust?: TrustInfo;
}

export interface ContentManifestInfo {
  id: string;
  name: string;
  version: string;
  type?: string;
  author?: string;
  license?: string;
  description?: string;
  file_count?: number;
  has_script?: boolean;
  trust?: TrustInfo;
}

export interface RegistryPackageDTO {
  type: string;
  id: string;
  name: string;
  version: string;
  description?: string;
  author?: string;
  license?: string;
  download: string;
  sha256: string;
  publisher?: string;
  requires?: { type: string; id: string; version?: string }[];
}

export interface PackageRefDTO {
  registry_name: string;
  registry_url: string;
  package: RegistryPackageDTO;
}

export interface RegistrySourceDTO {
  url: string;
  name?: string;
  package_count: number;
  error?: string;
}


