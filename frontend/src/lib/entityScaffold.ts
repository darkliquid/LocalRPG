import type { EntityTypeCatalog, FrontmatterKeySchema } from '../types';

export interface VoiceSelection {
  provider: string;
  voice_id: string;
  pitch?: number;
  speech_rate?: number;
  options?: Record<string, unknown>;
}

export interface ScaffoldInput {
  id: string;
  name: string;
  type: string;
  voice?: VoiceSelection;
}

// yamlScalar quotes a value so a name containing a colon, hash or quote cannot
// break the block it is written into.
const yamlScalar = (value: string): string => JSON.stringify(value);

// voiceBlock renders the voice key. The sub-keys and their order match what the
// codex writes when a voice archetype is applied, so both paths produce the same
// frontmatter. With no selection every sub-key is still written, empty.
function voiceBlock(voice?: VoiceSelection): string[] {
  const lines = [
    'voice:',
    `  voice_id: ${yamlScalar(voice?.voice_id ?? '')}`,
    `  provider: ${yamlScalar(voice?.provider ?? '')}`,
    `  pitch: ${voice?.pitch ?? 1}`,
    `  speech_rate: ${voice?.speech_rate ?? 1}`,
  ];
  if (voice?.options && Object.keys(voice.options).length > 0) {
    lines.push('  options:');
    for (const [key, value] of Object.entries(voice.options)) {
      lines.push(`    ${key}: ${JSON.stringify(value)}`);
    }
  }
  return lines;
}

function keyLines(key: FrontmatterKeySchema, input: ScaffoldInput): string[] {
  switch (key.name) {
    case 'id':
      return [`id: ${yamlScalar(input.id)}`];
    case 'name':
      return [`name: ${yamlScalar(input.name)}`];
    case 'type':
      return [`type: ${yamlScalar(input.type)}`];
    case 'voice':
      return voiceBlock(input.voice);
    case 'tags':
    case 'aliases':
      return [`${key.name}: []`];
    case 'state':
      return [`${key.name}: {}`];
    default:
      return [`${key.name}: ""`];
  }
}

// buildEntityMarkdown renders a new note: the frontmatter keys the chosen type
// offers, each preceded by the description the server supplies, then an H1 the
// user writes under. It is pure so it can be checked without a browser.
export function buildEntityMarkdown(catalog: EntityTypeCatalog, input: ScaffoldInput): string {
  const spec = catalog.types.find((candidate) => candidate.id === input.type);
  const keys = spec ? spec.keys : catalog.base_keys;

  const lines: string[] = ['---'];
  for (const key of keys) {
    lines.push(`# ${key.description}`);
    lines.push(...keyLines(key, input));
  }
  lines.push('---', '', `# ${input.name}`, '');
  return lines.join('\n');
}
