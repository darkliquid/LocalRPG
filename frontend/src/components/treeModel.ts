import type { EntitySummary, FolderNode } from '../types';

// The tree's model is plain functions over the folders and notes the server gave
// us. Keeping it out of the component is what makes the tree reviewable without a
// browser, and it is the only part of the tree that has logic worth reading.

export type ItemKind = 'folder' | 'note';

export interface TreeItem {
  id: string;
  kind: ItemKind;
  name: string;
  // folderPath is set on every item: a folder's own path, or the folder a note
  // sits in. It is what a drop destination resolves to.
  folderPath: string;
  entity?: EntitySummary;
}

export const ROOT_LABEL = 'Campaign';

// The id carries the kind, so one flat map can hold folders and notes without a
// folder path ever colliding with a note id.
export function folderItemId(path: string): string {
  return `d:${path}`;
}

export function noteItemId(id: string): string {
  return `n:${id}`;
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

export function findFolder(nodes: FolderNode[], path: string): FolderNode | undefined {
  for (const node of nodes) {
    if (node.path === path) return node;
    const hit = node.children ? findFolder(node.children, path) : undefined;
    if (hit) return hit;
  }
  return undefined;
}

// buildIndex flattens the folders and notes into the id-keyed map the tree's data
// loader reads, so a lookup is a map get rather than a walk.
export function buildIndex(folders: FolderNode[], entities: EntitySummary[]): Map<string, TreeItem> {
  const index = new Map<string, TreeItem>();
  index.set(folderItemId(''), {
    id: folderItemId(''),
    kind: 'folder',
    name: ROOT_LABEL,
    folderPath: '',
  });

  const walk = (nodes: FolderNode[]) => {
    for (const node of nodes) {
      index.set(folderItemId(node.path), {
        id: folderItemId(node.path),
        kind: 'folder',
        name: leafOf(node.path),
        folderPath: node.path,
      });
      if (node.children) walk(node.children);
    }
  };
  walk(folders);

  for (const entity of entities) {
    index.set(noteItemId(entity.id), {
      id: noteItemId(entity.id),
      kind: 'note',
      name: entity.name,
      folderPath: entity.folder ?? '',
      entity,
    });
  }
  return index;
}

// childFoldersOf returns the folders directly inside a path. The top level is the
// array itself rather than a node, because the server sends a list of top-level
// folders and only nests below that, so there is no node whose path is "".
export function childFoldersOf(folders: FolderNode[], path: string): FolderNode[] {
  if (path === '') return folders;
  return findFolder(folders, path)?.children ?? [];
}

// childrenOf lists a folder's child folders and notes in the order the tree shows
// them.
export function childrenOf(folders: FolderNode[], entities: EntitySummary[], path: string): string[] {
  const ids: string[] = [];
  for (const child of childFoldersOf(folders, path)) ids.push(folderItemId(child.path));
  for (const entity of entities) {
    if ((entity.folder ?? '') === path) ids.push(noteItemId(entity.id));
  }
  return ids;
}

// countNotesUnder counts a folder and everything beneath it, so the delete dialog
// can say how much is about to go.
export function countNotesUnder(folders: FolderNode[], entities: EntitySummary[], path: string): number {
  const inFolder = (folder: string) => entities.filter((entity) => (entity.folder ?? '') === folder).length;

  let total = inFolder(path);
  const walk = (nodes: FolderNode[]) => {
    for (const child of nodes) {
      total += inFolder(child.path);
      if (child.children) walk(child.children);
    }
  };
  walk(childFoldersOf(folders, path));
  return total;
}

export interface FolderMove {
  from: string;
  to: string;
}

// movesForChildren compares a parent's new children with what it already had and
// returns the moves that difference implies. The tree's drop handler knows the new
// children of the parent that gained an item, so anything in that list the parent
// did not already have is what just moved in.
export function movesForChildren(
  folders: FolderNode[],
  entities: EntitySummary[],
  index: Map<string, TreeItem>,
  parentPath: string,
  newChildren: string[],
): { entities: { id: string; folder: string }[]; folders: FolderMove[] } {
  const before = new Set(childrenOf(folders, entities, parentPath));
  const entityMoves: { id: string; folder: string }[] = [];
  const folderMoves: FolderMove[] = [];

  for (const childId of newChildren) {
    if (before.has(childId)) continue;
    const moved = index.get(childId);
    if (!moved) continue;

    if (moved.kind === 'note' && moved.entity) {
      entityMoves.push({ id: moved.entity.id, folder: parentPath });
      continue;
    }
    if (moved.kind === 'folder') {
      const to = parentPath === '' ? leafOf(moved.folderPath) : `${parentPath}/${leafOf(moved.folderPath)}`;
      if (to !== moved.folderPath) folderMoves.push({ from: moved.folderPath, to });
    }
  }
  return { entities: entityMoves, folders: folderMoves };
}
