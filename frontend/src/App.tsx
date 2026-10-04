import React, { useState, useEffect, useMemo, useRef, useCallback, Suspense, lazy } from 'react';
import { APIClient, HTTPError, GenerationError } from './api/client';
import { GameState, Turn, TurnSegment, EntityNote, EntitySummary, Recap, GraphData, AppConfig, LimitState, AudioProgressEvent } from './types';
import { ChronicleView } from './components/ChronicleView';
import { TurnSegments } from './components/TurnSegments';
import { TurnAudioState, segmentAudioKey } from './components/TurnSegments';
import { ActionConsole } from './components/ActionConsole';
import { MechanicsStrip } from './components/MechanicsStrip';
import { Drawers } from './components/Drawers';
import { CharacterSheetDrawer } from './components/CharacterSheetDrawer';
import { GraphDrawer } from './components/GraphDrawer';
import { CodexDrawer } from './components/CodexDrawer';
import { LivingWorldDrawer } from './components/LivingWorldDrawer';
import { ContextDrawer } from './components/ContextDrawer';
import { LauncherHub } from './components/LauncherHub';
import { ProloguePanel } from './components/ProloguePanel';
import { AddEntityModal } from './components/AddEntityModal';
import { ModelDownloadModal } from './components/ModelDownloadModal';
import { LimitChip } from './components/LimitChip';
import { User, Network, BookOpen, Clock, Film, Compass, Settings, X, Layers, AlertTriangle, HelpCircle, Download } from 'lucide-react';
import { formatGenerationError } from './lib/generationError';
import { useStreamedSpeech } from './hooks/useStreamedSpeech';
import { TurnStreamProcessor } from './lib/turnStreamProcessor';
import { slugify } from './lib/slug';
import { CinematicOverlay } from './components/CinematicOverlay';
import { MenuBar } from './components/MenuBar';
import { AboutModal } from './components/AboutModal';

// Everything below the campaign shell is only needed once its overlay is
// opened, so it loads on demand instead of in the first bundle. The chronicle,
// its drawers, and the launcher stay eager because they are the first paint.
const DocsModal = lazy(() => import('./components/DocsModal').then((m) => ({ default: m.DocsModal })));
const ContentStudio = lazy(() => import('./components/ContentStudio'));
const SettingsStudio = lazy(() => import('./components/SettingsStudio').then((m) => ({ default: m.SettingsStudio })));
const StoryTheater = lazy(() => import('./components/StoryTheater').then((m) => ({ default: m.StoryTheater })));
const ExportModal = lazy(() => import('./components/ExportModal').then((m) => ({ default: m.ExportModal })));

