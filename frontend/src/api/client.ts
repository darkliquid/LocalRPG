import {
  GameState,
  Advancement,
  Turn,
  TurnEvent,
  EntityNote,
  GraphData,
  GameSummary,
  EntitySummary,
  FolderNode,
  FrontmatterSchema,
  EntityTypeCatalog,
  SaveErrorBody,
  Recap,
  SystemInfo,
  WorldInfo,
  CreateGameRequest,
  GameSettingsPatch,
  GenerateCharacterRequest,
  GenerateCharacterResponse,
  GenerateTextRequest,
  GenerateTextResponse,
  CharacterPortraitDTO,
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
  ModelStatus,
  TTSInspectRequest,
  TTSInspectResponse,
  ModelCatalogueResponse,
  VoiceSearchRequest,
  VoiceSearchResponse,
  ProviderCatalog,
  TurnContext,
  WorkingEntry,
  GenerationFailure,
  EntityMemory,
  Usage,
  LimitsDTO,
  DocArticleSummary,
  DocArticle,
  ExportRequest,
  ExportJob,
  ExportEvent,
  ExportCapabilities,
  TTSBatchJob,
} from '../types';

// HTTPError carries the status of a failed request so callers can tell a missing
// campaign apart from a transient failure.
export class HTTPError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.name = 'HTTPError';
    this.status = status;
  }
}

// GenerationError carries the structured failure a generation endpoint returns,
// so a caller can show why nothing was generated instead of guessing.
export class GenerationError extends HTTPError {
  failure: GenerationFailure;
  constructor(status: number, failure: GenerationFailure) {
    super(status, failure.message);
    this.name = 'GenerationError';
    this.failure = failure;
  }
}

async function throwGenerationError(res: Response): Promise<never> {
  const text = await res.text();
  let failure: GenerationFailure = {
    code: 'provider_error',
    message: text || `generation failed with status ${res.status}`,
  };
  try {
    const body = JSON.parse(text) as { error?: GenerationFailure };
    if (body.error && body.error.code) failure = body.error;
  } catch {
    // A non-JSON body is left as the message.
  }
  throw new GenerationError(res.status, failure);
}

// WorldExistsError signals a 409 from world creation, so the form can point the
// user at the slug field instead of showing a generic failure.
export class WorldExistsError extends HTTPError {
  constructor(message: string) {
    super(409, message);
    this.name = 'WorldExistsError';
  }
}

export class APIClient {
  private gameID: string;

  static async getModels(): Promise<ModelStatus[]> {
    const res = await fetch('/api/models');
    if (!res.ok) throw new Error(`getModels: ${res.statusText}`);
    return res.json();
  }

  static async downloadModel(id: string): Promise<void> {
    const res = await fetch(`/api/models/${id}/download`, {
      method: 'POST',
    });
    if (!res.ok) throw new Error(`downloadModel: ${res.statusText}`);
  }

