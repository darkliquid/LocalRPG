import React, { useState } from 'react';
import { Send, Mic, Dices, MessageSquare, Zap, Compass, Square, Loader2 } from 'lucide-react';
import { useVoiceInput } from '../hooks/useVoiceInput';

interface ActionConsoleProps {
  onSubmit: (mode: string, text: string) => void;
  disabled?: boolean;
  streaming?: boolean;
  onStop?: () => void;
  sttType?: string;
}

export const ActionConsole: React.FC<ActionConsoleProps> = ({
  onSubmit,
  disabled,
  streaming,
  onStop,
  sttType,
}) => {
  const [text, setText] = useState('');
  const [mode, setMode] = useState<'do' | 'say' | 'story' | 'roll'>('do');

  const handleVoiceTranscribed = (transcript: string) => {
    setText((prev) => (prev ? `${prev} ${transcript}` : transcript));
  };

  const { isRecording, isTranscribing, error: voiceError, toggleRecording } = useVoiceInput({
    onTranscribed: handleVoiceTranscribed,
    sttType,
  });

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!text.trim() || disabled || streaming) return;
    onSubmit(mode, text.trim());
    setText('');
  };

  const isInputDisabled = disabled || streaming;

  return (
    <div className="border-t border-white/5 bg-black/40 backdrop-blur-md p-4 w-full">
      {/* Mode Switcher Tabs */}
      <div className="flex flex-wrap items-center gap-2 mb-3">
        <button
          type="button"
          onClick={() => setMode('do')}
          disabled={isInputDisabled}
          className={`flex items-center gap-1.5 px-3 py-1 rounded-lg text-xs font-cinzel tracking-wider transition-all cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed ${
            mode === 'do' ? 'bg-amber-600 text-stone-950 font-bold shadow-md' : 'bg-stone-900/60 text-stone-400 hover:bg-stone-800 hover:text-stone-200'
          }`}
        >
          <Zap className="w-3 h-3" />
          <span>DO</span>
        </button>
        <button
          type="button"
          onClick={() => setMode('say')}
          disabled={isInputDisabled}
          className={`flex items-center gap-1.5 px-3 py-1 rounded-lg text-xs font-cinzel tracking-wider transition-all cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed ${
            mode === 'say' ? 'bg-amber-600 text-stone-950 font-bold shadow-md' : 'bg-stone-900/60 text-stone-400 hover:bg-stone-800 hover:text-stone-200'
          }`}
        >
          <MessageSquare className="w-3 h-3" />
          <span>SAY</span>
        </button>
        <button
          type="button"
          onClick={() => setMode('story')}
          disabled={isInputDisabled}
          className={`flex items-center gap-1.5 px-3 py-1 rounded-lg text-xs font-cinzel tracking-wider transition-all cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed ${
            mode === 'story' ? 'bg-amber-600 text-stone-950 font-bold shadow-md' : 'bg-stone-900/60 text-stone-400 hover:bg-stone-800 hover:text-stone-200'
          }`}
        >
          <Compass className="w-3 h-3" />
          <span>STORY</span>
        </button>
        <button
          type="button"
          onClick={() => setMode('roll')}
          disabled={isInputDisabled}
          className={`flex items-center gap-1.5 px-3 py-1 rounded-lg text-xs font-cinzel tracking-wider transition-all cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed ${
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
          disabled={isInputDisabled}
          className="flex-1 min-w-0 bg-black/50 border border-white/10 rounded-xl px-4 py-2.5 text-stone-100 placeholder-stone-500 focus:outline-none focus:border-amber-500/80 font-sans text-base transition-all disabled:opacity-40 disabled:cursor-not-allowed"
        />
        <button
          type="button"
          onClick={toggleRecording}
          disabled={isInputDisabled || isTranscribing}
          className={`shrink-0 p-2.5 rounded-xl border transition-all cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed ${
            isRecording
              ? 'bg-red-900/60 text-red-300 border-red-500/80 animate-pulse shadow-lg shadow-red-900/40'
              : 'bg-stone-900/70 border-white/5 hover:bg-stone-800 text-stone-400 hover:text-amber-400'
          }`}
          title={
            isRecording
              ? 'Recording speech... Click to stop and transcribe'
              : isTranscribing
              ? 'Transcribing audio...'
              : 'Dictate action with voice'
          }
        >
          {isTranscribing ? (
            <Loader2 className="w-5 h-5 animate-spin text-amber-400" />
          ) : (
            <Mic className={`w-5 h-5 ${isRecording ? 'text-red-300' : ''}`} />
          )}
        </button>
        {streaming ? (
          <button
            type="button"
            onClick={onStop}
            className="shrink-0 flex items-center gap-1.5 px-3.5 py-2.5 rounded-xl bg-stone-800 hover:bg-stone-700 text-stone-200 text-sm font-cinzel cursor-pointer"
          >
            <Square className="w-4 h-4" />
            <span>STOP</span>
          </button>
        ) : (
          <button
            type="submit"
            disabled={disabled || !text.trim()}
            className="shrink-0 px-5 py-2.5 rounded-xl bg-amber-600 hover:bg-amber-500 disabled:opacity-40 disabled:cursor-not-allowed text-stone-950 font-cinzel font-bold flex items-center gap-1.5 shadow-lg transition-all cursor-pointer"
          >
            <span>Submit</span>
            <Send className="w-4 h-4" />
          </button>
        )}
      </form>
      {voiceError && (
        <div className="mt-1.5 text-xs text-red-400/90 font-mono px-1">
          {voiceError}
        </div>
      )}
    </div>
  );
};
