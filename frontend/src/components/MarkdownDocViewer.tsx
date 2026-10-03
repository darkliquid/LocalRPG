import React, { memo, useState } from 'react';
import ReactMarkdown, { type Components } from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { remarkAlert } from 'remark-github-blockquote-alert';
import { Copy, Check } from 'lucide-react';

interface MarkdownDocViewerProps {
  content: string;
  className?: string;
  onNavigate?: (articleID: string) => void;
}

interface CodeBlockProps {
  code: string;
  language?: string;
}

const CodeBlock: React.FC<CodeBlockProps> = ({ code, language }) => {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(code);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error('Failed to copy code:', err);
    }
  };

  return (
    <div className="my-4 rounded-xl overflow-hidden border border-white/10 bg-black/60 shadow-lg">
      <div className="flex items-center justify-between px-4 py-1.5 bg-white/[0.04] border-b border-white/10 text-xs font-mono text-stone-400">
        <span className="uppercase tracking-wider font-semibold">{language || 'text'}</span>
        <button
          onClick={handleCopy}
          className="flex items-center gap-1.5 px-2 py-1 rounded-md text-stone-300 hover:text-white hover:bg-white/10 transition-all cursor-pointer"
          title="Copy code to clipboard"
        >
          {copied ? (
            <>
              <Check className="w-3.5 h-3.5 text-emerald-400" />
              <span className="text-emerald-400 font-sans">Copied!</span>
            </>
          ) : (
            <>
              <Copy className="w-3.5 h-3.5" />
              <span className="font-sans">Copy</span>
            </>
          )}
        </button>
      </div>
      <pre className="p-4 overflow-x-auto text-xs sm:text-sm font-mono text-stone-200 leading-relaxed selection:bg-purple-500/30">
        <code>{code}</code>
      </pre>
    </div>
  );
};

// GitHub alert styling, keyed by the `markdown-alert-<type>` class the plugin
// emits. The descendant variant recolours the generated title.
const ALERT_STYLES: Record<string, string> = {
  note: 'border-blue-500/40 bg-blue-950/20 [&_.markdown-alert-title]:text-blue-300',
  tip: 'border-emerald-500/40 bg-emerald-950/20 [&_.markdown-alert-title]:text-emerald-300',
  important: 'border-purple-500/40 bg-purple-950/20 [&_.markdown-alert-title]:text-purple-300',
  warning: 'border-amber-500/40 bg-amber-950/20 [&_.markdown-alert-title]:text-amber-300',
  caution: 'border-red-500/40 bg-red-950/20 [&_.markdown-alert-title]:text-red-300',
};

