import React, { useState, useRef, useEffect, useMemo } from 'react';
import { ChevronDown, Play, X } from 'lucide-react';
import { ProviderVoice } from '../types';
import { playVoicePreview } from '../lib/audioPreview';

export interface VoiceComboboxProps {
  value: string;
  onChange: (value: string) => void;
  voices?: ProviderVoice[];
  placeholder?: string;
  disabled?: boolean;
  className?: string;
}

export const VoiceCombobox: React.FC<VoiceComboboxProps> = ({
  value,
  onChange,
  voices = [],
  placeholder = 'Select or enter voice ID...',
  disabled = false,
  className = '',
}) => {
  const [isOpen, setIsOpen] = useState(false);
  const [query, setQuery] = useState('');
  const containerRef = useRef<HTMLDivElement>(null);

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
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setIsOpen(false);
      }
    };
    document.addEventListener('mousedown', handleOutsideClick);
    return () => document.removeEventListener('mousedown', handleOutsideClick);
  }, []);

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
        className={`bg-stone-900 border border-stone-800 rounded px-2 py-1 text-xs font-mono text-stone-200 focus:outline-none focus:border-amber-500/60 ${className}`}
      />
    );
  }

  return (
    <div ref={containerRef} className={`relative flex items-center gap-1.5 ${className}`}>
      <div className="relative flex-1">
        <input
          type="text"
          value={isOpen ? query : (selectedVoice ? `${selectedVoice.name} (${selectedVoice.id})` : value)}
          onChange={(e) => {
            setQuery(e.target.value);
            onChange(e.target.value);
            if (!isOpen) setIsOpen(true);
          }}
          onFocus={() => {
            setQuery(selectedVoice ? selectedVoice.name : value);
            setIsOpen(true);
          }}
          onKeyDown={(e) => {
            if (e.key === 'Escape') {
              setIsOpen(false);
            }
          }}
          placeholder={placeholder}
          disabled={disabled}
          className="w-full bg-stone-900 border border-stone-800 rounded px-2 py-1 pr-6 text-xs text-stone-200 focus:outline-none focus:border-amber-500/60 truncate"
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
            onClick={() => setIsOpen((prev) => !prev)}
            tabIndex={-1}
            className="text-stone-500 hover:text-stone-300 cursor-pointer"
          >
            <ChevronDown className="w-3.5 h-3.5" />
          </button>
        </div>
      </div>

      {selectedVoice?.preview_url && (
        <button
          type="button"
          onClick={() => playVoicePreview(selectedVoice.preview_url as string)}
          className="p-1 rounded bg-stone-900 border border-stone-800 text-amber-400 hover:text-amber-300 cursor-pointer shrink-0"
          title="Audition Voice"
        >
          <Play className="w-3 h-3" />
        </button>
      )}

      {isOpen && (
        <div className="absolute left-0 top-full mt-1 w-full min-w-[260px] max-h-60 overflow-y-auto bg-stone-950 border border-stone-800 rounded-lg shadow-2xl z-50 p-1 space-y-0.5">
          {filteredVoices.length === 0 ? (
            <div className="px-2 py-1.5 text-[11px] text-stone-500 italic">No matching voices</div>
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
                    isSelected ? 'bg-amber-500/20 text-amber-300' : 'text-stone-300 hover:bg-stone-900'
                  }`}
                >
                  <div className="min-w-0">
                    <div className="font-medium truncate">{voice.name}</div>
                    <div className="text-[10px] font-mono text-stone-500 truncate">{voice.id}</div>
                  </div>
                  <div className="flex items-center gap-1.5 shrink-0">
                    {voice.categories?.[0] && (
                      <span className="text-[9px] px-1 py-0.5 rounded bg-stone-900 text-stone-400 border border-stone-800">
                        {voice.categories[0]}
                      </span>
                    )}
                    {voice.preview_url && (
                      <button
                        type="button"
                        onClick={(e) => {
                          e.stopPropagation();
                          playVoicePreview(voice.preview_url as string);
                        }}
                        className="p-1 rounded hover:bg-stone-800 text-amber-400 cursor-pointer"
                        title="Audition"
                      >
                        <Play className="w-3 h-3" />
                      </button>
                    )}
                  </div>
                </div>
              );
            })
          )}
        </div>
      )}
    </div>
  );
};
