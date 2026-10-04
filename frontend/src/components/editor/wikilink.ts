import type { CompletionSource } from '@codemirror/autocomplete';
import type { EntitySummary } from '../../types';
import { slugify } from '../../lib/slug';

// smartWikilink inserts the bare id when the display name already slugifies to it,
// and adds a label only when it carries information the id does not. The engine
// unwraps [[id|label]] and preserves the label when it rewrites a link.
export function smartWikilink(id: string, name: string): string {
  return slugify(name) === id ? `[[${id}]]` : `[[${id}|${name}]]`;
}

// scoreEntity ranks a note against a query. A name hit beats an id hit, an alias
// beats a tag, and an empty query matches everything so [[ alone lists the codex.
export function scoreEntity(entity: EntitySummary, query: string): number {
  if (query === '') return 1;
  const name = entity.name.toLowerCase();
  const id = entity.id.toLowerCase();
  if (name.startsWith(query)) return 100;
  if (name.includes(query)) return 80;
  if (id.includes(query)) return 60;
  if ((entity.aliases ?? []).some((alias) => alias.toLowerCase().includes(query))) return 40;
  if ((entity.tags ?? []).some((tag) => tag.toLowerCase().includes(query))) return 20;
  return 0;
}

// wikilinkCompletion offers the notes that exist, so an author never has to know
// an id in advance. It fires in the body and inside the frontmatter's link-bearing
// values, because both hold links.
export function wikilinkCompletion(entities: EntitySummary[]): CompletionSource {
  return (context) => {
    const match = context.matchBefore(/\[\[[^[\]]*$/);
    if (!match) return null;

    const query = match.text.slice(2).trim().toLowerCase();
    const options = entities
      .map((entity) => ({ entity, score: scoreEntity(entity, query) }))
      .filter((entry) => entry.score > 0)
      .sort((a, b) => b.score - a.score || a.entity.name.localeCompare(b.entity.name))
      .slice(0, 50)
      .map(({ entity }) => ({
        label: entity.name,
        type: 'text',
        detail: entity.folder ? `${entity.folder}/${entity.id}` : entity.id,
        apply: smartWikilink(entity.id, entity.name),
      }));

    if (options.length === 0) return null;
    return { from: match.from, options, validFor: /^\[\[[^[\]]*$/ };
  };
}
