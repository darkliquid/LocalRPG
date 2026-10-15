import { useFailures } from '../context/FailuresContext';
import React, { useState, useEffect, useRef, useMemo } from 'react';
import { DebugPanel } from './DebugPanel';
import { APIClient } from '../api/client';
import {
  AppConfig,
  AgentRoleConfig,
  TestProviderResponse,
  ModelStatus,
  ProviderDescriptor,
  TTSConfig,
  STTConfig,
  ImageConfig,
  EmbeddingProviderConfig,
  EmbeddingsConfig,
  StylePacks,
} from '../types';
import { ModelDownloadModal } from './ModelDownloadModal';
import { AlertCircle, Bug, CheckCircle, Cloud, Coins, Cpu, Folder, Layers, Save, Sliders, Volume2 } from 'lucide-react';
import { useProviderCatalog } from '../hooks/useProviderCatalog';
import { useAdvanced } from '../hooks/useAdvanced';
import { UsagePanel } from './UsagePanel';
import { TTSBatchPanel } from './TTSBatchPanel';
import { PathsPanel } from './settings/PathsPanel';
import { PreferencesPanel } from './settings/PreferencesPanel';
import { ProvidersPanel } from './settings/ProvidersPanel';
import { AgentsPanel } from './settings/AgentsPanel';
import { MediaPanel } from './settings/MediaPanel';

interface SettingsStudioProps {
  isCompact?: boolean;
  onSaved?: () => void;
  activeGameID?: string;
  onOpenDocs?: (articleID?: string) => void;
}

// presetsToMap turns the catalogue's descriptors for one family into the
// id-keyed shape the quick-load selects use, carrying each provider's tier and
// caveat so the picker can label it honestly.
const presetsToMap = <T,>(
  descriptors: ProviderDescriptor[],
): Record<string, { label: string; description: string; config: T; tier?: string; caveat?: string }> => {
  const map: Record<string, { label: string; description: string; config: T; tier?: string; caveat?: string }> = {};
  for (const descriptor of descriptors) {
    for (const preset of descriptor.presets ?? []) {
      map[preset.id] = {
        label: preset.label,
        description: preset.description,
        config: preset.config as unknown as T,
        tier: descriptor.tier,
        caveat: descriptor.caveat,
      };
    }
  }
  return map;
};

