import React, { useState } from 'react';
import { BrainCircuit, Cloud, Layers } from 'lucide-react';
import type { AppConfig, EmbeddingsConfig } from '../../types';
import { CloudKeysPanel } from './CloudKeysPanel';
import { EmbeddingsPanel } from './EmbeddingsPanel';
import type { EmbeddingPreset } from '../EmbeddingProviderEditor';
import { OfflinePreset } from '../OfflinePreset';
import { SubTabs } from '../ui/SubTabs';

interface ProvidersPanelProps {
  config: AppConfig;
  setConfig: (config: AppConfig) => void;
  embeddings: EmbeddingsConfig;
  setEmbeddings: (patch: Partial<EmbeddingsConfig>) => void;
  embeddingPresets: Record<string, EmbeddingPreset>;
  encoderInstalled: boolean;
  sharedGeminiKey: boolean;
  setMissingModelPrompt: (prompt: { id: string; name: string; sizeBytes: number } | null) => void;
}

// ProvidersPanel owns the account-level credentials and the embedding
// provider: the keys several providers share, and one place to choose the
// embedding model. It keeps its own secondary tab, so the studio shell does
// not track which of the three the user last read.
export const ProvidersPanel: React.FC<ProvidersPanelProps> = ({
  config,
  setConfig,
  embeddings,
  setEmbeddings,
  embeddingPresets,
  encoderInstalled,
  sharedGeminiKey,
  setMissingModelPrompt,
}) => {
  const [providersTab, setProvidersTab] = useState<'cloud' | 'embeddings' | 'offline'>('cloud');

  return (
  <div className="space-y-4 flex-1 overflow-y-auto pr-1">
    <SubTabs
      ariaLabel="Providers"
      tabs={[
        { id: 'cloud', label: 'Cloud Keys', icon: <Cloud className="w-3.5 h-3.5" /> },
        { id: 'embeddings', label: 'Embeddings', icon: <BrainCircuit className="w-3.5 h-3.5" /> },
        { id: 'offline', label: 'Offline', icon: <Layers className="w-3.5 h-3.5" /> },
      ]}
      active={providersTab}
      onChange={(id) => setProvidersTab(id as 'cloud' | 'embeddings' | 'offline')}
    />

    {providersTab === 'offline' && <OfflinePreset onChange={setConfig} />}

    {providersTab === 'embeddings' && (
      <EmbeddingsPanel
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

    {providersTab === 'cloud' && <CloudKeysPanel config={config} setConfig={setConfig} />}
  </div>
  );
};

export default ProvidersPanel;