  static subscribeModelEvents(onEvent: (status: ModelStatus) => void): () => void {
    const eventSource = new EventSource('/api/models/events');
    eventSource.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data) as ModelStatus;
        onEvent(data);
      } catch (err) {
        console.error('Failed to parse model event:', err);
      }
    };
    return () => eventSource.close();
  }

  static async exportCapabilities(): Promise<ExportCapabilities> {
    const res = await fetch('/api/export/capabilities');
    if (!res.ok) throw new Error(`exportCapabilities: ${res.statusText}`);
    return res.json();
  }

  static async startExport(req: ExportRequest): Promise<ExportJob> {
    const res = await fetch('/api/export', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    });
    if (!res.ok) throw new HTTPError(res.status, `startExport: ${res.statusText}`);
    return res.json();
  }

  static async cancelExport(gameID: string): Promise<void> {
    const res = await fetch(`/api/export/${encodeURIComponent(gameID)}`, { method: 'DELETE' });
    if (!res.ok) throw new Error(`cancelExport: ${res.statusText}`);
  }

  static async chooseExportDirectory(): Promise<string> {
    const res = await fetch('/api/export/choose-directory', { method: 'POST' });
    if (!res.ok) {
      if (res.status === 501) throw new HTTPError(res.status, 'No native directory dialog is available');
      throw new HTTPError(res.status, `chooseExportDirectory: ${res.statusText}`);
    }
    const data = (await res.json()) as { path?: string };
    return data.path ?? '';
  }

  static subscribeExportEvents(onEvent: (event: ExportEvent) => void): () => void {
    const eventSource = new EventSource('/api/export/events');
    eventSource.onmessage = (event) => {
      try {
        onEvent(JSON.parse(event.data) as ExportEvent);
      } catch (err) {
        console.error('Failed to parse export event:', err);
      }
    };
    return () => eventSource.close();
  }

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

  // listEntityMemories reads one entity's memory timeline for the codex.
  static async listEntityMemories(gameID: string, entityID: string, limit = 20): Promise<EntityMemory[]> {
    const res = await fetch(`/api/game/${encodeURIComponent(gameID)}/entity/${encodeURIComponent(entityID)}/memories?limit=${limit}`);
    if (!res.ok) throw new HTTPError(res.status, await res.text());
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

  // generateCharacter asks the backend for starter values for a system's
  // character creation prompts. It creates nothing.
  static async generateCharacter(payload: GenerateCharacterRequest): Promise<GenerateCharacterResponse> {
    const res = await fetch('/api/character/generate', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    if (!res.ok) return throwGenerationError(res);
    return res.json();
  }

  // generateText asks the backend for one field or a whole form's worth of
  // values. It creates nothing.
  static async generateText(payload: GenerateTextRequest): Promise<GenerateTextResponse> {
    const res = await fetch('/api/generate-text', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    if (!res.ok) return throwGenerationError(res);
    return res.json();
  }

  // regenerateCharacterPortrait asks the backend for a fresh portrait and
  // returns its cache-busted URL.
  static async regenerateCharacterPortrait(gameID: string, characterID: string): Promise<CharacterPortraitDTO> {
    const res = await fetch(
      `/api/game/${encodeURIComponent(gameID)}/character/${encodeURIComponent(characterID)}/portrait`,
      { method: 'POST' }
    );
    if (!res.ok) return throwGenerationError(res);
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

  static async uploadGameAsset(gameId: string, kind: 'banner' | 'icon', file: File): Promise<{ url: string }> {
    const formData = new FormData();
    formData.append('file', file);
    const res = await fetch(`/api/game/${encodeURIComponent(gameId)}/${kind}`, {
      method: 'POST',
      body: formData,
    });
    if (!res.ok) throw new HTTPError(res.status, await res.text());
    return res.json();
  }

  static async uploadWorldAsset(worldId: string, kind: 'banner' | 'icon', file: File): Promise<{ url: string }> {
    const formData = new FormData();
    formData.append('file', file);
    const res = await fetch(`/api/world/${encodeURIComponent(worldId)}/${kind}`, {
      method: 'POST',
      body: formData,
    });
    if (!res.ok) throw new HTTPError(res.status, await res.text());
    return res.json();
  }

  static async generateGameAsset(gameId: string, kind: 'banner' | 'icon', prompt?: string): Promise<{ url: string }> {
    const res = await fetch(`/api/game/${encodeURIComponent(gameId)}/generate-asset`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ kind, prompt }),
    });
    if (!res.ok) return throwGenerationError(res);
    return res.json();
  }

  static async generateWorldAsset(worldId: string, kind: 'banner' | 'icon', prompt?: string): Promise<{ url: string }> {
    const res = await fetch(`/api/world/${encodeURIComponent(worldId)}/generate-asset`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ kind, prompt }),
    });
    if (!res.ok) return throwGenerationError(res);
    return res.json();
  }

  // generateAssetPreview renders banner or icon bytes from inline form
  // metadata without persisting anything, so creation flows can show a result
  // before the campaign or world exists.
  static async generateAssetPreview(
    kind: 'banner' | 'icon',
    name: string,
    description: string,
    artStyle: string,
    genre: string = '',
    usageToken?: string
  ): Promise<Blob> {
    const res = await fetch('/api/generate-asset-preview', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ kind, name, description, art_style: artStyle, genre, usage_token: usageToken }),
    });
    if (!res.ok) return throwGenerationError(res);
    return res.blob();
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

  static async playTurnAudio(gameID: string, turnNumber: number, force = false): Promise<void> {
    const url = `/api/game/${encodeURIComponent(gameID)}/turn/${turnNumber}/play${force ? '?force=1' : ''}`;
    const res = await fetch(url, { method: 'POST' });
    if (!res.ok) return throwGenerationError(res);
  }

  // regenerateSegmentAudio re-synthesizes one beat and answers with its refreshed
  // clip URLs. The keys are unchanged when nothing about the line changed.
  static async regenerateSegmentAudio(
    gameID: string,
    turnNumber: number,
    segmentIndex: number
  ): Promise<string[]> {
    const url = `/api/game/${encodeURIComponent(gameID)}/turn/${turnNumber}/segment/${segmentIndex}/audio`;
    const res = await fetch(url, { method: 'POST' });
    if (res.status === 204) return [];
    if (!res.ok) return throwGenerationError(res);
    const body = (await res.json()) as { audio_urls?: string[] };
    return body.audio_urls ?? [];
  }

  static async playSegmentAudio(gameID: string, turnNumber: number, segmentIndex: number, force = false): Promise<void> {
    const url = `/api/game/${encodeURIComponent(gameID)}/turn/${turnNumber}/segment/${segmentIndex}/play${force ? '?force=1' : ''}`;
    const res = await fetch(url, { method: 'POST' });
    if (!res.ok) return throwGenerationError(res);
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

  async updateGameSettings(patch: GameSettingsPatch): Promise<void> {
    return APIClient.updateGameSettings(this.gameID, patch);
  }

  async advanceUnlock(unlockID: string): Promise<Advancement> {
    return APIClient.advanceUnlock(this.gameID, unlockID);
  }

  static async advanceUnlock(gameID: string, unlockID: string): Promise<Advancement> {
    const res = await fetch(`/api/game/${encodeURIComponent(gameID)}/advance`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ unlock_id: unlockID }),
    });
    if (!res.ok) throw new Error(`advanceUnlock: ${res.statusText}`);
    const data = await res.json();
    return data.advancement as Advancement;
  }

  static async updateGameSettings(gameID: string, patch: GameSettingsPatch): Promise<void> {
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

  static async createWorld(req: CreateWorldRequest): Promise<WorldDetail> {
    const res = await fetch('/api/worlds', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    });
    if (res.status === 409) {
      const text = await res.text();
      let message = text;
      try {
        const body = JSON.parse(text) as { error?: { message?: string } };
        if (body.error?.message) message = body.error.message;
      } catch {
        // Keep the raw text when the body is not JSON.
      }
      throw new WorldExistsError(message);
    }
    if (!res.ok) throw new HTTPError(res.status, await res.text());
    return res.json();
  }

  static async updateWorld(id: string, req: CreateWorldRequest): Promise<WorldDetail> {
    const res = await fetch(`/api/world/${id}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    });
    if (!res.ok) throw new HTTPError(res.status, await res.text());
    return res.json();
  }

  static async getWorldEntity(worldId: string, entityId: string): Promise<WorldEntityDetail> {
    const res = await fetch(`/api/world/${worldId}/entity/${entityId}`);
    if (!res.ok) throw new Error(`getWorldEntity: ${res.statusText}`);
    return res.json();
  }

  static async saveWorldEntity(worldId: string, entityId: string, markdown: string, folder?: string): Promise<void> {
    const res = await fetch(`/api/world/${worldId}/entity/${entityId}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ markdown, folder: folder ?? '' }),
    });
    if (!res.ok) throw new Error((await res.text()).trim() || `saveWorldEntity: ${res.statusText}`);
  }

  static async deleteWorldEntity(worldId: string, entityId: string): Promise<void> {
    const res = await fetch(`/api/world/${worldId}/entity/${entityId}`, {
      method: 'DELETE',
    });
    if (!res.ok) throw new Error(`deleteWorldEntity: ${res.statusText}`);
  }

  static async listWorldFolders(worldId: string): Promise<FolderNode[]> {
    const res = await fetch(`/api/world/${worldId}/folders`);
    if (!res.ok) throw new Error(`listWorldFolders: ${res.statusText}`);
    return res.json();
  }

  static async createWorldFolder(worldId: string, path: string): Promise<void> {
    const res = await fetch(`/api/world/${worldId}/folders`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path }),
    });
    if (!res.ok) throw new Error((await res.text()).trim() || `createWorldFolder: ${res.statusText}`);
  }

  static async moveWorldFolder(worldId: string, from: string, to: string): Promise<void> {
    const res = await fetch(`/api/world/${worldId}/folders`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ from, path: to }),
    });
    if (!res.ok) throw new Error((await res.text()).trim() || `moveWorldFolder: ${res.statusText}`);
  }

  static async deleteWorldFolder(worldId: string, path: string, recursive = false): Promise<void> {
    const params = new URLSearchParams({ path, recursive: String(recursive) });
    const res = await fetch(`/api/world/${worldId}/folders?${params.toString()}`, {
      method: 'DELETE',
    });
    if (!res.ok) throw new Error((await res.text()).trim() || `deleteWorldFolder: ${res.statusText}`);
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

  // openURL asks the desktop window to open a link in the system browser. It
  // rejects in browser and socket mode, where there is no window to ask and the
  // caller should open a tab itself.
  static async openURL(url: string): Promise<void> {
    const res = await fetch('/api/open-url', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ url }),
    });
    if (!res.ok) throw new Error(`openURL: ${res.statusText}`);
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

  static async inspectTTS(req: TTSInspectRequest): Promise<TTSInspectResponse> {
    const res = await fetch('/api/tts/inspect', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    });
    if (!res.ok) throw new Error(`inspectTTS: ${res.statusText}`);
    return res.json();
  }

  static async listProviders(): Promise<ProviderCatalog> {
    const res = await fetch('/api/providers');
    if (!res.ok) throw new Error(`listProviders: ${res.statusText}`);
    return res.json();
  }

  static async listGeminiModels(apiKey?: string): Promise<ModelCatalogueResponse> {
    const res = await fetch('/api/providers/models', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ api_key: apiKey }),
    });
    if (!res.ok) throw new Error(`listGeminiModels: ${res.statusText}`);
    return res.json();
  }

  static async searchTTSVoices(req: VoiceSearchRequest): Promise<VoiceSearchResponse> {
    const res = await fetch('/api/tts/voices/search', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    });
    if (!res.ok) throw new Error(`searchTTSVoices: ${res.statusText}`);
    return res.json();
  }

  static async uncachedBeats(gameID: string): Promise<{ cached: number; uncached: number }> {
    const res = await fetch(`/api/game/${gameID}/tts/uncached`);
    if (!res.ok) throw new HTTPError(res.status, await res.text());
    return res.json();
  }

  // listTTSBatchJobs returns a campaign's offline batch synthesis jobs.
  static async listTTSBatchJobs(gameID: string): Promise<TTSBatchJob[]> {
    const res = await fetch(`/api/game/${encodeURIComponent(gameID)}/tts/batch`);
    if (!res.ok) throw new HTTPError(res.status, await res.text());
    return res.json();
  }

  // startTTSBatch submits an offline backfill, or returns null when every clip
  // is already cached. A failure carries the server's reason (a provider error,
  // a missing key, a network fault) rather than a bare status. When force is set
  // every clip is re-rendered, overwriting the cache. Starting while a job is
  // already in flight returns that job rather than queueing a second one.
  static async startTTSBatch(gameID: string, force = false): Promise<TTSBatchJob | null> {
    const query = force ? '?force=1' : '';
    const res = await fetch(`/api/game/${encodeURIComponent(gameID)}/tts/batch${query}`, { method: 'POST' });
    if (!res.ok) throw new HTTPError(res.status, await res.text());
    return res.json();
  }

  // listAllTTSBatchJobs returns every campaign's offline batch jobs, for the
  // global manager.
  static async listAllTTSBatchJobs(): Promise<TTSBatchJob[]> {
    const res = await fetch('/api/tts/batch');
    if (!res.ok) throw new HTTPError(res.status, await res.text());
    return res.json();
  }

  // cancelTTSBatchJob cancels a submitted batch job.
  static async cancelTTSBatchJob(gameID: string, jobID: string): Promise<void> {
    const res = await fetch('/api/tts/batch/cancel', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ game_id: gameID, job_id: jobID }),
    });
    if (!res.ok) throw new HTTPError(res.status, await res.text());
  }

  // deleteTTSBatchJob removes a finished batch job.
  static async deleteTTSBatchJob(gameID: string, jobID: string): Promise<void> {
    const res = await fetch('/api/tts/batch/delete', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ game_id: gameID, job_id: jobID }),
    });
    if (!res.ok) throw new HTTPError(res.status, await res.text());
  }

  // clearTTSBatchJobs removes every finished batch job, for one campaign or all,
  // and returns how many were removed.
  static async clearTTSBatchJobs(gameID?: string): Promise<number> {
    const res = await fetch('/api/tts/batch/clear', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ game_id: gameID ?? '' }),
    });
    if (!res.ok) throw new HTTPError(res.status, await res.text());
    const body = (await res.json()) as { removed?: number };
    return body.removed ?? 0;
  }

  // resumeTTSBatchJob polls a job now and, once it is done, downloads and stores
  // its clips. It is how a job whose output arrived empty is completed without a
  // restart.
  static async resumeTTSBatchJob(gameID: string, jobID: string): Promise<TTSBatchJob | null> {
    const res = await fetch('/api/tts/batch/resume', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ game_id: gameID, job_id: jobID }),
    });
    if (!res.ok) throw new HTTPError(res.status, await res.text());
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
    body: { mode: string; input: string; pending_check_ref?: string; scene_only?: boolean },
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
      const text = await res.text();
      if (res.status === 429) {
        let retryAfterMs: number | undefined;
        const retryHeader = res.headers.get('Retry-After');
        if (retryHeader) {
          const secs = parseInt(retryHeader, 10);
          if (!isNaN(secs) && secs > 0) retryAfterMs = secs * 1000;
        }
        onEvent({
          type: 'error',
          code: 'rate_limited',
          message: text,
          retry_after_ms: retryAfterMs,
        });
        return;
      }
      throw new Error(`streamTurn: ${res.status} ${text}`);
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
    if (!res.ok) throw new HTTPError(res.status, `getGameState: ${res.statusText}`);
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

  async getEntityFrontmatterSchema(): Promise<FrontmatterSchema> {
    const res = await fetch('/api/schema/entity-frontmatter');
    if (!res.ok) throw new Error(`getEntityFrontmatterSchema: ${res.statusText}`);
    return res.json();
  }

  async getEntityTypes(): Promise<EntityTypeCatalog> {
    const res = await fetch('/api/schema/entity-types');
    if (!res.ok) throw new Error(`getEntityTypes: ${res.statusText}`);
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

  async saveEntity(entityID: string, markdown: string, folder?: string): Promise<void> {
    const res = await fetch(`/api/game/${this.gameID}/entity/${entityID}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ markdown, folder: folder ?? '' })
    });
    if (!res.ok) {
      const body = await res.text();
      let parsed: SaveErrorBody | null = null;
      try {
        parsed = JSON.parse(body) as SaveErrorBody;
      } catch {
        parsed = null;
      }
      const error = new Error(parsed?.error ?? (body.trim() || `saveEntity: ${res.statusText}`));
      if (parsed?.line) {
        (error as Error & { line?: number }).line = parsed.line;
      }
      throw error;
    }
  }

  async listFolders(): Promise<FolderNode[]> {
    const res = await fetch(`/api/game/${this.gameID}/folders`);
    if (!res.ok) throw new Error(`listFolders: ${res.statusText}`);
    return res.json();
  }

  async createFolder(path: string): Promise<void> {
    const res = await fetch(`/api/game/${this.gameID}/folders`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path }),
    });
    if (!res.ok) throw new Error((await res.text()).trim() || `createFolder: ${res.statusText}`);
  }

  async moveFolder(from: string, to: string): Promise<void> {
    const res = await fetch(`/api/game/${this.gameID}/folders`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ from, path: to }),
    });
    if (!res.ok) throw new Error((await res.text()).trim() || `moveFolder: ${res.statusText}`);
  }

  async deleteFolder(path: string, recursive = false): Promise<void> {
    const params = new URLSearchParams({ path, recursive: String(recursive) });
    const res = await fetch(`/api/game/${this.gameID}/folders?${params.toString()}`, {
      method: 'DELETE',
    });
    if (!res.ok) throw new Error((await res.text()).trim() || `deleteFolder: ${res.statusText}`);
  }

  async mergeEntity(sourceID: string, intoID: string): Promise<EntityNote> {
    const res = await fetch(`/api/game/${this.gameID}/entity/${sourceID}/merge`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ into: intoID, confirm: true }),
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

  static async getTurnContext(gameID: string): Promise<TurnContext> {
    const res = await fetch(`/api/game/${gameID}/context`);
    if (!res.ok) throw new Error(`getTurnContext: ${res.statusText}`);
    return res.json();
  }

  static async getWorkingSet(gameID: string): Promise<WorkingEntry[]> {
    const res = await fetch(`/api/game/${gameID}/working-set`);
    if (!res.ok) throw new Error(`getWorkingSet: ${res.statusText}`);
    return res.json();
  }

  async getTurnContext(): Promise<TurnContext> {
    return APIClient.getTurnContext(this.gameID);
  }

  async getWorkingSet(): Promise<WorkingEntry[]> {
    return APIClient.getWorkingSet(this.gameID);
  }

  static async getGameUsage(gameID: string): Promise<Usage> {
    const res = await fetch(`/api/game/${encodeURIComponent(gameID)}/usage`);
    if (!res.ok) throw new Error(`getGameUsage: ${res.statusText}`);
    return res.json();
  }

  static async getGlobalUsage(): Promise<Usage> {
    const res = await fetch('/api/usage');
    if (!res.ok) throw new Error(`getGlobalUsage: ${res.statusText}`);
    return res.json();
  }

  static async getLimits(): Promise<LimitsDTO> {
    const res = await fetch('/api/limits');
    if (!res.ok) throw new Error(`getLimits: ${res.statusText}`);
    return res.json();
  }

  static async getDocsList(): Promise<DocArticleSummary[]> {
    const res = await fetch('/api/docs');
    if (!res.ok) throw new HTTPError(res.status, `getDocsList: ${res.statusText}`);
    return res.json();
  }

  static async getDocArticle(id: string): Promise<DocArticle> {
    const res = await fetch(`/api/docs/${encodeURIComponent(id)}`);
    if (!res.ok) throw new HTTPError(res.status, `getDocArticle: ${res.statusText}`);
    return res.json();
  }

  async getGameUsage(): Promise<Usage> {
    return APIClient.getGameUsage(this.gameID);
  }
}

