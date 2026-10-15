import { useCallback, useEffect, useId, useRef, type KeyboardEvent, type ReactNode } from 'react';
import { X } from 'lucide-react';

// The element kinds a modal lets you tab through. Disabled controls and
// tabindex="-1" are excluded because they are not reachable by tabbing, which is
// what this list is for.
const FOCUSABLE =
  'a[href],button:not([disabled]),textarea:not([disabled]),input:not([disabled]),select:not([disabled]),[tabindex]:not([tabindex="-1"])';

export interface ModalProps {
  open: boolean;
  onClose: () => void;
  // title labels the dialog for a screen reader, so it is required rather than
  // optional decoration.
  title: string;
  children: ReactNode;
  // dismissable is false for a modal that must be answered: Escape and a backdrop
  // click are then refused, and the close button is not rendered.
  dismissable?: boolean;
  className?: string;
}

// Modal is the one dialog shell: it declares itself to assistive technology,
// moves focus in when it opens, keeps Tab inside it, closes on Escape, and
// returns focus to whatever opened it. Thirty-one copy-pasted overlays did none
// of that beyond drawing the same backdrop.
export function Modal({ open, onClose, title, children, dismissable = true, className }: ModalProps) {
  const dialogRef = useRef<HTMLDivElement>(null);
  const openerRef = useRef<HTMLElement | null>(null);
  const titleID = useId();

  // Remember what had focus so closing puts the user back: dropping focus on the
  // body makes a keyboard user find their place again from the top.
  useEffect(() => {
    if (!open) return;
    openerRef.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const dialog = dialogRef.current;
    const first = dialog?.querySelector<HTMLElement>(FOCUSABLE);
    (first ?? dialog)?.focus();
    return () => openerRef.current?.focus();
  }, [open]);

  // Escape closes, and Tab cycles inside: the point of a modal is that the page
  // behind it is not reachable while it is open.
  const handleKeyDown = useCallback(
    (event: KeyboardEvent<HTMLDivElement>) => {
      if (event.key === 'Escape' && dismissable) {
        event.stopPropagation();
        onClose();
        return;
      }
      if (event.key !== 'Tab') return;
      const dialog = dialogRef.current;
      if (!dialog) return;
      // The selector already excludes what tabbing cannot reach, and jsdom has no
      // layout, so a visibility filter here would be a filter that answers nothing
      // in a test and guesses in a browser.
      const items = Array.from(dialog.querySelectorAll<HTMLElement>(FOCUSABLE));
      if (items.length === 0) {
        event.preventDefault();
        return;
      }
      const first = items[0];
      const last = items[items.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    },
    [dismissable, onClose],
  );

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4"
      onClick={dismissable ? onClose : undefined}
      data-testid="modal-backdrop"
    >
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleID}
        tabIndex={-1}
        onKeyDown={handleKeyDown}
        onClick={(event) => event.stopPropagation()}
        className={`max-h-[90vh] w-full overflow-y-auto rounded-2xl border border-stone-800 bg-stone-950 p-6 shadow-2xl focus:outline-none ${className ?? ''}`}
      >
        <header className="mb-4 flex items-center justify-between gap-4">
          <h2 id={titleID} className="text-lg font-serif text-stone-100">
            {title}
          </h2>
          {dismissable && (
            <button
              type="button"
              onClick={onClose}
              aria-label="Close"
              className="cursor-pointer text-stone-400 hover:text-stone-200"
            >
              <X className="h-5 w-5" />
            </button>
          )}
        </header>
        {children}
      </div>
    </div>
  );
}
