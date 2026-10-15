import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { FailuresProvider, FailuresRegion, useFailures } from './FailuresContext';

function Reporter({ text, detail }: { text: string; detail?: string }) {
  const { report } = useFailures();
  return (
    <button type="button" onClick={() => report({ text, detail })}>
      fail
    </button>
  );
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe('FailuresRegion', () => {
  it('shows a reported failure and announces it', async () => {
    render(
      <FailuresProvider>
        <Reporter text="Could not save the campaign." />
        <FailuresRegion />
      </FailuresProvider>,
    );
    expect(screen.queryByText('Could not save the campaign.')).toBeNull();

    await userEvent.click(screen.getByRole('button', { name: 'fail' }));

    expect(screen.getByRole('status')).toHaveTextContent('Could not save the campaign.');
    expect(screen.getByText('Could not save the campaign.')).toBeInTheDocument();
  });

  it('keeps the detail behind a disclosure rather than leading with it', async () => {
    render(
      <FailuresProvider>
        <Reporter text="Could not save the campaign." detail="Error: 500 at /api/config" />
        <FailuresRegion />
      </FailuresProvider>,
    );
    await userEvent.click(screen.getByRole('button', { name: 'fail' }));

    expect(screen.getByText('Details')).toBeInTheDocument();
    expect(screen.getByText('Error: 500 at /api/config')).toBeInTheDocument();
  });

  it('replaces a repeated sentence rather than stacking it', async () => {
    render(
      <FailuresProvider>
        <Reporter text="Could not save the campaign." />
        <FailuresRegion />
      </FailuresProvider>,
    );
    await userEvent.click(screen.getByRole('button', { name: 'fail' }));
    await userEvent.click(screen.getByRole('button', { name: 'fail' }));

    expect(screen.getAllByText('Could not save the campaign.')).toHaveLength(1);
    expect(screen.queryByText(/earlier/)).toBeNull();
  });

  it('counts earlier failures', async () => {
    render(
      <FailuresProvider>
        <Reporter text="Could not save the campaign." />
        <Reporter text="Could not load the world." />
        <FailuresRegion />
      </FailuresProvider>,
    );
    const [first, second] = screen.getAllByRole('button', { name: 'fail' });
    await userEvent.click(first);
    await userEvent.click(second);

    expect(screen.getByText('Could not load the world.')).toBeInTheDocument();
    expect(screen.getByText('1 earlier failure')).toBeInTheDocument();
  });

  it('dismisses the failure it shows', async () => {
    render(
      <FailuresProvider>
        <Reporter text="Could not save the campaign." />
        <FailuresRegion />
      </FailuresProvider>,
    );
    await userEvent.click(screen.getByRole('button', { name: 'fail' }));
    await userEvent.click(screen.getByRole('button', { name: 'Dismiss failure' }));

    expect(screen.queryByText('Could not save the campaign.')).toBeNull();
    expect(screen.queryByRole('status')).toBeNull();
  });

  it('keeps the console line when there is no provider', async () => {
    const error = vi.spyOn(console, 'error').mockImplementation(() => undefined);
    render(
      <>
        <Reporter text="Could not save the campaign." detail="no provider" />
        <FailuresRegion />
      </>,
    );
    await userEvent.click(screen.getByRole('button', { name: 'fail' }));

    expect(error).toHaveBeenCalledWith('[failure] Could not save the campaign.', 'no provider');
    expect(screen.queryByRole('status')).toBeNull();
  });
});
