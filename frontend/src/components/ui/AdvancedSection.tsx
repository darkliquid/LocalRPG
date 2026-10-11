import React, { useState, type ReactNode } from 'react';
import { ChevronDown, ChevronRight } from 'lucide-react';

interface AdvancedSectionProps {
  label?: string;
  children: ReactNode;
  defaultOpen?: boolean;
}

// AdvancedSection keeps rarely-tuned settings out of the way until they are asked
// for, so a dense panel leads with what most people set and the rest is one click
// away. It is collapsed by default.
export const AdvancedSection: React.FC<AdvancedSectionProps> = ({
  label = 'Advanced',
  children,
  defaultOpen = false,
}) => {
  const [open, setOpen] = useState(defaultOpen);

  return (
    <div className="space-y-2">
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
        className="flex items-center gap-1.5 text-xs font-sans font-semibold text-stone-300 hover:text-purple-300 transition-colors cursor-pointer"
      >
        {open ? <ChevronDown className="w-3.5 h-3.5" /> : <ChevronRight className="w-3.5 h-3.5" />}
        <span>{label}</span>
      </button>
      {open && children}
    </div>
  );
};

export default AdvancedSection;