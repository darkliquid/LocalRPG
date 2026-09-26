import React, { useState, useRef, useEffect, useMemo, useCallback } from 'react';
import { createPortal } from 'react-dom';
import { ChevronDown, Play, X, Loader2 } from 'lucide-react';
import { ProviderVoice } from '../types';
import { playVoicePreview } from '../lib/audioPreview';

export interface VoiceComboboxProps {
  value: string;
  onChange: (value: string) => void;
  voices?: ProviderVoice[];
  placeholder?: string;
  disabled?: boolean;
  className?: string;
  // onAudition synthesizes a sample for a voice that has no preview_url (for
  // example a configured voice profile). When set it replaces preview playback.
  onAudition?: (voice: ProviderVoice) => void;
  auditioningId?: string | null;
}

// voiceLabel avoids the "Name (Name)" duplication providers produce when a
// voice's display name is just its id.
export function voiceLabel(voice: { id: string; name?: string }): string {
  const name = (voice.name || '').trim();
  if (!name) return voice.id;
  if (!voice.id || name === voice.id) return name;
  return `${name} (${voice.id})`;
}

export const VoiceCombobox: React.FC<VoiceComboboxProps> = ({
  value,
  onChange,
  voices = [],
  placeholder = 'Select or enter voice ID...',
  disabled = false,
  className = '',
  onAudition,
  auditioningId = null,
}) => {
  const [isOpen, setIsOpen] = useState(false);
  const [query, setQuery] = useState('');
  const containerRef = useRef<HTMLDivElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const [menu, setMenu] = useState<{ top: number; left: number; width: number; height: number } | null>(null);

  const MENU_MAX_HEIGHT = 240;
  const MENU_GAP = 4;

  // Position the menu against the viewport, flipping above the field when there
  // is no room below. The menu is portalled to the body so a scrolling or
  // clipping ancestor cannot cut it off.
  const positionMenu = useCallback(() => {
    const anchor = containerRef.current;
    if (!anchor) return;
    const rect = anchor.getBoundingClientRect();
    const width = Math.max(rect.width, 260);
    let left = rect.left;
    if (left + width > window.innerWidth - 8) left = window.innerWidth - width - 8;
    if (left < 8) left = 8;

    const spaceBelow = window.innerHeight - rect.bottom - MENU_GAP - 8;
    const spaceAbove = rect.top - MENU_GAP - 8;
    const up = spaceBelow < MENU_MAX_HEIGHT && spaceAbove > spaceBelow;
    const height = Math.max(120, Math.min(MENU_MAX_HEIGHT, up ? spaceAbove : spaceBelow));
    const top = up ? rect.top - MENU_GAP - height : rect.bottom + MENU_GAP;

    setMenu({ top, left, width, height });
  }, []);

  const selectedVoice = useMemo(() => {
    return voices.find((v) => v.id === value);
  }, [voices, value]);

  // Keep search query synced with selection when closed
  useEffect(() => {
    if (!isOpen) {
      setQuery(selectedVoice ? selectedVoice.name : value);
    }
  }, [value, selectedVoice, isOpen]);

  // Handle outside clicks to close dropdown
  useEffect(() => {
    const handleOutsideClick = (e: MouseEvent) => {
      const target = e.target as Node;
      if (containerRef.current?.contains(target)) return;
      if (menuRef.current?.contains(target)) return;
      setIsOpen(false);
    };
    document.addEventListener('mousedown', handleOutsideClick);
    return () => document.removeEventListener('mousedown', handleOutsideClick);
  }, []);

  // Reposition while open, and follow any ancestor scroll.
  useEffect(() => {
    if (!isOpen) return;
    positionMenu();
    const reposition = () => positionMenu();
    window.addEventListener('resize', reposition);
    window.addEventListener('scroll', reposition, true);
    return () => {
      window.removeEventListener('resize', reposition);
      window.removeEventListener('scroll', reposition, true);
    };
  }, [isOpen, positionMenu]);

  const filteredVoices = useMemo(() => {
    if (!voices.length) return [];
    const needle = query.trim().toLowerCase();
    if (!needle) return voices;
    return voices.filter((v) => {
      return (
        v.name.toLowerCase().includes(needle) ||
        v.id.toLowerCase().includes(needle) ||
        v.tags?.some((t) => t.toLowerCase().includes(needle)) ||
        v.categories?.some((c) => c.toLowerCase().includes(needle))
      );
    });
  }, [voices, query]);

  // If no catalog is available, render clean text input
  if (!voices || voices.length === 0) {
    return (
      <input
        type="text"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        disabled={disabled}
        className={`bg-stone-900 border border-stone-800 rounded px-2 py-1 text-xs font-mono text-stone-200 focus:outline-none focus:border-purple-500/60 ${className}`}
      />
    );
  }

  return (
    <div ref={containerRef} className={`relative flex items-center gap-1.5 ${className}`}>
      <div className="relative flex-1">
        <input
          type="text"
          value={isOpen ? query : (selectedVoice ? voiceLabel(selectedVoice) : value)}
          onChange={(e) => {
            setQuery(e.target.value);
            onChange(e.target.value);
            if (!isOpen) setIsOpen(true);
          }}
          onFocus={() => {
            // Open with an empty query so the whole catalog is browsable;
            // seeding the query with the current selection would filter the
            // list down to that one voice.
            setQuery('');
            setIsOpen(true);
          }}
          onKeyDown={(e) => {
            if (e.key === 'Escape') {
              setIsOpen(false);
            }
          }}
          placeholder={placeholder}
          disabled={disabled}
          className="w-full bg-stone-900 border border-stone-800 rounded px-2 py-1 pr-6 text-xs text-stone-200 focus:outline-none focus:border-purple-500/60 truncate"
        />
        <div className="absolute right-1.5 top-1/2 -translate-y-1/2 flex items-center gap-1">
          {isOpen && query && (
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                setQuery('');
                onChange('');
              }}
              tabIndex={-1}
              className="text-stone-500 hover:text-stone-300 cursor-pointer"
            >
              <X className="w-3 h-3" />
            </button>
          )}
          <button
            type="button"
            onClick={() =>
              setIsOpen((prev) => {
                if (!prev) setQuery('');
                return !prev;
              })
            }
            tabIndex={-1}
            className="text-stone-500 hover:text-stone-300 cursor-pointer"
          >
            <ChevronDown className="w-3.5 h-3.5" />
          </button>
        </div>
      </div>

      {selectedVoice && (onAudition || selectedVoice.preview_url) && (
        <button
          type="button"
          onClick={() => (onAudition ? onAudition(selectedVoice) : playVoicePreview(selectedVoice.preview_url as string))}
          disabled={!!onAudition && auditioningId === selectedVoice.id}
          className="p-1 rounded bg-stone-900 border border-stone-800 text-purple-400 hover:text-purple-300 cursor-pointer shrink-0 disabled:opacity-50"
          title="Audition Voice"
        >
          {onAudition && auditioningId === selectedVoice.id ? (
            <Loader2 className="w-3 h-3 animate-spin" />
          ) : (
            <Play className="w-3 h-3" />
          )}
        </button>
      )}

      {isOpen && menu &&
        createPortal(
          <div
            ref={menuRef}
            style={{ position: 'fixed', top: menu.top, left: menu.left, width: menu.width, maxHeight: menu.height }}
            className="overflow-y-auto bg-stone-950 border border-stone-800 rounded-lg shadow-2xl z-[120] p-1 space-y-0.5"
          >
          {filteredVoices.length === 0 ? (
            <div className="px-2 py-1.5 text-xs text-stone-500 italic">No matching voices</div>
          ) : (
            filteredVoices.map((voice) => {
              const isSelected = voice.id === value;
              return (
                <div
                  key={voice.id}
                  onClick={() => {
                    onChange(voice.id);
                    setIsOpen(false);
                  }}
                  className={`flex items-center justify-between gap-2 px-2 py-1.5 rounded text-xs cursor-pointer ${
                    isSelected ? 'bg-purple-500/20 text-purple-300' : 'text-stone-300 hover:bg-stone-900'
                  }`}
                >
                  <div className="min-w-0">
                    <div className="font-medium truncate">{voice.name || voice.id}</div>
                    {voice.id && voice.name !== voice.id && (
                      <div className="text-xs font-mono text-stone-500 truncate">{voice.id}</div>
                    )}
                    {voice.description && (
                      <div className="text-xs text-stone-500 truncate">{voice.description}</div>
                    )}
                    {voice.tags && voice.tags.length > 0 && (
                      <div className="flex flex-wrap gap-1 mt-0.5">
                        {voice.tags.map((tag) => (
                          <span
                            key={tag}
                            className="text-xs px-1 py-0.5 rounded bg-purple-500/10 text-purple-300/90 border border-purple-500/20"
                          >
                            {tag}
                          </span>
                        ))}
                      </div>
                    )}
                  </div>
                  <div className="flex items-center gap-1.5 shrink-0">
                    {voice.categories?.[0] && (
                      <span className="text-xs px-1 py-0.5 rounded bg-stone-900 text-stone-400 border border-stone-800">
                        {voice.categories[0]}
                      </span>
                    )}
                    {(onAudition || voice.preview_url) && (
                      <button
                        type="button"
                        onClick={(e) => {
                          e.stopPropagation();
                          if (onAudition) onAudition(voice);
                          else playVoicePreview(voice.preview_url as string);
                        }}
                        disabled={!!onAudition && auditioningId === voice.id}
                        className="p-1 rounded hover:bg-stone-800 text-purple-400 cursor-pointer disabled:opacity-50"
                        title="Audition"
                      >
                        {onAudition && auditioningId === voice.id ? (
                          <Loader2 className="w-3 h-3 animate-spin" />
                        ) : (
                          <Play className="w-3 h-3" />
                        )}
                      </button>
                    )}
                  </div>
                </div>
              );
            })
          )}
          </div>,
          document.body
        )}
    </div>
  );
};
