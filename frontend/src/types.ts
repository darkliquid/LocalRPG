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

export interface Turn {
  turn_number: number;
  input_text: string;
  mode: string;
  prose: string;
  speaker?: string;
  dialogue?: string;
  audio_url?: string;
  image_url?: string;
  entities_hit?: string[];
}

export interface EntityNote {
  id: string;
  name: string;
  type: string;
  markdown: string;
  state: Record<string, any>;
  backlinks: string[];
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


