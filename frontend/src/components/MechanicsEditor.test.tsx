import React from 'react';
import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { MechanicsEditor } from './MechanicsEditor';
import { MechanicsSpec } from '../types';

const Harness: React.FC<{ initial?: MechanicsSpec }> = ({ initial = {} }) => {
  const [mechanics, setMechanics] = React.useState<MechanicsSpec>(initial);
  return (
    <>
      <MechanicsEditor mechanics={mechanics} onChange={setMechanics} />
      <pre data-testid="dump">{JSON.stringify(mechanics)}</pre>
    </>
  );
};

const dump = (): MechanicsSpec => JSON.parse(screen.getByTestId('dump').textContent ?? '{}');

describe('MechanicsEditor', () => {
  it('adds and removes a stat', () => {
    const onChange = vi.fn();
    render(<MechanicsEditor mechanics={{}} onChange={onChange} />);
    fireEvent.click(screen.getByText(/add stat/i));
    const calls = onChange.mock.calls;
    const next = calls[calls.length - 1][0] as MechanicsSpec;
    expect(next.stats).toHaveLength(1);
  });

  it('toggles allow_freeform_state', () => {
    render(<Harness />);
    fireEvent.click(screen.getByLabelText(/allow freeform state/i));
    expect(dump().allow_freeform_state).toBe(true);
  });

  it('sets the engagement level', () => {
    render(<Harness />);
    fireEvent.change(screen.getByLabelText('Engagement'), { target: { value: 'auto' } });
    expect(dump().engagement).toBe('auto');
  });

  it('adds a difficulty', () => {
    render(<Harness />);
    fireEvent.click(screen.getByText(/add difficulty/i));
    expect(dump().checks?.difficulty).toHaveLength(1);
  });

  it('adds a profile with a ladder step', () => {
    render(<Harness />);
    fireEvent.click(screen.getByText(/add profile/i));
    fireEvent.click(screen.getByText(/add step/i));
    const profiles = dump().checks?.profiles ?? {};
    const name = Object.keys(profiles)[0];
    expect(profiles[name].ladder).toHaveLength(1);
  });

  it('adds an unlock with an effect', () => {
    render(<Harness />);
    fireEvent.click(screen.getByLabelText(/declare advancement/i));
    fireEvent.click(screen.getByText(/add unlock/i));
    fireEvent.click(screen.getByText(/add effect/i));
    expect(dump().advancement?.unlocks?.[0].effects).toHaveLength(1);
  });
});
