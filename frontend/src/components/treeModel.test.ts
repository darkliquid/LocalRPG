import { describe, expect, it } from 'vitest';
import {
  buildIndex,
  childrenOf,
  countNotesUnder,
  folderItemId,
  leafOf,
  movesForChildren,
  noteItemId,
  parentOf,
} from './treeModel';
import type { EntitySummary, FolderNode } from '../types';

// Mirror the shape the server sends: a list of top-level folders, nested below.
const folders = (paths: string[]): FolderNode[] => {
  const root: FolderNode[] = [];
  for (const path of paths) {
    let level = root;
    let prefix = '';
    for (const segment of path.split('/')) {
      prefix = prefix ? `${prefix}/${segment}` : segment;
      let node = level.find((candidate) => candidate.path === prefix);
      if (!node) {
        node = { path: prefix, name: segment, children: [] };
        level.push(node);
      }
      level = node.children ?? [];
    }
  }
  return root;
};

const entities: EntitySummary[] = [
  { id: 'a', name: 'A', type: 'character', folder: '' },
  { id: 'b', name: 'B', type: 'character', folder: 'factions' },
  { id: 'c', name: 'C', type: 'character', folder: 'factions/orders' },
];

describe('the tree model', () => {
  it('lists top-level folders as children of the root', () => {
    const tree = folders(['factions', 'factions/orders']);
    expect(childrenOf(tree, entities, '')).toEqual([folderItemId('factions'), noteItemId('a')]);
  });

  it('nests a folder under its own children', () => {
    const tree = folders(['factions', 'factions/orders']);
    expect(childrenOf(tree, entities, 'factions')).toEqual([folderItemId('factions/orders'), noteItemId('b')]);
  });

  it('lists a note in the deepest folder', () => {
    const tree = folders(['factions', 'factions/orders']);
    expect(childrenOf(tree, entities, 'factions/orders')).toEqual([noteItemId('c')]);
  });

  it('counts descendants', () => {
    const tree = folders(['factions', 'factions/orders']);
    expect(countNotesUnder(tree, entities, 'factions')).toBe(2);
  });

  it('reports path helpers', () => {
    expect([leafOf('factions/orders'), parentOf('factions/orders')]).toEqual(['orders', 'factions']);
  });

  it('shows a new top-level folder', () => {
    const tree = folders(['factions', 'factions/orders', 'places']);
    expect(childrenOf(tree, entities, '')).toEqual([
      folderItemId('factions'),
      folderItemId('places'),
      noteItemId('a'),
    ]);
  });

  it('puts a new folder in the index', () => {
    const tree = folders(['factions', 'factions/orders', 'places']);
    const index = buildIndex(tree, entities);
    expect([index.has(folderItemId('places')), index.get(folderItemId('places'))?.name]).toEqual([true, 'places']);
  });

  it('starts a new folder empty', () => {
    const tree = folders(['factions', 'factions/orders', 'places']);
    expect(childrenOf(tree, entities, 'places')).toEqual([]);
  });

  it('shows a new subfolder under its parent', () => {
    const tree = folders(['factions', 'factions/orders', 'factions/houses']);
    expect(childrenOf(tree, entities, 'factions')).toEqual([
      folderItemId('factions/orders'),
      folderItemId('factions/houses'),
      noteItemId('b'),
    ]);
  });

  it('moves a note dropped into a folder', () => {
    const tree = folders(['factions', 'factions/orders']);
    const afterDrop: EntitySummary[] = [
      { id: 'a', name: 'A', type: 'character', folder: '' },
      { id: 'c', name: 'C', type: 'character', folder: 'factions/orders' },
      { id: 'b', name: 'B', type: 'character', folder: 'factions/orders' },
    ];
    const moves = movesForChildren(
      tree,
      entities,
      buildIndex(tree, entities),
      'factions/orders',
      childrenOf(tree, afterDrop, 'factions/orders'),
    );
    expect(moves.entities).toEqual([{ id: 'b', folder: 'factions/orders' }]);
    expect(moves.folders).toEqual([]);
  });

  it('moves nothing when a drop changes nothing', () => {
    const tree = folders(['factions', 'factions/orders']);
    const noMoves = movesForChildren(
      tree,
      entities,
      buildIndex(tree, entities),
      'factions/orders',
      childrenOf(tree, entities, 'factions/orders'),
    );
    expect([noMoves.entities, noMoves.folders]).toEqual([[], []]);
  });

  it('moves a folder dropped into a folder', () => {
    const tree = folders(['factions', 'factions/orders', 'places']);
    const folderMoves = movesForChildren(tree, entities, buildIndex(tree, entities), 'factions', [
      folderItemId('factions/orders'),
      folderItemId('places'),
    ]);
    expect(folderMoves.folders).toEqual([{ from: 'places', to: 'factions/places' }]);
  });

  it('leaves a folder already at the root where it is', () => {
    const tree = folders(['factions', 'factions/orders', 'places']);
    const toRoot = movesForChildren(tree, entities, buildIndex(tree, entities), '', [
      folderItemId('factions'),
      folderItemId('places'),
    ]);
    expect(toRoot.folders).toEqual([]);
  });

  it('brings a note back out when dropped on the root', () => {
    const tree = folders(['factions', 'factions/orders']);
    const outOfFolder = movesForChildren(
      tree,
      entities,
      buildIndex(tree, entities),
      '',
      childrenOf(tree, [{ id: 'b', name: 'B', type: 'character', folder: '' }, ...entities.slice(0, 1)], ''),
    );
    expect(outOfFolder.entities).toEqual([{ id: 'b', folder: '' }]);
  });
});
