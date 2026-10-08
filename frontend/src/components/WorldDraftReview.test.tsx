import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import { WorldDraftReview } from './WorldDraftReview';
import { WorldDraftInfo } from '../types';

const draft: WorldDraftInfo = {
  id: 'ashen-reach',
  name: 'Ashen Reach',
  description: 'A dying frontier.',
  genre: 'dark fantasy',
  lore: '# Lore\n\nA.\n\n## History\n\nB.\n',
  sections: [
    { title: 'Lore', body: 'A.' },
    { title: 'History', body: 'B.' },
  ],
  entities: [{ id: 'saltmarch', name: 'Saltmarch', type: 'location', body: 'A port.', links: ['The Tidewatch'] }],
};

describe('WorldDraftReview', () => {
  it('counts accepted items', () => {
    render(<WorldDraftReview draft={draft} onCommit={vi.fn()} onDiscard={vi.fn()} />);
    expect(screen.getByText(/3 of 3 accepted/i)).toBeInTheDocument();
    expect(screen.getByText('Saltmarch')).toBeInTheDocument();
    expect(screen.getByText('History')).toBeInTheDocument();
  });

  it('rejecting disables commit when all are rejected', () => {
    const onCommit = vi.fn();
    render(<WorldDraftReview draft={draft} onCommit={onCommit} onDiscard={vi.fn()} />);

    fireEvent.click(screen.getByLabelText('Reject Lore'));
    fireEvent.click(screen.getByLabelText('Reject History'));
    fireEvent.click(screen.getByLabelText('Reject Saltmarch'));

    expect(screen.getByText(/0 of 3 accepted/i)).toBeInTheDocument();
    const commit = screen.getByRole('button', { name: /create world/i });
    expect(commit).toBeDisabled();

    fireEvent.click(commit);
    expect(onCommit).not.toHaveBeenCalled();
  });

  it('sends only the accepted set', () => {
    const onCommit = vi.fn();
    render(<WorldDraftReview draft={draft} onCommit={onCommit} onDiscard={vi.fn()} />);

    fireEvent.click(screen.getByLabelText('Reject History'));
    fireEvent.click(screen.getByRole('button', { name: /create world/i }));

    expect(onCommit).toHaveBeenCalledTimes(1);
    const req = onCommit.mock.calls[0][0];
    expect(req.draft_id).toBe('ashen-reach');
    expect(req.section_indexes).toEqual([0]);
    expect(req.entity_ids).toEqual(['saltmarch']);
  });

  it('editing an entity updates what commit sends', () => {
    const onCommit = vi.fn();
    render(<WorldDraftReview draft={draft} onCommit={onCommit} onDiscard={vi.fn()} />);

    fireEvent.click(screen.getByLabelText('Edit Saltmarch'));
    fireEvent.change(screen.getByLabelText('Saltmarch body'), {
      target: { value: 'A port watched by [[The Tidewatch]].' },
    });
    fireEvent.click(screen.getByRole('button', { name: /create world/i }));

    const req = onCommit.mock.calls[0][0];
    expect(req.accept_all).toBe(true);
    expect(req.edits?.[0].body).toBe('A port watched by [[The Tidewatch]].');
  });

  it('merges into an existing world when a target is named', () => {
    const onCommit = vi.fn();
    render(
      <WorldDraftReview draft={draft} targetWorldId="ember-peak" onCommit={onCommit} onDiscard={vi.fn()} />
    );
    expect(screen.getByText(/merging into ember-peak/i)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /merge accepted/i }));
    expect(onCommit.mock.calls[0][0].target_world_id).toBe('ember-peak');
  });

  it('discards the draft', () => {
    const onDiscard = vi.fn();
    render(<WorldDraftReview draft={draft} onCommit={vi.fn()} onDiscard={onDiscard} />);
    fireEvent.click(screen.getByRole('button', { name: /discard/i }));
    expect(onDiscard).toHaveBeenCalledWith('ashen-reach');
  });

  it('says when the draft came from the offline fallback', () => {
    render(
      <WorldDraftReview draft={{ ...draft, oracle: true }} onCommit={vi.fn()} onDiscard={vi.fn()} />
    );
    expect(screen.getByText(/built-in template generator/i)).toBeInTheDocument();
  });

  it('shows unresolved links that were dropped', () => {
    render(
      <WorldDraftReview
        draft={{
          ...draft,
          entities: [{ id: 'x', name: 'X', type: 'concept', body: 'B.', dropped_links: ['Nowhere'] }],
        }}
        onCommit={vi.fn()}
        onDiscard={vi.fn()}
      />
    );
    expect(screen.getByText(/dropped unresolved links: nowhere/i)).toBeInTheDocument();
  });
});
