import { useMemo, useState } from 'react';
import { ChevronDown, ChevronRight, FileText, Folder, FolderPlus, Trash2 } from 'lucide-react';
import type { EntitySummary, FolderNode } from '../types';

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
  const [dragEntity, setDragEntity] = useState<string | null>(null);
  const [dragFolder, setDragFolder] = useState<string | null>(null);

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

  const renderNotes = (folder: string) =>
    (byFolder.get(folder) ?? [])
      .filter(matches)
      .map((entity) => (
        <button
          key={entity.id}
          draggable
          onDragStart={() => setDragEntity(entity.id)}
          onDragEnd={() => setDragEntity(null)}
          onClick={() => onSelect(entity.id)}
          className={`w-full text-left px-2 py-1.5 rounded-lg text-xs transition-colors cursor-pointer ${
            selectedId === entity.id
              ? 'bg-purple-600/20 border border-purple-500/40 text-purple-100'
              : 'border border-transparent hover:bg-black/30 text-stone-200'
          }`}
        >
          <span className="flex items-center gap-1.5 truncate">
            <FileText className="w-3 h-3 shrink-0 text-stone-500" />
            <span className="truncate">{entity.name}</span>
          </span>
          {entity.filename_mismatch && (
            <span className="block pl-4 text-[10px] text-amber-400">file name differs from id</span>
          )}
        </button>
      ));

  const renderFolder = (node: FolderNode, depth: number) => {
    const isCollapsed = collapsed.has(node.path);
    return (
      <div key={node.path} style={{ paddingLeft: depth * 8 }}>
        <div
          className="flex items-center gap-1 rounded-lg px-1 py-1 hover:bg-black/30"
          onDragOver={(ev) => ev.preventDefault()}
          onDrop={() => {
            if (dragEntity) onMoveEntity(dragEntity, node.path);
            if (dragFolder && dragFolder !== node.path) {
              const leaf = dragFolder.split('/').pop() ?? dragFolder;
              onMoveFolder(dragFolder, `${node.path}/${leaf}`);
            }
            setDragEntity(null);
            setDragFolder(null);
          }}
        >
          <button onClick={() => toggle(node.path)} className="cursor-pointer text-stone-400">
            {isCollapsed ? <ChevronRight className="w-3 h-3" /> : <ChevronDown className="w-3 h-3" />}
          </button>
          <Folder className="w-3.5 h-3.5 text-purple-400/70" />
          <span
            draggable
            onDragStart={() => setDragFolder(node.path)}
            onDragEnd={() => setDragFolder(null)}
            className="flex-1 truncate text-xs text-stone-300 cursor-grab"
          >
            {node.name}
          </span>
          <button
            onClick={() => onDeleteFolder(node.path)}
            title="Delete folder"
            className="cursor-pointer text-stone-500 hover:text-red-400"
          >
            <Trash2 className="w-3 h-3" />
          </button>
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
          onClick={() => {
            const name = window.prompt('New folder name');
            if (name && name.trim()) onCreateFolder(name.trim());
          }}
          title="New folder"
          className="cursor-pointer rounded-lg border border-white/10 bg-black/40 p-1.5 text-stone-300 hover:text-purple-300"
        >
          <FolderPlus className="w-3.5 h-3.5" />
        </button>
      </div>

      <div
        className="min-h-0 flex-1 space-y-0.5 overflow-y-auto pr-1"
        onDragOver={(ev) => ev.preventDefault()}
        onDrop={() => {
          if (dragEntity) onMoveEntity(dragEntity, '');
          setDragEntity(null);
          setDragFolder(null);
        }}
      >
        {renderNotes('')}
        {folders.map((node) => renderFolder(node, 0))}
      </div>
    </div>
  );
}
