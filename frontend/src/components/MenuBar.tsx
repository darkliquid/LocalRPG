import React, { useCallback, useEffect, useRef, useState } from 'react';
import {
  BookOpen,
  Bug,
  FolderTree,
  Github,
  Info,
  Maximize,
  Minimize,
  RotateCcw,
  RotateCw,
  ZoomIn,
  ZoomOut,
} from 'lucide-react';
import { useMountTransition } from '../hooks/useMountTransition';
import { PROJECT_ISSUES_URL, PROJECT_REPO_URL, openExternal } from '../lib/project';

interface MenuBarProps {
  onOpenDocs: () => void;
  onOpenAbout: () => void;
  onOpenContentStudio: () => void;
}

interface MenuAction {
  label: string;
  icon: React.FC<{ className?: string }>;
  shortcut?: string;
  action: () => void;
  // separated draws a divider above the item, grouping a destructive or
  // informational action away from the ones it would otherwise sit beside.
  separated?: boolean;
}

interface MenuGroup {
  label: string;
  actions: MenuAction[];
}

// The webview honours the non-standard `zoom` property, which is the only way to
// scale the whole UI without the fixed-position layout a transform would distort.
const ZOOM_STEPS = [0.8, 0.9, 1, 1.1, 1.25, 1.5];
const DEFAULT_ZOOM_INDEX = ZOOM_STEPS.indexOf(1);

function applyZoom(level: number) {
  document.documentElement.style.setProperty('zoom', String(level));
}

