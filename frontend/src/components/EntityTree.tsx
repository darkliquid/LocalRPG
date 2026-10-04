import { useMemo, useState } from 'react';
import {
  ChevronDown,
  ChevronRight,
  FileText,
  Folder,
  FolderPlus,
  MoreVertical,
  MoveRight,
  Pencil,
  Trash2,
} from 'lucide-react';
import type { EntitySummary, FolderNode } from '../types';
import {
  DeleteFolderDialog,
  FolderNameDialog,
  MoveNoteDialog,
} from './TreeDialogs';

interface EntityTreeProps {
  folders: FolderNode[];
  entities: EntitySummary[];
  selectedId?: string;
  onSelect: (entityId: string) => void;
  onMoveEntity: (entityId: string, folder: string) => void;
  onMoveFolder: (from: string, to: string) => void;
  onCreateFolder: (path: string) => void;
  onDeleteFolder: (path: string) => void;
}

// groupByFolder is pure, so the shape of the tree is reviewable without a browser.
export function groupByFolder(entities: EntitySummary[]): Map<string, EntitySummary[]> {
  const byFolder = new Map<string, EntitySummary[]>();
  for (const entity of entities) {
    const folder = entity.folder ?? '';
    const bucket = byFolder.get(folder);
    if (bucket) {
      bucket.push(entity);
    } else {
      byFolder.set(folder, [entity]);
    }
  }
  for (const bucket of byFolder.values()) {
    bucket.sort((a, b) => a.name.localeCompare(b.name, undefined, { sensitivity: 'base' }));
  }
  return byFolder;
}

// noteIdFor resolves a link target to a note id, falling back to the final path
// segment so a hand-written [[guilds/silver-hand]] still lands.
export function noteIdFor(target: string, known: Set<string>): string | undefined {
  const trimmed = target.trim();
  if (known.has(trimmed)) return trimmed;
  const base = trimmed.slice(trimmed.lastIndexOf('/') + 1).trim();
  return known.has(base) ? base : undefined;
}

// parentOf is the folder a path sits inside, which is where a rename has to put it.
export function parentOf(path: string): string {
  const idx = path.lastIndexOf('/');
  return idx === -1 ? '' : path.slice(0, idx);
}

// leafOf is the folder's own name, the part a rename edits.
export function leafOf(path: string): string {
  const idx = path.lastIndexOf('/');
  return idx === -1 ? path : path.slice(idx + 1);
}

// moveData is the drag payload. It is written into dataTransfer because a drag
// with no data never fires a drop in some browsers.
export const MOVE_MIME = 'application/x-localrpg-move';

export interface MovePayload {
  kind: 'note' | 'folder';
  id: string;
}

export function encodeMove(payload: MovePayload): string {
  return JSON.stringify(payload);
}

export function decodeMove(raw: string): MovePayload | null {
  try {
    const parsed = JSON.parse(raw) as MovePayload;
    if (parsed?.kind === 'note' || parsed?.kind === 'folder') return parsed;
    return null;
  } catch {
    return null;
  }
}

type DialogState =
  | { kind: 'none' }
  | { kind: 'create'; parent: string }
  | { kind: 'rename'; path: string }
  | { kind: 'delete'; path: string }
  | { kind: 'move-note'; entity: EntitySummary };

