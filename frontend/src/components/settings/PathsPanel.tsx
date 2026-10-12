import React from 'react';
import { Folder } from 'lucide-react';
import type { AppConfig } from '../../types';

interface PathsPanelProps {
  config: AppConfig;
  setConfig: (config: AppConfig) => void;
}

// PathsPanel owns the four storage roots: where rule systems, world lore,
// campaigns, and the media cache are found.
export const PathsPanel: React.FC<PathsPanelProps> = ({ config, setConfig }) => (
  <div className="space-y-4 flex-1 overflow-y-auto pr-1">
    <div className="p-4 rounded-xl bg-glass-card border border-stone-800 space-y-4">
      <h3 className="font-sans text-sm font-bold text-purple-400 flex items-center gap-2">
        <Folder className="w-4 h-4" />
        <span>Storage & Discovery Paths</span>
      </h3>
      <p className="text-xs text-stone-400">
        Configure directories where LocalRPG looks for rule systems, world lore, saved campaigns, and generated media
        caches.
      </p>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div className="space-y-1.5">
          <label className="text-xs font-sans uppercase text-stone-300">Rule Systems Directory</label>
          <input
            type="text"
            value={config.paths.systems}
            onChange={(e) => setConfig({ ...config, paths: { ...config.paths, systems: e.target.value } })}
            className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
          />
        </div>

        <div className="space-y-1.5">
          <label className="text-xs font-sans uppercase text-stone-300">Worlds Directory</label>
          <input
            type="text"
            value={config.paths.worlds}
            onChange={(e) => setConfig({ ...config, paths: { ...config.paths, worlds: e.target.value } })}
            className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
          />
        </div>

        <div className="space-y-1.5">
          <label className="text-xs font-sans uppercase text-stone-300">Saved Campaigns Directory</label>
          <input
            type="text"
            value={config.paths.games}
            onChange={(e) => setConfig({ ...config, paths: { ...config.paths, games: e.target.value } })}
            className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
          />
        </div>

        <div className="space-y-1.5">
          <label className="text-xs font-sans uppercase text-stone-300">Media Cache Directory</label>
          <input
            type="text"
            value={config.paths.cache}
            onChange={(e) => setConfig({ ...config, paths: { ...config.paths, cache: e.target.value } })}
            className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
          />
        </div>
      </div>
    </div>
  </div>
);

export default PathsPanel;
