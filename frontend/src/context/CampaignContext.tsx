import { createContext, useContext, useState, useEffect, useMemo, useRef, useCallback, type ReactNode } from 'react';
import { APIClient, HTTPError, GenerationError } from '../api/client';
import {
  AppConfig,
  GameState,
  Turn,
  TurnSegment,
  TurnEvent,
  EntityNote,
  EntitySummary,
  Recap,
  GraphData,
  LimitState,
  AudioProgressEvent,
  PlaybackEntry,
} from '../types';
import { useFailures } from './FailuresContext';
import { useCharacterPortraits } from '../hooks/useCharacterPortraits';
import { useSettings } from './SettingsContext';
import { TurnAudioState, segmentAudioKey } from '../components/TurnSegments';
import { DiceEntry } from '../lib/diceRange';
import type { Adjudication, CounterProposal } from '../types';
import { formatGenerationError } from '../lib/generationError';
import { DEFAULT_KOKORO_MODEL_ID, DEFAULT_KOKORO_SIZE_BYTES, STORAGE_KEYS } from '../lib/constants';
import { useStreamedSpeech } from '../hooks/useStreamedSpeech';
import { TurnStreamProcessor } from '../lib/turnStreamProcessor';
import { slugify } from '../lib/slug';

// Everything below the campaign shell is only needed once its overlay is
// opened, so it loads on demand instead of in the first bundle. The chronicle,
// its drawers, and the launcher stay eager because they are the first paint.

