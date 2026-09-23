import React, { useState } from 'react';
import { Play, Sparkles, PenLine } from 'lucide-react';

interface ProloguePanelProps {
  gameName: string;
  playerName?: string;
  initialPrompt?: string;
  busy?: boolean;
  onBeginStory: (prompt: string) => void;
  onBeginWithAction: () => void;
}

// ProloguePanel is what a campaign shows before it has any history. It exists so
// the story always begins with the GM establishing a scene, rather than with a
// blank console the player has to invent a first move against.
export const ProloguePanel: React.FC<ProloguePanelProps> = ({
  gameName,
  playerName,
  initialPrompt = '',
  busy = false,
  onBeginStory,
  onBeginWithAction,
}) => {
  const [prompt, setPrompt] = useState(initialPrompt);

  return (
    <div className="h-full flex items-center justify-center px-6 py-10 overflow-y-auto">
      <div className="w-full max-w-2xl space-y-5 rounded-2xl bg-glass-card border border-purple-500/20 shadow-2xl p-6 md:p-8">
        <div className="space-y-2 text-center">
          <div className="flex items-center justify-center gap-2 text-xs font-mono uppercase tracking-widest text-purple-400">
            <Sparkles className="w-3.5 h-3.5" />
            <span>Prologue</span>
          </div>
          <h2 className="font-sans text-2xl font-bold text-white tracking-wide">{gameName}</h2>
          <p className="text-sm text-stone-300">
            {playerName ? `${playerName} has not stepped into the story yet. ` : ''}
            Let the Game Master set the opening scene, then take it from there.
          </p>
        </div>

        <div className="space-y-1.5">
          <label className="text-xs font-sans uppercase text-stone-300">
            Opening Prompt <span className="text-stone-500 normal-case">(optional)</span>
          </label>
          <textarea
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            rows={4}
            disabled={busy}
            placeholder="Where should the story open? e.g. Begin in a rain-soaked market at dusk, the city gates closing behind me."
            className="w-full resize-none bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs text-stone-100 focus:outline-none focus:border-purple-500/60 disabled:opacity-60"
          />
          <p className="text-[11px] text-stone-500">
            Leave it blank and the GM will invent the scene from your world and rules.
          </p>
        </div>

        <div className="flex flex-wrap items-center justify-center gap-3 pt-1">
          <button
            onClick={() => onBeginStory(prompt)}
            disabled={busy}
            className="inline-flex items-center gap-2 text-sm font-sans font-bold px-5 py-2.5 rounded-xl bg-purple-600 hover:bg-purple-500 text-white shadow-lg transition-all cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
          >
            <Play className="w-4 h-4 fill-stone-950" />
            <span>{busy ? 'Setting the scene...' : 'Begin the story'}</span>
          </button>
          <button
            onClick={onBeginWithAction}
            disabled={busy}
            className="inline-flex items-center gap-2 text-xs font-sans px-4 py-2.5 rounded-xl bg-stone-900 border border-stone-700 text-stone-300 hover:text-purple-300 hover:border-purple-500/40 transition-all cursor-pointer disabled:opacity-50"
          >
            <PenLine className="w-3.5 h-3.5" />
            <span>I&apos;ll take the first step</span>
          </button>
        </div>
      </div>
    </div>
  );
};
