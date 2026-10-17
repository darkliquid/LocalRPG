import { render, screen, fireEvent, waitFor, within } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { DocsModal } from './DocsModal';
import { APIClient } from '../api/client';
import { FailuresProvider } from '../context/FailuresContext';
import type { DocArticleSummary, DocArticle } from '../types';

const mockSummaries: DocArticleSummary[] = [
  {
    id: '01-getting-started',
    title: 'Getting Started',
    description: 'Learn the basic concepts of LocalRPG',
    category: 'Core Concepts',
    order: 1,
  },
  {
    id: '02-configuration',
    title: 'Configuration Guide',
    description: 'Set up local and cloud AI providers',
    category: 'Configuration & Providers',
    order: 2,
  },
];

const mockArticle1: DocArticle = {
  id: '01-getting-started',
  title: 'Getting Started',
  category: 'Core Concepts',
  description: 'Learn the basic concepts of LocalRPG',
  content: '# Welcome to LocalRPG\n\nThis is the beginner guide.',
  order: 1,
};

const mockArticle2: DocArticle = {
  id: '02-configuration',
  title: 'Configuration Guide',
  category: 'Configuration & Providers',
  description: 'Set up local and cloud AI providers',
  content: '# Configuration Guide\n\nHow to configure your models.',
  order: 2,
};

describe('DocsModal', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.spyOn(APIClient, 'getDocsList').mockResolvedValue(mockSummaries);
    vi.spyOn(APIClient, 'getDocArticle').mockImplementation(async (id: string) => {
      if (id === '02-configuration') return mockArticle2;
      return mockArticle1;
    });
  });

  it('does not render when isOpen is false', () => {
    render(
      <FailuresProvider>
        <DocsModal isOpen={false} onClose={vi.fn()} />
      </FailuresProvider>,
    );

    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('renders dialog and loads article summaries and first article', async () => {
    render(
      <FailuresProvider>
        <DocsModal isOpen={true} onClose={vi.fn()} />
      </FailuresProvider>,
    );

    expect(screen.getByRole('dialog', { name: 'Documentation & Reference' })).toBeInTheDocument();

    await waitFor(() => {
      expect(screen.getByText('This is the beginner guide.')).toBeInTheDocument();
    });

    expect(screen.getAllByText('Getting Started').length).toBeGreaterThan(0);
    expect(screen.getAllByText('Configuration Guide').length).toBeGreaterThan(0);
  });

  it('loads specific article when initialArticleID is passed', async () => {
    render(
      <FailuresProvider>
        <DocsModal isOpen={true} onClose={vi.fn()} initialArticleID="02-configuration" />
      </FailuresProvider>,
    );

    await waitFor(() => {
      expect(screen.getByText('How to configure your models.')).toBeInTheDocument();
    });
  });

  it('switches articles when an article summary in sidebar is clicked', async () => {
    render(
      <FailuresProvider>
        <DocsModal isOpen={true} onClose={vi.fn()} />
      </FailuresProvider>,
    );

    await waitFor(() => {
      expect(screen.getByText('This is the beginner guide.')).toBeInTheDocument();
    });

    const aside = document.querySelector('aside')!;
    const configBtn = within(aside).getByRole('button', { name: 'Configuration Guide' });
    fireEvent.click(configBtn);

    await waitFor(() => {
      expect(screen.getByText('How to configure your models.')).toBeInTheDocument();
    });
  });

  it('filters articles using search input', async () => {
    render(
      <FailuresProvider>
        <DocsModal isOpen={true} onClose={vi.fn()} />
      </FailuresProvider>,
    );

    await waitFor(() => {
      expect(screen.getByText('Getting Started')).toBeInTheDocument();
    });

    const searchInput = screen.getByPlaceholderText('Search guides, rules, syntax...');
    fireEvent.change(searchInput, { target: { value: 'models' } });

    expect(screen.queryByRole('button', { name: /Getting Started/ })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Configuration Guide/ })).toBeInTheDocument();
  });

  it('displays error message when docs list fails to load', async () => {
    vi.spyOn(APIClient, 'getDocsList').mockRejectedValue(new Error('Network error'));

    render(
      <FailuresProvider>
        <DocsModal isOpen={true} onClose={vi.fn()} />
      </FailuresProvider>,
    );

    await waitFor(() => {
      expect(screen.getByText('Failed to load documentation catalogue.')).toBeInTheDocument();
    });
  });

  it('calls onClose when close button is clicked', async () => {
    const onClose = vi.fn();
    render(
      <FailuresProvider>
        <DocsModal isOpen={true} onClose={onClose} />
      </FailuresProvider>,
    );

    const closeBtn = screen.getByRole('button', { name: 'Close' });
    fireEvent.click(closeBtn);
    expect(onClose).toHaveBeenCalled();
  });
});
