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
