import React, { useState } from 'react';
import { Send, Mic, Dices, MessageSquare, Zap, Compass } from 'lucide-react';

interface ActionConsoleProps {
  onSubmit: (mode: string, text: string) => void;
  disabled?: boolean;
}

export const ActionConsole: React.FC<ActionConsoleProps> = ({ onSubmit, disabled }) => {
  const [text, setText] = useState('');
  const [mode, setMode] = useState<'do' | 'say' | 'story' | 'roll'>('do');

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!text.trim() || disabled) return;
    onSubmit(mode, text.trim());
    setText('');
  };

  return (
    <div className="border-t border-white/5 bg-black/40 backdrop-blur-md p-4 w-full">
      {/* Mode Switcher Tabs */}
      <div className="flex items-center gap-2 mb-3">
        <button
          type="button"
          onClick={() => setMode('do')}
          className={`flex items-center gap-1.5 px-3 py-1 rounded-lg text-xs font-cinzel tracking-wider transition-all cursor-pointer ${
            mode === 'do' ? 'bg-amber-600 text-stone-950 font-bold shadow-md' : 'bg-stone-900/60 text-stone-400 hover:bg-stone-800 hover:text-stone-200'
          }`}
        >
          <Zap className="w-3 h-3" />
          <span>DO</span>
        </button>
        <button
          type="button"
          onClick={() => setMode('say')}
          className={`flex items-center gap-1.5 px-3 py-1 rounded-lg text-xs font-cinzel tracking-wider transition-all cursor-pointer ${
            mode === 'say' ? 'bg-amber-600 text-stone-950 font-bold shadow-md' : 'bg-stone-900/60 text-stone-400 hover:bg-stone-800 hover:text-stone-200'
          }`}
        >
          <MessageSquare className="w-3 h-3" />
          <span>SAY</span>
        </button>
        <button
          type="button"
          onClick={() => setMode('story')}
          className={`flex items-center gap-1.5 px-3 py-1 rounded-lg text-xs font-cinzel tracking-wider transition-all cursor-pointer ${
            mode === 'story' ? 'bg-amber-600 text-stone-950 font-bold shadow-md' : 'bg-stone-900/60 text-stone-400 hover:bg-stone-800 hover:text-stone-200'
          }`}
        >
          <Compass className="w-3 h-3" />
          <span>STORY</span>
        </button>
        <button
          type="button"
          onClick={() => setMode('roll')}
          className={`flex items-center gap-1.5 px-3 py-1 rounded-lg text-xs font-cinzel tracking-wider transition-all cursor-pointer ${
            mode === 'roll' ? 'bg-amber-600 text-stone-950 font-bold shadow-md' : 'bg-stone-900/60 text-stone-400 hover:bg-stone-800 hover:text-stone-200'
          }`}
        >
          <Dices className="w-3 h-3" />
          <span>ROLL</span>
        </button>
      </div>

      {/* Input Bar with STT and Submit */}
      <form onSubmit={handleSubmit} className="flex items-center gap-2">
        <input
          type="text"
          value={text}
          onChange={(e) => setText(e.target.value)}
          placeholder={
            mode === 'do' ? 'Describe your action...' :
            mode === 'say' ? 'What do you speak aloud?' :
            mode === 'story' ? 'Director note / narrative steering...' : 'Enter dice expression (e.g. 1d20+5, 4d6kh3)...'
          }
          disabled={disabled}
          className="flex-1 bg-black/50 border border-white/10 rounded-xl px-4 py-2.5 text-stone-100 placeholder-stone-500 focus:outline-none focus:border-amber-500/80 font-sans text-base transition-all"
        />
        <button
          type="button"
          className="p-2.5 rounded-xl bg-stone-900/70 border border-white/5 hover:bg-stone-800 text-stone-400 hover:text-amber-400 transition-all cursor-pointer"
          title="Speech-to-Text Voice Input"
        >
          <Mic className="w-5 h-5" />
        </button>
        <button
          type="submit"
          disabled={disabled || !text.trim()}
          className="px-5 py-2.5 rounded-xl bg-amber-600 hover:bg-amber-500 disabled:opacity-40 disabled:cursor-not-allowed text-stone-950 font-cinzel font-bold flex items-center gap-1.5 shadow-lg transition-all cursor-pointer"
        >
          <span>Submit</span>
          <Send className="w-4 h-4" />
        </button>
      </form>
    </div>
  );
};