export const App: React.FC = () => {
  // Always open in the launcher hub view rather than directly entering a campaign.
  const [activeGameID, setActiveGameID] = useState<string | null>(null);

  const client = useMemo(() => {
    return activeGameID ? new APIClient(activeGameID) : null;
  }, [activeGameID]);

  const [gameState, setGameState] = useState<GameState | null>(null);
  const [chronicle, setChronicle] = useState<Turn[]>([]);
  const [graph, setGraph] = useState<GraphData | null>(null);
  // The campaign's notes, as the codex lists them. It grows as turns create
  // entities, so it is refetched rather than held from first load.
  const [entities, setEntities] = useState<EntitySummary[]>([]);
  // A campaign's long memory, read with the rest of the corpus so the panel shows
  // it without a model call of its own.
  const [recap, setRecap] = useState<Recap | null>(null);
  const [selectedEntity, setSelectedEntity] = useState<EntityNote | null>(null);
  // TTS playback preferences live in the global settings, so the chronicle and
  // the story theater honour the same switches as the settings studio.
  const [config, setConfig] = useState<AppConfig | null>(null);

  // Active drawer tab: null, 'character', 'graph', 'codex', 'world'
  // The application plays audio itself when it can, which is the only way to
  // narrate a turn without a browser autoplay gesture.
  const [serverAudio, setServerAudio] = useState(false);
  // Per-turn audio status: generating → playing → idle (or error)
  const [turnAudioStatus, setTurnAudioStatus] = useState<Record<number, { state: TurnAudioState; message?: string }>>({});
  // Per-segment audio status, keyed `${turn}:${index}`.
  const [segmentAudioStatus, setSegmentAudioStatus] = useState<Record<string, { state: TurnAudioState; message?: string }>>({});
  // Narration plays while the turn streams when the browser owns the sound. The
  // clips already heard are handed to the chronicle so nothing repeats.
  const streamedSpeech = useStreamedSpeech(
    !serverAudio && (config?.media.tts.auto_play ?? false),
    config?.media.tts.master_volume ?? 1
  );
  const [streamedKeys, setStreamedKeys] = useState<ReadonlySet<string>>(new Set());
  const [audioProgress, setAudioProgress] = useState<AudioProgressEvent | null>(null);
  const [segmentAudioProgress, setSegmentAudioProgress] = useState<Record<number, string>>({});
  const [characterPortraits, setCharacterPortraits] = useState<Record<string, { url: string; hasCustom: boolean }>>({});

  const [activeDrawer, setActiveDrawer] = useState<string | null>(null);
  const [isTheaterOpen, setIsTheaterOpen] = useState(false);
  const [isExportOpen, setIsExportOpen] = useState(false);
  const [isSettingsOpen, setIsSettingsOpen] = useState(false);
  const [isDocsOpen, setIsDocsOpen] = useState(false);
  const [isContentStudioOpen, setIsContentStudioOpen] = useState(false);
  const [docsArticleID, setDocsArticleID] = useState<string | undefined>(undefined);
  const [isAboutOpen, setIsAboutOpen] = useState(false);
  // The build version the About dialog reports, read from the settings endpoint.
  const [appVersion, setAppVersion] = useState<string | undefined>(undefined);
  const [addressed, setAddressed] = useState<Set<number>>(new Set());
  const [modalEntity, setModalEntity] = useState<{ name: string; turnNumber: number } | null>(null);
  const [missingModel, setMissingModel] = useState<{ id: string; name: string; sizeBytes: number } | null>(null);
  const [dismissedModelPrompt, setDismissedModelPrompt] = useState(false);
  // The remembered campaign is only a hint until it is verified against the
  // campaign list, so the play shell can tell "loading" from "gone".
  const [campaignStatus, setCampaignStatus] = useState<'idle' | 'loading' | 'ready' | 'unavailable'>('idle');
  const [campaignError, setCampaignError] = useState<string | null>(null);
  // Advancement spending: the drawer disables its buttons and shows the refusal.
  const [advancementSpending, setAdvancementSpending] = useState(false);
  const [advancementError, setAdvancementError] = useState<string | null>(null);

  // Rate limits & funds failures
  const [limits, setLimits] = useState<LimitState[]>([]);
  const [rateLimitUntil, setRateLimitUntil] = useState<number | null>(null);
  const [fundsError, setFundsError] = useState<{ provider?: string; message: string } | null>(null);
  const [now, setNow] = useState<number>(Date.now());

  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, []);

  const fetchLimits = useCallback(async () => {
    try {
      const res = await APIClient.getLimits();
      setLimits(res.blocks || []);
      const fundsBlock = res.blocks?.find((b) => b.funds_failure);
      if (fundsBlock && fundsBlock.funds_failure) {
        setFundsError({
          provider: fundsBlock.provider,
          message: fundsBlock.funds_failure,
        });
      }
    } catch (err) {
      console.error('Failed to poll limits:', err);
    }
  }, []);

  useEffect(() => {
    fetchLimits();
    const interval = setInterval(fetchLimits, 15000);
    return () => clearInterval(interval);
  }, [fetchLimits]);

  const activeRateLimitSeconds = useMemo(() => {
    if (rateLimitUntil && rateLimitUntil > now) {
      return Math.ceil((rateLimitUntil - now) / 1000);
    }
    const gmBlock = limits.find(
      (b) => (b.role === 'gm' || b.role === 'extractor') && b.until && new Date(b.until).getTime() > now
    );
    if (gmBlock && gmBlock.until) {
      return Math.ceil((new Date(gmBlock.until).getTime() - now) / 1000);
    }
    return 0;
  }, [rateLimitUntil, limits, now]);

  const isRateLimited = activeRateLimitSeconds > 0;

  useEffect(() => {
    APIClient.getSettings()
      .then((res) => {
        setConfig(res.config);
        setAppVersion(res.app_version);
      })
      .catch(console.error);
    APIClient.audioStatus()
      .then((status) => setServerAudio(status.available))
      .catch(() => setServerAudio(false));
  }, []);

  const handleGameStateFailure = useCallback((err: unknown) => {
    if (err instanceof HTTPError && err.status === 404) {
      // The campaign is definitively gone: forget it and return to the launcher.
      localStorage.removeItem('localrpg_active_game');
      localStorage.removeItem('localrpg_last_played_game');
      setActiveGameID(null);
      setCampaignStatus('idle');
      setCampaignError(null);
      return;
    }
    // A transient failure must not erase where the player was.
    setCampaignStatus('unavailable');
    setCampaignError(err instanceof Error ? err.message : String(err));
  }, []);

  // Everything the drawers read is refetched together, so no panel can show a
  // corpus that is older than another's.
  const refreshCorpus = useCallback(() => {
    if (!client) return;
    client.getGameState()
      .then((state) => {
        setGameState(state);
        setCampaignStatus('ready');
      })
      .catch(handleGameStateFailure);
    client.getGraph().then(setGraph).catch(console.error);
    client.listEntities().then(setEntities).catch(console.error);
    client.getRecap().then(setRecap).catch(console.error);
  }, [client, handleGameStateFailure]);

  // A spend applies through the API and then refreshes the corpus, so the
  // drawer and the notification dot both reflect the new balance.
  const handleSpendAdvancement = useCallback(async (unlockID: string) => {
    if (!client) return;
    setAdvancementSpending(true);
    setAdvancementError(null);
    try {
      await client.advanceUnlock(unlockID);
      refreshCorpus();
    } catch (err) {
      setAdvancementError(err instanceof Error ? err.message : String(err));
    } finally {
      setAdvancementSpending(false);
    }
  }, [client, refreshCorpus]);

  // A campaign deletion, or a change of --dir, leaves a dangling id in storage.
  // It is verified against the campaign list before it is trusted; a list that
  // cannot be fetched leaves the selection alone and offers a retry instead.
  useEffect(() => {
    if (!activeGameID) {
      setCampaignStatus('idle');
      return;
    }
    let cancelled = false;
    setCampaignStatus('loading');
    setCampaignError(null);
    APIClient.listGames()
      .then((games) => {
        if (cancelled) return;
        if (games.some((game) => game.id === activeGameID)) {
          return;
        }
        localStorage.removeItem('localrpg_active_game');
        localStorage.removeItem('localrpg_last_played_game');
        setActiveGameID(null);
        setCampaignStatus('idle');
      })
      .catch((err) => {
        if (cancelled) return;
        setCampaignStatus('unavailable');
        setCampaignError(err instanceof Error ? err.message : String(err));
      });
    return () => {
      cancelled = true;
    };
  }, [activeGameID]);

  useEffect(() => {
    if (!client) {
      setGameState(null);
      setChronicle([]);
      setGraph(null);
      setEntities([]);
      setRecap(null);
      setAddressed(new Set());
      setAudioProgress(null);
      setSegmentAudioProgress({});
      setCharacterPortraits({});
      return;
    }
    client.getChronicle().then(setChronicle).catch(console.error);
    client.getFindings().then((res) => {
      setAddressed(new Set(res.addressed.map((a) => a.turn)));
    }).catch(console.error);
    refreshCorpus();
  }, [client, refreshCorpus]);

  useEffect(() => {
    if (chronicle.length === 0) return;
    const portraits: Record<string, { url: string; hasCustom: boolean }> = {};
    for (const turn of chronicle) {
      for (const seg of turn.segments || []) {
        const charId = seg.speaker_id || (seg.speaker ? slugify(seg.speaker) : undefined);
        if (charId && seg.portrait_url) {
          if (!portraits[charId] || seg.has_custom_portrait) {
            portraits[charId] = {
              url: seg.has_custom_portrait && !seg.portrait_url.includes('?') ? `${seg.portrait_url}?t=${Date.now()}` : seg.portrait_url,
              hasCustom: !!seg.has_custom_portrait,
            };
          }
        }
      }
    }
    setCharacterPortraits((prev) => {
      const next = { ...prev };
      let changed = false;
      for (const [id, p] of Object.entries(portraits)) {
        if (!next[id] || (!next[id].hasCustom && p.hasCustom)) {
          next[id] = p;
          changed = true;
        }
      }
      return changed ? next : prev;
    });
  }, [chronicle]);

  useEffect(() => {
    if (entities.length === 0) return;
    const portraits: Record<string, { url: string; hasCustom: boolean }> = {};
    for (const ent of entities) {
      if (ent.type === 'character' && ent.has_portrait) {
        portraits[ent.id] = {
          url: ent.portrait_url ? `${ent.portrait_url}?t=${Date.now()}` : `/api/game/${encodeURIComponent(activeGameID || '')}/character/${encodeURIComponent(ent.id)}/portrait?t=${Date.now()}`,
          hasCustom: true,
        };
      }
    }
    if (Object.keys(portraits).length > 0) {
      setCharacterPortraits((prev) => {
        const next = { ...prev };
        let changed = false;
        for (const [id, p] of Object.entries(portraits)) {
          if (!next[id] || (!next[id].hasCustom && p.hasCustom)) {
            next[id] = p;
            changed = true;
          }
        }
        return changed ? next : prev;
      });
    }
  }, [entities, activeGameID]);

  // Poll for background portrait completion when characters in chronicle lack custom portraits
  useEffect(() => {
    if (!client || !activeGameID || chronicle.length === 0) return;

    const hasPendingPortraits = chronicle.some((turn) =>
      (turn.segments || []).some((seg) => {
        if (seg.kind !== 'speech') return false;
        const charId = seg.speaker_id || (seg.speaker ? slugify(seg.speaker) : undefined);
        return charId && (!characterPortraits[charId] || !characterPortraits[charId].hasCustom);
      })
    );

    if (!hasPendingPortraits) return;

    let attempts = 0;
    const maxAttempts = 10;
    const interval = setInterval(() => {
      attempts++;
      client.listEntities()
        .then((ents) => {
          setEntities(ents);
          const allResolved = chronicle.every((turn) =>
            (turn.segments || []).every((seg) => {
              if (seg.kind !== 'speech') return true;
              const charId = seg.speaker_id || (seg.speaker ? slugify(seg.speaker) : undefined);
              if (!charId) return true;
              const matchingEnt = ents.find((e) => e.id === charId);
              return matchingEnt?.has_portrait;
            })
          );
          if (allResolved || attempts >= maxAttempts) {
            clearInterval(interval);
          }
        })
        .catch(console.error);
    }, 3000);

    return () => clearInterval(interval);
  }, [client, activeGameID, chronicle, characterPortraits]);

  const handleSelectGame = (gameId: string) => {
    localStorage.setItem('localrpg_last_played_game', gameId);
    localStorage.removeItem('localrpg_active_game');
    setActiveGameID(gameId);
  };

  const handleReturnToLauncher = () => {
    setActiveGameID(null);
    setActiveDrawer(null);
    setIsSettingsOpen(false);
    setCampaignStatus('idle');
    setCampaignError(null);
  };

  const handleOpenWikilink = async (entityId: string) => {
    if (!client) return;
    try {
      const ent = await client.getEntity(entityId);
      setSelectedEntity(ent);
      setActiveDrawer('codex');
    } catch (err) {
      console.error(err);
    }
  };

  const [turnInFlight, setTurnInFlight] = useState(false);
  const [pendingAction, setPendingAction] = useState<{ mode: string; text: string } | null>(null);
  const [streamedSegments, setStreamedSegments] = useState<TurnSegment[]>([]);
  const [toolActivity, setToolActivity] = useState<string | null>(null);
  const [turnError, setTurnError] = useState<string | null>(null);
  const abortRef = useRef<AbortController | null>(null);
  const streamProcessorRef = useRef<TurnStreamProcessor>(new TurnStreamProcessor());

  const handleActionSubmit = async (mode: string, text: string, pendingCheckRef?: string) => {
    if (!client || !activeGameID || turnInFlight) return;

    setTurnInFlight(true);
    setPendingAction({ mode, text });
    streamProcessorRef.current.reset();
    setStreamedSegments([]);
    setToolActivity(null);
    setTurnError(null);
    setAudioProgress(null);
    setSegmentAudioProgress({});
    streamedSpeech.reset();
    setStreamedKeys(new Set());

    const controller = new AbortController();
    abortRef.current = controller;

    try {
      await APIClient.streamTurn(
        activeGameID,
        { mode, input: text, pending_check_ref: pendingCheckRef },
        (event) => {
          if (event.type === 'chunk') {
            setToolActivity(null);
            if (event.text) {
              setStreamedSegments(streamProcessorRef.current.feedChunk(event.text));
            }
          } else if (event.type === 'segment' && event.segment) {
            setStreamedSegments(streamProcessorRef.current.feedSegment(event.segment));
          } else if (event.type === 'speech') {
            // A sentence the server synthesized mid-stream, played here while the
            // rest of the prose is still arriving.
            streamedSpeech.enqueue(event.audio_url ?? '', event.audio_key ?? '', event.index);
          } else if (event.type === 'audio_progress' && event.audio_progress) {
            setAudioProgress(event.audio_progress);
            if (event.audio_progress.sequence !== undefined && event.audio_progress.stage) {
              setSegmentAudioProgress((prev) => ({
                ...prev,
                [event.audio_progress!.sequence]: event.audio_progress!.stage,
              }));
            }
          } else if (event.type === 'portrait' && event.character_id) {
            setCharacterPortraits((prev) => ({
              ...prev,
              [event.character_id!]: {
                url: event.portrait_url || '',
                hasCustom: !!event.has_custom_portrait,
              },
            }));
          } else if (event.type === 'scene_image' && event.turn_number && event.image_url) {
            setChronicle((prev) =>
              prev.map((turn) =>
                turn.turn_number === event.turn_number
                  ? { ...turn, image_url: event.image_url }
                  : turn
              )
            );
          } else if (event.type === 'tool') {
            setToolActivity(
              event.tool_status === 'running'
                ? `${event.tool_name}...`
                : `${event.tool_name}: ${event.tool_summary ?? 'done'}`,
            );
          } else if (event.type === 'turn' && event.turn) {
            // Restore interactivity immediately so the user can submit the next turn
            // while any remaining audio synthesizes in the background.
            setTurnInFlight(false);
            setToolActivity(null);

            // Stop streamed speech so it does not overlap with chronicle playback,
            // and remember which keys were heard to completion.
            streamedSpeech.stop();
            setStreamedKeys(streamedSpeech.playedKeys());
            const turn = event.turn;
            setChronicle((prev) => [...prev, turn]);
            streamProcessorRef.current.reset();
            setStreamedSegments([]);
            // The turn now carries the action, so drop the pending block at once;
            // otherwise the action shows twice until the stream closes.
            setPendingAction(null);
            setFundsError(null);
            setRateLimitUntil(null);
            fetchLimits();
            // A turn can introduce characters, so the graph and the character
            // view are refreshed rather than left showing the state before it.
            refreshCorpus();
          } else if (event.type === 'model_missing') {
            if (!dismissedModelPrompt) {
              setMissingModel({
                id: event.model_id || 'kokoro-tts',
                name: event.name || 'Kokoro Voice Pack',
                sizeBytes: event.size_bytes || 90177536,
              });
            }
          } else if (event.type === 'error') {
            const reason = event.failure
              ? formatGenerationError(event.failure)
              : (event.detail || event.message || 'The turn failed.');
            setTurnError(reason);
            if (event.code === 'rate_limited' || event.failure?.code === 'rate_limited') {
              const retryMs = event.retry_after_ms ?? event.failure?.retry_after_ms ?? 30000;
              setRateLimitUntil(Date.now() + retryMs);
              fetchLimits();
            } else if (event.code === 'insufficient_funds' || event.failure?.code === 'insufficient_funds') {
              const provider = event.failure?.attempts?.[0]?.provider || 'AI';
              setFundsError({
                provider,
                message: event.failure?.message || event.detail || event.message || 'Insufficient funds/credits',
              });
              fetchLimits();
            }
            console.error('turn failed:', event.message);
          }
        },
        controller.signal
      );
    } catch (err) {
      console.error('turn failed:', err);
      setTurnError(err instanceof Error ? err.message : String(err));
    } finally {
      if (abortRef.current === controller) {
        abortRef.current = null;
        setTurnInFlight(false);
        setPendingAction(null);
        setToolActivity(null);
        streamedSpeech.stop();
        setAudioProgress((prev) => (prev && prev.ready_count + (prev.failed_count || 0) < prev.total_segments ? null : prev));
        setSegmentAudioProgress({});
      }
    }
  };

  const handleStopTurn = () => {
    abortRef.current?.abort();
    streamedSpeech.stop();
    streamProcessorRef.current.reset();
    setStreamedSegments([]);
    setPendingAction(null);
  };

  // Beginning the story saves the player's opening prompt and then runs the
  // campaign's first turn in the reserved Opening mode, so the GM establishes
  // the scene before the player is asked for anything.
  const handleBeginStory = async (prompt: string) => {
    if (!client || !activeGameID || turnInFlight) return;
    try {
      await APIClient.updateGameSettings(activeGameID, { opening_prompt: prompt });
      setGameState((prev) => (prev ? { ...prev, opening_prompt: prompt } : prev));
    } catch (err) {
      console.error('save opening prompt:', err);
    }
    await handleActionSubmit('Opening', '');
  };

  const handleBeginWithAction = () => {
    document.getElementById('action-console-input')?.focus();
  };

  const audioPollingRef = useRef<Record<string, ReturnType<typeof setInterval>>>({});

  const handlePlayTurnAudio = (turnNumber: number, segmentIndex?: number, force = false) => {
    if (!activeGameID) return;
    const key = segmentIndex === undefined ? null : segmentAudioKey(turnNumber, segmentIndex);
    const setStatus = (entry: { state: TurnAudioState; message?: string }) => {
      if (key === null) {
        setTurnAudioStatus((prev) => ({ ...prev, [turnNumber]: entry }));
      } else {
        setSegmentAudioStatus((prev) => ({ ...prev, [key]: entry }));
      }
    };

    // Transition to 'generating' immediately so the spinner shows
    setStatus({ state: 'generating' });

    const call = segmentIndex === undefined
      ? APIClient.playTurnAudio(activeGameID, turnNumber, force)
      : APIClient.playSegmentAudio(activeGameID, turnNumber, segmentIndex, force);

    const pollKey = key ?? `turn:${turnNumber}`;

    call
      .then(() => {
        setStatus({ state: 'playing' });
        // Poll until server reports playback finished
        const intervalId = setInterval(() => {
          APIClient.audioStatus()
            .then((status) => {
              if (!status.playing) {
                clearInterval(intervalId);
                delete audioPollingRef.current[pollKey];
                setStatus({ state: 'idle' });
              }
            })
            .catch(() => {
              clearInterval(intervalId);
              delete audioPollingRef.current[pollKey];
              setStatus({ state: 'idle' });
            });
        }, 500);
        audioPollingRef.current[pollKey] = intervalId;
      })
      .catch((err: unknown) => {
        const message =
          err instanceof GenerationError ? err.failure.message : err instanceof Error ? err.message : String(err);
        setStatus({ state: 'error', message });
      });
  };

  const handleStopAudio = () => {
    APIClient.stopAudio().catch(console.error);
    // Clear all polling and reset all turns that are playing
    for (const [key, intervalId] of Object.entries(audioPollingRef.current)) {
      clearInterval(intervalId);
      delete audioPollingRef.current[key];
    }
    setTurnAudioStatus((prev) => {
      const next = { ...prev };
      for (const key of Object.keys(next)) {
        if (next[Number(key)].state === 'playing' || next[Number(key)].state === 'generating') {
          next[Number(key)] = { state: 'idle' };
        }
      }
      return next;
    });
    setSegmentAudioStatus({});
  };

  const handleSaveEntity = async (entityId: string, markdown: string) => {
    if (!client) return;
    await client.saveEntity(entityId, markdown);
    const updated = await client.getEntity(entityId);
    // Only reselect when the player is still looking at the note that was saved;
    // a late response must not yank them onto a different note.
    setSelectedEntity((current) => (current && current.id === entityId ? updated : current));
    // A saved note can change its own type, links, or name, so the graph and the
    // codex listing follow it rather than going stale.
    refreshCorpus();
  };

  const handleMergeEntity = async (sourceID: string, intoID: string) => {
    if (!client || !activeGameID) return;

    try {
      const merged = await client.mergeEntity(sourceID, intoID);
      setSelectedEntity(merged);
      refreshCorpus();
    } catch (err) {
      console.error('merge failed:', err);
    }
  };

  const parseMissingEntityName = (note: string): string | null => {
    const match = note.match(/"([^"]+)" (?:speaks but has no note|is named but has no note|speaks but is not a known character)/i);
    return match ? match[1] : null;
  };

  const handleCorrect = (note: string, turnNumber?: number) => {
    const entityName = parseMissingEntityName(note);
    if (entityName && turnNumber !== undefined) {
      setModalEntity({ name: entityName, turnNumber });
    } else {
      const input = document.getElementById('action-console-input') as HTMLInputElement | null;
      if (input) {
        input.value = `/gm ${note}`;
        input.focus();
      }
    }
  };

  const handleQuickCreateEntity = async (name: string, type: string) => {
    if (!client || !modalEntity) return;
    const slug = slugify(name);
    try {
      const existing = entities.find((candidate) => candidate.id === slug);
      if (existing) {
        // Never overwrite a note that already exists; open it instead.
        await client.addressFinding(modalEntity.turnNumber, 'continuity');
        setAddressed((prev) => new Set(prev).add(modalEntity.turnNumber));
        const entity = await client.getEntity(slug);
        setSelectedEntity(entity);
      } else {
        const template = `---\nid: ${slug}\nname: ${name}\ntype: ${type}\n---\n\n`;
        await client.saveEntity(slug, template);
        await client.addressFinding(modalEntity.turnNumber, 'continuity');
        setAddressed((prev) => new Set(prev).add(modalEntity.turnNumber));
        refreshCorpus();
      }
    } catch (err) {
      console.error('quick create entity failed:', err);
    } finally {
      setModalEntity(null);
    }
  };

  const handleEditInCodexEntity = async (name: string, type: string) => {
    if (!client || !modalEntity) return;
    const slug = slugify(name);
    try {
      const existing = entities.find((candidate) => candidate.id === slug);
      if (!existing) {
        const template = `---\nid: ${slug}\nname: ${name}\ntype: ${type}\n---\n\n`;
        await client.saveEntity(slug, template);
      }
      await client.addressFinding(modalEntity.turnNumber, 'continuity');
      setAddressed((prev) => new Set(prev).add(modalEntity.turnNumber));
      refreshCorpus();
      const entity = await client.getEntity(slug);
      setSelectedEntity(entity);
      setActiveDrawer('codex');
    } catch (err) {
      console.error('edit in codex entity failed:', err);
    } finally {
      setModalEntity(null);
    }
  };

  const handleAddress = (turnNumber: number) => {
    client?.addressFinding(turnNumber, 'continuity').catch(console.error);
    setAddressed((prev) => new Set(prev).add(turnNumber));
  };

  // Find latest scene image for full-window atmospheric background, falling back to campaign banner
  const activeBgImage = chronicle.slice().reverse().find((t) => t.image_url)?.image_url || gameState?.banner_url;

  // A pending check the GM proposed under the ask policy, awaiting the player's roll.
  const pendingCheck = chronicle.length > 0 ? chronicle[chronicle.length - 1].pending_check : undefined;
  const lastTurnChecks = chronicle.length > 0 ? chronicle[chronicle.length - 1].checks ?? [] : [];

  // Open the built-in docs, optionally jumping straight to an article. Callers
  // that explain a setting (for example an unpriced usage row) link a reader to
  // the exact reference instead of leaving them to search.
  const openDocs = (articleID?: string) => {
    setDocsArticleID(articleID);
    setIsDocsOpen(true);
  };

  return (
    <div className="relative flex flex-col h-screen overflow-hidden text-stone-200">
      {/* Full-window atmospheric background layer */}
      <div
        id="app-bg"
        key={activeBgImage || 'default'}
        className="fixed inset-0 bg-cover bg-center bg-no-repeat animate-bg-fade-in transition-all duration-700 pointer-events-none"
        style={{
          backgroundImage: activeBgImage ? `url(${activeBgImage})` : 'radial-gradient(ellipse at center, #261e1b 0%, #0c0a09 100%)',
          backgroundColor: '#0c0a09',
        }}
      />

      {/* Cinematic dark overlay letting the banner shine through cleanly without film grain */}
      <div className="fixed inset-0 bg-gradient-to-b from-black/60 via-black/40 to-black/70 pointer-events-none" />

      {/* When no game is selected: Mount the Launcher Hub */}
      <div
        key={activeGameID ? 'play' : 'launcher'}
        className="relative flex-1 flex flex-col min-h-0 anim-fade-in"
      >
      {!activeGameID ? (
        <LauncherHub onSelectGame={handleSelectGame} />
      ) : (
        <>
          {/* Floating Translucent Acrylic Header */}
          <header className="relative z-10 mx-6 mt-4 mb-2 h-14 bg-glass rounded-2xl px-6 flex items-center justify-between shadow-2xl">
            <div className="flex items-center gap-4">
              <button
                onClick={handleReturnToLauncher}
                className="flex items-center gap-1.5 text-xs font-sans font-semibold px-3 py-1.5 rounded-xl transition-all cursor-pointer bg-white/[0.05] hover:bg-white/[0.1] text-stone-200 border border-white/10"
                title="Switch Campaign / Return to Hub"
              >
                <Compass className="w-3.5 h-3.5" />
                <span>Back</span>
              </button>

              <div className="flex items-center gap-2.5">
                <div className="w-2 h-2 rounded-full bg-purple-500 shadow-[0_0_8px_rgba(168,85,247,0.8)]" />
                <h1 className="font-sans text-base md:text-lg font-bold text-white tracking-tight">
                  {gameState?.game_name || activeGameID}
                </h1>
                {gameState?.mechanics_engagement && (
                  <span className="text-xs font-sans uppercase tracking-wider px-2 py-0.5 rounded-full border border-white/10 bg-white/[0.05] text-stone-400">
                    mechanics: {gameState.mechanics_engagement}
                  </span>
                )}
                {limits.map((block, idx) => (
                  <LimitChip key={idx} block={block} />
                ))}
              </div>
            </div>

            {/* Floating Drawer Trigger Pills */}
            <div className="flex items-center gap-2">
              <button
                onClick={() => setActiveDrawer('character')}
                className={`relative flex items-center gap-1.5 text-xs font-sans px-3 py-1.5 rounded-xl transition-all cursor-pointer ${
                  activeDrawer === 'character' ? 'bg-purple-600 text-white font-bold shadow-md' : 'text-stone-300 hover:text-white hover:bg-white/10'
                }`}
              >
                <User className="w-3.5 h-3.5" />
                <span className="hidden sm:inline">Character</span>
                {(gameState?.advancement?.pending ||
                  gameState?.advancement?.unlocks?.some((u) => u.affordable && u.requires_met && u.gate_open)) && (
                  <span className="absolute -top-0.5 -right-0.5 w-2 h-2 rounded-full bg-purple-400 shadow-[0_0_6px_rgba(168,85,247,0.9)]" />
                )}
              </button>
              <button
                onClick={() => setActiveDrawer('graph')}
                className={`flex items-center gap-1.5 text-xs font-sans px-3 py-1.5 rounded-xl transition-all cursor-pointer ${
                  activeDrawer === 'graph' ? 'bg-purple-600 text-white font-bold shadow-md' : 'text-stone-300 hover:text-white hover:bg-white/10'
                }`}
              >
                <Network className="w-3.5 h-3.5" />
                <span className="hidden sm:inline">Graph</span>
              </button>
              <button
                onClick={() => setActiveDrawer('codex')}
                className={`flex items-center gap-1.5 text-xs font-sans px-3 py-1.5 rounded-xl transition-all cursor-pointer ${
                  activeDrawer === 'codex' ? 'bg-purple-600 text-white font-bold shadow-md' : 'text-stone-300 hover:text-white hover:bg-white/10'
                }`}
              >
                <BookOpen className="w-3.5 h-3.5" />
                <span className="hidden sm:inline">Codex</span>
              </button>
              <button
                onClick={() => setActiveDrawer('world')}
                className={`flex items-center gap-1.5 text-xs font-sans px-3 py-1.5 rounded-xl transition-all cursor-pointer ${
                  activeDrawer === 'world' ? 'bg-purple-600 text-white font-bold shadow-md' : 'text-stone-300 hover:text-white hover:bg-white/10'
                }`}
              >
                <Clock className="w-3.5 h-3.5" />
                <span className="hidden sm:inline">World Arcs</span>
              </button>
              <button
                onClick={() => setActiveDrawer('context')}
                className={`flex items-center gap-1.5 text-xs font-sans px-3 py-1.5 rounded-xl transition-all cursor-pointer ${
                  activeDrawer === 'context' ? 'bg-purple-600 text-white font-bold shadow-md' : 'text-stone-300 hover:text-white hover:bg-white/10'
                }`}
                title="Inspect prompt context sections and continuity working set"
              >
                <Layers className="w-3.5 h-3.5" />
                <span className="hidden sm:inline">Context</span>
              </button>
              <button
                onClick={() => setIsTheaterOpen(true)}
                className="flex items-center gap-1.5 text-xs font-sans px-3 py-1.5 rounded-xl transition-all cursor-pointer text-stone-300 hover:text-white hover:bg-white/10"
                title="Open Story Theater replay mode"
              >
                <Film className="w-3.5 h-3.5 text-purple-400" />
                <span className="hidden sm:inline">Theater</span>
              </button>
              <button
                onClick={() => setIsExportOpen(true)}
                className="flex items-center gap-1.5 text-xs font-sans px-3 py-1.5 rounded-xl transition-all cursor-pointer text-stone-300 hover:text-white hover:bg-white/10"
                title="Export this story as a web player or video"
              >
                <Download className="w-3.5 h-3.5 text-purple-400" />
                <span className="hidden sm:inline">Export</span>
              </button>
              <button
                onClick={() => setIsSettingsOpen(true)}
                className={`flex items-center gap-1.5 text-xs font-sans px-3 py-1.5 rounded-xl transition-all cursor-pointer ${
                  isSettingsOpen ? 'bg-purple-600 text-white font-bold shadow-md' : 'text-stone-300 hover:text-white hover:bg-white/10'
                }`}
                title="Global Settings"
              >
                <Settings className="w-3.5 h-3.5 text-purple-400" />
                <span className="hidden sm:inline">Settings</span>
              </button>
              <button
                onClick={() => openDocs()}
                className={`flex items-center gap-1.5 text-xs font-sans px-3 py-1.5 rounded-xl transition-all cursor-pointer ${
                  isDocsOpen ? 'bg-purple-600 text-white font-bold shadow-md' : 'text-stone-300 hover:text-white hover:bg-white/10'
                }`}
                title="Help & Documentation"
              >
                <HelpCircle className="w-3.5 h-3.5 text-purple-400" />
                <span className="hidden sm:inline">Docs</span>
              </button>
            </div>
          </header>

          {/* Main Floating Translucent Chronicle & Action Container */}
          <main className="relative z-10 flex-1 overflow-hidden mx-6 mb-4 flex flex-col">
            <div className="flex-1 bg-glass-card rounded-2xl flex flex-col overflow-hidden shadow-2xl">
              {chronicle.length === 0 ? (
                gameState ? (
                  streamedSegments.length > 0 ? (
                    <div className="flex-1 overflow-y-auto px-8 py-6">
                      <TurnSegments
                        segments={streamedSegments}
                        onEntityClick={handleOpenWikilink}
                        displayMode={config?.media.tts.speech_cues?.display_mode}
                        characterPortraits={characterPortraits}
                        segmentProgress={segmentAudioProgress}
                      />
                    </div>
                  ) : (
                    <ProloguePanel
                      key={activeGameID ?? 'prologue'}
                      gameName={gameState.game_name || activeGameID || ''}
                      playerName={gameState.player?.name}
                      initialPrompt={gameState.opening_prompt || ''}
                      busy={turnInFlight}
                      onBeginStory={handleBeginStory}
                      onBeginWithAction={handleBeginWithAction}
                    />
                  )
                ) : campaignStatus === 'unavailable' ? (
                  <div className="flex-1 flex flex-col items-center justify-center gap-3 p-6 text-center">
                    <Compass className="w-8 h-8 text-purple-500/50" />
                    <div className="space-y-1">
                      <p className="font-sans text-sm text-purple-300 font-bold">This campaign could not be opened</p>
                      <p className="text-xs text-stone-400 font-mono max-w-md">
                        {campaignError ?? `"${activeGameID}" is unavailable.`}
                      </p>
                    </div>
                    <div className="flex items-center gap-2 pt-1">
                      <button
                        onClick={handleReturnToLauncher}
                        className="px-3.5 py-1.5 rounded-lg bg-purple-600 hover:bg-purple-500 text-white font-sans font-bold text-xs shadow-md transition-all cursor-pointer"
                      >
                        Return to Campaigns
                      </button>
                      <button
                        onClick={() => window.location.reload()}
                        className="px-3.5 py-1.5 rounded-lg bg-stone-900 border border-stone-700 text-stone-300 hover:text-purple-300 hover:border-purple-500/40 font-sans text-xs transition-colors cursor-pointer"
                      >
                        Retry
                      </button>
                    </div>
                  </div>
                ) : (
                  <div className="flex-1 flex items-center justify-center text-stone-500 font-mono text-sm animate-pulse">
                    Opening the chronicle...
                  </div>
                )
              ) : (
                <>
                  <ChronicleView
                    turns={chronicle}
                    onWikilinkClick={handleOpenWikilink}
                    // With application playback the browser must stay silent, so
                    // it never competes with the narrator or hits autoplay limits.
                    autoPlay={serverAudio ? false : config?.media.tts.auto_play ?? false}
                    volume={config?.media.tts.master_volume ?? 1}
                    serverPlayback={serverAudio}
                    onPlayTurnAudio={handlePlayTurnAudio}
                    onStopAudio={handleStopAudio}
                    onCorrect={handleCorrect}
                    addressedTurns={addressed}
                    onAddress={handleAddress}
                    turnInFlight={turnInFlight}
                    pendingAction={pendingAction}
                    streamedSegments={streamedSegments}
                    displayMode={config?.media.tts.speech_cues?.display_mode}
                    turnAudioStatus={turnAudioStatus}
                    segmentAudioStatus={segmentAudioStatus}
                    characterPortraits={characterPortraits}
                    segmentProgress={segmentAudioProgress}
                    gameId={activeGameID ?? undefined}
                    skipAudioKeys={streamedKeys}
                  />
                </>
              )}
              {toolActivity && (
                <div className="text-xs font-mono text-purple-400/80 px-4 pb-1">{toolActivity}</div>
              )}
              <MechanicsStrip
                engagement={gameState?.mechanics_engagement}
                checks={lastTurnChecks.length}
                outcome={lastTurnChecks[lastTurnChecks.length - 1]?.outcome}
              />
              {pendingCheck && (
                <div className="mx-4 mb-2 rounded-xl border border-purple-500/40 bg-purple-950/30 px-4 py-3 flex items-center justify-between gap-3">
                  <div className="min-w-0">
                    <div className="text-xs font-sans font-bold uppercase tracking-wider text-purple-300">Roll required</div>
                    <div className="text-xs font-sans text-stone-300 truncate">
                      {pendingCheck.request?.stakes || pendingCheck.request?.check_kind || 'The GM has called for a check.'}
                    </div>
                  </div>
                  <button
                    onClick={() => handleActionSubmit('Roll', '', pendingCheck.ref)}
                    disabled={turnInFlight || isRateLimited}
                    className="shrink-0 px-4 py-2 rounded-xl bg-purple-600 hover:bg-purple-500 disabled:opacity-50 text-white text-xs font-sans font-bold cursor-pointer"
                  >
                    Roll
                  </button>
                </div>
              )}

              {fundsError && (
                <div className="mx-4 mb-2 p-3 bg-red-950/80 border border-red-500/50 text-red-200 text-xs rounded-xl flex items-center justify-between shadow-lg">
                  <div className="flex items-center gap-2">
                    <AlertTriangle className="w-4 h-4 text-red-400 shrink-0" />
                    <span>
                      Provider {fundsError.provider || 'AI'} rejected the request: insufficient funds/credits. Add funds and retry — no other work is blocked.
                    </span>
                  </div>
                  <button
                    onClick={() => setFundsError(null)}
                    className="text-stone-400 hover:text-white p-1 rounded hover:bg-white/10 ml-2 cursor-pointer"
                  >
                    <X className="w-3.5 h-3.5" />
                  </button>
                </div>
              )}

              {isRateLimited && (
                <div className="mx-4 mb-2 p-3 bg-amber-950/80 border border-amber-500/50 text-amber-200 text-xs rounded-xl flex items-center gap-2 shadow-lg">
                  <Clock className="w-4 h-4 text-amber-400 shrink-0 animate-pulse" />
                  <span>
                    Rate limit backoff active. Submitting turns paused for {activeRateLimitSeconds}s.
                  </span>
                </div>
              )}

              <ActionConsole
                onSubmit={handleActionSubmit}
                disabled={isRateLimited}
                streaming={turnInFlight}
                onStop={handleStopTurn}
                sttType={config?.media.stt?.type}
                audioProgress={audioProgress}
              />
            </div>
          </main>

          {/* Flyout Drawer Modal */}
          <Drawers
            isOpen={activeDrawer !== null}
            onClose={() => setActiveDrawer(null)}
            size={activeDrawer === 'codex' || activeDrawer === 'graph' || activeDrawer === 'context' ? 'xl' : 'md'}
            title={
              activeDrawer === 'character' ? 'Character Sheet' :
              activeDrawer === 'graph' ? 'Lore Graph' :
              activeDrawer === 'codex' ? 'Codex Markdown Editor' :
              activeDrawer === 'context' ? 'Turn Context & Continuity' : 'Living World Arcs & Clocks'
            }
          >
            {activeDrawer === 'character' && (
              <CharacterSheetDrawer
                player={gameState?.player}
                advancement={gameState?.advancement}
                onSpend={handleSpendAdvancement}
                spending={advancementSpending}
                spendError={advancementError}
              />
            )}
            {activeDrawer === 'graph' && <GraphDrawer data={graph || undefined} onSelectNode={handleOpenWikilink} />}
            {activeDrawer === 'context' && (
              <ContextDrawer
                gameID={activeGameID}
                onSelectWikilink={handleOpenWikilink}
              />
            )}
            {activeDrawer === 'codex' && (
              <CodexDrawer
                gameID={activeGameID || undefined}
                entity={selectedEntity || undefined}
                entities={entities}
                voiceProfiles={config?.media.tts.voice_profiles ?? []}
                ttsConfig={config?.media.tts}
                onAddProfile={(profile) => {
                  if (!config) return;
                  const existing = config.media.tts.voice_profiles ?? [];
                  if (existing.some((p) => p.id === profile.id)) return;
                  const next = {
                    ...config,
                    media: {
                      ...config.media,
                      tts: { ...config.media.tts, voice_profiles: [...existing, profile] },
                    },
                  };
                  setConfig(next);
                  APIClient.saveSettings(next).catch(() => undefined);
                }}
                onSelect={handleOpenWikilink}
                onSave={handleSaveEntity}
                onMerge={handleMergeEntity}
              />
            )}
            {activeDrawer === 'world' && (
              <LivingWorldDrawer
                state={gameState || undefined}
                recap={recap || undefined}
                idleTurns={config?.agents.thread_idle_turns ?? 10}
                onRefreshRecap={() => {
                  client?.getRecap().then(setRecap).catch(console.error);
                }}
              />
            )}
          </Drawers>

          {/* Global Settings Modal Dialog */}
          {isSettingsOpen && (
            <div className="fixed inset-0 z-50 flex items-center justify-center p-4 sm:p-6 bg-black/80 backdrop-blur-sm anim-fade-in">
              <div className="relative w-full max-w-4xl max-h-[88vh] bg-stone-900/95 border border-purple-500/30 rounded-2xl shadow-2xl flex flex-col overflow-hidden">
                <div className="flex items-center justify-between px-6 py-4 border-b border-stone-800 shrink-0">
                  <div className="flex items-center gap-2">
                    <Settings className="w-5 h-5 text-purple-400" />
                    <h2 className="font-sans text-lg font-bold text-purple-400">Global Configuration</h2>
                  </div>
                  <button
                    onClick={() => setIsSettingsOpen(false)}
                    className="text-stone-400 hover:text-white p-1 rounded-lg hover:bg-stone-800 transition-colors cursor-pointer"
                  >
                    <X className="w-5 h-5" />
                  </button>
                </div>
                <div className="flex-1 min-h-0 overflow-y-auto p-6">
                  <Suspense fallback={null}>
                    <SettingsStudio
                      activeGameID={activeGameID ?? undefined}
                      onOpenDocs={openDocs}
                      onSaved={() =>
                        APIClient.getSettings()
                          .then((res) => setConfig(res.config))
                          .catch(console.error)
                      }
                    />
                  </Suspense>
                </div>
              </div>
            </div>
          )}

          {/* Full-Screen Visual Novel Story Theater */}
          <Suspense fallback={null}>
            <StoryTheater
              turns={chronicle}
              isOpen={isTheaterOpen}
              onClose={() => {
                handleStopAudio();
                setIsTheaterOpen(false);
              }}
              volume={config?.media.tts.master_volume ?? 1}
              gameId={activeGameID ?? undefined}
              playerId={gameState?.player?.id}
              playerName={gameState?.player?.name}
              campaignImage={gameState?.banner_url}
              serverPlayback={serverAudio}
              onPlayAudio={handlePlayTurnAudio}
              onStopAudio={handleStopAudio}
              segmentAudioStatus={segmentAudioStatus}
              onEntityClick={handleOpenWikilink}
              displayMode={config?.media.tts.speech_cues?.display_mode}
              limits={limits}
              skipAudioKeys={streamedKeys}
            />
          </Suspense>

          {/* Add Entity Modal */}
          <AddEntityModal
            isOpen={modalEntity !== null}
            entityName={modalEntity?.name ?? ''}
            onQuickCreate={handleQuickCreateEntity}
            onEditInCodex={handleEditInCodexEntity}
            onClose={() => setModalEntity(null)}
          />

          {/* Turn Failure Banner */}
          {turnError && (
            <div className="fixed bottom-4 left-1/2 -translate-x-1/2 z-50 max-w-xl px-4 py-3 rounded-xl bg-red-950/90 border border-red-500/40 text-red-100 text-xs font-sans shadow-2xl flex items-center gap-3">
              <span className="flex-1 select-text">{turnError}</span>
              <button
                onClick={() => setTurnError(null)}
                className="text-red-300 hover:text-white cursor-pointer"
              >
                Dismiss
              </button>
            </div>
          )}

          {/* Model Download Modal */}
          <ModelDownloadModal
            isOpen={missingModel !== null}
            modelId={missingModel?.id ?? ''}
            modelName={missingModel?.name ?? ''}
            sizeBytes={missingModel?.sizeBytes ?? 0}
            onClose={() => {
              setMissingModel(null);
              setDismissedModelPrompt(true);
            }}
          />

          {/* Story Export Modal */}
          <Suspense fallback={null}>
            <ExportModal
              isOpen={isExportOpen}
              gameID={activeGameID}
              onClose={() => setIsExportOpen(false)}
            />
          </Suspense>

          {/* Cinematic backdrop overlays honour the preference. The layer is
              inert and sits below the modals. */}
          <CinematicOverlay enabled={config?.preferences?.cinematic_effects ?? false} />
        </>
      )}
      </div>

      {/* The application menu stays out of the way until Alt reveals it, and the
          About dialog describes the app rather than the framework it is built
          on. Both sit above the launcher and a campaign alike. */}
      <MenuBar
        onOpenDocs={() => openDocs()}
        onOpenAbout={() => setIsAboutOpen(true)}
        onOpenContentStudio={() => setIsContentStudioOpen(true)}
      />

      <Suspense fallback={null}>
        <ContentStudio
          isOpen={isContentStudioOpen}
          onClose={() => setIsContentStudioOpen(false)}
          gameID={activeGameID ?? ''}
        />
      </Suspense>

      <Suspense fallback={null}>
        <DocsModal
          isOpen={isDocsOpen}
          initialArticleID={docsArticleID}
          onClose={() => {
            setIsDocsOpen(false);
            setDocsArticleID(undefined);
          }}
        />
      </Suspense>

      <AboutModal
        isOpen={isAboutOpen}
        onClose={() => setIsAboutOpen(false)}
        onOpenDocs={() => openDocs()}
        version={appVersion}
      />
    </div>
  );
};
