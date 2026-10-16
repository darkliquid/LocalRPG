import type { Dispatch, SetStateAction } from 'react';
import { BookOpen, Plus, Trash2 } from 'lucide-react';
import { AIGenerateButton } from '../../ui/AIGenerateButton';
import { HelpTip } from '../../ui/HelpTip';
import { Example } from '../../ui/Example';
import { CharacterCreationField, GenerationFailure } from '../../../types';

export interface SystemManifestTabProps {
  name: string;
  slugID: string;
  version: string;
  description: string;
  creationPreamble: string;
  creationFields: CharacterCreationField[];
  setName: (name: string) => void;
  setSlugID: (id: string) => void;
  setVersion: (version: string) => void;
  setDescription: (description: string) => void;
  setCreationPreamble: (preamble: string) => void;
  /** the fields list is edited in place as well as replaced, so it takes an updater. */
  setCreationFields: Dispatch<SetStateAction<CharacterCreationField[]>>;
  /** savedID is null until the system has been saved, which locks the id field. */
  savedID: string | null;
  markDirty: () => void;
  reportGenerationError: (failure: GenerationFailure) => void;
  getSystemContext: () => Record<string, string>;
  setDeleteTarget: (target: { id: string; name: string }) => void;
  setDeleteSystemError: (error: string | null) => void;
  setCanForceDelete: (canForce: boolean) => void;
}