// The campaign provider owns one campaign end to end: its data, the turn stream
// that writes it, and the playback that reads it. It is mounted above the component
// that chooses a campaign, because the stream and the audio state are written by the
// same code that loads the data, so no smaller owner can hold them.
//
// The value is not memoised: the provider re-renders on every state change it makes,
// which is exactly when the screen re-rendered before this change, so the render
// count is the same. Memoising sixty fields would freeze the ones that must not be.
function useCampaignValue() {
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
  // the story theater honour the same switches as the settings studio. One
  // provider owns them for the whole app.
  const { settings, applySettings } = useSettings();
  const config = settings?.config ?? null;
  const appVersion = settings?.app_version;

  // Active drawer tab: null, 'character', 'graph', 'codex', 'world'
  // The application plays audio itself when it can, which is the only way to
  // narrate a turn without a browser autoplay gesture.
  const [serverAudio, setServerAudio] = useState(false);
  const [playbackOwner, setPlaybackOwner] = useState<'device' | 'browser'>('browser');
  const isDeviceOwner = playbackOwner === 'device' || (serverAudio && playbackOwner !== 'browser');
  const browserPlaybackEnabled = !isDeviceOwner && (config?.media.tts.auto_play ?? false);
  // Per-turn audio status: generating → playing → idle (or error)
  // Why the server withheld an illustration for a turn, keyed by turn number. It is
  // shown beside the generate button, so a turn with no picture says why.
  const [sceneImagePending, setSceneImagePending] = useState<Record<number, string>>({});
  const { report } = useFailures();
  const [turnAudioStatus, setTurnAudioStatus] = useState<Record<number, { state: TurnAudioState; message?: string }>>(
    {},
  );
  // Per-segment audio status, keyed `${turn}:${index}`.
  const [segmentAudioStatus, setSegmentAudioStatus] = useState<
    Record<string, { state: TurnAudioState; message?: string }>
  >({});
  // Narration plays while the turn streams when the browser owns the sound. The
  // clips already heard are handed to the chronicle so nothing repeats.
  const streamedSpeech = useStreamedSpeech(browserPlaybackEnabled, config?.media.tts.master_volume ?? 1);
  const [streamedKeys, setStreamedKeys] = useState<ReadonlySet<string>>(new Set());
  const [playbackLedger, setPlaybackLedger] = useState<Record<string, PlaybackEntry>>({});
  const [audioProgress, setAudioProgress] = useState<AudioProgressEvent | null>(null);
  const [segmentAudioProgress, setSegmentAudioProgress] = useState<Record<number, string>>({});
  // Every character's portrait, from the two places one can arrive: the segments of
  // a turn and the entity list.
  const { portraits: characterPortraits, notePortrait } = useCharacterPortraits({
    chronicle,
    entities,
    activeGameID,
  });

  // The build version the About dialog reports, read from the settings endpoint.
  const [addressed, setAddressed] = useState<Set<number>>(new Set());
  const [missingModel, setMissingModel] = useState<{ id: string; name: string; sizeBytes: number } | null>(null);
  const [dismissedModelPrompt, setDismissedModelPrompt] = useState(false);
  // The remembered campaign is only a hint until it is verified against the
  // campaign list, so the play shell can tell "loading" from "gone".
  const [campaignStatus, setCampaignStatus] = useState<'idle' | 'loading' | 'ready' | 'unavailable'>('idle');
  const [campaignError, setCampaignError] = useState<string | null>(null);
  // Advancement spending: the drawer disables its buttons and shows the refusal.

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
      // A background poll of the rate-limit chip: a stale header is not something the
      // player can act on, so the console keeps the record and the region stays quiet.
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
      (b) => (b.role === 'gm' || b.role === 'extractor') && b.until && new Date(b.until).getTime() > now,
    );
    if (gmBlock && gmBlock.until) {
      return Math.ceil((new Date(gmBlock.until).getTime() - now) / 1000);
    }
    return 0;
  }, [rateLimitUntil, limits, now]);

  const isRateLimited = activeRateLimitSeconds > 0;

  useEffect(() => {
    APIClient.audioStatus()
      .then((status) => {
        setServerAudio(status.available);
        if (status.owner === 'device' || status.owner === 'browser') {
          setPlaybackOwner(status.owner);
        } else if (status.available) {
          setPlaybackOwner('device');
        } else {
          setPlaybackOwner('browser');
        }
      })
      .catch(() => {
        setServerAudio(false);
        setPlaybackOwner('browser');
      });
  }, [report]);

  const handleGameStateFailure = useCallback((err: unknown) => {
    if (err instanceof HTTPError && err.status === 404) {
      // The campaign is definitively gone: forget it and return to the launcher.
      localStorage.removeItem(STORAGE_KEYS.ACTIVE_GAME);
      localStorage.removeItem(STORAGE_KEYS.LAST_PLAYED_GAME);
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
    client
      .getGameState()
      .then((state) => {
        setGameState(state);
        setCampaignStatus('ready');
      })
      .catch(handleGameStateFailure);
    client
      .getGraph()
      .then(setGraph)
      .catch((err) =>
        report({ text: 'Could not load the entity graph.', detail: err instanceof Error ? err.message : String(err) }),
      );
    client
      .listEntities()
      .then(setEntities)
      .catch((err) =>
        report({ text: 'Could not load the entity list.', detail: err instanceof Error ? err.message : String(err) }),
      );
    client
      .getRecap()
      .then(setRecap)
      .catch((err) =>
        report({ text: 'Could not load the recap.', detail: err instanceof Error ? err.message : String(err) }),
      );
  }, [report, client, handleGameStateFailure]);

  // A spend applies through the API and then refreshes the corpus, so the
  // drawer and the notification dot both reflect the new balance.

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
        localStorage.removeItem(STORAGE_KEYS.ACTIVE_GAME);
        localStorage.removeItem(STORAGE_KEYS.LAST_PLAYED_GAME);
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
      return;
    }
    client
      .getChronicle()
      .then(setChronicle)
      .catch((err) =>
        report({ text: 'Could not load the chronicle.', detail: err instanceof Error ? err.message : String(err) }),
      );
    client
      .getFindings()
      .then((res) => {
        setAddressed(new Set(res.addressed.map((a) => a.turn)));
      })
      .catch((err) =>
        report({ text: 'Could not load the campaign.', detail: err instanceof Error ? err.message : String(err) }),
      );
    refreshCorpus();
  }, [report, client, refreshCorpus]);

  // Poll for background portrait completion when characters in chronicle lack custom portraits
  useEffect(() => {
    if (!client || !activeGameID || chronicle.length === 0) return;

    const hasPendingPortraits = chronicle.some((turn) =>
      (turn.segments || []).some((seg) => {
        if (seg.kind !== 'speech') return false;
        const charId = seg.speaker_id || (seg.speaker ? slugify(seg.speaker) : undefined);
        return charId && (!characterPortraits[charId] || !characterPortraits[charId].hasCustom);
      }),
    );

    if (!hasPendingPortraits) return;

    let attempts = 0;
    const maxAttempts = 10;
    const interval = setInterval(() => {
      attempts++;
      client
        .listEntities()
        .then((ents) => {
          setEntities(ents);
          const allResolved = chronicle.every((turn) =>
            (turn.segments || []).every((seg) => {
              if (seg.kind !== 'speech') return true;
              const charId = seg.speaker_id || (seg.speaker ? slugify(seg.speaker) : undefined);
              if (!charId) return true;
              const matchingEnt = ents.find((e) => e.id === charId);
              return matchingEnt?.has_portrait;
            }),
          );
          if (allResolved || attempts >= maxAttempts) {
            clearInterval(interval);
          }
        })
        // A poll that retries on its own interval while portraits are generated: a failed
        // attempt is not the player's to act on, and the portrait it waits for is simply not
        // there yet, which the screen already shows.
        .catch(console.error);
    }, 3000);

    return () => clearInterval(interval);
  }, [client, activeGameID, chronicle, characterPortraits]);

  const handleSelectGame = (gameId: string) => {
    localStorage.setItem(STORAGE_KEYS.LAST_PLAYED_GAME, gameId);
    localStorage.removeItem(STORAGE_KEYS.ACTIVE_GAME);
    setActiveGameID(gameId);
  };

  const [turnInFlight, setTurnInFlight] = useState(false);
  // turnStatus is the sentence the live region announces: two per turn, and never
  // the prose, which a reader would hear one token at a time.
  const [turnStatus, setTurnStatus] = useState('');
  const [pendingAction, setPendingAction] = useState<{ mode: string; text: string } | null>(null);
  const [streamedSegments, setStreamedSegments] = useState<TurnSegment[]>([]);
  const [toolActivity, setToolActivity] = useState<string | null>(null);
  const [turnError, setTurnError] = useState<string | null>(null);
  const abortRef = useRef<AbortController | null>(null);
  const streamProcessorRef = useRef<TurnStreamProcessor>(new TurnStreamProcessor());

  const handleTurnEvent = (event: TurnEvent) => {
    if (event.owner === 'device' || event.owner === 'browser') {
      setPlaybackOwner(event.owner);
    }
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
      if (event.audio_progress.owner === 'device' || event.audio_progress.owner === 'browser') {
        setPlaybackOwner(event.audio_progress.owner);
      }
      setAudioProgress(event.audio_progress);
      if (event.audio_progress.sequence !== undefined && event.audio_progress.stage) {
        setSegmentAudioProgress((prev) => ({
          ...prev,
          [event.audio_progress!.sequence]: event.audio_progress!.stage,
        }));
      }
    } else if (event.type === 'portrait' && event.character_id) {
      notePortrait(event.character_id, {
        url: event.portrait_url || '',
        hasCustom: !!event.has_custom_portrait,
      });
    } else if (event.type === 'scene_image_pending' && event.turn_number) {
      // The server withheld an illustration and said why: the budget is spent, or
      // the approval policy held it back. Nothing else carries that reason, so it
      // is kept here for the chronicle to explain the absence with.
      setSceneImagePending((prev) => ({ ...prev, [event.turn_number!]: event.message || '' }));
    } else if (event.type === 'scene_image' && event.turn_number && event.image_url) {
      setChronicle((prev) =>
        prev.map((turn) => (turn.turn_number === event.turn_number ? { ...turn, image_url: event.image_url } : turn)),
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
      // and remember which keys were heard to completion and offsets.
      streamedSpeech.stop();
      const offsets = streamedSpeech.ledger();
      setStreamedKeys(streamedSpeech.playedKeys());
      setPlaybackLedger(offsets);

      // Resend offsets at handover to reconcile server ledger for finalise pass
      if (activeGameID && Object.keys(offsets).length > 0) {
        APIClient.mergePlaybackLedger(activeGameID, {
          turn: event.turn.turn_number,
          entries: offsets,
        }).catch((err) =>
          report({
            text: 'Could not save your playback position.',
            detail: err instanceof Error ? err.message : String(err),
          }),
        );
      }
      const turn = event.turn;
      if (turn.input_text === '/undo' || turn.prose === 'Undid previous turn.') {
        if (client) {
          client
            .getChronicle()
            .then(setChronicle)
            .catch(() => {
              setChronicle((prev) => prev.filter((t) => t.turn_number <= turn.turn_number));
            });
        } else {
          setChronicle((prev) => prev.filter((t) => t.turn_number <= turn.turn_number));
        }
      } else {
        setChronicle((prev) => [...prev, turn]);
      }
      setTurnStatus(`Turn ${turn.turn_number} ready`);
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
          id: event.model_id || DEFAULT_KOKORO_MODEL_ID,
          name: event.name || 'Kokoro Voice Pack',
          sizeBytes: event.size_bytes || DEFAULT_KOKORO_SIZE_BYTES,
        });
      }
    } else if (event.type === 'error') {
      const reason = event.failure
        ? formatGenerationError(event.failure)
        : event.detail || event.message || 'The turn failed.';
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
      report({ text: 'The turn failed.', detail: event.message });
    }
  };

  // beginTurnStream shares the setup every streaming turn needs, so a player turn
  // and a pending-check resolution differ only in the request they post.
  const beginTurnStream = async (
    action: { mode: string; text: string },
    post: (onEvent: (event: TurnEvent) => void, signal: AbortSignal) => Promise<void>,
  ) => {
    if (!client || !activeGameID || turnInFlight) return;

    setTurnInFlight(true);
    setTurnStatus('Narrating the turn…');
    setPendingAction(action);
    streamProcessorRef.current.reset();
    setStreamedSegments([]);
    setToolActivity(null);
    setTurnError(null);
    setAudioProgress(null);
    setSegmentAudioProgress({});
    streamedSpeech.reset();
    setStreamedKeys(new Set());
    setPlaybackLedger({});

    const controller = new AbortController();
    abortRef.current = controller;

    try {
      await post(handleTurnEvent, controller.signal);
    } catch (err) {
      report({ text: 'The turn failed.', detail: err instanceof Error ? err.message : String(err) });
      setTurnError(err instanceof Error ? err.message : String(err));
    } finally {
      if (abortRef.current === controller) {
        abortRef.current = null;
        setTurnInFlight(false);
        setPendingAction(null);
        setToolActivity(null);
        streamedSpeech.stop();
        setAudioProgress((prev) =>
          prev && prev.ready_count + (prev.failed_count || 0) < prev.total_segments ? null : prev,
        );
        setSegmentAudioProgress({});
      }
    }
  };

  const handleActionSubmit = async (mode: string, text: string, pendingCheckRef?: string, sceneOnly = false) => {
    await beginTurnStream({ mode, text }, (onEvent, signal) =>
      APIClient.streamTurn(
        activeGameID!,
        { mode, input: text, pending_check_ref: pendingCheckRef, scene_only: sceneOnly || undefined },
        onEvent,
        signal,
      ),
    );
  };

  // handleResolveCheck rolls a pending check through IR-1's endpoint, so the
  // adjudication streams without a fresh player action.
  const handleResolveCheck = async (turnNumber: number, pendingCheckRef: string, manual?: DiceEntry) => {
    // One entered number is a total; several are the individual dice, which the
    // server sums and records so the chronicle can show the faces.
    const body = manual
      ? manual.dice.length > 1
        ? { pending_check_ref: pendingCheckRef, manual_dice: manual.dice }
        : { pending_check_ref: pendingCheckRef, manual_result: manual.total }
      : { pending_check_ref: pendingCheckRef };
    await beginTurnStream({ mode: 'Roll', text: '' }, (onEvent, signal) =>
      APIClient.resolveCheck(activeGameID!, turnNumber, body, onEvent, signal),
    );
  };

  // handleRenegotiate asks the GM to rule on a counter-proposal, then folds the
  // agreed terms and the exchange back into the chronicle so the card re-renders.
  const handleRenegotiate = async (
    turnNumber: number,
    pendingCheckRef: string,
    counter: CounterProposal,
  ): Promise<Adjudication | undefined> => {
    try {
      const result = await APIClient.renegotiate(activeGameID!, turnNumber, {
        pending_check_ref: pendingCheckRef,
        counter,
      });
      setChronicle((prev) =>
        prev.map((turn) =>
          turn.turn_number === turnNumber
            ? {
                ...turn,
                pending_check: result.pending_check ?? turn.pending_check,
                negotiations: result.negotiations ?? turn.negotiations,
              }
            : turn,
        ),
      );
      return result.ruling;
    } catch (err) {
      setFundsError({ message: err instanceof Error ? err.message : 'The GM could not be reached.' });
      return undefined;
    }
  };

  const handleStopTurn = () => {
    abortRef.current?.abort();
    streamedSpeech.stop();
    setPlaybackLedger(streamedSpeech.ledger());
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
      report({ text: 'Could not save the opening prompt.', detail: err instanceof Error ? err.message : String(err) });
    }
    await handleActionSubmit('Opening', '');
  };

  // Taking the first step still establishes the scene first: a quiet scene turn
  // restates the opening prompt, then the console takes the player's own first
  // step as turn 2.
  const handleBeginWithAction = async () => {
    if (gameState?.opening_prompt && !turnInFlight) {
      await handleActionSubmit('Opening', '', undefined, true);
    }
    document.getElementById('action-console-input')?.focus();
  };

  // audioPollingRef tracks how to end each in-flight playback subscription, so
  // Stop can close every EventSource.
  const audioPollingRef = useRef<Record<string, () => void>>({});

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

    const call =
      segmentIndex === undefined
        ? APIClient.playTurnAudio(activeGameID, turnNumber, force)
        : APIClient.playSegmentAudio(activeGameID, turnNumber, segmentIndex, force);

    const pollKey = key ?? `turn:${turnNumber}`;

    call
      .then(() => {
        setStatus({ state: 'playing' });
        // The server reports the completion over SSE, so the beat advances on a
        // real event rather than a status poll.
        const source = new EventSource(APIClient.audioEventsURL());
        let settled = false;
        const settle = (entry: { state: TurnAudioState; message?: string }) => {
          if (settled) return;
          settled = true;
          source.close();
          delete audioPollingRef.current[pollKey];
          setStatus(entry);
        };
        source.onmessage = (evt) => {
          try {
            const data = JSON.parse(evt.data) as {
              playing?: boolean;
              turn?: number;
              segment?: number;
              error?: string;
            };
            // Only the beat this subscription started advances, so a stale
            // completion from a replaced queue cannot skip the current line.
            if (data.turn !== turnNumber) return;
            if (segmentIndex !== undefined && data.segment !== segmentIndex) return;
            // The server reports a real failure with a reason, so silence is never
            // presented as a finished beat.
            if (data.error) {
              settle({ state: 'error', message: data.error });
              return;
            }
            if (data.playing !== false) return;
            settle({ state: 'idle' });
          } catch {
            // A malformed event is ignored; the safety timeout still settles.
          }
        };
        // A stream that errors before a completion is a failure, not a normal end.
        source.onerror = () => settle({ state: 'error', message: 'The audio stream ended unexpectedly.' });
        // A completion event that never arrives must not stall the theatre, so a
        // generous safety timeout ends the beat, and says so.
        const safety = setTimeout(() => settle({ state: 'error', message: 'Audio timed out.' }), 60_000);
        audioPollingRef.current[pollKey] = () => {
          clearTimeout(safety);
          settle({ state: 'idle' });
        };
      })
      .catch((err: unknown) => {
        const message =
          err instanceof GenerationError ? err.failure.message : err instanceof Error ? err.message : String(err);
        setStatus({ state: 'error', message });
      });
  };

  const handleStopAudio = () => {
    APIClient.stopAudio().catch((err) =>
      report({ text: 'Could not stop the audio.', detail: err instanceof Error ? err.message : String(err) }),
    );
    // Close every in-flight playback subscription and reset what is playing.
    for (const [key, close] of Object.entries(audioPollingRef.current)) {
      close();
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
      report({ text: 'Could not merge the notes.', detail: err instanceof Error ? err.message : String(err) });
    }
  };

  const handleAddress = (turnNumber: number) => {
    client?.addressFinding(turnNumber, 'continuity').catch((err) =>
      report({
        text: 'Could not address the continuity note.',
        detail: err instanceof Error ? err.message : String(err),
      }),
    );
    setAddressed((prev) => new Set(prev).add(turnNumber));
  };

  // Find latest scene image for full-window atmospheric background, falling back to campaign banner
  const activeBgImage =
    chronicle
      .slice()
      .reverse()
      .find((t) => t.image_url)?.image_url || gameState?.banner_url;

  // A pending check the GM proposed under the ask policy, awaiting the player's roll.
  const pendingCheck = chronicle.length > 0 ? chronicle[chronicle.length - 1].pending_check : undefined;
  const pendingTurnNumber = chronicle.length > 0 ? chronicle[chronicle.length - 1].turn_number : 0;
  const lastTurnChecks = chronicle.length > 0 ? (chronicle[chronicle.length - 1].checks ?? []) : [];

  // Open the built-in docs, optionally jumping straight to an article. Callers
  // that explain a setting (for example an unpriced usage row) link a reader to
  // the exact reference instead of leaving them to search.

  // The playback switches the chronicle and the theatre both honour, derived once
  // here rather than by each of them: with application playback the browser must
  // stay silent, so it never competes with the narrator or hits autoplay limits.
  const autoPlay = serverAudio ? false : (config?.media.tts.auto_play ?? false);
  const volume = config?.media.tts.master_volume ?? 1;
  const displayMode = config?.media.tts.speech_cues?.display_mode;

  // The screen's flows call these rather than reaching into the provider's state:
  // a context that hands out setters is a prop list wearing a context's clothes.

  // A voice profile added from the codex is a settings change the whole app sees,
  // so it is applied here and saved rather than being a second copy of the config.
  const addVoiceProfile = useCallback(
    (next: AppConfig) => {
      if (settings) applySettings({ ...settings, config: next });
      APIClient.saveSettings(next).catch(() => undefined);
    },
    [applySettings, settings],
  );

  const leaveCampaign = useCallback(() => {
    setActiveGameID(null);
    setCampaignStatus('idle');
    setCampaignError(null);
  }, []);

  const spendAdvancement = useCallback(
    async (unlockID: string) => {
      if (!client) return;
      await client.advanceUnlock(unlockID);
      refreshCorpus();
    },
    [client, refreshCorpus],
  );

  const openEntity = useCallback(
    async (entityId: string) => {
      if (!client) return;
      setSelectedEntity(await client.getEntity(entityId));
    },
    [client],
  );

  const clearSelectedEntity = useCallback(() => setSelectedEntity(null), []);

  const generateTurnImage = useCallback(
    async (turnNumber: number) => {
      if (!activeGameID) return;
      await APIClient.generateTurnSceneImage(activeGameID, turnNumber);
    },
    [activeGameID],
  );

  const reloadEntities = useCallback(async () => {
    if (client) setEntities(await client.listEntities());
  }, [client]);

  const reloadRecap = useCallback(async () => {
    if (client) setRecap(await client.getRecap());
  }, [client]);

  const dismissFundsError = useCallback(() => setFundsError(null), []);
  const dismissTurnError = useCallback(() => setTurnError(null), []);
  const dismissModelPrompt = useCallback(() => {
    setMissingModel(null);
    setDismissedModelPrompt(true);
  }, []);

  // addMissingEntity writes the note a finding asked for, or opens the one that is
  // already there, and answers whether it was already there: the screen decides
  // what to do with that, the provider owns what it means.
  const addMissingEntity = useCallback(
    async (name: string, type: string, turnNumber: number) => {
      if (!client) return false;
      const slug = slugify(name);
      const existing = entities.some((candidate) => candidate.id === slug);
      if (existing) {
        // Never overwrite a note that already exists; open it instead.
        await client.addressFinding(turnNumber, 'continuity');
        setAddressed((prev) => new Set(prev).add(turnNumber));
        setSelectedEntity(await client.getEntity(slug));
        return true;
      }
      const template = `---\nid: ${slug}\nname: ${name}\ntype: ${type}\n---\n\n`;
      await client.saveEntity(slug, template);
      await client.addressFinding(turnNumber, 'continuity');
      setAddressed((prev) => new Set(prev).add(turnNumber));
      refreshCorpus();
      return false;
    },
    [client, entities, refreshCorpus],
  );

  const saveWizardEntity = useCallback(
    async ({ id, markdown, turnNumber }: { id: string; markdown: string; turnNumber: number }) => {
      if (!client) return;
      await client.saveEntity(id, markdown);
      await client.addressFinding(turnNumber, 'continuity');
      setAddressed((prev) => new Set(prev).add(turnNumber));
      refreshCorpus();
      setSelectedEntity(await client.getEntity(id));
    },
    [client, refreshCorpus],
  );

  return {
    activeGameID,
    activeRateLimitSeconds,
    addressed,
    appVersion,
    audioProgress,
    campaignError,
    campaignStatus,
    activeBgImage,
    autoPlay,
    characterPortraits,
    chronicle,
    client,
    config,
    entities,
    fundsError,
    gameState,
    graph,
    handleActionSubmit,
    handleAddress,
    handleBeginStory,
    handleBeginWithAction,
    handleMergeEntity,
    handlePlayTurnAudio,
    handleRenegotiate,
    handleResolveCheck,
    handleSaveEntity,
    handleSelectGame,
    handleStopAudio,
    handleStopTurn,
    isRateLimited,
    lastTurnChecks,
    limits,
    missingModel,
    pendingAction,
    pendingCheck,
    pendingTurnNumber,
    playbackLedger,
    recap,
    refreshCorpus,
    sceneImagePending,
    segmentAudioProgress,
    segmentAudioStatus,
    selectedEntity,
    serverAudio,
    streamedKeys,
    streamedSegments,
    toolActivity,
    turnAudioStatus,
    turnError,
    turnInFlight,
    turnStatus,
    volume,
    // The actions the screen's flows call, so no consumer reaches for a setter.
    addMissingEntity,
    addVoiceProfile,
    clearSelectedEntity,
    dismissFundsError,
    dismissModelPrompt,
    dismissTurnError,
    displayMode,
    generateTurnImage,
    leaveCampaign,
    openEntity,
    reloadEntities,
    reloadRecap,
    saveWizardEntity,
    spendAdvancement,
  };
}

type CampaignValue = ReturnType<typeof useCampaignValue>;

const CampaignContext = createContext<CampaignValue | null>(null);

export function CampaignProvider({ children }: { children: ReactNode }) {
  const value = useCampaignValue();
  return <CampaignContext.Provider value={value}>{children}</CampaignContext.Provider>;
}

// useCampaign returns the active campaign. Outside a provider it throws rather than
// answering with an empty campaign, because every consumer of this one is inside a
// campaign by construction.
export function useCampaign(): CampaignValue {
  const value = useContext(CampaignContext);
  if (!value) throw new Error('useCampaign must be used inside a CampaignProvider');
  return value;
}
