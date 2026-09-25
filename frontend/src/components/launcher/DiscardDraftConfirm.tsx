import React from 'react';
import { AlertCircle } from 'lucide-react';

interface DiscardDraftConfirmProps {
  isOpen: boolean;
  onCancel: () => void;
  onDiscard: () => void;
  title?: string;
  description?: string;
}

export const DiscardDraftConfirm: React.FC<DiscardDraftConfirmProps> = ({
  isOpen,
  onCancel,
  onDiscard,
  title = 'Discard unsaved world?',
  description = 'This world has not been saved. Leaving now discards every field you entered.',
}) => {
  if (!isOpen) return null;
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm select-none">
      <div className="w-full max-w-md bg-stone-900 border border-white/15 rounded-2xl p-5 shadow-2xl space-y-4">
        <div className="flex items-center gap-2 text-amber-300">
          <AlertCircle className="w-4 h-4" />
          <h3 className="font-sans text-sm font-bold text-white">{title}</h3>
        </div>
        <p className="text-xs font-sans text-stone-400">{description}</p>
        <div className="flex justify-end gap-2">
          <button
            onClick={onCancel}
            className="text-xs font-sans px-3 py-1.5 rounded-lg border border-stone-700 text-stone-300 hover:text-white hover:bg-stone-800 transition-all cursor-pointer"
          >
            Keep editing
          </button>
          <button
            onClick={onDiscard}
            className="text-xs font-sans font-bold px-3 py-1.5 rounded-lg bg-red-600 hover:bg-red-500 text-white transition-all cursor-pointer"
          >
            Discard
          </button>
        </div>
      </div>
    </div>
  );
};