export default function EntityTree({
  folders,
  entities,
  selectedId,
  onSelect,
  onMoveEntity,
  onMoveFolder,
  onCreateFolder,
  onDeleteFolder,
}: EntityTreeProps) {
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const [filter, setFilter] = useState('');
  const [menuFor, setMenuFor] = useState<string | null>(null);
  const [dialog, setDialog] = useState<DialogState>({ kind: 'none' });
  const [dragging, setDragging] = useState<MovePayload | null>(null);
  const [dropTarget, setDropTarget] = useState<string | null>(null);

  const byFolder = useMemo(() => groupByFolder(entities), [entities]);
  const needle = filter.trim().toLowerCase();
  const matches = (entity: EntitySummary) =>
    needle === '' ||
    entity.name.toLowerCase().includes(needle) ||
    entity.id.toLowerCase().includes(needle) ||
    (entity.aliases ?? []).some((alias) => alias.toLowerCase().includes(needle));

  const toggle = (path: string) =>
    setCollapsed((prev) => {
      const next = new Set(prev);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      return next;
    });

  const closeMenu = () => setMenuFor(null);

  // countNotesUnder counts a folder and everything beneath it, so the delete
  // dialog can say how much is about to go.
  const countNotesUnder = (path: string): number => {
    let total = (byFolder.get(path) ?? []).length;
    const walk = (nodes: FolderNode[]) => {
      for (const node of nodes) {
        total += (byFolder.get(node.path) ?? []).length;
        if (node.children) walk(node.children);
      }
    };
    const find = (nodes: FolderNode[]): FolderNode | undefined => {
      for (const node of nodes) {
        if (node.path === path) return node;
        const hit = node.children ? find(node.children) : undefined;
        if (hit) return hit;
      }
      return undefined;
    };
    const self = find(folders);
    if (self?.children) walk(self.children);
    return total;
  };

  const dropProps = (folder: string) => ({
    onDragOver: (ev: React.DragEvent) => {
      if (!dragging) return;
      ev.preventDefault();
      ev.dataTransfer.dropEffect = 'move';
      setDropTarget(folder);
    },
    onDragLeave: () => setDropTarget((current) => (current === folder ? null : current)),
    onDrop: (ev: React.DragEvent) => {
      ev.preventDefault();
      const raw = ev.dataTransfer.getData(MOVE_MIME);
      const payload = decodeMove(raw) ?? dragging;
      if (!payload) return;
      if (payload.kind === 'note') {
        onMoveEntity(payload.id, folder);
      } else if (folder !== payload.id && !folder.startsWith(`${payload.id}/`)) {
        onMoveFolder(payload.id, folder === '' ? leafOf(payload.id) : `${folder}/${leafOf(payload.id)}`);
      }
      setDragging(null);
      setDropTarget(null);
      closeMenu();
    },
  });

  const dragProps = (payload: MovePayload) => ({
    draggable: true,
    onDragStart: (ev: React.DragEvent) => {
      ev.dataTransfer.setData(MOVE_MIME, encodeMove(payload));
      ev.dataTransfer.effectAllowed = 'move';
      setDragging(payload);
    },
    onDragEnd: () => {
      setDragging(null);
      setDropTarget(null);
    },
  });

  const menuButton = (key: string) => (
    <button
      type="button"
      onClick={(ev) => {
        ev.stopPropagation();
        setMenuFor((current) => (current === key ? null : key));
      }}
      title="More actions"
      className="shrink-0 cursor-pointer rounded p-0.5 text-stone-500 hover:text-stone-200"
    >
      <MoreVertical className="w-3 h-3" />
    </button>
  );

  const renderNotes = (folder: string) =>
    (byFolder.get(folder) ?? [])
      .filter(matches)
      .map((entity) => {
        const key = `note:${entity.id}`;
        return (
          <div
            key={entity.id}
            {...dragProps({ kind: 'note', id: entity.id })}
            onClick={() => onSelect(entity.id)}
            className={`group flex cursor-pointer items-center gap-1 rounded-lg px-2 py-1.5 text-xs transition-colors ${
              selectedId === entity.id
                ? 'bg-purple-600/20 border border-purple-500/40 text-purple-100'
                : 'border border-transparent hover:bg-black/30 text-stone-200'
            }`}
          >
            <FileText className="w-3 h-3 shrink-0 text-stone-500" />
            <span className="flex-1 truncate">
              {entity.name}
              {entity.filename_mismatch && <span className="ml-1 text-[10px] text-amber-400">(file name differs)</span>}
            </span>
            <span className="relative">
              {menuButton(key)}
              {menuFor === key && (
                <div className="absolute right-0 z-20 mt-1 w-44 rounded-lg border border-white/10 bg-stone-900 p-1 shadow-2xl">
                  <button
                    type="button"
                    onClick={(ev) => {
                      ev.stopPropagation();
                      closeMenu();
                      setDialog({ kind: 'move-note', entity });
                    }}
                    className="flex w-full cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-left text-xs text-stone-200 hover:bg-white/10"
                  >
                    <MoveRight className="w-3 h-3" />
                    <span>Move to folder...</span>
                  </button>
                </div>
              )}
            </span>
          </div>
        );
      });

  const renderFolder = (node: FolderNode, depth: number) => {
    const isCollapsed = collapsed.has(node.path);
    const key = `folder:${node.path}`;
    const isDropTarget = dropTarget === node.path;
    return (
      <div key={node.path} style={{ paddingLeft: depth * 10 }}>
        <div
          {...dropProps(node.path)}
          {...dragProps({ kind: 'folder', id: node.path })}
          className={`group flex items-center gap-1 rounded-lg px-1 py-1 ${
            isDropTarget ? 'bg-purple-600/20 ring-1 ring-purple-500/50' : 'hover:bg-black/30'
          }`}
        >
          <button
            type="button"
            onClick={() => toggle(node.path)}
            className="cursor-pointer text-stone-400"
            title={isCollapsed ? 'Expand' : 'Collapse'}
          >
            {isCollapsed ? <ChevronRight className="w-3 h-3" /> : <ChevronDown className="w-3 h-3" />}
          </button>
          <Folder className="w-3.5 h-3.5 text-purple-400/70" />
          <span className="flex-1 truncate text-xs text-stone-300">{leafOf(node.path)}</span>
          <span className="relative">
            {menuButton(key)}
            {menuFor === key && (
              <div className="absolute right-0 z-20 mt-1 w-44 rounded-lg border border-white/10 bg-stone-900 p-1 shadow-2xl">
                <button
                  type="button"
                  onClick={(ev) => {
                    ev.stopPropagation();
                    closeMenu();
                    setDialog({ kind: 'create', parent: node.path });
                  }}
                  className="flex w-full cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-left text-xs text-stone-200 hover:bg-white/10"
                >
                  <FolderPlus className="w-3 h-3" />
                  <span>New subfolder</span>
                </button>
                <button
                  type="button"
                  onClick={(ev) => {
                    ev.stopPropagation();
                    closeMenu();
                    setDialog({ kind: 'rename', path: node.path });
                  }}
                  className="flex w-full cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-left text-xs text-stone-200 hover:bg-white/10"
                >
                  <Pencil className="w-3 h-3" />
                  <span>Rename folder</span>
                </button>
                <button
                  type="button"
                  onClick={(ev) => {
                    ev.stopPropagation();
                    closeMenu();
                    setDialog({ kind: 'delete', path: node.path });
                  }}
                  className="flex w-full cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-left text-xs text-red-300 hover:bg-red-950/40"
                >
                  <Trash2 className="w-3 h-3" />
                  <span>Delete folder</span>
                </button>
              </div>
            )}
          </span>
        </div>
        {!isCollapsed && (
          <div>
            {renderNotes(node.path)}
            {(node.children ?? []).map((child) => renderFolder(child, depth + 1))}
          </div>
        )}
      </div>
    );
  };

  const rootNotes = renderNotes('');

  return (
    <div className="flex min-h-0 flex-col gap-2">
      <div className="flex items-center gap-1.5">
        <input
          value={filter}
          onChange={(ev) => setFilter(ev.target.value)}
          placeholder="Filter notes..."
          className="flex-1 rounded-lg border border-white/10 bg-black/50 px-2 py-1.5 text-xs text-stone-200 placeholder-stone-500 focus:border-purple-500/60 focus:outline-none"
        />
        <button
          type="button"
          onClick={() => setDialog({ kind: 'create', parent: '' })}
          title="New folder"
          className="cursor-pointer rounded-lg border border-white/10 bg-black/40 p-1.5 text-stone-300 hover:text-purple-300"
        >
          <FolderPlus className="w-3.5 h-3.5" />
        </button>
      </div>

      <div
        {...dropProps('')}
        className={`min-h-0 flex-1 space-y-0.5 overflow-y-auto pr-1 rounded-lg ${
          dropTarget === '' && dragging ? 'bg-purple-600/10 ring-1 ring-purple-500/40' : ''
        }`}
      >
        {rootNotes}
        {folders.map((node) => renderFolder(node, 0))}
        {folders.length === 0 && rootNotes.length === 0 && (
          <p className="p-2 text-xs italic text-stone-500">No notes yet.</p>
        )}
      </div>

      <p className="text-[10px] font-sans text-stone-500">
        Drag a note onto a folder to file it, or drag it here to take it back out.
      </p>

      <FolderNameDialog
        isOpen={dialog.kind === 'create'}
        mode="create"
        parent={dialog.kind === 'create' ? dialog.parent : ''}
        onCancel={() => setDialog({ kind: 'none' })}
        onSubmit={(name) => {
          if (dialog.kind === 'create') {
            onCreateFolder(dialog.parent === '' ? name : `${dialog.parent}/${name}`);
          }
          setDialog({ kind: 'none' });
        }}
      />

      <FolderNameDialog
        isOpen={dialog.kind === 'rename'}
        mode="rename"
        currentName={dialog.kind === 'rename' ? leafOf(dialog.path) : ''}
        onCancel={() => setDialog({ kind: 'none' })}
        onSubmit={(name) => {
          if (dialog.kind === 'rename') {
            const parent = parentOf(dialog.path);
            onMoveFolder(dialog.path, parent === '' ? name : `${parent}/${name}`);
          }
          setDialog({ kind: 'none' });
        }}
      />

      <DeleteFolderDialog
        isOpen={dialog.kind === 'delete'}
        path={dialog.kind === 'delete' ? dialog.path : ''}
        noteCount={dialog.kind === 'delete' ? countNotesUnder(dialog.path) : 0}
        onCancel={() => setDialog({ kind: 'none' })}
        onSubmit={() => {
          if (dialog.kind === 'delete') onDeleteFolder(dialog.path);
          setDialog({ kind: 'none' });
        }}
      />

      <MoveNoteDialog
        isOpen={dialog.kind === 'move-note'}
        noteName={dialog.kind === 'move-note' ? dialog.entity.name : ''}
        folders={folders}
        currentFolder={dialog.kind === 'move-note' ? dialog.entity.folder ?? '' : ''}
        onCancel={() => setDialog({ kind: 'none' })}
        onSubmit={(folder) => {
          if (dialog.kind === 'move-note') onMoveEntity(dialog.entity.id, folder);
          setDialog({ kind: 'none' });
        }}
      />
    </div>
  );
}
