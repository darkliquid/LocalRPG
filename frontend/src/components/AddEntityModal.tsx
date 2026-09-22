import React, { useState } from 'react';
import { UserPlus, BookOpen, Check, X } from 'lucide-react';

interface AddEntityModalProps {
  isOpen: boolean;
  entityName: string;
  onQuickCreate: (name: string, type: string) => void;
  onEditInCodex: (name: string, type: string) => void;
  onClose: () => void;
}

export const AddEntityModal: React.FC<AddEntityModalProps> = ({
  isOpen,
  entityName,
  onQuickCreate,
  onEditInCodex,
  onClose,
}) => {
  const [type, setType] = useState('character');

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm animate-fade-in">
      <div className="relative w-full max-w-md bg-stone-900 border border-amber-500/40 rounded-2xl p-6 shadow-2xl space-y-5">
        <div className="flex items-center justify-between border-b border-stone-800 pb-3">
          <div className="flex items-center gap-2">
            <UserPlus className="w-5 h-5 text-amber-400" />
            <h3 className="font-cinzel text-base font-bold text-amber-400">Add Canon Note</h3>
          </div>
          <button
            onClick={onClose}
            className="text-stone-400 hover:text-white p-1 rounded-lg hover:bg-stone-800 transition-colors cursor-pointer"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        <p className="text-xs text-stone-300 leading-relaxed">
          The narrator mentioned <span className="font-bold text-amber-300">"{entityName}"</span>, but no note exists in this campaign's canon yet. Adding a note registers them in the world without disrupting the current scene.
        </p>

        <div className="space-y-1.5">
          <label className="text-xs font-cinzel text-stone-400 uppercase tracking-wider block">
            Entity Type
          </label>
          <div className="grid grid-cols-3 gap-1.5">
            {['character', 'location', 'faction', 'item', 'arc', 'concept'].map((t) => (
              <button
                key={t}
                type="button"
                onClick={() => setType(t)}
                className={`text-xs font-mono py-1.5 px-2 rounded-lg border text-center transition-colors cursor-pointer ${
                  type === t
                    ? 'bg-amber-600/30 border-amber-500 text-amber-200 font-bold'
                    : 'bg-black/40 border-stone-800 text-stone-400 hover:text-stone-200'
                }`}
              >
                {t}
              </button>
            ))}
          </div>
        </div>

        <div className="flex items-center justify-end gap-2 pt-2">
          <button
            type="button"
            onClick={onClose}
            className="px-3 py-1.5 rounded-xl border border-stone-700 text-stone-400 hover:bg-stone-800 text-xs font-cinzel cursor-pointer transition-colors"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={() => onEditInCodex(entityName, type)}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-xl border border-amber-500/50 bg-stone-900 hover:bg-stone-800 text-amber-300 text-xs font-cinzel cursor-pointer transition-colors"
          >
            <BookOpen className="w-3.5 h-3.5" />
            <span>Edit in Codex</span>
          </button>
          <button
            type="button"
            onClick={() => onQuickCreate(entityName, type)}
            className="flex items-center gap-1.5 px-4 py-1.5 rounded-xl bg-amber-600 hover:bg-amber-500 text-stone-950 font-cinzel font-bold text-xs shadow-lg cursor-pointer transition-colors"
          >
            <Check className="w-3.5 h-3.5" />
            <span>Quick Register</span>
          </button>
        </div>
      </div>
    </div>
  );
};
