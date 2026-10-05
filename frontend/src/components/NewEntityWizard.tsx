import React, { useEffect, useState } from 'react';
import { Check, Sparkles, X } from 'lucide-react';
import type { EntityTypeCatalog, TTSConfig, VoiceProfile } from '../types';
import { APIClient } from '../api/client';
import { useMountTransition } from '../hooks/useMountTransition';
import { buildEntityMarkdown, type VoiceSelection } from '../lib/entityScaffold';
import { slugify } from '../lib/slug';
import { VoiceProfileSelect } from './VoiceProfileSelect';

export interface NewEntityWizardProps {
  isOpen: boolean;
  initialName?: string;
  existingIds: string[];
  ttsConfig?: TTSConfig;
  voiceProfiles?: VoiceProfile[];
  confirmLabel?: string;
  onConfirm: (input: { id: string; name: string; markdown: string }) => void | Promise<void>;
  onOpenExisting?: (id: string) => void;
  onClose: () => void;
}

// NewEntityWizard scaffolds a note's frontmatter from the server's entity type
// catalogue and hands the markdown to its host. It does not save: the codex, the
// content studio and the world studio each write to a different endpoint.
export const NewEntityWizard: React.FC<NewEntityWizardProps> = ({
  isOpen,
  initialName,
  existingIds,
  ttsConfig,
  voiceProfiles,
  confirmLabel = 'Create note',
  onConfirm,
  onOpenExisting,
  onClose,
}) => {
  const { mounted, state } = useMountTransition(isOpen, 200);
  const [catalog, setCatalog] = useState<EntityTypeCatalog | null>(null);
  const [catalogError, setCatalogError] = useState('');
  const [resolvedTTS, setResolvedTTS] = useState<TTSConfig | null>(ttsConfig ?? null);
  const [resolvedProfiles, setResolvedProfiles] = useState<VoiceProfile[]>(voiceProfiles ?? []);
  const [settingsLoaded, setSettingsLoaded] = useState(false);
  const [name, setName] = useState(initialName ?? '');
  const [id, setId] = useState(initialName ? slugify(initialName) : '');
  const [idTouched, setIdTouched] = useState(false);
  const [type, setType] = useState('');
  const [voiceProfileId, setVoiceProfileId] = useState('');
  const [saving, setSaving] = useState(false);

  const spec = catalog?.types.find((candidate) => candidate.id === type);
  const wantsVoice = spec?.keys.some((key) => key.name === 'voice') ?? false;
  const profiles = resolvedProfiles;

  useEffect(() => {
    if (!isOpen) return;
    setName(initialName ?? '');
    setId(initialName ? slugify(initialName) : '');
    setIdTouched(false);
    setType('');
    setVoiceProfileId('');
    setCatalogError('');
    let cancelled = false;
    new APIClient('')
      .getEntityTypes()
      .then((next) => {
        if (cancelled) return;
        setCatalog(next);
        setType((current) => current || next.types[0]?.id || '');
      })
      .catch((err) => {
        if (!cancelled) setCatalogError(err instanceof Error ? err.message : 'Could not load entity types');
      });
    return () => {
      cancelled = true;
    };
  }, [isOpen, initialName]);

  // The voice picker shares VoiceProfileSelect with the codex and the campaign
  // settings, so it needs the same two inputs. A host that already holds them
  // passes them; one that does not gets them from one settings read.
  useEffect(() => {
    if (ttsConfig && voiceProfiles) {
      setSettingsLoaded(true);
      return;
    }
    if (settingsLoaded) return;
    let cancelled = false;
    APIClient.getSettings()
      .then((settings) => {
        if (cancelled) return;
        if (!ttsConfig) setResolvedTTS(settings.config.media.tts);
        if (!voiceProfiles) setResolvedProfiles(settings.config.media.tts.voice_profiles ?? []);
        setSettingsLoaded(true);
      })
      .catch(() => {
        if (!cancelled) setSettingsLoaded(true);
      });
    return () => {
      cancelled = true;
    };
  }, [ttsConfig, voiceProfiles, settingsLoaded]);

  const collision = id !== '' && existingIds.includes(id);
  const idValid = /^[a-z0-9][a-z0-9-]*$/.test(id);
  const canConfirm = !!catalog && !!spec && idValid && !collision && name.trim() !== '' && !saving;

  const selectedProfile = profiles.find((profile) => profile.id === voiceProfileId);
  const selection: VoiceSelection | undefined = selectedProfile
    ? {
        provider: selectedProfile.provider ?? '',
        voice_id: selectedProfile.voice_id,
        pitch: selectedProfile.pitch,
        speech_rate: selectedProfile.speech_rate,
        options: selectedProfile.options,
      }
    : undefined;

  const preview =
    catalog && spec
      ? buildEntityMarkdown(catalog, { id, name: name.trim() || 'Name', type: spec.id, voice: selection })
      : '';

  if (!mounted) return null;

  const confirm = async () => {
    if (!catalog || !spec || !canConfirm) return;
    setSaving(true);
    try {
      await onConfirm({
        id,
        name: name.trim(),
        markdown: buildEntityMarkdown(catalog, { id, name: name.trim(), type: spec.id, voice: selection }),
      });
    } finally {
      setSaving(false);
    }
  };

  return (
    <div
      data-state={state}
      className={`fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm ${
        state === 'enter' ? 'anim-fade-in' : 'anim-fade-out pointer-events-none'
      }`}
      style={{ '--anim-dur': '200ms' } as React.CSSProperties}
    >
      <div
        className={`relative flex max-h-[85vh] w-full max-w-lg flex-col overflow-hidden rounded-2xl border border-purple-500/40 bg-stone-900 shadow-2xl ${
          state === 'enter' ? 'anim-scale-in' : 'anim-scale-out'
        }`}
      >
        <div className="flex items-center justify-between border-b border-stone-800 px-6 py-4">
          <div className="flex items-center gap-2">
            <Sparkles className="h-5 w-5 text-purple-400" />
            <h3 className="font-sans text-base font-bold text-purple-400">New Note</h3>
          </div>
          <button
            onClick={onClose}
            className="cursor-pointer rounded-lg p-1 text-stone-400 hover:bg-stone-800 hover:text-white"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="min-h-0 flex-1 space-y-5 overflow-y-auto p-6">
          {catalogError && (
            <p className="rounded border border-red-900/60 bg-red-950/40 p-2 text-xs text-red-200">{catalogError}</p>
          )}

          <div className="space-y-2">
            <label className="block text-xs uppercase tracking-wider text-stone-400">Name</label>
            <input
              type="text"
              value={name}
              onChange={(e) => {
                setName(e.target.value);
                if (!idTouched) setId(slugify(e.target.value));
              }}
              placeholder="Lady Evelyn Vance"
              className="w-full rounded-xl border border-stone-800 bg-stone-950 px-3 py-2 text-xs text-stone-100 focus:border-purple-500/50 focus:outline-none"
            />
            <input
              type="text"
              value={id}
              onChange={(e) => {
                setIdTouched(true);
                setId(e.target.value);
              }}
              placeholder="lady-evelyn"
              className="w-full rounded-xl border border-stone-800 bg-stone-950 px-3 py-2 font-mono text-xs text-stone-300 focus:border-purple-500/50 focus:outline-none"
            />
            {collision && (
              <p className="text-xs text-amber-300">
                A note with this id already exists.{' '}
                {onOpenExisting && (
                  <button type="button" onClick={() => onOpenExisting(id)} className="underline">
                    Open it instead
                  </button>
                )}
              </p>
            )}
            {!collision && id !== '' && !idValid && (
              <p className="text-xs text-red-300">The id may only contain lowercase letters, digits and hyphens.</p>
            )}
          </div>

          <div className="space-y-2">
            <label className="block text-xs uppercase tracking-wider text-stone-400">Type</label>
            <div className="grid grid-cols-3 gap-1.5">
              {(catalog?.types ?? []).map((candidate) => (
                <button
                  key={candidate.id}
                  type="button"
                  onClick={() => setType(candidate.id)}
                  title={candidate.description}
                  className={`rounded-lg border px-2 py-1.5 text-center font-mono text-xs transition-colors cursor-pointer ${
                    type === candidate.id
                      ? 'border-purple-500 bg-purple-600/30 font-bold text-purple-200'
                      : 'border-stone-800 bg-black/40 text-stone-400 hover:text-stone-200'
                  }`}
                >
                  {candidate.label}
                </button>
              ))}
            </div>
            {spec && <p className="text-xs text-stone-500">{spec.description}</p>}
          </div>

          {wantsVoice && (
            <div className="space-y-2">
              <label className="block text-xs uppercase tracking-wider text-stone-400">Voice</label>
              <VoiceProfileSelect
                profiles={profiles}
                value={voiceProfileId}
                onChange={(profile) => setVoiceProfileId(profile?.id ?? '')}
                ttsConfig={resolvedTTS ?? undefined}
                previewText={`Greetings. I am ${name.trim() || 'ready for the journey'}.`}
                placeholder={profiles.length === 0 ? 'Configure voices in Settings' : 'Select a voice...'}
                allowDefault
                defaultLabel="No voice"
              />
            </div>
          )}

          <div className="space-y-2">
            <label className="block text-xs uppercase tracking-wider text-stone-400">Preview</label>
            <pre className="max-h-40 overflow-auto rounded-lg border border-stone-800 bg-black/40 p-2 font-mono text-[10px] leading-relaxed text-stone-400">
              {preview}
            </pre>
          </div>
        </div>

        <div className="flex items-center justify-end gap-2 border-t border-stone-800 px-6 py-3">
          <button
            type="button"
            onClick={onClose}
            className="cursor-pointer px-3 py-1.5 text-xs text-stone-400 hover:text-white"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={() => void confirm()}
            disabled={!canConfirm}
            className="flex cursor-pointer items-center gap-1.5 rounded-xl bg-purple-600 px-4 py-1.5 text-xs font-bold text-white disabled:cursor-not-allowed disabled:opacity-50"
          >
            <Check className="h-3.5 w-3.5" />
            <span>{confirmLabel}</span>
          </button>
        </div>
      </div>
    </div>
  );
};
