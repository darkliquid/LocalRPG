import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import { BaseSystemCatalogue } from './BaseSystemCatalogue';

const base = { id: 'd20_dc', name: 'd20 + DC' };

describe('BaseSystemCatalogue', () => {
  it('offers clone and derive per base', () => {
    render(<BaseSystemCatalogue bases={[base]} onClone={() => {}} onDerive={() => {}} />);
    expect(screen.getByRole('button', { name: /start from this base/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /derive/i })).toBeInTheDocument();
  });

  it('clones a base', () => {
    const onClone = vi.fn();
    render(<BaseSystemCatalogue bases={[base]} onClone={onClone} onDerive={() => {}} />);
    fireEvent.click(screen.getByRole('button', { name: /start from this base/i }));
    expect(onClone).toHaveBeenCalledWith(base);
  });

  it('derives a variant from an instruction', () => {
    const onDerive = vi.fn();
    render(<BaseSystemCatalogue bases={[base]} onClone={() => {}} onDerive={onDerive} />);

    fireEvent.click(screen.getByRole('button', { name: /derive/i }));
    fireEvent.change(screen.getByLabelText(/how should the variant change/i), {
      target: { value: 'add sanity' },
    });
    fireEvent.click(screen.getByRole('button', { name: /derive variant/i }));

    expect(onDerive).toHaveBeenCalledWith(base, 'add sanity');
  });

  it('will not derive without an instruction', () => {
    render(<BaseSystemCatalogue bases={[base]} onClone={() => {}} onDerive={() => {}} />);
    fireEvent.click(screen.getByRole('button', { name: /derive/i }));
    expect(screen.getByRole('button', { name: /derive variant/i })).toBeDisabled();
  });
});
