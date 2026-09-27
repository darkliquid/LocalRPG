import React, { memo, useState } from 'react';
import { Copy, Check, Info, Lightbulb, AlertTriangle, AlertCircle } from 'lucide-react';

interface MarkdownDocViewerProps {
  content: string;
  className?: string;
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

const inlinePattern = /(`[^`]+`)|(\*\*[^*]+\*\*)|(\*[^*]+\*)|(_[^_]+_)|(\[[^\]]+\]\([^)]+\))/g;

const renderInlineProse = (text: string): React.ReactNode[] => {
  const nodes: React.ReactNode[] = [];
  let lastIndex = 0;
  let key = 0;
  let match: RegExpExecArray | null;

  inlinePattern.lastIndex = 0;
  while ((match = inlinePattern.exec(text)) !== null) {
    if (match.index > lastIndex) {
      nodes.push(text.slice(lastIndex, match.index));
    }

    const token = match[0];
    if (token.startsWith('`')) {
      nodes.push(
        <code key={key++} className="px-1.5 py-0.5 rounded bg-white/[0.08] border border-white/10 font-mono text-[0.88em] text-purple-300">
          {token.slice(1, -1)}
        </code>
      );
    } else if (token.startsWith('**')) {
      nodes.push(
        <strong key={key++} className="font-semibold text-white">
          {token.slice(2, -2)}
        </strong>
      );
    } else if (token.startsWith('*') || token.startsWith('_')) {
      nodes.push(
        <em key={key++} className="italic text-stone-300">
          {token.slice(1, -1)}
        </em>
      );
    } else if (token.startsWith('[')) {
      const linkMatch = token.match(/\[([^\]]+)\]\(([^)]+)\)/);
      if (linkMatch) {
        nodes.push(
          <a
            key={key++}
            href={linkMatch[2]}
            target="_blank"
            rel="noopener noreferrer"
            className="text-purple-400 hover:text-purple-300 underline font-medium"
          >
            {linkMatch[1]}
          </a>
        );
      } else {
        nodes.push(token);
      }
    }

    lastIndex = match.index + token.length;
  }

  if (lastIndex < text.length) {
    nodes.push(text.slice(lastIndex));
  }
  return nodes;
};

