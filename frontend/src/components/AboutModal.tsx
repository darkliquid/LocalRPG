import React from 'react';
import { BookOpen, Bug, Github, Sparkles, X } from 'lucide-react';
import { useMountTransition } from '../hooks/useMountTransition';
import {
  PROJECT_ISSUES_URL,
  PROJECT_NAME,
  PROJECT_REPO_URL,
  PROJECT_TAGLINE,
  openExternal,
} from '../lib/project';

interface AboutModalProps {
  isOpen: boolean;
  onClose: () => void;
  onOpenDocs: () => void;
  version?: string;
}

const HIGHLIGHTS = [
  'Runs entirely on your machine, with no account and no cloud round-trip.',
  'Pluggable LLM, speech, and image providers, including offline built-ins.',
  'Schema-agnostic rules: systems, worlds, and campaigns are just Markdown and YAML.',
];

export const AboutModal: React.FC<AboutModalProps> = ({ isOpen, onClose, onOpenDocs, version }) => {
  const { mounted, state } = useMountTransition(isOpen, 200);

  if (!mounted) return null;

  return (
    <div
      data-state={state}
      className={`fixed inset-0 z-[80] flex items-center justify-center bg-black/60 backdrop-blur-xs p-4 ${
        state === 'enter' ? 'anim-fade-in' : 'anim-fade-out pointer-events-none'
      }`}
      style={{ '--anim-dur': '200ms' } as React.CSSProperties}
      role="dialog"
      aria-modal="true"
      aria-label={`About ${PROJECT_NAME}`}
    >
      <div
        className={`w-full max-w-md bg-stone-900 border border-stone-800 rounded-xl shadow-2xl p-6 text-stone-200 ${
          state === 'enter' ? 'anim-scale-in' : 'anim-scale-out'
        }`}
      >
        <div className="flex items-start justify-between mb-4">
          <div className="flex items-center gap-3">
            <div className="p-2 bg-purple-500/10 text-purple-400 rounded-lg">
              <Sparkles className="w-6 h-6" />
            </div>
            <div>
              <h3 className="text-lg font-semibold text-stone-100 font-sans leading-tight">{PROJECT_NAME}</h3>
              <p className="text-xs text-stone-400 font-sans">{PROJECT_TAGLINE}</p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="text-stone-400 hover:text-stone-200 p-1 transition cursor-pointer"
            aria-label="Close"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        <p className="text-sm text-stone-400 leading-relaxed font-serif mb-4">
          A local-first, turn-based tabletop RPG client. The engine keeps no rules of its own: every
          system, world, and campaign is content you can read, edit, and share.
        </p>

        <ul className="space-y-2 mb-5">
          {HIGHLIGHTS.map((line) => (
            <li key={line} className="flex items-start gap-2 text-xs text-stone-400 leading-relaxed">
              <span className="mt-1.5 w-1 h-1 rounded-full bg-purple-500 shrink-0" />
              <span>{line}</span>
            </li>
          ))}
        </ul>

        <div className="flex items-center gap-2 mb-5">
          <button
            onClick={() => {
              onClose();
              onOpenDocs();
            }}
            className="flex-1 py-2 px-3 bg-purple-600 hover:bg-purple-500 text-white font-semibold text-sm rounded-lg flex items-center justify-center gap-2 transition shadow-md cursor-pointer"
          >
            <BookOpen className="w-4 h-4" />
            Documentation
          </button>
          <button
            onClick={() => openExternal(PROJECT_REPO_URL)}
            className="py-2 px-3 bg-stone-800 hover:bg-stone-700 text-stone-200 text-sm rounded-lg flex items-center justify-center gap-2 transition cursor-pointer"
            title="Project Repository"
          >
            <Github className="w-4 h-4" />
          </button>
          <button
            onClick={() => openExternal(PROJECT_ISSUES_URL)}
            className="py-2 px-3 bg-stone-800 hover:bg-stone-700 text-stone-200 text-sm rounded-lg flex items-center justify-center gap-2 transition cursor-pointer"
            title="Report an Issue"
          >
            <Bug className="w-4 h-4" />
          </button>
        </div>

        <div className="flex items-center justify-between border-t border-white/10 pt-3">
          <span className="text-xs text-stone-500 font-mono">
            {version ? `Version ${version}` : 'Development build'}
          </span>
          <button
            onClick={onClose}
            className="text-xs text-stone-400 hover:text-stone-200 transition cursor-pointer"
          >
            Close
          </button>
        </div>
      </div>
    </div>
  );
};