// MenuBar is the application menu. Wails v3 cannot hide a native menu bar on
// Linux, so the app draws its own and keeps it out of the way until the player
// taps Alt, the same gesture a native Windows menu bar answers to.
export const MenuBar: React.FC<MenuBarProps> = ({ onOpenDocs, onOpenAbout, onOpenContentStudio }) => {
  const [revealed, setRevealed] = useState(false);
  const [openGroup, setOpenGroup] = useState<string | null>(null);
  const [isFullscreen, setIsFullscreen] = useState(false);

  const barRef = useRef<HTMLDivElement | null>(null);
  const revealedRef = useRef(revealed);
  // The zoom level is not rendered, so it lives in a ref rather than state.
  const zoomRef = useRef(DEFAULT_ZOOM_INDEX);
  const altHeld = useRef(false);
  const altUsedWithKey = useRef(false);

  const { mounted, state } = useMountTransition(revealed, 150);

  useEffect(() => {
    revealedRef.current = revealed;
  }, [revealed]);

  const close = useCallback(() => {
    setRevealed(false);
    setOpenGroup(null);
  }, []);

  // Alt reveals the menu and a second tap hides it. An Alt chord (Alt+Tab,
  // Alt+Left) must not toggle it, so a press only counts when no other key
  // arrives while Alt is held.
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Alt') {
        if (!altHeld.current) {
          altHeld.current = true;
          altUsedWithKey.current = false;
        }
        return;
      }
      if (altHeld.current) {
        altUsedWithKey.current = true;
      }
      if (e.key === 'Escape' && revealedRef.current) {
        close();
      }
    };

    const onKeyUp = (e: KeyboardEvent) => {
      if (e.key !== 'Alt') return;
      const tappedAlone = altHeld.current && !altUsedWithKey.current;
      altHeld.current = false;
      altUsedWithKey.current = false;
      if (!tappedAlone) return;
      setRevealed((prev) => {
        if (prev) setOpenGroup(null);
        return !prev;
      });
    };

    const onBlur = () => {
      altHeld.current = false;
      altUsedWithKey.current = false;
    };

    window.addEventListener('keydown', onKeyDown);
    window.addEventListener('keyup', onKeyUp);
    window.addEventListener('blur', onBlur);
    return () => {
      window.removeEventListener('keydown', onKeyDown);
      window.removeEventListener('keyup', onKeyUp);
      window.removeEventListener('blur', onBlur);
    };
  }, [close]);

  // A click anywhere outside the bar dismisses it, the way a native menu closes.
  useEffect(() => {
    if (!revealed) return;
    const onPointerDown = (e: MouseEvent) => {
      if (barRef.current && !barRef.current.contains(e.target as Node)) {
        close();
      }
    };
    window.addEventListener('mousedown', onPointerDown);
    return () => window.removeEventListener('mousedown', onPointerDown);
  }, [revealed, close]);

  useEffect(() => {
    const onChange = () => setIsFullscreen(document.fullscreenElement !== null);
    document.addEventListener('fullscreenchange', onChange);
    return () => document.removeEventListener('fullscreenchange', onChange);
  }, []);

  const stepZoom = (delta: number) => {
    const next = Math.min(ZOOM_STEPS.length - 1, Math.max(0, zoomRef.current + delta));
    zoomRef.current = next;
    applyZoom(ZOOM_STEPS[next]);
  };

  const resetZoom = () => {
    zoomRef.current = DEFAULT_ZOOM_INDEX;
    applyZoom(1);
  };

  const toggleFullscreen = () => {
    if (document.fullscreenElement) {
      void document.exitFullscreen();
    } else {
      void document.documentElement.requestFullscreen();
    }
  };

  const groups: MenuGroup[] = [
    {
      label: 'View',
      actions: [
        { label: 'Reload', icon: RotateCw, shortcut: 'Ctrl+R', action: () => window.location.reload() },
        {
          label: isFullscreen ? 'Exit Fullscreen' : 'Enter Fullscreen',
          icon: isFullscreen ? Minimize : Maximize,
          shortcut: 'F11',
          action: toggleFullscreen,
        },
        { label: 'Zoom In', icon: ZoomIn, shortcut: 'Ctrl++', action: () => stepZoom(1) },
        { label: 'Zoom Out', icon: ZoomOut, shortcut: 'Ctrl+-', action: () => stepZoom(-1) },
        { label: 'Reset Zoom', icon: RotateCcw, shortcut: 'Ctrl+0', action: resetZoom },
      ],
    },
    {
      label: 'Help',
      actions: [
        { label: 'Content Studio', icon: FolderTree, action: onOpenContentStudio },
        { label: 'Documentation', icon: BookOpen, action: onOpenDocs },
        { label: 'Report an Issue', icon: Bug, action: () => openExternal(PROJECT_ISSUES_URL) },
        { label: 'Project Repository', icon: Github, action: () => openExternal(PROJECT_REPO_URL) },
        { label: 'About LocalRPG', icon: Info, separated: true, action: onOpenAbout },
      ],
    },
  ];

  const runAction = (action: MenuAction) => {
    action.action();
    close();
  };

  if (!mounted) return null;

  return (
    <div
      ref={barRef}
      data-state={state}
      className={`fixed top-0 left-0 right-0 z-[70] bg-glass border-b border-white/10 shadow-2xl ${
        state === 'enter' ? 'anim-fade-in' : 'anim-fade-out pointer-events-none'
      }`}
      style={{ '--anim-dur': '150ms' } as React.CSSProperties}
      role="menubar"
      aria-label="Application menu"
    >
      <div className="flex items-stretch h-9 px-2 font-sans text-xs select-none">
        {groups.map((group) => {
          const isOpen = openGroup === group.label;
          return (
            <div key={group.label} className="relative flex items-stretch">
              <button
                type="button"
                role="menuitem"
                aria-haspopup="true"
                aria-expanded={isOpen}
                onClick={() => setOpenGroup(isOpen ? null : group.label)}
                onMouseEnter={() => {
                  // Once a menu is open, sliding across the bar switches to it,
                  // which is how a native menu bar behaves.
                  if (openGroup !== null) setOpenGroup(group.label);
                }}
                className={`px-3 rounded-md transition-colors cursor-pointer ${
                  isOpen ? 'bg-purple-600/80 text-white font-semibold' : 'text-stone-300 hover:text-white hover:bg-white/10'
                }`}
              >
                {group.label}
              </button>

              {isOpen && (
                <div
                  role="menu"
                  className="absolute top-full left-0 mt-1 min-w-[15rem] bg-glass-card rounded-xl shadow-2xl border border-white/10 py-1.5 anim-fade-in"
                  style={{ '--anim-dur': '120ms' } as React.CSSProperties}
                >
                  {group.actions.map((action) => {
                    const Icon = action.icon;
                    return (
                      <React.Fragment key={action.label}>
                        {action.separated && <div className="my-1.5 border-t border-white/10" />}
                        <button
                          type="button"
                          role="menuitem"
                          onClick={() => runAction(action)}
                          className="w-full flex items-center gap-3 px-3 py-1.5 text-left text-stone-200 hover:bg-purple-600/80 hover:text-white transition-colors cursor-pointer"
                        >
                          <Icon className="w-3.5 h-3.5 shrink-0 opacity-80" />
                          <span className="flex-1">{action.label}</span>
                          {action.shortcut && (
                            <span className="text-[0.65rem] text-stone-500 font-mono">{action.shortcut}</span>
                          )}
                        </button>
                      </React.Fragment>
                    );
                  })}
                </div>
              )}
            </div>
          );
        })}

        <div className="flex-1" />
        <span className="flex items-center pr-2 text-[0.65rem] uppercase tracking-widest text-stone-500 font-mono">
          Alt to hide
        </span>
      </div>
    </div>
  );
};
