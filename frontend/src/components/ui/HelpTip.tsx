import React, { useId, useState } from 'react';
import { HelpCircle } from 'lucide-react';

export interface HelpTipProps {
  // label names the concept the tip explains, and becomes the accessible name.
  label: string;
  children: React.ReactNode;
  className?: string;
}

// HelpTip is a small "?" affordance that explains a concept on hover or click,
// so a dense editor documents itself without a separate manual. Clicking pins
// the tip open, so it survives the pointer leaving the button.
export const HelpTip: React.FC<HelpTipProps> = ({ label, children, className }) => {
  const [hovered, setHovered] = useState(false);
  const [pinned, setPinned] = useState(false);
  const id = useId();
  const open = hovered || pinned;

  return (
    <span
      className={`relative inline-flex align-middle ${className ?? ''}`}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
    >
      <button
        type="button"
        aria-label={`Help: ${label}`}
        aria-expanded={open}
        aria-controls={id}
        onClick={(event) => {
          event.preventDefault();
          event.stopPropagation();
          setPinned((value) => !value);
        }}
        className="inline-flex h-4 w-4 items-center justify-center rounded-full border border-stone-600 text-stone-400 hover:border-purple-500/60 hover:text-purple-300 focus:outline-none focus:ring-1 focus:ring-purple-500/60 cursor-help transition-colors"
      >
        <HelpCircle className="h-3 w-3" />
      </button>
      {open && (
        <span
          id={id}
          role="tooltip"
          className="absolute left-0 top-5 z-40 w-64 space-y-1.5 rounded-lg border border-purple-500/30 bg-stone-950/98 p-2.5 text-[11px] font-sans normal-case leading-relaxed tracking-normal text-stone-300 shadow-xl"
        >
          {children}
        </span>
      )}
    </span>
  );
};

export default HelpTip;
