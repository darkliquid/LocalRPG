import { render, screen, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { RegistryModal } from './RegistryModal';
import { APIClient } from '../api/client';

describe('RegistryModal', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('renders package results and check updates button', async () => {
    vi.spyOn(APIClient, 'searchRegistry').mockResolvedValue([
      {
        registry_name: 'Community Registry',
        registry_url: 'https://example.org/index.json',
        package: {
          type: 'world',
          id: 'test_world',
          name: 'Test World',
          version: '1.0.0',
          description: 'A test world description',
          download: 'https://example.org/test.lrpgpack',
          sha256: 'abcd1234',
        },
      },
    ]);
    vi.spyOn(APIClient, 'checkRegistryUpdates').mockResolvedValue([]);

    render(
      <RegistryModal
        isOpen={true}
        onClose={vi.fn()}
      />
    );

    expect(screen.getByText('Content Registry')).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.getByText('Test World')).toBeInTheDocument();
    });
    expect(screen.getByText('A test world description')).toBeInTheDocument();
    expect(screen.getByText('Install')).toBeInTheDocument();
  });

  it('does not render when isOpen is false', () => {
    const { container } = render(
      <RegistryModal
        isOpen={false}
        onClose={vi.fn()}
      />
    );
    expect(container).toBeEmptyDOMElement();
  });
});
