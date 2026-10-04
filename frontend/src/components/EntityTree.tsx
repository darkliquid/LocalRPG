import { useEffect, useMemo, useRef, useState } from 'react';
import {
  ChevronDown,
  ChevronRight,
  FileText,
  Folder,
  FolderPlus,
  MoveRight,
  Pencil,
  Trash2,
} from 'lucide-react';
import {
  createOnDropHandler,
  dragAndDropFeature,
  expandAllFeature,
  hotkeysCoreFeature,
  propMemoizationFeature,
  selectionFeature,
  syncDataLoaderFeature,
  type ItemInstance,
  type TreeConfig,
} from '@headless-tree/core';
import { useTree } from '@headless-tree/react';
import type { EntitySummary, FolderNode } from '../types';
import { DeleteFolderDialog, FolderNameDialog, MoveNoteDialog } from './TreeDialogs';
import RowMenu from './RowMenu';
import {
  ROOT_LABEL,
  buildIndex,
  childrenOf,
  countNotesUnder,
  folderItemId,
  leafOf,
  movesForChildren,
  noteItemId,
  parentOf,
  type TreeItem,
} from './treeModel';

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

type DialogState =
  | { kind: 'none' }
  | { kind: 'create'; parent: string }
  | { kind: 'rename'; path: string }
  | { kind: 'delete'; path: string }
  | { kind: 'move-note'; entity: EntitySummary };

