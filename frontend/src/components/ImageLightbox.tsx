import React, { useEffect } from 'react';
import { createPortal } from 'react-dom';
import { X } from 'lucide-react';
import { useMountTransition } from '../hooks/useMountTransition';
import { EntityAvatar } from './EntityAvatar';

interface ImageLightboxProps {
  src: string;
  alt: string;
  onClose: () => void;
  isOpen?: boolean;
}

export const ImageLightbox: React.FC<ImageLightboxProps> = ({ src, alt, onClose, isOpen = true }) => {
  const { mounted, state } = useMountTransition(isOpen, 220);

  // Close on Escape
  useEffect(() => {
    if (!isOpen) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [isOpen, onClose]);

  if (!mounted) return null;

  return createPortal(
    <div
      data-state={state}
      className={`fixed inset-0 z-[200] flex items-center justify-center bg-black/80 backdrop-blur-sm ${
        state === 'enter' ? 'anim-fade-in' : 'anim-fade-out pointer-events-none'
      }`}
      style={{ '--anim-dur': '220ms' } as React.CSSProperties}
      onClick={onClose}
    >
      <div
        className={`relative max-w-[90vw] max-h-[90vh] flex items-center justify-center ${
          state === 'enter' ? 'anim-scale-in' : 'anim-scale-out'
        }`}
        onClick={(e) => e.stopPropagation()}
      >
        <EntityAvatar
          src={src}
          name={alt}
          alt={alt}
          className="max-w-full max-h-[85vh] rounded-2xl shadow-2xl object-contain border border-white/10"
        />
        <button
          onClick={onClose}
          className="absolute top-2 right-2 p-1.5 rounded-full bg-black/60 text-white hover:bg-black/80 transition-colors cursor-pointer"
          title="Close"
        >
          <X className="w-5 h-5" />
        </button>
      </div>
    </div>,
    document.body
  );
};
