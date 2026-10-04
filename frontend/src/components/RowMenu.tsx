import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { MoreVertical } from 'lucide-react';

export interface RowMenuItem {
  label: string;
  icon: ReactNode;
  onSelect: () => void;
  destructive?: boolean;
}

interface RowMenuProps {
  items: RowMenuItem[];
  label?: string;
}

const MENU_WIDTH = 176;

// RowMenu is the per-row action menu. It renders through a portal into the
// document body with fixed positioning, because an absolutely positioned menu
// inside the tree is clipped by the tree's own scroll container and scrolls away
// with the rows. It also flips upwards when there is no room below.
export default function RowMenu({ items, label = 'More actions' }: RowMenuProps) {
  const buttonRef = useRef<HTMLButtonElement | null>(null);
  const [open, setOpen] = useState(false);
  const [position, setPosition] = useState<{ top: number; left: number } | null>(null);

  useLayoutEffect(() => {
    if (!open || !buttonRef.current) return;
    const rect = buttonRef.current.getBoundingClientRect();
    const height = items.length * 30 + 10;

    const left = Math.max(8, Math.min(rect.right - MENU_WIDTH, window.innerWidth - MENU_WIDTH - 8));
    const below = rect.bottom + 4;
    const top = below + height > window.innerHeight - 8 ? Math.max(8, rect.top - height - 4) : below;

    setPosition({ top, left });
  }, [open, items.length]);

  useEffect(() => {
    if (!open) return;
    const close = () => setOpen(false);
    const onKey = (ev: KeyboardEvent) => {
      if (ev.key === 'Escape') setOpen(false);
    };
    window.addEventListener('click', close);
    window.addEventListener('keydown', onKey);
    window.addEventListener('resize', close);
    // Capture, so a scroll inside the tree closes it rather than leaving the menu
    // stranded away from its row.
    window.addEventListener('scroll', close, true);
    return () => {
      window.removeEventListener('click', close);
      window.removeEventListener('keydown', onKey);
      window.removeEventListener('resize', close);
      window.removeEventListener('scroll', close, true);
    };
  }, [open]);

  return (
    <>
      <button
        ref={buttonRef}
        type="button"
        title={label}
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={(ev) => {
          ev.stopPropagation();
          setOpen((current) => !current);
        }}
        className="shrink-0 cursor-pointer rounded p-0.5 text-stone-500 hover:text-stone-200"
      >
        <MoreVertical className="w-3 h-3" />
      </button>

      {open &&
        position &&
        createPortal(
          <div
            role="menu"
            style={{ position: 'fixed', top: position.top, left: position.left, width: MENU_WIDTH }}
            onClick={(ev) => ev.stopPropagation()}
            className="z-[60] rounded-lg border border-white/10 bg-stone-900 p-1 shadow-2xl"
          >
            {items.map((item) => (
              <button
                key={item.label}
                type="button"
                role="menuitem"
                onClick={() => {
                  setOpen(false);
                  item.onSelect();
                }}
                className={`flex w-full cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-left text-xs hover:bg-white/10 ${
                  item.destructive ? 'text-red-300 hover:bg-red-950/40' : 'text-stone-200'
                }`}
              >
                {item.icon}
                <span>{item.label}</span>
              </button>
            ))}
          </div>,
          document.body,
        )}
    </>
  );
}
