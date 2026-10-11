import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import { ScenarioEditor } from './ScenarioEditor';
import type { Scenario } from '../types';

const blank: Scenario = { name: '', seed: 0, steps: [] };

describe('ScenarioEditor', () => {
  it('saves a named scenario with its step', () => {
    const onSave = vi.fn();
    render(<ScenarioEditor scenario={blank} onSave={onSave} onCancel={() => {}} />);

    fireEvent.change(screen.getByPlaceholderText(/a strong hit/i), { target: { value: 'strong hit' } });
    fireEvent.change(screen.getByPlaceholderText('the check profile name'), { target: { value: 'standard' } });
    fireEvent.click(screen.getByRole('button', { name: /save scenario/i }));

    expect(onSave).toHaveBeenCalledTimes(1);
    const saved = onSave.mock.calls[0][0] as Scenario;
    expect(saved.name).toBe('strong hit');
    expect(saved.steps).toHaveLength(1);
    expect(saved.steps[0].input).toBe('standard');
  });

  it('will not save without a name', () => {
    render(<ScenarioEditor scenario={blank} onSave={() => {}} onCancel={() => {}} />);
    expect(screen.getByRole('button', { name: /save scenario/i })).toBeDisabled();
  });

  it('edits an existing scenario', () => {
    const onSave = vi.fn();
    const existing: Scenario = {
      name: 'ladder',
      seed: 7,
      steps: [{ action: 'check', input: 'standard', expect: { outcome: 'strong' } }],
    };

    render(<ScenarioEditor scenario={existing} onSave={onSave} onCancel={() => {}} />);

    expect(screen.getByDisplayValue('ladder')).toBeInTheDocument();
    fireEvent.change(screen.getByPlaceholderText('strong'), { target: { value: 'weak' } });
    fireEvent.click(screen.getByRole('button', { name: /save scenario/i }));

    expect((onSave.mock.calls[0][0] as Scenario).steps[0].expect?.outcome).toBe('weak');
  });
});