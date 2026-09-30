import React from 'react';
import { TurnSegment } from '../../types';
import { MarkdownProse } from '../MarkdownProse';

interface TheaterDialogueProps {
  segment?: TurnSegment;
  fallback: string;
  speaker?: string;
  isPlayer?: boolean;
  onEntityClick?: (entityId: string) => void;
  displayMode?: 'stage_directions' | 'hidden' | 'raw';
  // reveal is the share of the line to show, so an exported bundle can type it out
  // while the app shows it whole. The advance caret waits until it is complete.
  reveal?: number;
  onAdvance: () => void;
}

// TheaterDialogue shows one beat at a time: a speaker name plate, an optional
// portrait, and the line. Clicking anywhere on the panel advances.
export const TheaterDialogue: React.FC<TheaterDialogueProps> = ({
  segment,
  fallback,
  speaker,
  isPlayer = false,
  onEntityClick,
  displayMode,
  reveal = 1,
  onAdvance,
}) => {
  const text = segment?.text ?? fallback;
  const shown = reveal >= 1 ? text : text.slice(0, Math.max(1, Math.round(text.length * reveal)));
  const isSpeech = segment?.kind === 'speech';
  const name = isSpeech ? segment?.speaker || speaker || 'Unknown' : 'Narrator';

  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onAdvance}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          onAdvance();
        }
      }}
      className="relative w-full max-w-4xl mx-auto text-left pointer-events-auto cursor-pointer"
      title="Click to continue"
      aria-label={isSpeech ? `${name}: ${text}` : text}
    >
      <div className="flex items-end gap-3">
        {isSpeech && segment?.portrait_url && (
          <div
            className={`w-16 h-16 rounded-lg overflow-hidden shrink-0 border-2 shadow-2xl bg-black/40 ${
              isPlayer ? 'border-sky-400/80' : 'border-purple-400/80'
            }`}
          >
            <img src={segment.portrait_url} alt="" aria-hidden="true" className="w-full h-full object-cover" />
          </div>
        )}
        <div
          className={`relative w-full bg-stone-950/90 backdrop-blur-xl rounded-2xl border shadow-2xl px-6 py-5 min-h-[20vh] ${
            isPlayer ? 'border-sky-400/50' : isSpeech ? 'border-purple-400/50' : 'border-white/15'
          }`}
        >
          <span
            className={`absolute -top-3.5 left-6 px-3 py-1 rounded-md text-sm font-sans font-extrabold tracking-wide shadow-lg ${
              isPlayer ? 'bg-sky-600 text-white' : isSpeech ? 'bg-purple-600 text-white' : 'bg-stone-700 text-stone-100'
            }`}
          >
            {name}
          </span>
          <MarkdownProse
            text={isSpeech ? `\u201c${shown}\u201d` : shown}
            onEntityClick={onEntityClick}
            displayMode={displayMode}
            className={
              isSpeech
                ? 'text-stone-50 text-xl leading-relaxed italic space-y-2'
                : 'text-stone-200 text-xl leading-relaxed tracking-wide font-serif space-y-3'
            }
          />
          {reveal >= 1 && (
            <span className="absolute bottom-2 right-3 text-purple-300/70 animate-pulse text-xs" aria-hidden="true">
              &#9662;
            </span>
          )}
        </div>
      </div>
    </div>
  );
};
