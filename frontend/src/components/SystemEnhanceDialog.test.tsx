import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { SystemEnhanceDialog } from './SystemEnhanceDialog';
import { APIClient } from '../api/client';
import { SystemProposal } from '../types';

const proposals: SystemProposal[] = [
  { kind: 'stat', title: 'Sanity', stat: { id: 'sanity' }, reason: 'adds a mental track', valid: true },
  { kind: 'skill', title: 'Occult', skill: { id: 'occult', stat: 'sanity' }, valid: true },
];

afterEach(() => {
  vi.restoreAllMocks();
});

describe('SystemEnhanceDialog', () => {
  it('accepts and rejects proposals', async () => {
    const apply = vi
      .spyOn(APIClient, 'applySystemEnhancements')
      .mockResolvedValue({ written: ['mechanics'] });

    render(<SystemEnhanceDialog systemId="s" onClose={() => {}} proposals={proposals} />);

    expect(screen.getByText(/2 of 2 accepted/i)).toBeInTheDocument();
    expect(screen.getByText(/why: adds a mental track/i)).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /reject occult/i }));
    expect(screen.getByText(/1 of 2 accepted/i)).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /apply accepted/i }));

    await waitFor(() => expect(apply).toHaveBeenCalledTimes(1));
    expect(apply.mock.calls[0][1].proposals).toEqual([proposals[0]]);
  });

  it('disables apply when everything is rejected', () => {
    render(<SystemEnhanceDialog systemId="s" onClose={() => {}} proposals={proposals} />);

    fireEvent.click(screen.getByRole('button', { name: /reject sanity/i }));
    fireEvent.click(screen.getByRole('button', { name: /reject occult/i }));

    expect(screen.getByRole('button', { name: /apply accepted/i })).toBeDisabled();
  });

  it('asks for proposals from an instruction', async () => {
    const enhance = vi.spyOn(APIClient, 'enhanceSystem').mockResolvedValue({ proposals });

    render(<SystemEnhanceDialog systemId="s" onClose={() => {}} />);

    fireEvent.change(screen.getByLabelText(/what should be added/i), {
      target: { value: 'add sanity' },
    });
    fireEvent.click(screen.getByRole('button', { name: /propose changes/i }));

    await waitFor(() => expect(enhance).toHaveBeenCalledTimes(1));
    expect(enhance.mock.calls[0][1]).toEqual({ instruction: 'add sanity' });
    await waitFor(() => expect(screen.getByText('Sanity')).toBeInTheDocument());
  });

  it('marks a proposal that would break the system', () => {
    render(
      <SystemEnhanceDialog
        systemId="s"
        onClose={() => {}}
        proposals={[
          {
            kind: 'skill',
            title: 'Occult',
            skill: { id: 'occult', stat: 'missing' },
            valid: false,
            problems: ['mechanics.skills.occult: unknown stat "missing"'],
          },
        ]}
      />
    );
    expect(screen.getByText(/unknown stat/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /apply accepted/i })).toBeDisabled();
  });
});
