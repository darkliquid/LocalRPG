import { useEffect, useState } from 'react';
import { FolderPlus, FolderTree, MoveRight, Pencil } from 'lucide-react';
import type { FolderNode } from '../types';

// flattenFolders turns the tree into the list a destination picker needs, with the
// root first because moving a note back to the top level is the common case.
export function flattenFolders(folders: FolderNode[]): string[] {
  const out: string[] = [];
  const walk = (nodes: FolderNode[]) => {
    for (const node of nodes) {
      out.push(node.path);
      if (node.children) walk(node.children);
    }
  };
  walk(folders);
  return out;
}

interface DialogShellProps {
  title: string;
  icon: React.ReactNode;
  onCancel: () => void;
  onSubmit: () => void;
  submitLabel: string;
  children: React.ReactNode;
  destructive?: boolean;
}

// DialogShell keeps every dialog in the tree the same shape as the rest of the
// app's dialogs, so the folder tools do not look like a different application.
const DialogShell: React.FC<DialogShellProps> = ({
  title,
  icon,
  onCancel,
  onSubmit,
  submitLabel,
  children,
  destructive,
}) => (
  <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm select-none">
    <form
      className="w-full max-w-md bg-stone-900 border border-white/15 rounded-2xl p-5 shadow-2xl space-y-4"
      onSubmit={(ev) => {
        ev.preventDefault();
        onSubmit();
      }}
    >
      <div className={`flex items-center gap-2 ${destructive ? 'text-red-300' : 'text-purple-300'}`}>
        {icon}
        <h3 className="font-sans text-sm font-bold text-white">{title}</h3>
      </div>
      {children}
      <div className="flex justify-end gap-2">
        <button
          type="button"
          onClick={onCancel}
          className="text-xs font-sans px-3 py-1.5 rounded-lg border border-stone-700 text-stone-300 hover:text-white hover:bg-stone-800 transition-all cursor-pointer"
        >
          Cancel
        </button>
        <button
          type="submit"
          className={`text-xs font-sans font-bold px-3 py-1.5 rounded-lg text-white transition-all cursor-pointer ${
            destructive ? 'bg-red-600 hover:bg-red-500' : 'bg-purple-600 hover:bg-purple-500'
          }`}
        >
          {submitLabel}
        </button>
      </div>
    </form>
  </div>
);

export interface FolderNameDialogProps {
  isOpen: boolean;
  mode: 'create' | 'rename';
  // parent is the folder a new folder goes inside; '' means the top level.
  parent?: string;
  currentName?: string;
  onCancel: () => void;
  onSubmit: (name: string) => void;
}

// FolderNameDialog names a new folder or renames an existing one. A rename is a
// move to a new path in the same parent, so it needs no separate backend route.
export const FolderNameDialog: React.FC<FolderNameDialogProps> = ({
  isOpen,
  mode,
  parent = '',
  currentName = '',
  onCancel,
  onSubmit,
}) => {
  const [name, setName] = useState(currentName);

  useEffect(() => {
    if (isOpen) setName(currentName);
  }, [isOpen, currentName]);

  if (!isOpen) return null;

  const trimmed = name.trim();
  const where = parent === '' ? 'the top level' : `"${parent}"`;

  return (
    <DialogShell
      title={mode === 'create' ? 'New folder' : 'Rename folder'}
      icon={mode === 'create' ? <FolderPlus className="w-4 h-4" /> : <Pencil className="w-4 h-4" />}
      submitLabel={mode === 'create' ? 'Create folder' : 'Rename'}
      onCancel={onCancel}
      onSubmit={() => {
        if (trimmed) onSubmit(trimmed);
      }}
    >
      <p className="text-xs font-sans text-stone-400">
        {mode === 'create' ? `This folder is created inside ${where}.` : `Renaming a folder moves everything in it.`}
      </p>
      <input
        autoFocus
        value={name}
        onChange={(ev) => setName(ev.target.value)}
        placeholder="e.g. factions"
        className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-sm text-stone-100 placeholder-stone-600 focus:outline-none focus:border-purple-500/50 transition-colors"
      />
      <p className="text-[11px] font-sans text-stone-500">
        A folder name cannot contain a slash and cannot start with a dot.
      </p>
    </DialogShell>
  );
};

export interface MoveNoteDialogProps {
  isOpen: boolean;
  noteName: string;
  folders: FolderNode[];
  currentFolder?: string;
  onCancel: () => void;
  onSubmit: (folder: string) => void;
}

// MoveNoteDialog is the move that does not depend on dragging, so a note can be
// filed into a folder, or taken back out of one, without a pointer gesture.
export const MoveNoteDialog: React.FC<MoveNoteDialogProps> = ({
  isOpen,
  noteName,
  folders,
  currentFolder = '',
  onCancel,
  onSubmit,
}) => {
  const [target, setTarget] = useState(currentFolder);

  useEffect(() => {
    if (isOpen) setTarget(currentFolder);
  }, [isOpen, currentFolder]);

  if (!isOpen) return null;

  const destinations = flattenFolders(folders);

  return (
    <DialogShell
      title="Move note"
      icon={<MoveRight className="w-4 h-4" />}
      submitLabel="Move note"
      onCancel={onCancel}
      onSubmit={() => onSubmit(target)}
    >
      <p className="text-xs font-sans text-stone-400">
        Move <span className="text-stone-200">{noteName}</span> to a folder. Its links keep working, because a link
        points at the note's id rather than at its path.
      </p>
      <select
        autoFocus
        value={target}
        onChange={(ev) => setTarget(ev.target.value)}
        className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-sm text-stone-100 focus:outline-none focus:border-purple-500/50 transition-colors cursor-pointer"
      >
        <option value="">Campaign root</option>
        {destinations.map((path) => (
          <option key={path} value={path}>
            {path}
          </option>
        ))}
      </select>
      {destinations.length === 0 && (
        <p className="text-[11px] font-sans text-stone-500">
          There are no folders yet. Create one with the folder button above the tree.
        </p>
      )}
    </DialogShell>
  );
};

export interface DeleteFolderDialogProps {
  isOpen: boolean;
  path: string;
  noteCount: number;
  onCancel: () => void;
  onSubmit: () => void;
}

// DeleteFolderDialog names the folder and says how many notes go with it, because
// the deletion is recursive and that is the only place the blast radius is shown.
export const DeleteFolderDialog: React.FC<DeleteFolderDialogProps> = ({
  isOpen,
  path,
  noteCount,
  onCancel,
  onSubmit,
}) => {
  if (!isOpen) return null;
  return (
    <DialogShell
      title="Delete folder"
      icon={<FolderTree className="w-4 h-4" />}
      submitLabel="Delete folder"
      destructive
      onCancel={onCancel}
      onSubmit={onSubmit}
    >
      <p className="text-xs font-sans text-stone-400">
        Delete <span className="text-stone-200">{path}</span> and every folder inside it.
      </p>
      <p className="text-xs font-sans text-red-300">
        {noteCount === 0
          ? 'The folder is empty.'
          : `${noteCount} ${noteCount === 1 ? 'note' : 'notes'} in it will be deleted too. This cannot be undone.`}
      </p>
    </DialogShell>
  );
};