// EntityTree renders the folder tree. Placement, dragging, keyboard navigation and
// the drag line come from Headless Tree; what this file owns is the markup, the
// Tailwind styling and the translation of a drop into the two calls the server
// understands. The model it reads lives in treeModel.ts.
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
  const [filter, setFilter] = useState('');
  const [dialog, setDialog] = useState<DialogState>({ kind: 'none' });

  const needle = filter.trim().toLowerCase();
  // The filter narrows notes only. Folders stay visible, so a match inside a
  // collapsed folder is still reachable.
  const visibleEntities = useMemo(
    () =>
      needle === ''
        ? entities
        : entities.filter(
            (entity) =>
              entity.name.toLowerCase().includes(needle) ||
              entity.id.toLowerCase().includes(needle) ||
              (entity.aliases ?? []).some((alias) => alias.toLowerCase().includes(needle)),
          ),
    [entities, needle],
  );

  const index = useMemo(() => buildIndex(folders, visibleEntities), [folders, visibleEntities]);

  // The drop handler is rebuilt every render so it always closes over the current
  // index and children. Headless Tree does the placement arithmetic and hands back
  // the new children of the parent that gained an item; the model turns that
  // difference into moves.
  const config: TreeConfig<TreeItem> = {
    rootItemId: folderItemId(''),
    indent: 12,
    getItemName: (item) => item.getItemData().name,
    isItemFolder: (item) => item.getItemData().kind === 'folder',
    dataLoader: {
      getItem: (id) => index.get(id) as TreeItem,
      getChildren: (id) => childrenOf(folders, visibleEntities, index.get(id)?.folderPath ?? ''),
    },
    features: [
      syncDataLoaderFeature,
      selectionFeature,
      hotkeysCoreFeature,
      dragAndDropFeature,
      expandAllFeature,
      propMemoizationFeature,
    ],
    initialState: { expandedItems: [folderItemId('')] },
    onDrop: createOnDropHandler((parent: ItemInstance<TreeItem>, newChildren: string[]) => {
      const moves = movesForChildren(
        folders,
        visibleEntities,
        index,
        parent.getItemData().folderPath,
        newChildren,
      );
      for (const move of moves.entities) onMoveEntity(move.id, move.folder);
      for (const move of moves.folders) onMoveFolder(move.from, move.to);
    }),
  };

  const tree = useTree<TreeItem>(config);

  // The tree caches its structure, so a move made through the server has to be
  // announced or the rows would keep their old parents.
  const firstRun = useRef(true);
  useEffect(() => {
    if (firstRun.current) {
      firstRun.current = false;
      return;
    }
    tree.rebuildTree();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [folders, visibleEntities]);

  // The selection lives in the tree, so the drawer's chosen note is pushed in.
  useEffect(() => {
    tree.setSelectedItems(selectedId ? [noteItemId(selectedId)] : []);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedId]);

  // A menu closes on any click elsewhere, including one on another row.

  const renderRow = (item: ItemInstance<TreeItem>) => {
    const data = item.getItemData();
    const isFolder = data.kind === 'folder';
    const isSelected = !isFolder && data.entity?.id === selectedId;
    const key = item.getId();

    return (
      <div
        key={key}
        {...item.getProps()}
        style={{ paddingLeft: `${item.getItemMeta().level * 12}px` }}
        onClick={() => {
          if (!isFolder && data.entity) onSelect(data.entity.id);
        }}
        className={`flex cursor-pointer items-center gap-1 rounded-lg border px-1 py-1 text-xs transition-colors ${
          isSelected
            ? 'border-purple-500/40 bg-purple-600/20 text-purple-100'
            : item.isDragTarget()
              ? 'border-purple-500/50 bg-purple-600/20 text-stone-100'
              : 'border-transparent text-stone-200 hover:bg-black/30'
        } ${item.isFocused() ? 'outline outline-1 outline-purple-500/40' : ''}`}
      >
        {isFolder ? (
          <button
            type="button"
            onClick={(ev) => {
              ev.stopPropagation();
              if (item.isExpanded()) item.collapse();
              else item.expand();
            }}
            className="cursor-pointer text-stone-400"
            title={item.isExpanded() ? 'Collapse' : 'Expand'}
          >
            {item.isExpanded() ? <ChevronDown className="w-3 h-3" /> : <ChevronRight className="w-3 h-3" />}
          </button>
        ) : (
          <span className="w-3 shrink-0" />
        )}

        {isFolder ? (
          <Folder className="w-3.5 h-3.5 shrink-0 text-purple-400/70" />
        ) : (
          <FileText className="w-3 h-3 shrink-0 text-stone-500" />
        )}

        <span className="flex-1 truncate">
          {data.name}
          {!isFolder && data.entity?.filename_mismatch && (
            <span className="ml-1 text-[10px] text-amber-400">(file name differs)</span>
          )}
        </span>

        <span className="relative shrink-0">
          <RowMenu
            label={isFolder ? 'Folder actions' : 'Note actions'}
            items={
              isFolder
                ? [
                    {
                      label: 'New subfolder',
                      icon: <FolderPlus className="w-3 h-3" />,
                      onSelect: () => setDialog({ kind: 'create', parent: data.folderPath }),
                    },
                    ...(data.folderPath === ''
                      ? []
                      : [
                          {
                            label: 'Rename folder',
                            icon: <Pencil className="w-3 h-3" />,
                            onSelect: () => setDialog({ kind: 'rename', path: data.folderPath }),
                          },
                          {
                            label: 'Delete folder',
                            icon: <Trash2 className="w-3 h-3" />,
                            destructive: true,
                            onSelect: () => setDialog({ kind: 'delete', path: data.folderPath }),
                          },
                        ]),
                  ]
                : data.entity
                  ? [
                      {
                        label: 'Move to folder...',
                        icon: <MoveRight className="w-3 h-3" />,
                        onSelect: () =>
                          setDialog({ kind: 'move-note', entity: data.entity as EntitySummary }),
                      },
                    ]
                  : []
            }
          />
        </span>
      </div>
    );
  };

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

      <div className="relative min-h-0 flex-1 overflow-y-auto pr-1">
        <div {...tree.getContainerProps('Content')} className="space-y-0.5">
          {tree.getItems().map(renderRow)}
          <div style={tree.getDragLineStyle()} className="absolute h-0.5 rounded bg-purple-500" />
        </div>
      </div>

      <p className="text-[10px] font-sans text-stone-500">
        Drag a note or folder onto another folder to move it, or onto {ROOT_LABEL} to take it back out.
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
        noteCount={dialog.kind === 'delete' ? countNotesUnder(folders, entities, dialog.path) : 0}
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
