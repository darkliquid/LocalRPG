import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import EntityTree from './EntityTree';
import type { EntitySummary } from '../types';

const entity: EntitySummary = { id: 'saltmarch', name: 'Saltmarch', type: 'location' };
const noop = () => {};

function renderTree(onDeleteEntity?: (id: string) => void) {
  render(
    <EntityTree
      folders={[]}
      entities={[entity]}
      onSelect={noop}
      onMoveEntity={noop}
      onMoveFolder={noop}
      onCreateFolder={noop}
      onDeleteFolder={noop}
      onDeleteEntity={onDeleteEntity}
    />,
  );
}

describe('EntityTree note menu', () => {
  it('offers Delete on a note and calls onDeleteEntity after confirmation', async () => {
    const onDeleteEntity = vi.fn();
    renderTree(onDeleteEntity);

    fireEvent.click(screen.getByTitle('Note actions'));
    fireEvent.click(await screen.findByRole('menuitem', { name: /^delete$/i }));
    fireEvent.click(screen.getByRole('button', { name: /delete note/i }));

    await waitFor(() => expect(onDeleteEntity).toHaveBeenCalledWith('saltmarch'));
  });

  it('shows no Delete item when the consumer cannot delete', () => {
    renderTree(undefined);

    fireEvent.click(screen.getByTitle('Note actions'));
    expect(screen.queryByRole('menuitem', { name: /^delete$/i })).not.toBeInTheDocument();
  });
});