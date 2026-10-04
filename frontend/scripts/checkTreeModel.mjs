// Checks the folder tree's model, which is the only part of the tree with logic.
// The model is plain TypeScript with no React, so it can be bundled and executed
// here rather than only exercised by hand in a browser.
//
// This exists because the model shipped a bug that made every top-level folder
// invisible: childrenOf() looked for a folder node whose path was "", but the
// server sends a list of top-level folders and only nests below that, so the root
// has no node. Nothing caught it until someone made a folder and it did not appear.
import { execFileSync } from 'node:child_process';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const here = fileURLToPath(new URL('..', import.meta.url));
const out = mkdtempSync(join(tmpdir(), 'localrpg-tree-'));

try {
  const bundle = join(out, 'treeModel.mjs');
  execFileSync(
    'npx',
    ['esbuild', 'src/components/treeModel.ts', '--bundle', '--format=esm', `--outfile=${bundle}`, '--log-level=error'],
    { cwd: here, stdio: 'inherit' },
  );

  const {
    buildIndex,
    childrenOf,
    countNotesUnder,
    folderItemId,
    leafOf,
    movesForChildren,
    noteItemId,
    parentOf,
  } = await import(pathToFileURL(bundle).href);

  // Mirror the shape the server sends: a list of top-level folders, nested below.
  const folders = (paths) => {
    const root = [];
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
        level = node.children;
      }
    }
    return root;
  };

  const entities = [
    { id: 'a', name: 'A', folder: '' },
    { id: 'b', name: 'B', folder: 'factions' },
    { id: 'c', name: 'C', folder: 'factions/orders' },
  ];

  let failures = 0;
  const check = (label, got, want) => {
    const g = JSON.stringify(got);
    const w = JSON.stringify(want);
    if (g !== w) {
      console.error(`FAIL ${label}\n  got  ${g}\n  want ${w}`);
      failures++;
      return;
    }
    console.log(`ok   ${label}`);
  };

  let tree = folders(['factions', 'factions/orders']);

  // The regression that motivated this check.
  check('top-level folders are children of the root', childrenOf(tree, entities, ''), [
    folderItemId('factions'),
    noteItemId('a'),
  ]);
  check('a folder nests its own children', childrenOf(tree, entities, 'factions'), [
    folderItemId('factions/orders'),
    noteItemId('b'),
  ]);
  check('the deepest folder lists its note', childrenOf(tree, entities, 'factions/orders'), [noteItemId('c')]);
  check('counts include descendants', countNotesUnder(tree, entities, 'factions'), 2);
  check('path helpers', [leafOf('factions/orders'), parentOf('factions/orders')], ['orders', 'factions']);

  // A newly created top-level folder has to show up.
  tree = folders(['factions', 'factions/orders', 'places']);
  check('a new top-level folder appears', childrenOf(tree, entities, ''), [
    folderItemId('factions'),
    folderItemId('places'),
    noteItemId('a'),
  ]);
  const index = buildIndex(tree, entities);
  check('a new folder is in the index', [index.has(folderItemId('places')), index.get(folderItemId('places')).name], [
    true,
    'places',
  ]);
  check('a new folder starts empty', childrenOf(tree, entities, 'places'), []);

  // A new subfolder has to show up under its parent.
  tree = folders(['factions', 'factions/orders', 'factions/houses']);
  check('a new subfolder appears under its parent', childrenOf(tree, entities, 'factions'), [
    folderItemId('factions/orders'),
    folderItemId('factions/houses'),
    noteItemId('b'),
  ]);

  // A drop is reported as the target parent's new children.
  tree = folders(['factions', 'factions/orders']);
  const afterDrop = [
    { id: 'a', name: 'A', folder: '' },
    { id: 'c', name: 'C', folder: 'factions/orders' },
    { id: 'b', name: 'B', folder: 'factions/orders' },
  ];
  const moves = movesForChildren(
    tree,
    entities,
    buildIndex(tree, entities),
    'factions/orders',
    childrenOf(tree, afterDrop, 'factions/orders'),
  );
  check('a note dropped into a folder moves there', moves.entities, [{ id: 'b', folder: 'factions/orders' }]);
  check('a note drop moves no folders', moves.folders, []);

  const noMoves = movesForChildren(
    tree,
    entities,
    buildIndex(tree, entities),
    'factions/orders',
    childrenOf(tree, entities, 'factions/orders'),
  );
  check('a drop that changes nothing moves nothing', [noMoves.entities, noMoves.folders], [[], []]);

  // A folder dropped onto another folder, and one already at the root.
  tree = folders(['factions', 'factions/orders', 'places']);
  const folderMoves = movesForChildren(tree, entities, buildIndex(tree, entities), 'factions', [
    folderItemId('factions/orders'),
    folderItemId('places'),
  ]);
  check('a folder dropped into a folder moves there', folderMoves.folders, [
    { from: 'places', to: 'factions/places' },
  ]);

  const toRoot = movesForChildren(tree, entities, buildIndex(tree, entities), '', [
    folderItemId('factions'),
    folderItemId('places'),
  ]);
  check('a folder already at the root needs no move', toRoot.folders, []);

  // A note dropped onto the root comes back out of its folder.
  const outOfFolder = movesForChildren(
    tree,
    entities,
    buildIndex(tree, entities),
    '',
    childrenOf(tree, [{ id: 'b', name: 'B', folder: '' }, ...entities.slice(0, 1)], ''),
  );
  check('a note dropped on the root comes back out', outOfFolder.entities, [{ id: 'b', folder: '' }]);

  if (failures > 0) {
    console.error(`\n${failures} tree model check(s) failed`);
    process.exit(1);
  }
  console.log('\ntree model is consistent');
} finally {
  rmSync(out, { recursive: true, force: true });
}
