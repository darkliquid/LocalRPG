import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import { CodexDrawer } from './CodexDrawer';
import { EntityNote, EntitySummary } from '../types';

// The drawer reads its corpus and folder tree from a campaign, so the fixture
// omits gameID: every loader effect then returns early and the test needs no
// API mock.
const note: EntityNote = {
  id: 'the-tidewatch',
  name: 'The Tidewatch',
  type: 'faction',
  markdown: 'Rivals of [[Saltmarch]].',
  state: {},
  backlinks: [],
};

const other: EntitySummary = { id: 'saltmarch', name: 'Saltmarch', type: 'location' };

describe('CodexDrawer', () => {
  it('declares the merge confirmation as a dialog and leaves it on Escape', () => {
    const onMerge = vi.fn();

    render(<CodexDrawer entity={note} entities={[other]} onSelect={() => {}} onSave={() => {}} onMerge={onMerge} />);

    fireEvent.click(screen.getByRole('button', { name: /merge note/i }));

    const dialog = screen.getByRole('dialog', { name: 'Merge Note' });
    expect(dialog).toBeInTheDocument();

    fireEvent.keyDown(dialog, { key: 'Escape' });

    expect(screen.queryByRole('dialog', { name: 'Merge Note' })).not.toBeInTheDocument();
    expect(onMerge).not.toHaveBeenCalled();
  });
});
