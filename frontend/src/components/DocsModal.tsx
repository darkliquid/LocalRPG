import React, { useState, useEffect, useMemo } from 'react';
import {
  X,
  Search,
  BookOpen,
  Folder,
  ChevronRight,
  ArrowLeft,
  ArrowRight,
  Compass,
  Sliders,
  Wrench,
  FileCode,
} from 'lucide-react';
import { APIClient } from '../api/client';
import { DocArticleSummary, DocArticle } from '../types';
import { MarkdownDocViewer } from './MarkdownDocViewer';

interface DocsModalProps {
  isOpen: boolean;
  onClose: () => void;
  initialArticleID?: string;
}

const CATEGORY_ICONS: Record<string, React.FC<{ className?: string }>> = {
  'Core Concepts': Compass,
  'Configuration & Providers': Sliders,
  'Studio Guides': Wrench,
  'Codex & Content Reference': FileCode,
};

export const DocsModal: React.FC<DocsModalProps> = ({
  isOpen,
  onClose,
  initialArticleID,
}) => {
  const [summaries, setSummaries] = useState<DocArticleSummary[]>([]);
  const [selectedArticleID, setSelectedArticleID] = useState<string>('');
  const [currentArticle, setCurrentArticle] = useState<DocArticle | null>(null);
  const [searchQuery, setSearchQuery] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Load article summaries list on open
  useEffect(() => {
    if (!isOpen) return;

    let isMounted = true;
    APIClient.getDocsList()
      .then((list) => {
        if (!isMounted) return;
        setSummaries(list);

        const targetID =
          initialArticleID && list.some((a) => a.id === initialArticleID)
            ? initialArticleID
            : list[0]?.id || '';

        setSelectedArticleID(targetID);
      })
      .catch((err) => {
        if (!isMounted) return;
        console.error('Failed to load docs list:', err);
        setError('Failed to load documentation catalogue.');
      });

    return () => {
      isMounted = false;
    };
  }, [isOpen, initialArticleID]);

  // Load selected article content
  useEffect(() => {
    if (!selectedArticleID || !isOpen) return;

    let isMounted = true;
    setIsLoading(true);
    setError(null);

    APIClient.getDocArticle(selectedArticleID)
      .then((article) => {
        if (!isMounted) return;
        setCurrentArticle(article);
        setIsLoading(false);
      })
      .catch((err) => {
        if (!isMounted) return;
        console.error(`Failed to load doc ${selectedArticleID}:`, err);
        setError('Failed to load documentation article.');
        setIsLoading(false);
      });

    return () => {
      isMounted = false;
    };
  }, [selectedArticleID, isOpen]);

  // Keyboard shortcut: Escape to close
  useEffect(() => {
    if (!isOpen) return;
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, onClose]);

  // Grouped categories
  const categories = useMemo(() => {
    const map = new Map<string, DocArticleSummary[]>();
    for (const doc of summaries) {
      const cat = doc.category || 'General';
      if (!map.has(cat)) {
        map.set(cat, []);
      }
      map.get(cat)!.push(doc);
    }
    return Array.from(map.entries()).map(([name, articles]) => ({
      name,
      articles: articles.sort((a, b) => a.order - b.order),
    }));
  }, [summaries]);

  // Filtered articles when searching
  const filteredArticles = useMemo(() => {
    const q = searchQuery.trim().toLowerCase();
    if (!q) return null;
    return summaries.filter(
      (a) =>
        a.title.toLowerCase().includes(q) ||
        a.description.toLowerCase().includes(q) ||
        a.category.toLowerCase().includes(q) ||
        a.id.toLowerCase().includes(q)
    );
  }, [summaries, searchQuery]);

  // Previous and Next article navigation
  const currentIndex = summaries.findIndex((a) => a.id === selectedArticleID);
  const prevArticle = currentIndex > 0 ? summaries[currentIndex - 1] : null;
  const nextArticle =
    currentIndex >= 0 && currentIndex < summaries.length - 1
      ? summaries[currentIndex + 1]
      : null;

  if (!isOpen) return null;

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label="Documentation & Reference"
      className="fixed inset-0 z-50 flex items-center justify-center p-3 sm:p-6 bg-black/80 backdrop-blur-md anim-fade-in"
    >
      <div className="relative w-full max-w-6xl h-[90vh] bg-stone-900/95 border border-white/15 rounded-3xl overflow-hidden shadow-2xl flex flex-col anim-scale-in">
        {/* Top Header */}
        <div className="p-4 px-6 border-b border-white/10 flex items-center justify-between bg-stone-950/70 gap-4">
          <div className="flex items-center gap-3">
            <div className="w-9 h-9 rounded-xl bg-purple-950/60 border border-purple-500/30 flex items-center justify-center text-purple-400 shadow-md">
              <BookOpen className="w-5 h-5" />
            </div>
            <div>
              <h2 className="text-base font-sans font-bold text-white tracking-tight">
                Documentation & Reference
              </h2>
              <p className="text-xs text-stone-400 hidden sm:block">
                Guides, architecture, provider setup, and codex schemas
              </p>
            </div>
          </div>

          {/* Quick Search */}
          <div className="relative flex-1 max-w-md mx-4">
            <Search className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-stone-400 pointer-events-none" />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Search guides, rules, syntax..."
              className="w-full bg-white/[0.05] border border-white/10 rounded-xl pl-9 pr-8 py-1.5 text-xs sm:text-sm text-stone-200 placeholder-stone-500 focus:outline-none focus:border-purple-500/50 focus:ring-1 focus:ring-purple-500/30 transition-all"
            />
            {searchQuery && (
              <button
                onClick={() => setSearchQuery('')}
                className="absolute right-2.5 top-1/2 -translate-y-1/2 text-stone-400 hover:text-white text-xs cursor-pointer"
              >
                ✕
              </button>
            )}
          </div>

          {/* Close button */}
          <button
            onClick={onClose}
            className="w-8 h-8 rounded-full bg-white/[0.05] hover:bg-white/[0.1] border border-white/10 flex items-center justify-center text-stone-400 hover:text-white transition-all cursor-pointer"
            title="Close (Escape)"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* Two-Pane Body */}
        <div className="flex-1 flex min-h-0 overflow-hidden">
          {/* Left Navigation Sidebar */}
          <aside className="w-64 sm:w-80 border-r border-white/10 bg-stone-950/40 flex flex-col min-h-0">
            <div className="flex-1 overflow-y-auto p-3 space-y-4">
              {filteredArticles ? (
                // Search Results View
                <div className="space-y-1">
                  <div className="text-[11px] font-sans font-semibold uppercase tracking-wider text-purple-400 px-3 py-1">
                    Search Results ({filteredArticles.length})
                  </div>
                  {filteredArticles.length === 0 ? (
                    <div className="text-xs text-stone-500 px-3 py-4 text-center">
                      No matching articles found.
                    </div>
                  ) : (
                    filteredArticles.map((article) => {
                      const isSelected = article.id === selectedArticleID;
                      return (
                        <button
                          key={article.id}
                          onClick={() => {
                            setSelectedArticleID(article.id);
                          }}
                          className={`w-full text-left px-3 py-2 rounded-xl transition-all flex flex-col gap-0.5 cursor-pointer ${
                            isSelected
                              ? 'bg-purple-600/30 border border-purple-500/50 text-white shadow-sm'
                              : 'text-stone-300 hover:bg-white/[0.05] hover:text-white'
                          }`}
                        >
                          <div className="text-xs font-semibold">{article.title}</div>
                          <div className="text-[11px] text-stone-400 line-clamp-1">
                            {article.description}
                          </div>
                        </button>
                      );
                    })
                  )}
                </div>
              ) : (
                // Categorized Tree View
                categories.map((category) => {
                  const Icon = CATEGORY_ICONS[category.name] || Folder;
                  return (
                    <div key={category.name} className="space-y-1">
                      <div className="flex items-center gap-2 px-3 py-1 text-[11px] font-sans font-semibold uppercase tracking-wider text-stone-400">
                        <Icon className="w-3.5 h-3.5 text-purple-400" />
                        <span>{category.name}</span>
                        <span className="ml-auto text-[10px] text-stone-500">
                          {category.articles.length}
                        </span>
                      </div>
                      <div className="space-y-0.5">
                        {category.articles.map((article) => {
                          const isSelected = article.id === selectedArticleID;
                          return (
                            <button
                              key={article.id}
                              onClick={() => setSelectedArticleID(article.id)}
                              className={`w-full text-left px-3 py-2 rounded-xl transition-all flex items-center justify-between gap-2 cursor-pointer ${
                                isSelected
                                  ? 'bg-purple-600/30 border border-purple-500/50 text-white font-medium shadow-sm'
                                  : 'text-stone-400 hover:bg-white/[0.04] hover:text-stone-200'
                              }`}
                            >
                              <span className="text-xs truncate">{article.title}</span>
                              {isSelected && (
                                <ChevronRight className="w-3.5 h-3.5 text-purple-400 shrink-0" />
                              )}
                            </button>
                          );
                        })}
                      </div>
                    </div>
                  );
                })
              )}
            </div>
          </aside>

          {/* Right Reader Content */}
          <main className="flex-1 flex flex-col min-h-0 bg-stone-900/60 overflow-hidden">
            {/* Breadcrumb Navigation */}
            {currentArticle && (
              <div className="px-6 py-2.5 border-b border-white/10 bg-stone-950/30 flex items-center gap-2 text-xs text-stone-400 shrink-0">
                <span className="hover:text-stone-200">Documentation</span>
                <ChevronRight className="w-3 h-3 text-stone-600" />
                <span className="text-purple-400 font-medium">
                  {currentArticle.category}
                </span>
                <ChevronRight className="w-3 h-3 text-stone-600" />
                <span className="text-stone-200 truncate">{currentArticle.title}</span>
              </div>
            )}

            {/* Scrollable Document Body */}
            <div className="flex-1 overflow-y-auto p-6 sm:p-8 space-y-6 select-text">
              {isLoading ? (
                <div className="flex items-center justify-center h-48 text-stone-400 text-sm">
                  Loading documentation...
                </div>
              ) : error ? (
                <div className="p-4 rounded-xl border border-red-500/30 bg-red-950/20 text-red-300 text-sm">
                  {error}
                </div>
              ) : currentArticle ? (
                <>
                  <div className="mb-6">
                    <p className="text-sm text-stone-400 mt-1 italic">
                      {currentArticle.description}
                    </p>
                  </div>
                  <MarkdownDocViewer
                    content={currentArticle.content}
                    onNavigate={(articleID) => setSelectedArticleID(articleID)}
                  />
                </>
              ) : (
                <div className="text-stone-500 text-center py-12">
                  Select an article from the sidebar to begin reading.
                </div>
              )}

              {/* Prev / Next Article Footer */}
              {!isLoading && currentArticle && (prevArticle || nextArticle) && (
                <div className="pt-8 mt-8 border-t border-white/10 flex items-center justify-between gap-4">
                  {prevArticle ? (
                    <button
                      onClick={() => setSelectedArticleID(prevArticle.id)}
                      className="flex items-center gap-2 px-4 py-2.5 rounded-xl border border-white/10 bg-white/[0.04] hover:bg-white/[0.08] text-stone-300 hover:text-white text-xs transition-all cursor-pointer"
                    >
                      <ArrowLeft className="w-3.5 h-3.5 text-purple-400" />
                      <div className="text-left">
                        <div className="text-[10px] text-stone-500 uppercase font-semibold">
                          Previous
                        </div>
                        <div className="font-medium text-stone-200 truncate max-w-[180px]">
                          {prevArticle.title}
                        </div>
                      </div>
                    </button>
                  ) : (
                    <div />
                  )}

                  {nextArticle && (
                    <button
                      onClick={() => setSelectedArticleID(nextArticle.id)}
                      className="flex items-center gap-2 px-4 py-2.5 rounded-xl border border-white/10 bg-white/[0.04] hover:bg-white/[0.08] text-stone-300 hover:text-white text-xs transition-all cursor-pointer ml-auto"
                    >
                      <div className="text-right">
                        <div className="text-[10px] text-stone-500 uppercase font-semibold">
                          Next
                        </div>
                        <div className="font-medium text-stone-200 truncate max-w-[180px]">
                          {nextArticle.title}
                        </div>
                      </div>
                      <ArrowRight className="w-3.5 h-3.5 text-purple-400" />
                    </button>
                  )}
                </div>
              )}
            </div>
          </main>
        </div>
      </div>
    </div>
  );
};
