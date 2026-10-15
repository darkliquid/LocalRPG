import React from 'react';
import { AlertCircle } from 'lucide-react';
import { Modal } from '../Modal';

interface DiscardDraftConfirmProps {
  isOpen: boolean;
  onCancel: () => void;
  onDiscard: () => void;
  title?: string;
  description?: string;
}

// DiscardDraftConfirm is the confirmation that leaves without saving, and it is
// the Modal shell with three lines of content. The shell brings what it needs:
// the dialog semantics, focus moved in and returned, and Escape — which here
// means "keep editing", because that is what cancelling does.
export const DiscardDraftConfirm: React.FC<DiscardDraftConfirmProps> = ({
  isOpen,
  onCancel,
  onDiscard,
  title = 'Discard unsaved world?',
  description = 'This world has not been saved. Leaving now discards every field you entered.',
}) => (
  <Modal open={isOpen} onClose={onCancel} title={title} className="max-w-md">
    <div className="space-y-4">
      <p className="flex items-center gap-2 text-xs font-sans text-stone-400">
        <AlertCircle className="h-4 w-4 shrink-0 text-amber-300" aria-hidden="true" />
        {description}
      </p>
      <div className="flex justify-end gap-2">
        <button
          type="button"
          onClick={onCancel}
          className="cursor-pointer rounded-lg border border-stone-700 px-3 py-1.5 font-sans text-xs text-stone-300 transition-all hover:bg-stone-800 hover:text-white"
        >
          Keep editing
        </button>
        <button
          type="button"
          onClick={onDiscard}
          className="cursor-pointer rounded-lg bg-red-600 px-3 py-1.5 font-sans text-xs font-bold text-white transition-all hover:bg-red-500"
        >
          Discard
        </button>
      </div>
    </div>
  </Modal>
);
