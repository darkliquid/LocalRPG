import {
  GameState,
  Turn,
  TurnEvent,
  EntityNote,
  GraphData,
  GameSummary,
  EntitySummary,
  Recap,
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
  TraceEvent,
  AddressedFinding,
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

  static async deleteGame(gameID: string): Promise<void> {
    const res = await fetch(`/api/game/${gameID}`, { method: 'DELETE' });
    if (!res.ok) throw new Error(`deleteGame: ${res.statusText}`);
  }

  static async restartGame(gameID: string): Promise<GameSummary> {
    const res = await fetch(`/api/game/${gameID}/restart`, { method: 'POST' });
    if (!res.ok) throw new Error(`restartGame: ${res.statusText}`);
    return res.json();
  }

  static async audioStatus(): Promise<{ available: boolean; playing: boolean }> {
    const res = await fetch('/api/audio/status');
    if (!res.ok) throw new Error(`audioStatus: ${res.statusText}`);
    return res.json();
  }

  static async stopAudio(): Promise<void> {
    const res = await fetch('/api/audio/stop', { method: 'POST' });
    if (!res.ok) throw new Error(`stopAudio: ${res.statusText}`);
  }

  static async playTurnAudio(gameID: string, turnNumber: number): Promise<void> {
    const res = await fetch(`/api/game/${gameID}/turn/${turnNumber}/play`, { method: 'POST' });
    if (!res.ok) throw new Error(`playTurnAudio: ${res.statusText}`);
  }

  static async playSegmentAudio(gameID: string, turnNumber: number, segmentIndex: number): Promise<void> {
    const res = await fetch(`/api/game/${gameID}/turn/${turnNumber}/segment/${segmentIndex}/play`, { method: 'POST' });
    if (!res.ok) throw new Error(`playSegmentAudio: ${res.statusText}`);
  }

  static async traceEvents(limit = 200, gameID?: string): Promise<TraceEvent[]> {
    const query = new URLSearchParams({ limit: String(limit) });
    if (gameID) query.set('game', gameID);
    const res = await fetch(`/api/trace?${query.toString()}`);
    if (!res.ok) throw new Error(`traceEvents: ${res.statusText}`);
    return res.json();
  }

  static async clearTrace(): Promise<void> {
    const res = await fetch('/api/trace', { method: 'DELETE' });
    if (!res.ok) throw new Error(`clearTrace: ${res.statusText}`);
  }

  static async updateGameSettings(gameID: string, patch: { opening_prompt?: string }): Promise<void> {
    const res = await fetch(`/api/game/${gameID}/settings`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(patch),
    });
    if (!res.ok) throw new Error(`updateGameSettings: ${res.statusText}`);
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

  static async transcribeAudio(audioBlob: Blob): Promise<{ text: string }> {
    const formData = new FormData();
    formData.append('audio', audioBlob, 'speech.webm');

    const res = await fetch('/api/stt', {
      method: 'POST',
      body: formData,
    });

    if (!res.ok) {
      const errText = await res.text();
      throw new Error(errText || `STT failed with status ${res.status}`);
    }

    return res.json();
  }

  // streamTurn posts a player action and reports each NDJSON line as it arrives.
  // It must not assume the body arrives progressively: a client that buffers the
  // response produces the same events in the same order.
  static async streamTurn(
    gameID: string,
    body: { mode: string; input: string },
    onEvent: (event: TurnEvent) => void,
    signal?: AbortSignal
  ): Promise<void> {
    const res = await fetch(`/api/game/${gameID}/turn`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
      signal,
    });

    if (!res.ok) {
      throw new Error(`streamTurn: ${res.status} ${await res.text()}`);
    }
    if (!res.body) {
      throw new Error('streamTurn: response has no body');
    }

    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';

    for (;;) {
      const { value, done } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });

      let newline = buffer.indexOf('\n');
      while (newline !== -1) {
        const line = buffer.slice(0, newline).trim();
        buffer = buffer.slice(newline + 1);
        if (line) onEvent(JSON.parse(line) as TurnEvent);
        newline = buffer.indexOf('\n');
      }
    }

    const tail = buffer.trim();
    if (tail) onEvent(JSON.parse(tail) as TurnEvent);
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

  async getRecap(): Promise<Recap> {
    const res = await fetch(`/api/game/${this.gameID}/recap`);
    if (!res.ok) throw new Error(`getRecap: ${res.statusText}`);
    return res.json();
  }

  async listEntities(): Promise<EntitySummary[]> {
    const res = await fetch(`/api/game/${this.gameID}/entities`);
    if (!res.ok) throw new Error(`listEntities: ${res.statusText}`);
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

  async mergeEntity(sourceID: string, intoID: string): Promise<EntityNote> {
    const res = await fetch(`/api/game/${this.gameID}/entity/${sourceID}/merge`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ into: intoID }),
    });
    if (!res.ok) throw new Error(`mergeEntity: ${res.statusText}`);
    return res.json();
  }

  async getFindings(): Promise<{ addressed: AddressedFinding[] }> {
    const res = await fetch(`/api/game/${this.gameID}/findings`);
    if (!res.ok) throw new Error(`getFindings: ${res.statusText}`);
    return res.json();
  }

  async addressFinding(turn: number, rule: string): Promise<void> {
    const res = await fetch(`/api/game/${this.gameID}/findings`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ turn, rule }),
    });
    if (!res.ok) throw new Error(`addressFinding: ${res.statusText}`);
  }
}
