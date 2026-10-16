import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { WorldsStudio } from './WorldsStudio';
import { APIClient } from '../api/client';

// The lore and prompt tabs mount CodeMirror, which needs a real layout; the
// studio's own rendering does not depend on it, so it is replaced with a stub.
vi.mock('./editor/MarkdownEditor', () => ({ default: () => <div data-testid="editor" /> }));

const world = {
  id: 'ember-peak',
  name: 'Ember Peak',
  description: 'A frontier town.',
  genre: 'fantasy',
  default_system: '',
  art_style: '',
  tags: [],
  lore_prompt: '',
  entities: [{ id: 'saltmarch', name: 'Saltmarch', type: 'location' }],
};

function mockStudio() {
  vi.spyOn(APIClient, 'listWorlds').mockResolvedValue([
    { id: 'ember-peak', name: 'Ember Peak', description: '', genre: '', compatible_systems: [] },
  ]);
  vi.spyOn(APIClient, 'listSystems').mockResolvedValue([]);
  vi.spyOn(APIClient, 'listWorldFolders').mockResolvedValue([]);
  vi.spyOn(APIClient, 'getWorld').mockResolvedValue(world);
  vi.spyOn(APIClient, 'getWorldEntity').mockImplementation(async (_worldId, entityId) => ({
    id: entityId,
    markdown: `---\nid: ${entityId}\nname: ${entityId}\ntype: concept\n---\n\nA note.\n`,
  }));
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe('WorldsStudio tabs', () => {
  it('renders every tab with its own content', async () => {
    mockStudio();
    render(<WorldsStudio />);

    // A world has to be opened before the tabs exist.
    await waitFor(() => expect(screen.getAllByText('Ember Peak').length).toBeGreaterThan(0));
    fireEvent.click(screen.getAllByText('Ember Peak')[0]);

    expect(await screen.findByText('World Setting Name')).toBeInTheDocument();

    for (const [tab, landmark] of [
      ['lore.md', /AI Storyteller Atmosphere Instructions/],
      [/Starter Entities/, /Templates/],
    ] as const) {
      fireEvent.click(screen.getByRole('button', { name: tab }));
      expect(await screen.findByText(landmark)).toBeInTheDocument();
    }
  });
});
