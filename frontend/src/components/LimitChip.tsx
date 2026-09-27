import React, { useState, useEffect } from 'react';
import { LimitState } from '../types';
import { Clock } from 'lucide-react';

interface LimitChipProps {
  block: LimitState;
}

export const LimitChip: React.FC<LimitChipProps> = ({ block }) => {
  const [secondsRemaining, setSecondsRemaining] = useState<number>(() => {
    if (!block.until) return 0;
    const diff = new Date(block.until).getTime() - Date.now();
    return Math.max(0, Math.ceil(diff / 1000));
  });

  useEffect(() => {
    if (!block.until) return;
    const update = () => {
      const diff = new Date(block.until!).getTime() - Date.now();
      setSecondsRemaining(Math.max(0, Math.ceil(diff / 1000)));
    };
    update();
    const timer = setInterval(update, 1000);
    return () => clearInterval(timer);
  }, [block.until]);

  if (secondsRemaining <= 0) return null;

  return (
    <div
      className="flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-mono bg-amber-500/10 border border-amber-500/30 text-amber-300 animate-pulse shadow-sm"
      title={`Provider ${block.provider} (${block.role}) is temporarily rate-limited. Resumes in ${secondsRemaining}s.`}
    >
      <Clock className="w-3.5 h-3.5 text-amber-400" />
      <span>
        {block.provider} ({block.role}): {secondsRemaining}s
      </span>
    </div>
  );
};
