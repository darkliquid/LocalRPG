// Lints the embedded help articles with the markdownlint engine directly.
//
// The engine is `markdownlint`, whose only dependencies are micromark and
// string-width. The two CLIs that wrap it add file globbing, and that is where
// the trouble was: markdownlint-cli2 pulls globby -> micromatch -> braces, and
// braces has an unfixed stack-exhaustion advisory. markdownlint-cli avoids braces
// but pins js-yaml ~5.2.1, which is inside a different advisory's range. Neither
// wrapper buys us anything here, because the file list is a single literal
// directory, so this enumerates it and calls the engine.
//
// CI runs `mise run lint:docs`, which runs this, so the swap needs no workflow
// change.
import { readdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { join, relative } from 'node:path';
import { lint } from 'markdownlint/sync';

const frontendDir = fileURLToPath(new URL('..', import.meta.url));
const docsDir = fileURLToPath(new URL('../../pkg/gui/docs/', import.meta.url));

// These three rules are the only deviations from the default set. They were the
// contents of the .markdownlint-cli2.jsonc this replaced; the explanations are the
// comments that file carried.
const config = {
  // Documentation is authored as prose that wraps naturally in the viewer, so line
  // length is not enforced.
  MD013: false,
  // Each article carries its title in frontmatter and repeats it as the H1.
  MD025: { front_matter_title: '' },
  // MarkdownDocViewer renders a small, known set of inline HTML.
  MD033: false,
};

const files = readdirSync(docsDir)
  .filter((name) => name.endsWith('.md'))
  .sort()
  .map((name) => join(docsDir, name));

if (files.length === 0) {
  console.error(`no markdown files found in ${docsDir}`);
  process.exit(1);
}

const results = lint({ files, config });

let issues = 0;
let failingFiles = 0;
for (const file of files) {
  const errors = results[file];
  if (!errors || errors.length === 0) continue;
  failingFiles++;
  // Sorted by position, so the output reads top to bottom and matches what the
  // CLI this replaced printed.
  const ordered = [...errors].sort(
    (a, b) => a.lineNumber - b.lineNumber || (a.errorRange?.[0] ?? 0) - (b.errorRange?.[0] ?? 0),
  );
  for (const error of ordered) {
    issues++;
    const rule = error.ruleNames.join('/');
    const context = error.errorContext ? ` [Context: "${error.errorContext}"]` : '';
    const detail = error.errorDetail ? ` [${error.errorDetail}]` : '';
    const column = error.errorRange ? `:${error.errorRange[0]}` : '';
    // Relative to frontend/, which is how the glob used to name these files.
    const shown = relative(frontendDir, file);
    console.log(
      `${shown}:${error.lineNumber}${column} ${error.severity} ${rule} ${error.ruleDescription}${context}${detail}`,
    );
  }
}

console.log(`Linting: ${files.length} files`);
console.log(`Summary: ${issues} issues in ${failingFiles} files`);

process.exit(issues === 0 ? 0 : 1);
