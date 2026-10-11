import React, { useMemo, useState } from 'react';
import {
  Mic,
  Sparkles,
  Volume2,
} from 'lucide-react';
import type {
  AppConfig,
  ImageConfig,
  ModelStatus,
  ProviderVoice,
  STTConfig,
  TestProviderResponse,
  TTSConfig,
} from '../../types';
import { APIClient } from '../../api/client';
import { ProviderManager } from '../ProviderManager';
import { STTProviderEditor } from '../providers/STTProviderEditor';
import { ImageProviderEditor } from '../providers/ImageProviderEditor';
import { TTSProviderEditor } from '../providers/TTSProviderEditor';
import { VoiceCatalogModal } from '../VoiceCatalogModal';
import { SubTabs } from '../ui/SubTabs';
import { KOKORO_VOICE_PROFILES } from '../../lib/voiceProfiles';
import { hasWebSpeechSupport } from '../../lib/webSpeech';
import { DEFAULT_TTS_PREVIEW_TEXT, mediaEntryValue, setMediaEntry } from '../../lib/mediaProviders';
import { useTTSInspect } from '../../hooks/useTTSInspect';

type PresetMap<T> = Record<string, { label: string; description: string; config: T; tier?: string; caveat?: string }>;

interface MediaPanelProps {
  config: AppConfig;
  setConfig: (config: AppConfig) => void;
  ttsPresets: PresetMap<TTSConfig>;
  sttPresets: PresetMap<STTConfig>;
  imagePresets: PresetMap<ImageConfig>;
  models: ModelStatus[];
  handleTestProvider: (category: 'llm' | 'tts' | 'stt' | 'image', provider: unknown, testPrompt?: string) => void;
  testingCategory: string | null;
  testResult: { category: string; res: TestProviderResponse } | null;
  setMissingModelPrompt: (prompt: { id: string; name: string; sizeBytes: number } | null) => void;
}