// SystemManifestTab is the system's own metadata: its name and id, the version and
// description a catalogue shows, and the prompts a player answers when a campaign
// starts with it. The delete control lives here because it acts on the manifest.
export function SystemManifestTab({
  name,
  slugID,
  version,
  description,
  creationPreamble,
  creationFields,
  setName,
  setSlugID,
  setVersion,
  setDescription,
  setCreationPreamble,
  setCreationFields,
  savedID,
  markDirty,
  reportGenerationError,
  getSystemContext,
  setDeleteTarget,
  setDeleteSystemError,
  setCanForceDelete,
}: SystemManifestTabProps) {
  return (
    <div className="flex-1 overflow-y-auto min-h-0 space-y-4 pr-2">
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <div className="space-y-1.5">
          <div className="flex items-center justify-between">
            <label className="text-xs font-sans uppercase tracking-wider text-stone-300">System Name</label>
            <AIGenerateButton
              formType="system"
              fieldName="name"
              onError={reportGenerationError}
              getContext={getSystemContext}
              onGenerated={(val) => {
                setName(val);
                markDirty();
                if (!savedID) {
                  setSlugID(
                    val
                      .toLowerCase()
                      .replace(/[^a-z0-9]+/g, '-')
                      .replace(/^-|-$/g, ''),
                  );
                }
              }}
            />
          </div>
          <input
            type="text"
            required
            placeholder="e.g. Iron Realm D20"
            value={name}
            onChange={(e) => {
              setName(e.target.value);
              markDirty();
              if (!savedID) {
                setSlugID(
                  e.target.value
                    .toLowerCase()
                    .replace(/[^a-z0-9]+/g, '-')
                    .replace(/^-|-$/g, ''),
                );
              }
            }}
            className="w-full bg-stone-950 border border-stone-800 rounded-xl px-4 py-2.5 text-sm text-stone-100 placeholder-stone-600 focus:outline-none focus:border-purple-500/50 transition-colors"
          />
        </div>

        <div className="space-y-1.5">
          <label className="text-xs font-sans uppercase tracking-wider text-stone-300">Version</label>
          <input
            type="text"
            placeholder="1.0.0"
            value={version}
            onChange={(e) => {
              setVersion(e.target.value);
              markDirty();
            }}
            className="w-full bg-stone-950 border border-stone-800 rounded-xl px-4 py-2.5 text-sm text-stone-100 placeholder-stone-600 focus:outline-none focus:border-purple-500/50 transition-colors font-mono"
          />
        </div>
      </div>

      <div className="space-y-1.5">
        <label className="text-xs font-sans uppercase tracking-wider text-stone-300">Directory Slug ID</label>
        <input
          type="text"
          disabled={!!savedID}
          placeholder="e.g. iron-realm"
          value={slugID}
          onChange={(e) => {
            setSlugID(e.target.value);
            markDirty();
          }}
          className="w-full bg-stone-950 border border-stone-800 rounded-xl px-4 py-2.5 text-sm text-stone-100 placeholder-stone-600 focus:outline-none focus:border-purple-500/50 transition-colors font-mono disabled:opacity-60"
        />
        <p className="text-xs text-stone-400">Unique identifier for the system directory on disk.</p>
      </div>

      <div className="space-y-1.5">
        <div className="flex items-center justify-between">
          <label className="text-xs font-sans uppercase tracking-wider text-stone-300">
            Rulebook Overview & Philosophy
          </label>
          <AIGenerateButton
            formType="system"
            fieldName="description"
            onError={reportGenerationError}
            getContext={getSystemContext}
            onGenerated={(val) => {
              setDescription(val);
              markDirty();
            }}
            seed={description}
          />
        </div>
        <textarea
          rows={5}
          placeholder="Describe core dice mechanics, resolution philosophy, and character stats..."
          value={description}
          onChange={(e) => {
            setDescription(e.target.value);
            markDirty();
          }}
          className="w-full bg-stone-950 border border-stone-800 rounded-xl p-4 text-sm text-stone-100 placeholder-stone-600 focus:outline-none focus:border-purple-500/50 transition-colors resize-none"
        />
      </div>

      <div className="space-y-3 pt-2 border-t border-stone-800/70">
        <div className="flex items-center justify-between gap-3">
          <div className="flex items-center gap-2">
            <BookOpen className="w-4 h-4 text-purple-400" />
            <span className="font-sans text-xs uppercase font-bold text-stone-200">Character Creation Prompts</span>
            <span className="text-xs font-mono text-stone-500">({creationFields.length})</span>
            <HelpTip label="Character Creation Prompts">
              <>
                Each prompt a player answers when starting with this system. The id is the key the campaign records, the
                label is shown to the player, and the prompt guides the AI generator. The kind decides how the answer is
                collected: text and long are written answers, number is a number, select offers the options you list,
                and voice picks a voice profile. A voice field is never required and is never generated. Generatable
                marks a field the AI may fill.
              </>
            </HelpTip>
          </div>
          <button
            type="button"
            onClick={() => {
              setCreationFields((prev) => [
                ...prev,
                { id: `prompt_${prev.length + 1}`, label: 'New Prompt', kind: 'text', generatable: true },
              ]);
              markDirty();
            }}
            className="flex items-center gap-1 text-xs px-2 py-1 rounded bg-purple-600/20 border border-purple-500/40 text-purple-300 hover:bg-purple-600/30 transition cursor-pointer"
          >
            <Plus className="w-3 h-3" />
            <span>Add Prompt</span>
          </button>
        </div>

        <input
          type="text"
          placeholder="Preamble shown above the prompts (optional)"
          value={creationPreamble}
          onChange={(e) => {
            setCreationPreamble(e.target.value);
            markDirty();
          }}
          className="w-full bg-stone-950 border border-stone-800 rounded-xl px-4 py-2.5 text-xs text-stone-100 placeholder-stone-600 focus:outline-none focus:border-purple-500/50 transition-colors"
        />

        <Example label="A select prompt">
          {`character_creation:\n  preamble: Answer in the tone of a wandering chronicle.\n  fields:\n    - id: calling\n      label: Calling\n      kind: select\n      prompt: The character's trade or vocation.\n      required: true\n      options: [sellsword, scholar, thief, priest]\n      default: scholar`}
        </Example>

        {creationFields.length === 0 ? (
          <p className="text-xs text-stone-500">
            With no prompts defined, the studio falls back to appearance, age, gender, pronouns, background and voice.
          </p>
        ) : (
          <div className="space-y-2">
            {creationFields.map((field, index) => (
              <div key={index} className="rounded-lg bg-stone-950/70 border border-stone-800/80 p-2.5 space-y-2">
                <div className="grid grid-cols-2 gap-2">
                  <input
                    type="text"
                    placeholder="id"
                    value={field.id}
                    onChange={(e) => {
                      const updated = [...creationFields];
                      updated[index] = { ...updated[index], id: e.target.value };
                      setCreationFields(updated);
                      markDirty();
                    }}
                    className="bg-stone-900 border border-stone-800 rounded px-2 py-1 text-xs font-mono text-purple-300 focus:outline-none"
                  />
                  <input
                    type="text"
                    placeholder="Label"
                    value={field.label}
                    onChange={(e) => {
                      const updated = [...creationFields];
                      updated[index] = { ...updated[index], label: e.target.value };
                      setCreationFields(updated);
                      markDirty();
                    }}
                    className="bg-stone-900 border border-stone-800 rounded px-2 py-1 text-xs text-stone-200 focus:outline-none"
                  />
                </div>
                <input
                  type="text"
                  placeholder="Prompt shown to the player"
                  value={field.prompt || ''}
                  onChange={(e) => {
                    const updated = [...creationFields];
                    updated[index] = { ...updated[index], prompt: e.target.value };
                    setCreationFields(updated);
                    markDirty();
                  }}
                  className="w-full bg-stone-900 border border-stone-800 rounded px-2 py-1 text-xs text-stone-200 focus:outline-none"
                />
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
                  <input
                    type="text"
                    placeholder="Default value (optional)"
                    value={field.default || ''}
                    onChange={(e) => {
                      const updated = [...creationFields];
                      updated[index] = { ...updated[index], default: e.target.value };
                      setCreationFields(updated);
                      markDirty();
                    }}
                    className="w-full bg-stone-900 border border-stone-800 rounded px-2 py-1 text-xs text-stone-200 focus:outline-none"
                  />
                  {field.kind === 'select' && (
                    <input
                      type="text"
                      placeholder="Options, comma-separated"
                      value={(field.options ?? []).join(', ')}
                      onChange={(e) => {
                        const updated = [...creationFields];
                        updated[index] = {
                          ...updated[index],
                          options: e.target.value
                            .split(',')
                            .map((value) => value.trim())
                            .filter(Boolean),
                        };
                        setCreationFields(updated);
                        markDirty();
                      }}
                      className="w-full bg-stone-900 border border-stone-800 rounded px-2 py-1 text-xs text-stone-200 focus:outline-none"
                    />
                  )}
                </div>
                <div className="flex flex-wrap items-center gap-3 text-xs text-stone-400">
                  <select
                    value={field.kind || 'text'}
                    onChange={(e) => {
                      const updated = [...creationFields];
                      updated[index] = {
                        ...updated[index],
                        kind: e.target.value as CharacterCreationField['kind'],
                      };
                      setCreationFields(updated);
                      markDirty();
                    }}
                    className="bg-stone-900 border border-stone-800 rounded px-2 py-1 text-xs text-stone-200 focus:outline-none cursor-pointer"
                  >
                    {['text', 'long', 'number', 'select', 'voice'].map((kind) => (
                      <option key={kind} value={kind}>
                        {kind}
                      </option>
                    ))}
                  </select>
                  <label className="flex items-center gap-1 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={!!field.required}
                      onChange={(e) => {
                        const updated = [...creationFields];
                        updated[index] = { ...updated[index], required: e.target.checked };
                        setCreationFields(updated);
                        markDirty();
                      }}
                      className="rounded bg-stone-950 border-stone-800 text-purple-600 focus:ring-0"
                    />
                    <span>Required</span>
                  </label>
                  <label className="flex items-center gap-1 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={!!field.generatable}
                      disabled={field.kind === 'voice'}
                      onChange={(e) => {
                        const updated = [...creationFields];
                        updated[index] = { ...updated[index], generatable: e.target.checked };
                        setCreationFields(updated);
                        markDirty();
                      }}
                      className="rounded bg-stone-950 border-stone-800 text-purple-600 focus:ring-0 disabled:opacity-40"
                    />
                    <span>Generatable</span>
                  </label>
                  <button
                    type="button"
                    onClick={() => {
                      setCreationFields((prev) => prev.filter((_, i) => i !== index));
                      markDirty();
                    }}
                    className="ml-auto flex items-center gap-1 text-red-400/80 hover:text-red-300 cursor-pointer"
                  >
                    <Trash2 className="w-3 h-3" />
                    <span>Remove</span>
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {savedID && (
        <div className="p-4 bg-red-950/20 border border-red-500/30 rounded-2xl flex items-center justify-between mt-4">
          <div>
            <div className="text-xs font-sans font-bold text-red-400 flex items-center gap-1.5">
              <Trash2 className="w-3.5 h-3.5" />
              <span>Danger Zone: Delete System</span>
            </div>
            <p className="text-xs font-sans text-stone-400 mt-1">
              Permanently delete this ruleset and its scripts from disk. This cannot be undone.
            </p>
          </div>
          <button
            type="button"
            onClick={() => {
              setDeleteTarget({ id: savedID, name: name || savedID });
              setDeleteSystemError(null);
              setCanForceDelete(false);
            }}
            className="px-3 py-1.5 rounded-lg bg-red-600 hover:bg-red-500 text-white font-sans text-xs font-bold transition-all cursor-pointer shadow"
          >
            Delete System
          </button>
        </div>
      )}
    </div>
  );
}
