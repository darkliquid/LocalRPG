import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { RegistryModal } from './RegistryModal';
import { APIClient } from '../api/client';
import type { PackageRefDTO, RegistrySourceDTO } from '../types';

const testWorld: PackageRefDTO = {
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
};

const oneSource: RegistrySourceDTO[] = [
  { url: 'https://example.org/index.json', name: 'Example', package_count: 2 },
];

function mockRegistry(sources: RegistrySourceDTO[] = [], packages: PackageRefDTO[] = []) {
  vi.spyOn(APIClient, 'listRegistrySources').mockResolvedValue(sources);
  vi.spyOn(APIClient, 'searchRegistry').mockResolvedValue(packages);
  vi.spyOn(APIClient, 'checkRegistryUpdates').mockResolvedValue([]);
}

describe('RegistryModal', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('renders package results and check updates button', async () => {
    mockRegistry(oneSource, [testWorld]);

    render(<RegistryModal isOpen={true} onClose={vi.fn()} />);

    expect(screen.getByText('Content Registry')).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.getByText('Test World')).toBeInTheDocument();
    });
    expect(screen.getByText('A test world description')).toBeInTheDocument();
    expect(screen.getByText('Install')).toBeInTheDocument();
  });

  it('does not render when isOpen is false', () => {
    const { container } = render(<RegistryModal isOpen={false} onClose={vi.fn()} />);
    expect(container).toBeEmptyDOMElement();
  });

  it('shows the publisher and makes no provenance claim before install', async () => {
    mockRegistry(oneSource, [
      {
        registry_name: 'Community Registry',
        registry_url: 'https://example.org/index.json',
        package: {
          type: 'system',
          id: 'sys_one',
          name: 'System One',
          version: '1.0.0',
          download: 'https://example.org/one.lrpgpack',
          sha256: 'abcd1234',
          publisher: 'deadbeefcafef00d',
        },
      },
    ]);

    render(<RegistryModal isOpen={true} onClose={vi.fn()} />);

    await waitFor(() => {
      expect(screen.getByText(/Publisher deadbeef/)).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText('Install'));

    expect(screen.getByText('Import Content Package')).toBeInTheDocument();
    expect(screen.queryByText(/Provenance:/)).not.toBeInTheDocument();
  });

  describe('empty states', () => {
    it('offers to add a source when none are configured', async () => {
      mockRegistry([], []);

      render(<RegistryModal isOpen={true} onClose={vi.fn()} />);

      await waitFor(() =>
        expect(screen.getByText(/No registries configured/i)).toBeInTheDocument()
      );
      expect(screen.queryByText(/matching ""/)).not.toBeInTheDocument();
    });

    it('says the catalogue is empty when configured sources hold nothing', async () => {
      mockRegistry([{ url: 'https://example.org/index.json', name: 'Example', package_count: 0 }], []);

      render(<RegistryModal isOpen={true} onClose={vi.fn()} />);

      await waitFor(() => expect(screen.getByText(/contain no packages/i)).toBeInTheDocument());
    });

    it('names the query when nothing matches it', async () => {
      mockRegistry(oneSource, []);

      render(<RegistryModal isOpen={true} onClose={vi.fn()} />);

      fireEvent.change(screen.getByPlaceholderText(/Search packages/i), { target: { value: 'zzz' } });

      await waitFor(() => expect(screen.getByText(/No packages match "zzz"/)).toBeInTheDocument());
    });

    it('labels an empty query as all packages', async () => {
      mockRegistry(oneSource, [testWorld]);

      render(<RegistryModal isOpen={true} onClose={vi.fn()} />);

      await waitFor(() => expect(screen.getByText('All packages')).toBeInTheDocument());
    });
  });

  describe('sources panel', () => {
    it('adds a source', async () => {
      mockRegistry([], []);
      const add = vi
        .spyOn(APIClient, 'addRegistrySource')
        .mockResolvedValue({ url: 'https://example.org/index.json', package_count: 0 });

      render(<RegistryModal isOpen={true} onClose={vi.fn()} />);

      fireEvent.click(await screen.findByRole('button', { name: /Sources/i }));
      fireEvent.change(screen.getByPlaceholderText(/registry URL/i), {
        target: { value: 'https://example.org/index.json' },
      });
      fireEvent.click(screen.getByRole('button', { name: /^Add$/ }));

      await waitFor(() => expect(add).toHaveBeenCalledWith('https://example.org/index.json'));
    });

    it('removes a source', async () => {
      mockRegistry(oneSource, []);
      const remove = vi.spyOn(APIClient, 'removeRegistrySource').mockResolvedValue(undefined);

      render(<RegistryModal isOpen={true} onClose={vi.fn()} />);

      fireEvent.click(await screen.findByRole('button', { name: /Sources/i }));
      fireEvent.click(await screen.findByRole('button', { name: /Remove/i }));

      await waitFor(() => expect(remove).toHaveBeenCalledWith('https://example.org/index.json'));
    });
  });
});