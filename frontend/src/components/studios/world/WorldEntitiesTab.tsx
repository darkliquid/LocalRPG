import { Save, Trash2 } from 'lucide-react';
import { FolderNode, WorldEntitySummary } from '../../../types';
import { APIClient } from '../../../api/client';
import EntityTree from '../../EntityTree';
import MarkdownEditor from '../../editor/MarkdownEditor';

export interface WorldEntitiesTabProps {
  entities: WorldEntitySummary[];
  entityMarkdown: string;
  setEntityMarkdown: (markdown: string) => void;
  selectedEntityID: string | null;
  worldFolders: FolderNode[];
  savedID: string | null;
  slugID: string;
  setIsNewEntityModal: (open: boolean) => void;
  markDirty: () => void;
  handleSelectEntity: (id: string) => void | Promise<void>;
  handleSaveEntity: () => void | Promise<void>;
  handleDeleteEntity: (id: string) => void | Promise<void>;
  refreshWorldFolders: () => void | Promise<void>;
  loadWorldDetail: (id: string) => void | Promise<void>;
  setToast: (toast: { type: 'success' | 'error'; message: string }) => void;
}

// WorldEntitiesTab manages a world's starter entities: the notes a new campaign
// copies in, edited here as templates with the same tree and editor the codex uses.
export function WorldEntitiesTab({
  entities,
  entityMarkdown,
  setEntityMarkdown,
  selectedEntityID,
  worldFolders,
  savedID,
  slugID,
  setIsNewEntityModal,
  markDirty,
  handleSelectEntity,
  handleSaveEntity,
  handleDeleteEntity,
  refreshWorldFolders,
  loadWorldDetail,
  setToast,
}: WorldEntitiesTabProps) {
  return (
    <div className="flex-1 flex gap-4 min-h-0 overflow-hidden">
      {/* Entity List */}
      <div className="w-64 shrink-0 bg-stone-950/60 rounded-xl border border-stone-800/80 p-3 flex flex-col gap-2 min-h-0 h-full">
        <div className="flex items-center justify-between pb-2 border-b border-stone-800/60 shrink-0">
          <span className="text-xs font-sans uppercase tracking-wider text-stone-400">Templates</span>
          <button
            type="button"
            onClick={() => setIsNewEntityModal(true)}
            className="text-xs font-sans font-bold px-2 py-0.5 rounded bg-purple-600 text-white cursor-pointer hover:bg-purple-500"
          >
            + Add
          </button>
        </div>

        <div className="flex-1 min-h-0 flex flex-col overflow-hidden">
          {entities.length === 0 ? (
            <div className="text-xs text-stone-500 py-6 text-center">
              No starter templates. Click + Add to create one!
            </div>
          ) : (
            <EntityTree
              folders={worldFolders}
              entities={entities}
              selectedId={selectedEntityID ?? undefined}
              onSelect={(id) => void handleSelectEntity(id)}
              onMoveEntity={async (id, folder) => {
                if (!savedID) {
                  setToast({ type: 'error', message: 'Save the world before organising its templates.' });
                  return;
                }
                const note = await APIClient.getWorldEntity(savedID, id);
                await APIClient.saveWorldEntity(savedID, id, note.markdown, folder);
                await loadWorldDetail(savedID);
                await refreshWorldFolders();
              }}
              onMoveFolder={async (from, to) => {
                if (!savedID) return;
                await APIClient.moveWorldFolder(savedID, from, to);
                await refreshWorldFolders();
              }}
              onCreateFolder={async (path) => {
                if (!savedID) {
                  setToast({ type: 'error', message: 'Save the world before creating folders.' });
                  return;
                }
                await APIClient.createWorldFolder(savedID, path);
                await refreshWorldFolders();
              }}
              onDeleteFolder={async (path) => {
                if (!savedID) return;
                await APIClient.deleteWorldFolder(savedID, path, true);
                await refreshWorldFolders();
              }}
              onDeleteEntity={(id) => void handleDeleteEntity(id)}
            />
          )}
        </div>
      </div>

      {/* Entity Markdown Editor */}
      <div className="flex-1 min-w-0 flex flex-col gap-2 min-h-0 overflow-hidden">
        {selectedEntityID ? (
          <>
            <div className="flex items-center justify-between text-xs font-mono text-stone-400 px-1 shrink-0">
              <span>
                worlds/{savedID || slugID || 'draft'}/entities/
                {entities.find((e) => e.id === selectedEntityID)?.folder
                  ? `${entities.find((e) => e.id === selectedEntityID)?.folder}/`
                  : ''}
                {selectedEntityID}.md
              </span>
              <div className="flex items-center gap-2">
                <button
                  type="button"
                  onClick={() => void handleDeleteEntity(selectedEntityID)}
                  title="Delete entity template"
                  className="flex items-center gap-1 px-2 py-1 rounded-lg border border-red-500/40 text-red-300 hover:bg-red-950/40 font-bold text-xs cursor-pointer transition-all"
                >
                  <Trash2 className="w-3 h-3" />
                  <span>Delete</span>
                </button>
                <button
                  type="button"
                  onClick={handleSaveEntity}
                  className="flex items-center gap-1 px-3 py-1 rounded-lg bg-purple-600 hover:bg-purple-500 text-white font-bold text-xs cursor-pointer shadow transition-all"
                >
                  <Save className="w-3 h-3" />
                  <span>{savedID ? 'Save Entity' : 'Update Draft'}</span>
                </button>
              </div>
            </div>
            <MarkdownEditor
              key={`${savedID || slugID || 'draft'}-${selectedEntityID}`}
              value={entityMarkdown}
              onChange={(next) => {
                setEntityMarkdown(next);
                markDirty();
              }}
              language="markdown-frontmatter"
              ariaLabel="World entity template markdown"
            />
          </>
        ) : (
          <div className="flex-1 flex items-center justify-center text-xs text-stone-500 font-mono">
            Select or create an entity template to edit its Markdown.
          </div>
        )}
      </div>
    </div>
  );
}
