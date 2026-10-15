import { useCallback, useEffect, useRef, type KeyboardEvent } from 'react';

// The element kinds a dialog lets you tab through. Disabled controls and
// tabindex="-1" are excluded because tabbing cannot reach them, which is what
// this list is for.
const FOCUSABLE =
  'a[href],button:not([disabled]),textarea:not([disabled]),input:not([disabled]),select:not([disabled]),[tabindex]:not([tabindex="-1"])';

// useDialogFocus gives any dialog the keyboard behaviour a modal owes its user:
// focus moves to its first control when it opens, Tab cycles inside it rather
// than reaching the page behind, Escape closes it, and focus returns to whatever
// opened it when it closes. A bespoke dialog that cannot take the Modal shell —
// because its layout is its own — attaches the ref and the handler instead, and
// gets the same behaviour from the same code.
//
// onClose is optional: a dialog that must be answered passes nothing, and Escape
// is then ignored rather than closing it.
export function useDialogFocus<T extends HTMLElement = HTMLElement>(open: boolean, onClose?: () => void) {
  const dialogRef = useRef<T | null>(null);
  const openerRef = useRef<HTMLElement | null>(null);

  // Remember what had focus so closing puts the user back: dropping focus on the
  // body makes a keyboard user find their place again from the top.
  useEffect(() => {
    if (!open) return;
    openerRef.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const first = dialogRef.current?.querySelector<HTMLElement>(FOCUSABLE);
    (first ?? dialogRef.current)?.focus();
    return () => openerRef.current?.focus();
  }, [open]);

  const onKeyDown = useCallback(
    (event: KeyboardEvent<HTMLElement>) => {
      if (event.key === 'Escape') {
        if (!onClose) return;
        event.stopPropagation();
        onClose();
        return;
      }
      if (event.key !== 'Tab') return;
      const dialog = dialogRef.current;
      if (!dialog) return;
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
    [onClose],
  );

  return { dialogRef, onKeyDown };
}
