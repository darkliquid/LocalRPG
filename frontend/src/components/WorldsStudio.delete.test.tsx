import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { WorldsStudio } from './WorldsStudio';import { APIClient } from '../api/client';

const world = {
  id: 'ember-peak',
  name: 'Ember Peak',
  description: 'A frontier town.',
  genre: 'fantasy',
  default_system: '',
  art_style: '',
  tags: [],
  lore_prompt: '',
  entities: [
    { id: 'saltmarch', name: 'Saltmarch', type: 'location' },
    { id: 'the-tidewatch', name: 'The Tidewatch', type: 'faction' },
  ],
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
  vi.spyOn(APIClient, 'deleteWorldEntity').mockResolvedValue(undefined);
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe('WorldsStudio entity deletion', () => {
  it('deletes a selected entity without crashing', async () => {
    mockStudio();
    render(<WorldsStudio />);

    // Open the world, then its entity tab.
    await waitFor(() => expect(screen.getAllByText('Ember Peak').length).toBeGreaterThan(0));
    fireEvent.click(screen.getAllByText('Ember Peak')[0]);
    await waitFor(() => expect(screen.getByText(/starter entities/i)).toBeInTheDocument());
    fireEvent.click(screen.getByText(/starter entities/i));

    await waitFor(() => expect(screen.getByText(/saltmarch\.md/)).toBeInTheDocument());

    fireEvent.click(screen.getByTitle('Delete entity template'));

    await waitFor(() => expect(APIClient.deleteWorldEntity).toHaveBeenCalledWith('ember-peak', 'saltmarch'));
    // The screen is still rendered: a React error would have blanked it.
    await waitFor(() => expect(screen.getByText(/starter entities/i)).toBeInTheDocument());
  });

  it('deletes the last entity without crashing', async () => {
    mockStudio();
    vi.spyOn(APIClient, 'getWorld').mockResolvedValue({
      ...world,
      entities: [{ id: 'saltmarch', name: 'Saltmarch', type: 'location' }],
    });

    render(<WorldsStudio />);
    await waitFor(() => expect(screen.getAllByText('Ember Peak').length).toBeGreaterThan(0));
    fireEvent.click(screen.getAllByText('Ember Peak')[0]);
    await waitFor(() => expect(screen.getByText(/starter entities/i)).toBeInTheDocument());
    fireEvent.click(screen.getByText(/starter entities/i));

    await waitFor(() => expect(screen.getByText(/saltmarch\.md/)).toBeInTheDocument());
    fireEvent.click(screen.getByTitle('Delete entity template'));

    await waitFor(() => expect(APIClient.deleteWorldEntity).toHaveBeenCalled());
    await waitFor(() => expect(screen.getByText(/starter entities/i)).toBeInTheDocument());
  });
});