export const SettingsStudio: React.FC<SettingsStudioProps> = ({ isCompact, onSaved, activeGameID, onOpenDocs }) => {
  const { report } = useFailures();
  const [config, setConfig] = useState<AppConfig | null>(null);
  const [activeFilePath, setActiveFilePath] = useState<string>('');
  const [isOverride, setIsOverride] = useState(false);
  const [activeSubTab, setActiveSubTab] = useState<
    'paths' | 'providers' | 'agents' | 'media' | 'batch' | 'preferences' | 'usage' | 'debug'
  >('paths');
  // The Providers area splits into credentials, embeddings, and the offline preset,
  // so the tab leads with one job rather than all three.
  const [isLoading, setIsLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const [feedback, setFeedback] = useState<{ type: 'success' | 'error'; message: string } | null>(null);
  const [configWarnings, setConfigWarnings] = useState<string[]>([]);
  // The style packs the config directory holds, so the picker lists what exists
  // rather than only what is configured.
  const [stylePacks, setStylePacks] = useState<StylePacks | null>(null);

  // Diagnostics test state
  const [testingCategory, setTestingCategory] = useState<string | null>(null);
  const [testResult, setTestResult] = useState<{ category: string; res: TestProviderResponse } | null>(null);
  const previewAudioRef = useRef<HTMLAudioElement | null>(null);
  // The provider catalogue feeds preset lists so a provider that publishes
  // presets is the source of truth; the built-in fallback covers the rest.
  const { byFamily, providers: catalogProviders } = useProviderCatalog(Boolean(config));

  // The tier vocabulary is data, not a hardcoded legend: collect the caveats the
  // catalogue actually sends, so the explanation cannot drift from the providers.
  const tierCaveats = useMemo(() => {
    const map: Record<string, string> = {};
    for (const p of catalogProviders) {
      if (p.tier && !(p.tier in map)) map[p.tier] = p.caveat ?? '';
    }
    return map;
  }, [catalogProviders]);

  const llmDescriptors = byFamily('llm');

  // "Show advanced settings" is a preference, not configuration: it decides how
  // much of the studio is on screen, and persists across reloads.
  const [advanced, setAdvanced] = useAdvanced();

  // Model status state
  const [models, setModels] = useState<ModelStatus[]>([]);
  const [missingModelPrompt, setMissingModelPrompt] = useState<{
    id: string;
    name: string;
    sizeBytes: number;
  } | null>(null);
  useEffect(() => {
    loadSettings();
    APIClient.getModels()
      .then(setModels)
      .catch((err) =>
        report({ text: 'Could not load the model list.', detail: err instanceof Error ? err.message : String(err) }),
      );
    const unsubscribe = APIClient.subscribeModelEvents((status) => {
      setModels((prev) => {
        const next = [...prev];
        const idx = next.findIndex((m) => m.id === status.id);
        if (idx >= 0) {
          next[idx] = status;
        } else {
          next.push(status);
        }
        return next;
      });
    });
    return () => unsubscribe();
  }, [report]);

  useEffect(
    () => () => {
      if (previewAudioRef.current) {
        previewAudioRef.current.pause();
        previewAudioRef.current = null;
      }
    },
    [],
  );

  const playVoicePreview = (dataURI: string, volume: number) => {
    if (previewAudioRef.current) {
      previewAudioRef.current.pause();
    }
    const audio = new Audio(dataURI);
    audio.volume = Math.min(1, Math.max(0, volume));
    previewAudioRef.current = audio;
    audio.play().catch(() => {
      setFeedback({
        type: 'error',
        message: 'Audio generated, but the browser blocked playback. Click the page and retry.',
      });
    });
  };

  const loadSettings = async () => {
    setIsLoading(true);
    try {
      const res = await APIClient.getSettings();
      setConfig(res.config);
      setActiveFilePath(res.config_file_path);
      setIsOverride(res.is_local_override);
      setConfigWarnings(res.warnings ?? []);
      try {
        setStylePacks(await APIClient.getStylePacks());
      } catch {
        // A missing pack listing must not stop the settings from loading.
        setStylePacks(null);
      }
    } catch (err) {
      setFeedback({ type: 'error', message: err instanceof Error ? err.message : 'Failed to load settings' });
    } finally {
      setIsLoading(false);
    }
  };

  const handleSave = async () => {
    if (!config) return;
    setIsSaving(true);
    setFeedback(null);
    try {
      const res = await APIClient.saveSettings(config);
      setConfig(res.config);
      setActiveFilePath(res.config_file_path);
      setIsOverride(res.is_local_override);
      setConfigWarnings(res.warnings ?? []);
      try {
        setStylePacks(await APIClient.getStylePacks());
      } catch {
        setStylePacks(null);
      }
      setFeedback({ type: 'success', message: 'Settings saved and live-reloaded successfully.' });
      if (onSaved) onSaved();
    } catch (err) {
      setFeedback({ type: 'error', message: err instanceof Error ? err.message : 'Failed to save settings' });
    } finally {
      setIsSaving(false);
    }
  };

  const handleTestProvider = async (category: 'llm' | 'tts' | 'stt' | 'image', provider: any, testPrompt?: string) => {
    if (category === 'tts') {
      const isTargetKokoro =
        provider?.type === 'builtin' &&
        (provider?.builtin_name === 'sherpa-onnx' || provider?.builtin_name === 'kokoro');
      const kokoroInstalled = models.find((m) => m.id === 'kokoro-tts')?.installed;
      if (isTargetKokoro && !kokoroInstalled) {
        setMissingModelPrompt({
          id: 'kokoro-tts',
          name: 'Kokoro Voice Pack',
          sizeBytes: 90177536,
        });
        return;
      }
    }

    setTestingCategory(category);
    setTestResult(null);
    try {
      const res = await APIClient.testProvider({
        category,
        provider,
        test_prompt: testPrompt ?? (category === 'llm' ? 'Are the stars shining?' : undefined),
      });
      setTestResult({ category, res });
      if (res.model_missing) {
        setMissingModelPrompt({
          id: res.model_id || 'kokoro-tts',
          name: 'Kokoro Voice Pack',
          sizeBytes: 90177536,
        });
      }
      if (category === 'tts' && res.success && res.audio_data_uri) {
        playVoicePreview(res.audio_data_uri, (provider as { master_volume?: number })?.master_volume ?? 1);
      }
    } catch (err) {
      setTestResult({
        category,
        res: { success: false, latency_ms: 0, message: err instanceof Error ? err.message : 'Test failed' },
      });
    } finally {
      setTestingCategory(null);
    }
  };

  if (isLoading || !config) {
    return (
      <div className="p-8 text-center text-stone-400 font-mono text-sm animate-pulse">
        Loading system configuration...
      </div>
    );
  }

  const agentPresets = presetsToMap<AgentRoleConfig>(byFamily('llm'));
  const ttsPresets = presetsToMap<TTSConfig>(byFamily('tts'));
  const sttPresets = presetsToMap<STTConfig>(byFamily('stt'));
  const imagePresets = presetsToMap<ImageConfig>(byFamily('image'));
  const embeddingPresets = presetsToMap<EmbeddingProviderConfig>(byFamily('embedding'));

  // The embeddings family lives at the top level and has no default singleton:
  // the provider selector names the active entry.
  const embeddings: EmbeddingsConfig = config.embeddings ?? { enabled: false, provider: '' };
  const encoderInstalled = models.find((m) => m.id === 'embedding-encoder')?.installed ?? false;
  const sharedGeminiKey = Boolean(config.providers?.gemini?.api_key);
  const setEmbeddings = (patch: Partial<EmbeddingsConfig>) =>
    setConfig({ ...config, embeddings: { ...embeddings, ...patch } });

  return (
    <div className={`flex flex-col h-full ${isCompact ? 'p-2 space-y-4' : 'space-y-6'}`}>
      {/* Settings Navigation & Status Header */}
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-stone-800 pb-3">
        <div className="flex flex-wrap items-center gap-1 bg-stone-950/70 p-1 rounded-xl border border-stone-800">
          <button
            onClick={() => setActiveSubTab('paths')}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-sans transition-all cursor-pointer ${
              activeSubTab === 'paths'
                ? 'bg-purple-600 text-white font-bold shadow'
                : 'text-stone-400 hover:text-stone-200'
            }`}
          >
            <Folder className="w-3.5 h-3.5" />
            <span>Paths</span>
          </button>
          <button
            onClick={() => setActiveSubTab('providers')}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-sans transition-all cursor-pointer ${
              activeSubTab === 'providers'
                ? 'bg-purple-600 text-white font-bold shadow'
                : 'text-stone-400 hover:text-stone-200'
            }`}
          >
            <Cloud className="w-3.5 h-3.5" />
            <span>Providers</span>
          </button>
          <button
            onClick={() => setActiveSubTab('agents')}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-sans transition-all cursor-pointer ${
              activeSubTab === 'agents'
                ? 'bg-purple-600 text-white font-bold shadow'
                : 'text-stone-400 hover:text-stone-200'
            }`}
          >
            <Cpu className="w-3.5 h-3.5" />
            <span>AI Agents</span>
          </button>
          <button
            onClick={() => setActiveSubTab('media')}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-sans transition-all cursor-pointer ${
              activeSubTab === 'media'
                ? 'bg-purple-600 text-white font-bold shadow'
                : 'text-stone-400 hover:text-stone-200'
            }`}
          >
            <Volume2 className="w-3.5 h-3.5" />
            <span>Media Engines</span>
          </button>
          <button
            onClick={() => setActiveSubTab('batch')}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-sans transition-all cursor-pointer ${
              activeSubTab === 'batch'
                ? 'bg-purple-600 text-white font-bold shadow'
                : 'text-stone-400 hover:text-stone-200'
            }`}
          >
            <Layers className="w-3.5 h-3.5" />
            <span>Batch Jobs</span>
          </button>
          <button
            onClick={() => setActiveSubTab('preferences')}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-sans transition-all cursor-pointer ${
              activeSubTab === 'preferences'
                ? 'bg-purple-600 text-white font-bold shadow'
                : 'text-stone-400 hover:text-stone-200'
            }`}
          >
            <Sliders className="w-3.5 h-3.5" />
            <span>Preferences</span>
          </button>
          <button
            onClick={() => setActiveSubTab('usage')}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-sans transition-all cursor-pointer ${
              activeSubTab === 'usage'
                ? 'bg-purple-600 text-white font-bold shadow'
                : 'text-stone-400 hover:text-stone-200'
            }`}
          >
            <Coins className="w-3.5 h-3.5" />
            <span>Usage</span>
          </button>
          <button
            onClick={() => setActiveSubTab('debug')}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-sans transition-all cursor-pointer ${
              activeSubTab === 'debug'
                ? 'bg-purple-600 text-white font-bold shadow'
                : 'text-stone-400 hover:text-stone-200'
            }`}
          >
            <Bug className="w-3.5 h-3.5" />
            <span>Debug</span>
          </button>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <label
            className="flex items-center gap-2 text-xs font-sans text-stone-400 bg-stone-900 px-2.5 py-1 rounded-lg border border-stone-800 cursor-pointer"
            title="Shows the limit sections and the Advanced sub-tab. Stored in this browser, so a desktop window and a browser tab keep their own setting."
          >
            <input
              type="checkbox"
              checked={advanced}
              onChange={(e) => setAdvanced(e.target.checked)}
              className="w-3.5 h-3.5 accent-purple-600 cursor-pointer"
            />
            <span>Show advanced settings</span>
          </label>
          <span className="text-xs font-mono text-stone-400 bg-stone-900 px-2.5 py-1 rounded-lg border border-stone-800">
            {isOverride ? 'Workspace Override' : 'Global User Config'}: {activeFilePath}
          </span>
          <button
            onClick={handleSave}
            disabled={isSaving}
            className="flex items-center gap-1.5 text-xs font-sans font-bold px-4 py-1.5 rounded-xl transition-all cursor-pointer bg-purple-600 hover:bg-purple-500 disabled:opacity-50 text-white shadow active:scale-95"
          >
            <Save className="w-3.5 h-3.5" />
            <span>{isSaving ? 'Saving...' : 'Save Settings'}</span>
          </button>
        </div>
      </div>

      {feedback && (
        <div
          className={`p-3 rounded-xl text-xs flex items-center gap-2 ${
            feedback.type === 'success'
              ? 'bg-emerald-950/50 border border-emerald-500/40 text-emerald-200'
              : 'bg-red-950/50 border border-red-500/40 text-red-200'
          }`}
        >
          {feedback.type === 'success' ? <CheckCircle className="w-4 h-4" /> : <AlertCircle className="w-4 h-4" />}
          <span>{feedback.message}</span>
        </div>
      )}

      {configWarnings.length > 0 && (
        <div className="p-3 rounded-xl text-xs flex items-start gap-2 bg-amber-950/50 border border-amber-500/40 text-amber-200">
          <AlertCircle className="w-4 h-4 mt-0.5 shrink-0" />
          <div className="space-y-1">
            <span className="font-semibold">Configuration problems</span>
            <ul className="list-disc list-inside space-y-0.5">
              {configWarnings.map((warning, index) => (
                <li key={index}>{warning}</li>
              ))}
            </ul>
          </div>
        </div>
      )}

      {/* Tab 1: Storage & Discovery Paths */}
      {activeSubTab === 'paths' && <PathsPanel config={config} setConfig={setConfig} />}

      {/* Tab: Ecosystem Providers */}
      {activeSubTab === 'providers' && (
        <ProvidersPanel
          config={config}
          setConfig={setConfig}
          embeddings={embeddings}
          setEmbeddings={setEmbeddings}
          embeddingPresets={embeddingPresets}
          encoderInstalled={encoderInstalled}
          sharedGeminiKey={sharedGeminiKey}
          setMissingModelPrompt={setMissingModelPrompt}
        />
      )}

      {/* Tab 2: AI Agents & Roles */}
      {/* Tab 2: AI Agents & Roles */}
      {activeSubTab === 'agents' && (
        <AgentsPanel
          config={config}
          setConfig={setConfig}
          advanced={advanced}
          agentPresets={agentPresets}
          llmDescriptors={llmDescriptors}
          tierCaveats={tierCaveats}
          handleTestProvider={handleTestProvider}
          testingCategory={testingCategory}
          testResult={testResult}
        />
      )}

      {/* Tab 3: Media Engines (TTS / STT / Image) */}
      {/* Tab 3: Media Engines (TTS / STT / Image) */}
      {activeSubTab === 'media' && (
        <MediaPanel
          config={config}
          setConfig={setConfig}
          ttsPresets={ttsPresets}
          sttPresets={sttPresets}
          imagePresets={imagePresets}
          models={models}
          handleTestProvider={handleTestProvider}
          testingCategory={testingCategory}
          testResult={testResult}
          setMissingModelPrompt={setMissingModelPrompt}
        />
      )}
      {/* Tab: Batch Speech Backfill */}
      {activeSubTab === 'batch' && (
        <div className="space-y-4 flex-1 overflow-y-auto pr-1">
          <TTSBatchPanel />
        </div>
      )}

      {/* Tab 4: Preferences & Appearance */}
      {activeSubTab === 'preferences' && (
        <PreferencesPanel config={config} setConfig={setConfig} stylePacks={stylePacks} />
      )}

      {/* Tab: Usage & Spend */}
      {activeSubTab === 'usage' && <UsagePanel activeGameID={activeGameID} onOpenDocs={onOpenDocs} />}

      {/* Tab: Debug (developer view over the trace) */}
      {activeSubTab === 'debug' && <DebugPanel config={config} setConfig={setConfig} />}

      <ModelDownloadModal
        isOpen={missingModelPrompt !== null}
        modelId={missingModelPrompt?.id ?? ''}
        modelName={missingModelPrompt?.name ?? ''}
        sizeBytes={missingModelPrompt?.sizeBytes ?? 0}
        onClose={() => setMissingModelPrompt(null)}
      />
    </div>
  );
};
