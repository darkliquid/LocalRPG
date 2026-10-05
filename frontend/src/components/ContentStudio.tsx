import { useEffect, useMemo, useState } from 'react';
import { X } from 'lucide-react';
import { APIClient } from '../api/client';
import type { EntityNote, EntitySummary, FolderNode } from '../types';
import EntityTree from './EntityTree';
import MarkdownEditor, { type MarkdownEditorProps } from './editor/MarkdownEditor';
import { loadEntityIndex, invalidateEntityIndex } from './editor/entityIndex';
import { NewEntityWizard } from './NewEntityWizard';

type DocKind = 'entities' | 'prompts' | 'manifests';

interface ContentStudioProps {
  isOpen: boolean;
  onClose: () => void;
  gameID: string;
}

// ContentStudio is the full-screen authoring surface: the folder tree on the
// left, one document on the right. It reuses the codex's note endpoints rather
// than inventing a second write path, so a save here is the same save.
export default function ContentStudio({ isOpen, onClose, gameID }: ContentStudioProps) {
  const [kind, setKind] = useState<DocKind>('entities');
  const [folders, setFolders] = useState<FolderNode[]>([]);
  const [entities, setEntities] = useState<EntitySummary[]>([]);
  const [note, setNote] = useState<EntityNote | null>(null);
  const [draft, setDraft] = useState('');
  const [saved, setSaved] = useState('');
  const [error, setError] = useState('');
  const [linkTargets, setLinkTargets] = useState<EntitySummary[]>([]);
  const [saveFailure, setSaveFailure] = useState<{ line: number; message: string } | null>(null);
  const [isNewNoteOpen, setIsNewNoteOpen] = useState(false);

  const client = useMemo(() => new APIClient(gameID), [gameID]);

  useEffect(() => {
    if (!isOpen || !gameID) return;
    let cancelled = false;
    client
      .listFolders()
      .then((tree) => {
        if (!cancelled) setFolders(tree);
      })
      .catch(() => {
        if (!cancelled) setFolders([]);
      });
    client
      .listEntities()
      .then((list) => {
        if (!cancelled) setEntities(list);
      })
      .catch(() => {
        if (!cancelled) setEntities([]);
      });
    return () => {
      cancelled = true;
    };
  }, [isOpen, gameID, client]);

  useEffect(() => {
    if (!isOpen || !gameID) return;
    let cancelled = false;
    loadEntityIndex(client, gameID)
      .then((list) => {
        if (!cancelled) setLinkTargets(list);
      })
      .catch(() => {
        if (!cancelled) setLinkTargets([]);
      });
    return () => {
      cancelled = true;
    };
  }, [isOpen, gameID, client]);

  useEffect(() => {
    if (!note) return;
    setDraft(note.markdown);
    setSaved(note.markdown);
  }, [note]);

  const language: MarkdownEditorProps['language'] =
    kind === 'entities' ? 'markdown-frontmatter' : kind === 'manifests' ? 'yaml' : 'markdown';

  const isDirty = draft !== saved;

  if (!isOpen) return null;

  const save = async () => {
    if (!note) return;
    try {
      setError('');
      setSaveFailure(null);
      await client.saveEntity(note.id, draft, note.folder);
      setSaved(draft);
      invalidateEntityIndex();
      setLinkTargets(await loadEntityIndex(client, gameID, true));
    } catch (err) {
      const withLine = err as Error & { line?: number };
      if (withLine.line) {
        setSaveFailure({ line: withLine.line, message: withLine.message });
      }
      setError(withLine.message);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex flex-col bg-stone-950/98 backdrop-blur-sm">
      <header className="flex items-center justify-between border-b border-white/10 px-5 py-3">
        <div className="flex items-center gap-4">
          <h2 className="font-sans text-sm font-bold uppercase tracking-wider text-stone-200">
            Content Studio
          </h2>
          <nav className="flex gap-1">
            {(['entities', 'prompts', 'manifests'] as DocKind[]).map((tab) => (
              <button
                key={tab}
                onClick={() => setKind(tab)}
                className={`rounded-lg px-2.5 py-1 text-xs capitalize transition-colors cursor-pointer ${
                  kind === tab
                    ? 'bg-purple-600 font-bold text-white'
                    : 'text-stone-400 hover:bg-white/10 hover:text-stone-100'
                }`}
              >
                {tab}
              </button>
            ))}
          </nav>
        </div>
        <div className="flex items-center gap-3">
          {isDirty && (
            <span className="text-[10px] uppercase tracking-wider text-amber-400">Unsaved</span>
          )}
          <button onClick={onClose} className="cursor-pointer text-stone-400 hover:text-white">
            <X className="h-4 w-4" />
          </button>
        </div>
      </header>

      {error && (
        <div className="border-b border-red-500/40 bg-red-950/40 px-5 py-2 text-xs text-red-200">
          {error}
        </div>
      )}

      <div className="flex min-h-0 flex-1">
        <aside className="w-64 shrink-0 border-r border-white/10 p-3">
          <button
            type="button"
            onClick={() => setIsNewNoteOpen(true)}
            className="mb-2 w-full rounded-lg border border-purple-500/50 bg-purple-600/20 px-2 py-1 text-xs font-sans font-bold text-purple-200 transition-colors cursor-pointer hover:bg-purple-600/40"
          >
            + New
          </button>
          <EntityTree
            folders={folders}
            entities={entities}
            selectedId={note?.id}
            onSelect={async (id) => {
              setError('');
              setNote(await client.getEntity(id));
            }}
            onMoveEntity={async (id, folder) => {
              const target = await client.getEntity(id);
              await client.saveEntity(id, target.markdown, folder);
              setEntities(await client.listEntities());
            }}
            onMoveFolder={async (from, to) => {
              await client.moveFolder(from, to);
              setFolders(await client.listFolders());
            }}
            onCreateFolder={async (path) => {
              await client.createFolder(path);
              setFolders(await client.listFolders());
            }}
            onDeleteFolder={async (path) => {
              await client.deleteFolder(path, true);
              setFolders(await client.listFolders());
            }}
          />
        </aside>

        <main className="min-h-0 min-w-0 flex-1 p-3">
          {note ? (
            <MarkdownEditor
              key={note.id}
              value={draft}
              onChange={setDraft}
              language={language}
              ariaLabel={`${kind} document`}
              minHeight="100%"
              onSave={() => void save()}
              linkTargets={linkTargets}
              serverError={saveFailure}
            />
          ) : (
            <div className="flex h-full items-center justify-center text-xs font-mono text-stone-500">
              Select a note to edit.
            </div>
          )}
        </main>
      </div>

      <NewEntityWizard
        isOpen={isNewNoteOpen}
        existingIds={entities.map((candidate) => candidate.id)}
        onClose={() => setIsNewNoteOpen(false)}
        onOpenExisting={async (id) => {
          setIsNewNoteOpen(false);
          setNote(await client.getEntity(id));
        }}
        onConfirm={async ({ id, markdown }) => {
          await client.saveEntity(id, markdown);
          invalidateEntityIndex();
          setEntities(await client.listEntities());
          setLinkTargets(await loadEntityIndex(client, gameID, true));
          setIsNewNoteOpen(false);
          setNote(await client.getEntity(id));
        }}
      />
    </div>
  );
}
