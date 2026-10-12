import { render, screen, fireEvent } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { ImageProviderEditor } from './ImageProviderEditor';
import type { ImageConfig } from '../../types';

const entry = (): ImageConfig => ({ type: 'gemini', auto_generate: true });

describe('ImageProviderEditor location art', () => {
  it('defaults to the authored look with what happened to it layered on', () => {
    render(
      <ImageProviderEditor
        entry={entry()}
        onChange={() => {}}
        presets={{}}
        geminiSharedKey={false}
        onTest={() => {}}
        testingCategory={null}
        testResult={null}
      />,
    );

    const select = screen.getByLabelText(/location art/i) as HTMLSelectElement;
    expect(select.value).toBe('original_and_current');
  });

  it('reports a change to the authored look alone', () => {
    const onChange = vi.fn();
    render(
      <ImageProviderEditor
        entry={entry()}
        onChange={onChange}
        presets={{}}
        geminiSharedKey={false}
        onTest={() => {}}
        testingCategory={null}
        testResult={null}
      />,
    );

    fireEvent.change(screen.getByLabelText(/location art/i), { target: { value: 'original' } });

    expect(onChange).toHaveBeenCalledWith({ ...entry(), location_art_appearance: 'original' });
  });
});
