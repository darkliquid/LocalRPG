import React, { useState } from 'react';
import { X, Maximize2, Minimize2 } from 'lucide-react';

interface DrawersProps {
  isOpen: boolean;
  onClose: () => void;
  title: string;
  children: React.ReactNode;
  size?: 'md' | 'lg' | 'xl' | 'full';
}

const sizeClasses: Record<'md' | 'lg' | 'xl' | 'full', string> = {
  md: 'max-w-md',
  lg: 'max-w-lg',
  xl: 'max-w-3xl',
  full: 'max-w-full',
};

export const Drawers: React.FC<DrawersProps> = ({ isOpen, onClose, title, children, size = 'md' }) => {
  const [isMaximized, setIsMaximized] = useState(false);

  if (!isOpen) return null;

  const currentSizeClass = isMaximized ? 'w-full max-w-full' : `w-full ${sizeClasses[size]}`;

  return (
    <div className="fixed inset-0 z-50 flex justify-end bg-black/60 backdrop-blur-xs transition-opacity duration-300">
      <div className={`${currentSizeClass} bg-glass-drawer h-full p-6 shadow-2xl flex flex-col transform transition-all duration-300`}>
        <div className="flex items-center justify-between pb-4 border-b border-white/10 mb-4 shrink-0">
          <span className="font-sans text-purple-400 font-bold tracking-wider text-base">{title}</span>
          <div className="flex items-center gap-2">
            <button
              onClick={() => setIsMaximized((prev) => !prev)}
              className="p-1 hover:text-purple-300 cursor-pointer text-stone-400 transition-colors"
              title={isMaximized ? 'Restore drawer width' : 'Maximize drawer width'}
            >
              {isMaximized ? <Minimize2 className="w-4 h-4" /> : <Maximize2 className="w-4 h-4" />}
            </button>
            <button
              onClick={onClose}
              className="p-1 hover:text-purple-300 cursor-pointer text-stone-400 transition-colors"
              title="Close drawer"
            >
              <X className="w-5 h-5" />
            </button>
          </div>
        </div>
        <div className="flex-1 overflow-y-auto pr-1 min-h-0">
          {children}
        </div>
      </div>
    </div>
  );
};
