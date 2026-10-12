import React from 'react';
import { Sparkles } from 'lucide-react';
import type { AppConfig } from '../../types';
import { AdvancedSection } from '../ui/AdvancedSection';

interface GenerationLimitsPanelProps {
  config: AppConfig;
  setConfig: (config: AppConfig) => void;
  advanced: boolean;
}

// GenerationLimitsPanel bounds world generation: how many model calls a run
// from a brief may make, and how much source an import may read.
export const GenerationLimitsPanel: React.FC<GenerationLimitsPanelProps> = ({ config, setConfig, advanced }) => (
  <AdvancedSection label="Advanced" defaultOpen={advanced}>
    <div className="p-4 rounded-xl bg-glass-card border border-stone-800 space-y-4">
      <h3 className="font-sans text-sm font-bold text-purple-400 flex items-center gap-2">
        <Sparkles className="w-4 h-4" />
        <span>Generation Limits</span>
      </h3>
      <p className="text-xs text-stone-400">
        World generation spends model calls. A generation from a brief, a batch of entities, or an enhancement is a
        short pipeline, so its call count bounds it. Importing a folder or a list of pages reads whatever you point it
        at, so the amount of source it will read bounds that instead, and the calls follow from it.
      </p>
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div className="space-y-1.5">
          <label className="text-xs font-sans uppercase text-stone-300">Call Limit</label>
          <input
            type="number"
            min={1}
            max={1000}
            step={1}
            value={config.generation?.max_calls ?? 20}
            onChange={(e) => {
              const parsed = parseInt(e.target.value, 10);
              setConfig({
                ...config,
                generation: { ...config.generation, max_calls: Number.isNaN(parsed) ? 20 : parsed },
              });
            }}
            className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
          />
          <p className="text-xs text-stone-500">
            The most calls one generation from a brief, batch, or enhancement may make.
          </p>
        </div>
        <div className="space-y-1.5">
          <label className="text-xs font-sans uppercase text-stone-300">Source Chunk Limit</label>
          <input
            type="number"
            min={1}
            max={20000}
            step={10}
            value={config.generation?.max_chunks ?? 200}
            onChange={(e) => {
              const parsed = parseInt(e.target.value, 10);
              setConfig({
                ...config,
                generation: { ...config.generation, max_chunks: Number.isNaN(parsed) ? 200 : parsed },
              });
            }}
            className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
          />
          <p className="text-xs text-stone-500">
            The most chunks one import may read. Four chunks are one call, so 200 chunks is 50 calls.
          </p>
        </div>
      </div>
    </div>
  </AdvancedSection>
);

export default GenerationLimitsPanel;
