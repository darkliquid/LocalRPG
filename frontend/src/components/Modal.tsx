import { useId, type ReactNode } from 'react';
import { X } from 'lucide-react';
import { useDialogFocus } from '../hooks/useDialogFocus';

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
  const { dialogRef, onKeyDown } = useDialogFocus<HTMLDivElement>(open, dismissable ? onClose : undefined);
  const titleID = useId();

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
        onKeyDown={onKeyDown}
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
