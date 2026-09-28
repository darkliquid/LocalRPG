import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { ChevronDown, Loader2, Play, X } from 'lucide-react';
import { TTSConfig, VoiceProfile } from '../types';
import { useVoicePreview } from '../hooks/useVoicePreview';

export interface VoiceProfileSelectProps {
  profiles: VoiceProfile[];
  // value is the selected profile id, or '' for the default/empty option.
  value: string;
  onChange: (profile: VoiceProfile | null) => void;
  // ttsConfig is what a preview synthesises with. Without it, or when the
  // provider is disabled, the preview buttons are hidden rather than failing.
  ttsConfig?: TTSConfig;
  previewText?: string;
  placeholder?: string;
  // allowDefault adds a row that clears the selection, for a role that falls
  // back to the provider's own default voice.
  allowDefault?: boolean;
  defaultLabel?: string;
  disabled?: boolean;
  className?: string;
}

// VoiceProfileSelect picks a configured voice archetype and can preview each
// one, so every place a profile is chosen behaves the same way.
export const VoiceProfileSelect: React.FC<VoiceProfileSelectProps> = ({
  profiles,
  value,
  onChange,
  ttsConfig,
  previewText,
  placeholder = 'Select a voice profile...',
  allowDefault = false,
  defaultLabel = 'Default (provider setting)',
  disabled = false,
  className = '',
}) => {
  const [isOpen, setIsOpen] = useState(false);
  const [query, setQuery] = useState('');
  const containerRef = useRef<HTMLDivElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const [menu, setMenu] = useState<{ top: number; left: number; width: number; height: number } | null>(null);

  const MENU_MAX_HEIGHT = 260;
  const MENU_GAP = 4;

  const { previewingId, error, previewProfile } = useVoicePreview();
  const canPreview = !!ttsConfig && ttsConfig.type !== 'disabled';

  const selected = useMemo(() => profiles.find((p) => p.id === value), [profiles, value]);

  const positionMenu = useCallback(() => {
    const anchor = containerRef.current;
    if (!anchor) return;
    const rect = anchor.getBoundingClientRect();
    const width = Math.max(rect.width, 280);
    let left = rect.left;
    if (left + width > window.innerWidth - 8) left = window.innerWidth - width - 8;
    if (left < 8) left = 8;

    const spaceBelow = window.innerHeight - rect.bottom - MENU_GAP - 8;
    const spaceAbove = rect.top - MENU_GAP - 8;
    const up = spaceBelow < MENU_MAX_HEIGHT && spaceAbove > spaceBelow;
    const height = Math.max(140, Math.min(MENU_MAX_HEIGHT, up ? spaceAbove : spaceBelow));
    const top = up ? rect.top - MENU_GAP - height : rect.bottom + MENU_GAP;

    setMenu({ top, left, width, height });
  }, []);

  useEffect(() => {
    if (!isOpen) setQuery('');
  }, [isOpen]);

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

  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return profiles;
    return profiles.filter(
      (p) =>
        p.name.toLowerCase().includes(needle) ||
        p.id.toLowerCase().includes(needle) ||
        p.voice_id.toLowerCase().includes(needle) ||
        (p.tags ?? []).some((t) => t.toLowerCase().includes(needle)),
    );
  }, [profiles, query]);

  const displayName = selected ? selected.name || selected.id : '';

  const chooseProfile = (profile: VoiceProfile) => {
    onChange(profile);
    setIsOpen(false);
  };

  return (
    <div className={`relative ${className}`}>
      <div ref={containerRef} className="relative flex items-center gap-1.5">
        <div className="relative flex-1">
          <input
            type="text"
            value={isOpen ? query : displayName}
            onChange={(e) => {
              setQuery(e.target.value);
              if (!isOpen) setIsOpen(true);
            }}
            onFocus={() => {
              setQuery('');
              setIsOpen(true);
            }}
            onKeyDown={(e) => {
              if (e.key === 'Escape') setIsOpen(false);
            }}
            placeholder={selected ? displayName : placeholder}
            disabled={disabled}
            className="w-full bg-stone-900 border border-stone-800 rounded px-2 py-1 pr-6 text-xs text-stone-200 focus:outline-none focus:border-purple-500/60 truncate"
          />
          <div className="absolute right-1.5 top-1/2 -translate-y-1/2 flex items-center gap-1">
            {selected && (
              <button
                type="button"
                onClick={(e) => {
                  e.stopPropagation();
                  onChange(null);
                }}
                tabIndex={-1}
                className="text-stone-500 hover:text-stone-300 cursor-pointer"
                title="Clear selection"
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

        {selected && canPreview && (
          <button
            type="button"
            onClick={() => previewProfile(selected, ttsConfig as TTSConfig, previewText)}
            disabled={previewingId === selected.id}
            className="p-1 rounded bg-stone-900 border border-stone-800 text-purple-400 hover:text-purple-300 cursor-pointer shrink-0 disabled:opacity-50"
            title="Preview Voice Profile"
          >
            {previewingId === selected.id ? (
              <Loader2 className="w-3 h-3 animate-spin" />
            ) : (
              <Play className="w-3 h-3" />
            )}
          </button>
        )}
      </div>

      {error && <p className="mt-1 text-xs font-sans text-amber-400">{error}</p>}

      {isOpen &&
        menu &&
        createPortal(
          <div
            ref={menuRef}
            style={{ position: 'fixed', top: menu.top, left: menu.left, width: menu.width, maxHeight: menu.height }}
            className="overflow-y-auto bg-stone-950 border border-stone-800 rounded-lg shadow-2xl z-[120] p-1 space-y-0.5"
          >
            {allowDefault && (
              <div
                onClick={() => {
                  onChange(null);
                  setIsOpen(false);
                }}
                className={`flex items-center justify-between gap-2 px-2 py-1.5 rounded text-xs cursor-pointer ${
                  value === '' ? 'bg-purple-500/20 text-purple-300' : 'text-stone-300 hover:bg-stone-900'
                }`}
              >
                <span className="italic">{defaultLabel}</span>
              </div>
            )}

            {filtered.length === 0 ? (
              <div className="px-2 py-1.5 text-xs text-stone-500 italic">No matching voice profiles</div>
            ) : (
              filtered.map((profile) => {
                const isSelected = profile.id === value;
                return (
                  <div
                    key={profile.id}
                    onClick={() => chooseProfile(profile)}
                    className={`flex items-center justify-between gap-2 px-2 py-1.5 rounded text-xs cursor-pointer ${
                      isSelected ? 'bg-purple-500/20 text-purple-300' : 'text-stone-300 hover:bg-stone-900'
                    }`}
                  >
                    <div className="min-w-0">
                      <div className="font-medium truncate">{profile.name || profile.id}</div>
                      <div className="text-xs font-mono text-stone-500 truncate">
                        {profile.id} :: {profile.voice_id}
                      </div>
                      {profile.description && (
                        <div className="text-xs text-stone-500 truncate">{profile.description}</div>
                      )}
                      {profile.tags && profile.tags.length > 0 && (
                        <div className="flex flex-wrap gap-1 mt-0.5">
                          {profile.tags.map((tag) => (
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
                    {canPreview && (
                      <button
                        type="button"
                        onClick={(e) => {
                          e.stopPropagation();
                          void previewProfile(profile, ttsConfig as TTSConfig, previewText);
                        }}
                        disabled={previewingId === profile.id}
                        className="p-1 rounded hover:bg-stone-800 text-purple-400 cursor-pointer disabled:opacity-50 shrink-0"
                        title="Preview"
                      >
                        {previewingId === profile.id ? (
                          <Loader2 className="w-3 h-3 animate-spin" />
                        ) : (
                          <Play className="w-3 h-3" />
                        )}
                      </button>
                    )}
                  </div>
                );
              })
            )}
          </div>,
          document.body,
        )}
    </div>
  );
};
