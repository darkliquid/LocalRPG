import type { Extension } from '@codemirror/state';
import { markdown } from '@codemirror/lang-markdown';
import { yaml, yamlFrontmatter } from '@codemirror/lang-yaml';

export type EditorLanguage = 'markdown' | 'markdown-frontmatter' | 'yaml';

// languageExtensions maps a document kind to its parser. It is a plain function
// with no editor instance, so the mapping is reviewable on its own.
export function languageExtensions(language: EditorLanguage): Extension[] {
  switch (language) {
    case 'markdown-frontmatter':
      // A note is markdown whose head is a YAML block, so the frontmatter is
      // parsed as YAML while the body keeps markdown parsing.
      return [yamlFrontmatter({ content: markdown() })];
    case 'yaml':
      return [yaml()];
    case 'markdown':
    default:
      return [markdown()];
  }
}
