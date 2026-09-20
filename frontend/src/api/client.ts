import {
  GameState,
  Turn,
  EntityNote,
  GraphData,
  GameSummary,
  SystemInfo,
  WorldInfo,
  CreateGameRequest,
  SystemDetail,
  CreateSystemRequest,
  WorldDetail,
  CreateWorldRequest,
  WorldEntityDetail,
  AppConfig,
  SettingsResponse,
  TestProviderRequest,
  TestProviderResponse,
} from '../types';

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

  static async getSystem(id: string): Promise<SystemDetail> {
    const res = await fetch(`/api/system/${id}`);
    if (!res.ok) throw new Error(`getSystem: ${res.statusText}`);
    return res.json();
  }

  static async saveSystem(req: CreateSystemRequest): Promise<SystemDetail> {
    const isNew = !req.id;
    const url = isNew ? '/api/systems' : `/api/system/${req.id}`;
    const method = isNew ? 'POST' : 'PUT';
    const res = await fetch(url, {
      method,
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    });
    if (!res.ok) throw new Error(`saveSystem: ${res.statusText}`);
    return res.json();
  }

  static async getWorld(id: string): Promise<WorldDetail> {
    const res = await fetch(`/api/world/${id}`);
    if (!res.ok) throw new Error(`getWorld: ${res.statusText}`);
    return res.json();
  }

  static async saveWorld(req: CreateWorldRequest): Promise<WorldDetail> {
    const isNew = !req.id;
    const url = isNew ? '/api/worlds' : `/api/world/${req.id}`;
    const method = isNew ? 'POST' : 'PUT';
    const res = await fetch(url, {
      method,
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    });
    if (!res.ok) throw new Error(`saveWorld: ${res.statusText}`);
    return res.json();
  }

  static async getWorldEntity(worldId: string, entityId: string): Promise<WorldEntityDetail> {
    const res = await fetch(`/api/world/${worldId}/entity/${entityId}`);
    if (!res.ok) throw new Error(`getWorldEntity: ${res.statusText}`);
    return res.json();
  }

  static async saveWorldEntity(worldId: string, entityId: string, markdown: string): Promise<void> {
    const res = await fetch(`/api/world/${worldId}/entity/${entityId}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'text/plain' },
      body: markdown,
    });
    if (!res.ok) throw new Error(`saveWorldEntity: ${res.statusText}`);
  }

  static async deleteWorldEntity(worldId: string, entityId: string): Promise<void> {
    const res = await fetch(`/api/world/${worldId}/entity/${entityId}`, {
      method: 'DELETE',
    });
    if (!res.ok) throw new Error(`deleteWorldEntity: ${res.statusText}`);
  }

  static async getSettings(): Promise<SettingsResponse> {
    const res = await fetch('/api/settings');
    if (!res.ok) throw new Error(`getSettings: ${res.statusText}`);
    return res.json();
  }

  static async saveSettings(cfg: AppConfig): Promise<SettingsResponse> {
    const res = await fetch('/api/settings', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ config: cfg }),
    });
    if (!res.ok) throw new Error(`saveSettings: ${res.statusText}`);
    return res.json();
  }

  static async testProvider(req: TestProviderRequest): Promise<TestProviderResponse> {
    const res = await fetch('/api/settings/test-provider', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    });
    if (!res.ok) throw new Error(`testProvider: ${res.statusText}`);
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
