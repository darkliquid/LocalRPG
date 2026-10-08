import React, { useMemo, useState } from 'react';
import { Check, Pencil, Sparkles, Trash2, X } from 'lucide-react';
import { DraftCommitRequest, WorldDraftEntity, WorldDraftInfo } from '../types';

export interface WorldDraftReviewProps {
  draft: WorldDraftInfo;
  // targetWorldId merges the accepted set into an existing world instead of
  // creating a new one.
  targetWorldId?: string;
  onCommit: (req: DraftCommitRequest) => void;
  onDiscard: (draftId: string) => void;
  loading?: boolean;
  error?: string | null;
}

interface DraftItem {
  key: string;
  title: string;
  subtitle?: string;
  body: string;
  links?: string[];
  dropped?: string[];
  entity?: WorldDraftEntity;
}

// WorldDraftReview renders a generated world for accept-or-reject review. It
// writes nothing itself: it hands the accepted set to its caller.
export const WorldDraftReview: React.FC<WorldDraftReviewProps> = ({
  draft,
  targetWorldId,
  onCommit,
  onDiscard,
  loading = false,
  error = null,
}) => {
  const [rejectedSections, setRejectedSections] = useState<Set<number>>(new Set());
  const [rejectedEntities, setRejectedEntities] = useState<Set<string>>(new Set());
  const [edits, setEdits] = useState<Record<string, WorldDraftEntity>>({});
  const [editing, setEditing] = useState<string | null>(null);

  const sections = draft.sections ?? [];
  const entities = useMemo(
    () => (draft.entities ?? []).map((entity) => edits[entity.id] ?? entity),
    [draft.entities, edits]
  );

  const acceptedSections = sections.filter((_, index) => !rejectedSections.has(index));
  const acceptedEntities = entities.filter((entity) => !rejectedEntities.has(entity.id));
  const acceptedCount = acceptedSections.length + acceptedEntities.length;
  const totalCount = sections.length + entities.length;

  const toggleSection = (index: number) => {
    setRejectedSections((prev) => {
      const next = new Set(prev);
      if (next.has(index)) next.delete(index);
      else next.add(index);
      return next;
    });
  };

  const toggleEntity = (id: string) => {
    setRejectedEntities((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const editEntity = (entity: WorldDraftEntity, patch: Partial<WorldDraftEntity>) => {
    setEdits((prev) => ({ ...prev, [entity.id]: { ...(prev[entity.id] ?? entity), ...patch } }));
  };

  const handleCommit = () => {
    if (acceptedCount === 0) return;
    const acceptAll = rejectedSections.size === 0 && rejectedEntities.size === 0;
    const editedList = Object.values(edits).filter((e) => !rejectedEntities.has(e.id));
    onCommit({
      draft_id: draft.id,
      target_world_id: targetWorldId,
      accept_all: acceptAll,
      section_indexes: acceptAll
        ? undefined
        : sections
            .map((_, index) => index)
            .filter((index) => !rejectedSections.has(index)),
      entity_ids: acceptAll
        ? undefined
        : (draft.entities ?? [])
            .map((e) => e.id)
            .filter((id) => !rejectedEntities.has(id)),
      edits: editedList.length > 0 ? editedList : undefined,
    });
  };

  const items: DraftItem[] = [
    ...sections.map((section, index) => ({
      key: `section-${index}`,
      title: section.title || 'Overview',
      subtitle: 'Lore',
      body: section.body,
    })),
    ...entities.map((entity) => ({
      key: `entity-${entity.id}`,
      title: entity.name,
      subtitle: entity.type,
      body: entity.body,
      links: entity.links,
      dropped: entity.dropped_links,
      entity,
    })),
  ];

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm">
      <div className="relative w-full max-w-3xl overflow-hidden border border-white/10 rounded-2xl bg-neutral-900/90 shadow-2xl backdrop-blur-xl">
        <div className="flex items-center justify-between p-6 border-b border-white/10">
          <div className="flex items-center gap-3">
            <div className="p-2.5 rounded-xl bg-emerald-500/10 border border-emerald-500/20 text-emerald-400">
              <Sparkles className="w-5 h-5" />
            </div>
            <div>
              <h3 className="text-lg font-semibold text-white">{draft.name || draft.id}</h3>
              <p className="text-xs text-neutral-400">
                {draft.genre || 'No genre'}
                {targetWorldId ? ` · merging into ${targetWorldId}` : ''}
              </p>
            </div>
          </div>
          <span
            className="text-xs text-neutral-300 bg-neutral-800/80 px-2.5 py-1 rounded-md border border-white/5"
            aria-live="polite"
          >
            {acceptedCount} of {totalCount} accepted
          </span>
        </div>

        <div className="p-6 space-y-3 max-h-[65vh] overflow-y-auto">
          {draft.oracle && (
            <p className="p-3 text-xs rounded-xl bg-amber-500/10 border border-amber-500/25 text-amber-200">
              No model provider was configured, so this draft comes from the built-in template
              generator.
            </p>
          )}
          {error && (
            <p className="p-3 text-sm rounded-xl bg-red-950/40 border border-red-500/30 text-red-300">
              {error}
            </p>
          )}

          {items.map((item, index) => {
            const isSection = item.key.startsWith('section-');
            const accepted = isSection
              ? !rejectedSections.has(index)
              : !rejectedEntities.has(item.entity!.id);
            const isEditing = editing === item.key;
            return (
              <div
                key={item.key}
                className={`p-4 rounded-xl border transition-colors ${
                  accepted ? 'border-white/10 bg-white/[0.03]' : 'border-white/5 bg-white/[0.01] opacity-50'
                }`}
              >
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <h4 className="text-sm font-semibold text-white truncate">{item.title}</h4>
                    {item.subtitle && (
                      <span className="text-[11px] uppercase tracking-wide text-neutral-500">
                        {item.subtitle}
                      </span>
                    )}
                  </div>
                  <div className="flex items-center gap-1.5 shrink-0">
                    {item.entity && (
                      <button
                        type="button"
                        aria-label={`Edit ${item.title}`}
                        onClick={() => setEditing(isEditing ? null : item.key)}
                        className="p-1.5 text-neutral-400 hover:text-white rounded-lg hover:bg-white/5"
                      >
                        <Pencil className="w-4 h-4" />
                      </button>
                    )}
                    <button
                      type="button"
                      aria-label={accepted ? `Reject ${item.title}` : `Accept ${item.title}`}
                      onClick={() => (isSection ? toggleSection(index) : toggleEntity(item.entity!.id))}
                      className={`p-1.5 rounded-lg hover:bg-white/5 ${
                        accepted ? 'text-emerald-400' : 'text-neutral-500'
                      }`}
                    >
                      {accepted ? <Check className="w-4 h-4" /> : <X className="w-4 h-4" />}
                    </button>
                  </div>
                </div>

                {isEditing && item.entity ? (
                  <div className="mt-3 space-y-2">
                    <input
                      aria-label={`${item.title} name`}
                      value={item.entity.name}
                      onChange={(event) =>
                        editEntity(item.entity!, { name: event.target.value })
                      }
                      className="w-full px-3 py-2 text-sm text-white bg-white/[0.03] border border-white/10 rounded-lg focus:border-purple-500/50 focus:outline-none"
                    />
                    <textarea
                      aria-label={`${item.title} body`}
                      value={item.entity.body}
                      onChange={(event) =>
                        editEntity(item.entity!, { body: event.target.value })
                      }
                      rows={4}
                      className="w-full px-3 py-2 text-sm text-white bg-white/[0.03] border border-white/10 rounded-lg focus:border-purple-500/50 focus:outline-none resize-none"
                    />
                  </div>
                ) : (
                  <p className="mt-2 text-xs text-neutral-300 whitespace-pre-wrap line-clamp-4">
                    {item.body}
                  </p>
                )}

                {item.links && item.links.length > 0 && (
                  <p className="mt-2 text-[11px] text-neutral-500">
                    Links: {item.links.join(', ')}
                  </p>
                )}
                {item.dropped && item.dropped.length > 0 && (
                  <p className="mt-1 text-[11px] text-amber-400/80">
                    Dropped unresolved links: {item.dropped.join(', ')}
                  </p>
                )}
              </div>
            );
          })}

          {items.length === 0 && (
            <p className="text-sm text-neutral-400">This draft is empty.</p>
          )}
        </div>

        <div className="flex items-center justify-between gap-3 p-6 border-t border-white/10">
          <button
            onClick={() => onDiscard(draft.id)}
            disabled={loading}
            className="flex items-center gap-2 px-4 py-2 text-sm text-red-300 rounded-xl hover:bg-red-500/10 transition-colors"
          >
            <Trash2 className="w-4 h-4" />
            Discard
          </button>
          <button
            onClick={handleCommit}
            disabled={loading || acceptedCount === 0}
            className="px-4 py-2 text-sm font-medium text-white rounded-xl bg-emerald-600 hover:bg-emerald-500 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
          >
            {loading ? 'Committing...' : targetWorldId ? 'Merge accepted' : 'Create world'}
          </button>
        </div>
      </div>
    </div>
  );
};

export default WorldDraftReview;