export const MarkdownDocViewer: React.FC<MarkdownDocViewerProps> = memo(({ content, className = '' }) => {
  const normalized = (content ?? '').replace(/\r\n/g, '\n');
  if (!normalized.trim()) return null;

  // Split content by fenced code blocks first
  const parts = normalized.split(/(```[\s\S]*?```)/g);

  return (
    <div className={`space-y-4 text-stone-300 font-sans leading-relaxed text-sm sm:text-base ${className}`}>
      {parts.map((part, partIndex) => {
        if (part.startsWith('```') && part.endsWith('```')) {
          const firstLineEnd = part.indexOf('\n');
          const language = part.slice(3, firstLineEnd).trim();
          const code = part.slice(firstLineEnd + 1, -3);
          return <CodeBlock key={partIndex} code={code} language={language} />;
        }

        // Process standard markdown blocks
        const blocks = part.split(/\n{2,}/);
        return blocks.map((block, blockIndex) => {
          const trimmed = block.trim();
          if (!trimmed) return null;

          // Headings
          if (trimmed.startsWith('# ')) {
            return (
              <h1 key={`${partIndex}-${blockIndex}`} className="text-2xl sm:text-3xl font-bold text-white tracking-tight pt-4 pb-2 border-b border-white/10">
                {renderInlineProse(trimmed.slice(2))}
              </h1>
            );
          }
          if (trimmed.startsWith('## ')) {
            return (
              <h2 key={`${partIndex}-${blockIndex}`} className="text-xl sm:text-2xl font-bold text-purple-300 tracking-tight pt-4 pb-1">
                {renderInlineProse(trimmed.slice(3))}
              </h2>
            );
          }
          if (trimmed.startsWith('### ')) {
            return (
              <h3 key={`${partIndex}-${blockIndex}`} className="text-lg font-semibold text-stone-100 tracking-tight pt-2">
                {renderInlineProse(trimmed.slice(4))}
              </h3>
            );
          }
          if (trimmed.startsWith('#### ')) {
            return (
              <h4 key={`${partIndex}-${blockIndex}`} className="text-base font-semibold text-purple-200/90 pt-1">
                {renderInlineProse(trimmed.slice(5))}
              </h4>
            );
          }

          // Horizontal rule
          if (/^(-{3,}|\*{3,}|_{3,})$/.test(trimmed)) {
            return <hr key={`${partIndex}-${blockIndex}`} className="my-6 border-white/10" />;
          }

          // Callouts: > [!NOTE], > [!TIP], > [!IMPORTANT], > [!WARNING]
          if (trimmed.startsWith('>')) {
            const lines = trimmed.split('\n').map((l) => l.replace(/^>\s?/, ''));
            const header = lines[0]?.trim() || '';

            let calloutType: 'note' | 'tip' | 'important' | 'warning' | null = null;
            let title = '';
            let bodyLines = lines;

            if (header.startsWith('[!NOTE]')) {
              calloutType = 'note';
              title = 'Note';
              bodyLines = lines.slice(1);
            } else if (header.startsWith('[!TIP]')) {
              calloutType = 'tip';
              title = 'Tip';
              bodyLines = lines.slice(1);
            } else if (header.startsWith('[!IMPORTANT]')) {
              calloutType = 'important';
              title = 'Important';
              bodyLines = lines.slice(1);
            } else if (header.startsWith('[!WARNING]')) {
              calloutType = 'warning';
              title = 'Warning';
              bodyLines = lines.slice(1);
            }

            if (calloutType) {
              const styles = {
                note: { border: 'border-blue-500/40', bg: 'bg-blue-950/20', text: 'text-blue-300', icon: Info },
                tip: { border: 'border-emerald-500/40', bg: 'bg-emerald-950/20', text: 'text-emerald-300', icon: Lightbulb },
                important: { border: 'border-purple-500/40', bg: 'bg-purple-950/20', text: 'text-purple-300', icon: AlertCircle },
                warning: { border: 'border-amber-500/40', bg: 'bg-amber-950/20', text: 'text-amber-300', icon: AlertTriangle },
              }[calloutType];
              const IconComponent = styles.icon;

              return (
                <div key={`${partIndex}-${blockIndex}`} className={`my-4 p-4 rounded-xl border ${styles.border} ${styles.bg}`}>
                  <div className={`flex items-center gap-2 font-semibold text-sm ${styles.text} mb-1.5`}>
                    <IconComponent className="w-4 h-4" />
                    <span>{title}</span>
                  </div>
                  <div className="text-sm text-stone-300 space-y-1">
                    {bodyLines.map((line, li) => (
                      <p key={li}>{renderInlineProse(line)}</p>
                    ))}
                  </div>
                </div>
              );
            }

            // Standard blockquote
            return (
              <blockquote key={`${partIndex}-${blockIndex}`} className="border-l-2 border-purple-500/50 pl-4 py-1 italic text-stone-300">
                {lines.map((l, li) => (
                  <p key={li}>{renderInlineProse(l)}</p>
                ))}
              </blockquote>
            );
          }

          // Tables
          const lines = trimmed.split('\n');
          if (lines.length >= 2 && lines[0].includes('|') && lines[1].includes('|') && lines[1].includes('-')) {
            const headerCells = lines[0].split('|').map((c) => c.trim()).filter(Boolean);
            const rowLines = lines.slice(2);

            return (
              <div key={`${partIndex}-${blockIndex}`} className="my-4 overflow-x-auto rounded-xl border border-white/10 bg-black/40">
                <table className="w-full text-left border-collapse text-xs sm:text-sm">
                  <thead>
                    <tr className="border-b border-white/10 bg-white/[0.04]">
                      {headerCells.map((cell, ci) => (
                        <th key={ci} className="py-2.5 px-4 font-semibold text-stone-200">
                          {renderInlineProse(cell)}
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {rowLines.map((row, ri) => {
                      const cells = row.split('|').map((c) => c.trim()).filter(Boolean);
                      return (
                        <tr key={ri} className="border-b border-white/5 hover:bg-white/[0.02]">
                          {cells.map((cell, ci) => (
                            <td key={ci} className="py-2.5 px-4 text-stone-300">
                              {renderInlineProse(cell)}
                            </td>
                          ))}
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            );
          }

          // Bulleted list
          if (lines.every((line) => /^\s*[-*]\s+/.test(line))) {
            return (
              <ul key={`${partIndex}-${blockIndex}`} className="list-disc list-outside ml-6 space-y-1 text-stone-300">
                {lines.map((line, li) => (
                  <li key={li}>{renderInlineProse(line.replace(/^\s*[-*]\s+/, ''))}</li>
                ))}
              </ul>
            );
          }

          // Numbered list
          if (lines.every((line) => /^\s*\d+\.\s+/.test(line))) {
            return (
              <ol key={`${partIndex}-${blockIndex}`} className="list-decimal list-outside ml-6 space-y-1 text-stone-300">
                {lines.map((line, li) => (
                  <li key={li}>{renderInlineProse(line.replace(/^\s*\d+\.\s+/, ''))}</li>
                ))}
              </ol>
            );
          }

          // Regular paragraph
          return (
            <p key={`${partIndex}-${blockIndex}`} className="leading-relaxed">
              {renderInlineProse(trimmed)}
            </p>
          );
        });
      })}
    </div>
  );
});

MarkdownDocViewer.displayName = 'MarkdownDocViewer';
