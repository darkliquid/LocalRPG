import React, { useState } from 'react';
import { Copy, Sparkles } from 'lucide-react';

export interface BaseSystem {
  id: string;
  name: string;
  description?: string;
}

export interface BaseSystemCatalogueProps {
  bases: BaseSystem[];
  onClone: (base: BaseSystem) => void;
  onDerive: (base: BaseSystem, instruction: string) => void;
  busy?: boolean;
}

// BaseSystemCatalogue presents the reference systems as starting points. Start
// from this base copies one into a new system to edit by hand; Derive generates a
// variant from an instruction.
export const BaseSystemCatalogue: React.FC<BaseSystemCatalogueProps> = ({
  bases,
  onClone,
  onDerive,
  busy,
}) => {
  const [deriving, setDeriving] = useState<string | null>(null);
  const [instruction, setInstruction] = useState('');

  return (
    <div className="space-y-2" data-testid="base-system-catalogue">
      {bases.map((base) => (
        <div key={base.id} className="p-3 rounded-xl border border-white/10 bg-white/[0.03]">
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <h4 className="text-sm font-semibold text-white truncate">{base.name}</h4>
              {base.description && (
                <p className="text-xs text-neutral-400 line-clamp-2">{base.description}</p>
              )}
            </div>
            <div className="flex items-center gap-2 shrink-0">
              <button
                type="button"
                onClick={() => onClone(base)}
                className="flex items-center gap-1.5 text-xs px-2.5 py-1.5 rounded-lg border border-white/10 hover:bg-white/5 text-neutral-200 transition-colors"
              >
                <Copy className="w-3.5 h-3.5" />
                Start from this base
              </button>
              <button
                type="button"
                onClick={() => {
                  setDeriving(deriving === base.id ? null : base.id);
                  setInstruction('');
                }}
                className="flex items-center gap-1.5 text-xs px-2.5 py-1.5 rounded-lg border border-amber-500/40 bg-amber-600/15 hover:bg-amber-600/25 text-amber-300 transition-colors"
              >
                <Sparkles className="w-3.5 h-3.5" />
                Derive
              </button>
            </div>
          </div>
          {deriving === base.id && (
            <div className="mt-3 space-y-2">
              <label htmlFor={`derive-${base.id}`} className="text-xs text-neutral-300">
                How should the variant change?
              </label>
              <textarea
                id={`derive-${base.id}`}
                value={instruction}
                onChange={(event) => setInstruction(event.target.value)}
                rows={2}
                placeholder="add a sanity stat and an occult skill"
                className="w-full px-3 py-2 text-sm text-white bg-white/[0.03] border border-white/10 rounded-xl focus:border-amber-500/50 focus:outline-none resize-none"
              />
              <div className="flex justify-end">
                <button
                  type="button"
                  disabled={busy || instruction.trim() === ''}
                  onClick={() => onDerive(base, instruction)}
                  className="text-xs font-medium px-3 py-1.5 rounded-lg bg-amber-600 hover:bg-amber-500 text-white disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
                >
                  Derive variant
                </button>
              </div>
            </div>
          )}
        </div>
      ))}
    </div>
  );
};

export default BaseSystemCatalogue;