const baseComponents: Components = {
  h1: ({ children }) => (
    <h1 className="text-2xl sm:text-3xl font-bold text-white tracking-tight pt-4 pb-2 border-b border-white/10">
      {children}
    </h1>
  ),
  h2: ({ children }) => (
    <h2 className="text-xl sm:text-2xl font-bold text-purple-300 tracking-tight pt-4 pb-1">{children}</h2>
  ),
  h3: ({ children }) => (
    <h3 className="text-lg font-semibold text-stone-100 tracking-tight pt-2">{children}</h3>
  ),
  h4: ({ children }) => (
    <h4 className="text-base font-semibold text-purple-200/90 pt-1">{children}</h4>
  ),
  p: ({ className, children, ...props }) => {
    const isAlertTitle = className?.includes('markdown-alert-title') ?? false;
    return (
      <p
        className={isAlertTitle ? `${className} flex items-center gap-2 font-semibold text-sm mb-1.5` : 'leading-relaxed'}
        {...props}
      >
        {children}
      </p>
    );
  },
  ul: ({ children }) => (
    <ul className="list-disc list-outside ml-6 space-y-1 text-stone-300 marker:text-stone-500">{children}</ul>
  ),
  ol: ({ children }) => (
    <ol className="list-decimal list-outside ml-6 space-y-1 text-stone-300 marker:text-stone-500">{children}</ol>
  ),
  li: ({ children }) => <li className="leading-relaxed">{children}</li>,
  strong: ({ children }) => <strong className="font-semibold text-white">{children}</strong>,
  em: ({ children }) => <em className="italic text-stone-300">{children}</em>,
  hr: () => <hr className="my-6 border-white/10" />,
  blockquote: ({ children }) => (
    <blockquote className="border-l-2 border-purple-500/50 pl-4 py-1 italic text-stone-300">{children}</blockquote>
  ),
  div: ({ className, children, ...props }) => {
    if (className?.includes('markdown-alert')) {
      const type = Object.keys(ALERT_STYLES).find((t) => className.includes(`markdown-alert-${t}`)) ?? 'note';
      return (
        <div
          className={`my-4 p-4 rounded-xl border text-sm text-stone-300 [&_svg]:w-4 [&_svg]:h-4 [&_svg]:shrink-0 ${ALERT_STYLES[type]} ${className}`}
          {...props}
        >
          {children}
        </div>
      );
    }
    return (
      <div className={className} {...props}>
        {children}
      </div>
    );
  },
  pre: ({ children }) => <>{children}</>,
  code: ({ className, children }) => {
    const text = String(children ?? '');
    const isBlock = Boolean(className) || text.includes('\n');
    if (!isBlock) {
      return (
        <code className="px-1.5 py-0.5 rounded bg-white/[0.08] border border-white/10 font-mono text-[0.88em] text-purple-300">
          {children}
        </code>
      );
    }
    const language = /language-(\w+)/.exec(className ?? '')?.[1];
    return <CodeBlock code={text.replace(/\n$/, '')} language={language} />;
  },
  table: ({ children }) => (
    <div className="my-4 overflow-x-auto rounded-xl border border-white/10 bg-black/40">
      <table className="w-full text-left border-collapse text-xs sm:text-sm">{children}</table>
    </div>
  ),
  thead: ({ children }) => <thead className="border-b border-white/10 bg-white/[0.04]">{children}</thead>,
  tr: ({ children }) => <tr className="border-b border-white/5 hover:bg-white/[0.02]">{children}</tr>,
  th: ({ children }) => <th className="py-2.5 px-4 font-semibold text-stone-200">{children}</th>,
  td: ({ children }) => <td className="py-2.5 px-4 text-stone-300">{children}</td>,
};

export const MarkdownDocViewer: React.FC<MarkdownDocViewerProps> = memo(({ content, className = '', onNavigate }) => {
  if (!content?.trim()) return null;

  // A link with no scheme is a cross-reference to another embedded article,
  // so it switches the open document instead of opening a new tab.
  const isInternal = (href: string): boolean => !/^[a-z][a-z0-9+.-]*:/i.test(href) && !href.startsWith('#');

  const markdownComponents: Components = {
    ...baseComponents,
    a: ({ href, children }) => {
      const target = href ?? '';
      if (isInternal(target) && onNavigate) {
        return (
          <a
            href={target}
            onClick={(event) => {
              event.preventDefault();
              onNavigate(target.replace(/\.md$/, ''));
            }}
            className="text-purple-400 hover:text-purple-300 underline font-medium cursor-pointer"
          >
            {children}
          </a>
        );
      }
      return (
        <a
          href={href}
          target="_blank"
          rel="noopener noreferrer"
          className="text-purple-400 hover:text-purple-300 underline font-medium"
        >
          {children}
        </a>
      );
    },
  };

  return (
    <div className={`space-y-4 text-stone-300 font-sans leading-relaxed text-sm sm:text-base select-text ${className}`}>
      <ReactMarkdown remarkPlugins={[remarkGfm, remarkAlert]} components={markdownComponents}>
        {content}
      </ReactMarkdown>
    </div>
  );
});

MarkdownDocViewer.displayName = 'MarkdownDocViewer';
