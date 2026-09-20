import React from 'react';
import { X } from 'lucide-react';

interface DrawersProps {
  isOpen: boolean;
  onClose: () => void;
  title: string;
  children: React.ReactNode;
}

export const Drawers: React.FC<DrawersProps> = ({ isOpen, onClose, title, children }) => {
  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex justify-end bg-black/60 backdrop-blur-xs transition-opacity duration-300">
      <div className="w-full max-w-md bg-glass-drawer h-full p-6 shadow-2xl flex flex-col transform transition-transform duration-300">
        <div className="flex items-center justify-between pb-4 border-b border-white/10 mb-4">
          <span className="font-cinzel text-amber-400 font-bold tracking-wider text-base">{title}</span>
          <button onClick={onClose} className="p-1 hover:text-amber-300 cursor-pointer text-stone-400 transition-colors">
            <X className="w-5 h-5" />
          </button>
        </div>
        <div className="flex-1 overflow-y-auto pr-1">
          {children}
        </div>
      </div>
    </div>
  );
};
