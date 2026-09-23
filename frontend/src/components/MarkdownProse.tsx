import React, { memo } from 'react';

interface MarkdownProseProps {
  text: string;
  onEntityClick?: (entityId: string) => void;
  className?: string;
  displayMode?: 'stage_directions' | 'hidden' | 'raw';
}

// The inline grammar is deliberately small: emphasis, code, entity links, and optional
// vocal performance tags. It is applied to the same string whether it is arriving chunk
// by chunk or read back from the chronicle, so streamed prose and replayed prose format identically.
const inlinePattern = /(\[\[[^\]]+\]\])|(\[[a-zA-Z][a-zA-Z\s_-]{1,28}\])|(`[^`]+`)|(\*\*[^*]+\*\*)|(\*[^*]+\*)|(_[^_]+_)/g;

const renderInline = (
  text: string,
  onEntityClick?: (entityId: string) => void,
  displayMode: 'stage_directions' | 'hidden' | 'raw' = 'stage_directions'
): React.ReactNode[] => {
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
    if (token.startsWith('[[')) {
      const inner = token.slice(2, -2);
      const separator = inner.indexOf('|');
      const target = (separator === -1 ? inner : inner.slice(0, separator)).trim();
      const label = (separator === -1 ? inner : inner.slice(separator + 1)).trim() || target;
      nodes.push(
        onEntityClick ? (
          <button
            key={key++}
            onClick={() => onEntityClick(target)}
            className="text-amber-400 hover:text-amber-300 underline font-medium cursor-pointer transition-colors"
          >
            {label}
          </button>
        ) : (
          <span key={key++} className="text-amber-300/90">
            {label}
          </span>
        )
      );
    } else if (token.startsWith('[')) {
      if (displayMode === 'hidden') {
        // Stripped from visual display
      } else if (displayMode === 'raw') {
        nodes.push(token);
      } else {
        nodes.push(
          <span
            key={key++}
            className="inline-flex items-center text-[0.76em] font-sans font-semibold uppercase tracking-wider text-amber-400/90 bg-amber-950/40 border border-amber-700/40 px-1.5 py-0.2 rounded-md mx-1 select-none not-italic align-baseline"
            title="Performance direction"
          >
            {token.slice(1, -1)}
          </span>
        );
      }
    } else if (token.startsWith('`')) {
      nodes.push(
        <code key={key++} className="px-1 py-0.5 rounded bg-black/40 font-mono text-[0.9em] text-stone-200">
          {token.slice(1, -1)}
        </code>
      );
    } else if (token.startsWith('**')) {
      nodes.push(
        <strong key={key++} className="font-semibold text-stone-100">
          {token.slice(2, -2)}
        </strong>
      );
    } else {
      nodes.push(
        <em key={key++} className="italic">
          {token.slice(1, -1)}
        </em>
      );
    }

    lastIndex = match.index + token.length;
  }

  if (lastIndex < text.length) {
    nodes.push(text.slice(lastIndex));
  }
  return nodes;
};

const sceneBreakPattern = /^(-{3,}|\*{3,}|_{3,})$/;
const headingPattern = /^(#{1,6})\s+(.*)$/;
const listItemPattern = /^\s*[-*]\s+/;

// MarkdownProse renders the constrained Markdown a narrator actually produces.
// Blank lines separate paragraphs, single newlines are kept as deliberate beats,
// and raw HTML is never trusted: it is escaped by virtue of being plain text.
export const MarkdownProse: React.FC<MarkdownProseProps> = memo(({ text, onEntityClick, className, displayMode = 'stage_directions' }) => {
  const raw = (text ?? '').replace(/\r\n/g, '\n');
  const normalized = displayMode === 'hidden'
    ? raw.replace(/(?:^|\s)\[[a-zA-Z][a-zA-Z\s_-]{1,28}\](?:\s|$)/g, ' ').trim()
    : raw;

  if (normalized.trim() === '') {
    return null;
  }

  const blocks = normalized.split(/\n{2,}/);

  return (
    <div className={className}>
      {blocks.map((block, index) => {
        const trimmed = block.trim();

        if (sceneBreakPattern.test(trimmed)) {
          return <hr key={index} className="my-4 border-white/10" />;
        }

        const heading = trimmed.match(headingPattern);
        if (heading) {
          const level = heading[1].length;
          const content = renderInline(heading[2], onEntityClick, displayMode);
          if (level <= 2) {
            return (
              <h2 key={index} className="font-cinzel text-lg font-bold text-amber-300 tracking-wide mt-2">
                {content}
              </h2>
            );
          }
          return (
            <h3 key={index} className="font-cinzel text-base font-semibold text-amber-200/90 tracking-wide mt-2">
              {content}
            </h3>
          );
        }

        const lines = block.split('\n');
        if (lines.every((line) => listItemPattern.test(line))) {
          return (
            <ul key={index} className="list-disc list-inside space-y-1">
              {lines.map((line, itemIndex) => (
                <li key={itemIndex}>{renderInline(line.replace(listItemPattern, ''), onEntityClick, displayMode)}</li>
              ))}
            </ul>
          );
        }

        if (trimmed.startsWith('>')) {
          return (
            <blockquote
              key={index}
              className="border-l-2 border-amber-500/60 pl-3 my-1 text-stone-300 italic whitespace-pre-wrap"
            >
              {renderInline(lines.map((line) => line.replace(/^>\s?/, '')).join('\n'), onEntityClick, displayMode)}
            </blockquote>
          );
        }

        return (
          <p key={index} className="whitespace-pre-wrap">
            {renderInline(block, onEntityClick, displayMode)}
          </p>
        );
      })}
    </div>
  );
});

MarkdownProse.displayName = 'MarkdownProse';