// MediaPanel owns the three media engines and the voice catalogue. Which engine
// is on screen, the selected provider entry, and the inspected voice list are
// all its own state, so the studio shell carries none of it.
export const MediaPanel: React.FC<MediaPanelProps> = ({
  config,
  setConfig,
  ttsPresets,
  sttPresets,
  imagePresets,
  models,
  handleTestProvider,
  testingCategory,
  testResult,
  setMissingModelPrompt,
}) => {
  const [mediaTab, setMediaTab] = useState<'tts' | 'stt' | 'images'>('tts');
  // Web Speech runs in the browser, not the backend, so its availability is a
  // property of this window rather than of the provider catalogue.
  const webSpeechAvailable = hasWebSpeechSupport();
  const [ttsPreviewText, setTtsPreviewText] = useState<string>(DEFAULT_TTS_PREVIEW_TEXT);
  const [isCatalogModalOpen, setIsCatalogModalOpen] = useState(false);

  // The selected entry of each media family: "default" is the singleton, a name
  // is a map entry. The manager edits whichever entry is selected.
  const [selectedTts, setSelectedTts] = useState<string>('default');
  const [selectedStt, setSelectedStt] = useState<string>('default');
  const [selectedImage, setSelectedImage] = useState<string>('default');

  // The inspect describes the configuration in hand, so a provider is described
  // before it is saved. The fallback keeps the hook unconditional during load.
  const activeTtsConfig = config ? (mediaEntryValue(config, 'tts', selectedTts) as TTSConfig) : undefined;
  const inspectConfig = activeTtsConfig ?? { type: 'disabled' as const, auto_play: false, master_volume: 1 };
  const { inspect, loading: inspecting, refresh: refreshInspect, error: inspectError } = useTTSInspect(
    inspectConfig,
    Boolean(config)
  );

  const isBuiltinKokoro =
    activeTtsConfig?.type === 'builtin' &&
    (activeTtsConfig?.builtin_name === 'sherpa-onnx' || activeTtsConfig?.builtin_name === 'kokoro');
  const isKokoro =
    Boolean(isBuiltinKokoro) ||
    (activeTtsConfig?.type === 'http' &&
      (activeTtsConfig?.model === 'kokoro' || (activeTtsConfig?.endpoint || '').includes('8880')));

  const kokoroCatalogVoices: ProviderVoice[] = useMemo(
    () =>
      KOKORO_VOICE_PROFILES.map((p) => ({
        id: p.voice_id,
        name: p.name,
        tags: p.tags,
        description: p.description,
      })),
    []
  );

  const availableTTSVoices = useMemo(() => {
    if (inspect?.catalog?.voices && inspect.catalog.voices.length > 0) {
      return inspect.catalog.voices;
    }
    if (isKokoro) {
      return kokoroCatalogVoices;
    }
    return [];
  }, [inspect?.catalog?.voices, isKokoro, kokoroCatalogVoices]);

  const kokoroStatus = models.find((m) => m.id === 'kokoro-tts');
  const isGeminiTTS =
    activeTtsConfig?.type === 'gemini' ||
    (activeTtsConfig?.type === 'builtin' && activeTtsConfig?.builtin_name === 'gemini');
  const isElevenLabsTTS =
    activeTtsConfig?.type === 'builtin' && activeTtsConfig?.builtin_name === 'elevenlabs';
  const isCartesiaTTS =
    activeTtsConfig?.type === 'cartesia' ||
    (activeTtsConfig?.type === 'builtin' && activeTtsConfig?.builtin_name === 'cartesia');

  return (
    <>
  <div className="space-y-4 flex-1 overflow-y-auto pr-1">
    <SubTabs
      ariaLabel="Media Engines"
      tabs={[
        { id: 'tts', label: 'TTS', icon: <Volume2 className="w-3.5 h-3.5" /> },
        { id: 'stt', label: 'STT', icon: <Mic className="w-3.5 h-3.5" /> },
        { id: 'images', label: 'Images', icon: <Sparkles className="w-3.5 h-3.5" /> },
      ]}
      active={mediaTab}
      onChange={(id) => setMediaTab(id as 'tts' | 'stt' | 'images')}
    />

    {mediaTab === 'tts' && (
    <ProviderManager
      family="tts"
      config={config}
      onChange={setConfig}
      selected={selectedTts}
      onSelect={setSelectedTts}
      renderEditor={(_name, entry, onChange) => (
        <TTSProviderEditor
          entry={entry as TTSConfig}
          onChange={onChange as (next: TTSConfig) => void}
          presets={ttsPresets}
          voices={availableTTSVoices}
          inspect={inspect}
          inspecting={inspecting}
          onRefreshInspect={refreshInspect}
          inspectError={inspectError}
          kokoroStatus={kokoroStatus}
          isBuiltinKokoro={isBuiltinKokoro}
          isKokoro={isKokoro}
          isGeminiTTS={isGeminiTTS}
          isElevenLabsTTS={isElevenLabsTTS}
          isCartesiaTTS={isCartesiaTTS}
          geminiSharedKey={Boolean(config.providers?.gemini?.api_key)}
          cartesiaSharedKey={Boolean(config.providers?.cartesia?.api_key)}
          previewText={ttsPreviewText}
          onPreviewTextChange={setTtsPreviewText}
          onOpenVoiceCatalog={() => setIsCatalogModalOpen(true)}
          onTest={handleTestProvider}
          testingCategory={testingCategory}
          testResult={testResult}
          onMissingModel={setMissingModelPrompt}
        />
      )}
    />
    )}

    {mediaTab === 'stt' && (
    <ProviderManager
      family="stt"
      config={config}
      onChange={setConfig}
      selected={selectedStt}
      onSelect={setSelectedStt}
      renderEditor={(_name, entry, onChange) => (
        <STTProviderEditor
          entry={entry as STTConfig}
          onChange={onChange as (next: STTConfig) => void}
          presets={sttPresets}
          webSpeechAvailable={webSpeechAvailable}
          cartesiaSharedKey={Boolean(config.providers?.cartesia?.api_key)}
          onTest={handleTestProvider}
          testingCategory={testingCategory}
          testResult={testResult}
        />
      )}
    />
    )}

    {mediaTab === 'images' && (
    <ProviderManager
      family="image"
      config={config}
      onChange={setConfig}
      selected={selectedImage}
      onSelect={setSelectedImage}
      renderEditor={(_name, entry, onChange) => (
        <ImageProviderEditor
          entry={entry as ImageConfig}
          onChange={onChange as (next: ImageConfig) => void}
          presets={imagePresets}
          geminiSharedKey={Boolean(config.providers?.gemini?.api_key)}
          onTest={handleTestProvider}
          testingCategory={testingCategory}
          testResult={testResult}
        />
      )}
    />
    )}
  </div>

      <VoiceCatalogModal
        isOpen={isCatalogModalOpen}
        onClose={() => setIsCatalogModalOpen(false)}
        voices={inspect?.catalog?.voices ?? []}
        providerKey={inspect?.provider_key || 'tts'}
        onSearch={
          isGeminiTTS
            ? async (query) => {
                const res = await APIClient.searchTTSVoices({
                  config: mediaEntryValue(config, 'tts', selectedTts) as TTSConfig,
                  query,
                });
                if (res.error) throw new Error(res.error);
                return res.voices;
              }
            : undefined
        }
        searchLabel="Search the Gemini extended voice library"
        onAddProfile={(newProfile) => {
          const entry = mediaEntryValue(config, 'tts', selectedTts) as TTSConfig;
          const current = entry.voice_profiles || [];
          setConfig(setMediaEntry(config, 'tts', selectedTts, { ...entry, voice_profiles: [...current, newProfile] }));
        }}
      />
    </>
  );
};

export default MediaPanel;
