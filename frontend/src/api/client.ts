import { GameState, Turn, EntityNote, GraphData, GameSummary, SystemInfo, WorldInfo, CreateGameRequest } from '../types';

export class APIClient {
  private gameID: string;

  static async listGames(): Promise<GameSummary[]> {
    const res = await fetch('/api/games');
    if (!res.ok) throw new Error(`listGames: ${res.statusText}`);
    return res.json();
  }

  static async listSystems(): Promise<SystemInfo[]> {
    const res = await fetch('/api/systems');
    if (!res.ok) throw new Error(`listSystems: ${res.statusText}`);
    return res.json();
  }

  static async listWorlds(): Promise<WorldInfo[]> {
    const res = await fetch('/api/worlds');
    if (!res.ok) throw new Error(`listWorlds: ${res.statusText}`);
    return res.json();
  }

  static async createGame(payload: CreateGameRequest): Promise<GameSummary> {
    const res = await fetch('/api/games', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    if (!res.ok) throw new Error(`createGame: ${res.statusText}`);
    return res.json();
  }

  constructor(gameID: string) {
    this.gameID = gameID;
  }

  async getGameState(): Promise<GameState> {
    const res = await fetch(`/api/game/${this.gameID}/state`);
    if (!res.ok) throw new Error(`getGameState: ${res.statusText}`);
    return res.json();
  }

  async getChronicle(): Promise<Turn[]> {
    const res = await fetch(`/api/game/${this.gameID}/chronicle`);
    if (!res.ok) throw new Error(`getChronicle: ${res.statusText}`);
    return res.json();
  }

  async getGraph(): Promise<GraphData> {
    const res = await fetch(`/api/game/${this.gameID}/graph`);
    if (!res.ok) throw new Error(`getGraph: ${res.statusText}`);
    return res.json();
  }

  async getEntity(entityID: string): Promise<EntityNote> {
    const res = await fetch(`/api/game/${this.gameID}/entity/${entityID}`);
    if (!res.ok) throw new Error(`getEntity: ${res.statusText}`);
    return res.json();
  }

  async saveEntity(entityID: string, markdown: string): Promise<void> {
    const res = await fetch(`/api/game/${this.gameID}/entity/${entityID}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ markdown })
    });
    if (!res.ok) throw new Error(`saveEntity: ${res.statusText}`);
  }
}
