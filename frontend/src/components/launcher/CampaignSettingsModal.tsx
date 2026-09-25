import React, { useState, useEffect, useRef } from 'react';
import { GameSummary, VoiceProfile } from '../../types';
import { APIClient } from '../../api/client';
import { ProceduralBanner, ProceduralIcon } from './ProceduralAsset';
import { X, Upload, Sparkles, AlertTriangle, Volume2, MapPin, Check, Save } from 'lucide-react';
import { AIGenerateButton } from '../ui/AIGenerateButton';

interface CampaignSettingsModalProps {
  isOpen: boolean;
  game: GameSummary | null;
  onClose: () => void;
  onUploadAsset: (gameId: string, kind: 'banner' | 'icon', file: File) => Promise<void>;
  onGenerateAsset: (gameId: string, kind: 'banner' | 'icon') => Promise<void>;
  onRestartGame: (gameId: string) => Promise<void>;
  onDeleteGame: (gameId: string) => Promise<void>;
}

export const CampaignSettingsModal: React.FC<CampaignSettingsModalProps> = ({
  isOpen,
  game,
  onClose,
  onUploadAsset,
  onGenerateAsset,
  onRestartGame,
  onDeleteGame,
}) => {
  const [generatingKind, setGeneratingKind] = useState<'banner' | 'icon' | null>(null);
  const [confirmAction, setConfirmAction] = useState<'restart' | 'delete' | null>(null);
  const [isBusy, setIsBusy] = useState(false);

  // Settings State
  const [narratorVoice, setNarratorVoice] = useState('');
  const [openingPrompt, setOpeningPrompt] = useState('');
  const [startLocation, setStartLocation] = useState('');
  const [voiceProfiles, setVoiceProfiles] = useState<VoiceProfile[]>([]);
  const [isSavingSettings, setIsSavingSettings] = useState(false);
  const [settingsSaved, setSettingsSaved] = useState(false);

  const bannerInputRef = useRef<HTMLInputElement>(null);
  const iconInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (!isOpen || !game) return;

    // Load available voice profiles
    APIClient.getSettings()
      .then((res) => {
        setVoiceProfiles(res.config.media.tts.voice_profiles || []);
      })
      .catch((err) => console.error('Failed to load voice profiles', err));

    // Load current campaign game state
    const client = new APIClient(game.id);
    client
      .getGameState()
      .then((state) => {
        setNarratorVoice(state.narrator_voice || '');
        setOpeningPrompt(state.opening_prompt || '');
        setStartLocation(state.start_location || '');
      })
      .catch((err) => console.error('Failed to load game state settings', err));

    setSettingsSaved(false);
  }, [isOpen, game]);

  if (!isOpen || !game) return null;

  const handleSaveSettings = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsSavingSettings(true);
    setSettingsSaved(false);
    try {
      await APIClient.updateGameSettings(game.id, {
        narrator_voice: narratorVoice.trim() || '',
        opening_prompt: openingPrompt.trim() || '',
        start_location: startLocation.trim() || '',
      });
      setSettingsSaved(true);
      setTimeout(() => setSettingsSaved(false), 3000);
    } catch (err) {
      console.error('Failed to update game settings:', err);
    } finally {
      setIsSavingSettings(false);
    }
  };

  const getCampaignContext = (): Record<string, string> => ({
    campaign_name: game?.name || '',
    start_location: startLocation,
    opening_prompt: openingPrompt,
  });

  const handleBannerUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) {
      setIsBusy(true);
      try {
        await onUploadAsset(game.id, 'banner', file);
      } catch (err) {
        console.error('banner upload failed:', err);
      } finally {
        setIsBusy(false);
      }
    }
  };

  const handleIconUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) {
      setIsBusy(true);
      try {
        await onUploadAsset(game.id, 'icon', file);
      } catch (err) {
        console.error('icon upload failed:', err);
      } finally {
        setIsBusy(false);
      }
    }
  };

  const handleAIGenerate = async (kind: 'banner' | 'icon') => {
    setGeneratingKind(kind);
    try {
      await onGenerateAsset(game.id, kind);
    } catch (err) {
      console.error('AI generation failed:', err);
    } finally {
      setGeneratingKind(null);
    }
  };

  const handleRestart = async () => {
    setIsBusy(true);
    try {
      await onRestartGame(game.id);
      setConfirmAction(null);
      onClose();
    } catch (err) {
      console.error('restart failed:', err);
    } finally {
      setIsBusy(false);
    }
  };

  const handleDelete = async () => {
    setIsBusy(true);
    try {
      await onDeleteGame(game.id);
      setConfirmAction(null);
      onClose();
    } catch (err) {
      console.error('delete failed:', err);
    } finally {
      setIsBusy(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/75 backdrop-blur-md select-none animate-in fade-in duration-200">
      <div className="relative w-full max-w-xl bg-stone-900/95 border border-white/15 rounded-3xl overflow-hidden shadow-2xl flex flex-col max-h-[90vh]">
        {/* Header */}
        <div className="p-5 px-6 border-b border-white/10 flex items-center justify-between bg-stone-950/50">
          <div>
            <h2 className="text-lg font-sans font-bold text-white">Campaign Settings</h2>
            <div className="text-xs font-sans text-stone-400 mt-0.5">{game.name}</div>
          </div>
          <button
            onClick={onClose}
            className="w-8 h-8 rounded-full bg-white/[0.05] hover:bg-white/[0.1] border border-white/10 flex items-center justify-center text-stone-400 hover:text-white transition-all cursor-pointer"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* Content */}
        <div className="p-6 overflow-y-auto space-y-6">
          {/* Narrative & Audio Settings */}
          <form onSubmit={handleSaveSettings} className="space-y-4 p-4 bg-stone-950 border border-white/10 rounded-2xl">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-1.5 text-xs font-sans font-bold uppercase tracking-wider text-purple-400">
                <Volume2 className="w-3.5 h-3.5" />
                <span>Narrative & Audio Settings</span>
              </div>
              {settingsSaved && (
                <span className="flex items-center gap-1 text-[11px] font-sans text-emerald-400 bg-emerald-950/50 border border-emerald-500/30 px-2 py-0.5 rounded-full">
                  <Check className="w-3 h-3" />
                  Saved
                </span>
              )}
            </div>

            {/* Narrator Voice */}
            <div className="space-y-1">
              <label className="text-[11px] font-sans text-stone-300">
                Narrator Voice
              </label>
              <select
                value={narratorVoice}
                onChange={(e) => setNarratorVoice(e.target.value)}
                className="w-full bg-stone-900 border border-white/10 rounded-xl px-3 py-2 text-xs text-stone-100 focus:outline-none focus:border-purple-500 cursor-pointer"
              >
                <option value="">Default (Provider Setting)</option>
                {voiceProfiles.map((p) => (
                  <option key={p.id} value={p.voice_id}>
                    {p.name} ({p.voice_id})
                  </option>
                ))}
              </select>
              <p className="text-[10px] font-sans text-stone-500">
                Voice used to narrate scenes, descriptions, and GM responses in this campaign.
              </p>
            </div>

            {/* Start Location */}
            <div className="space-y-1">
              <div className="flex items-center justify-between">
                <label className="text-[11px] font-sans text-stone-300 flex items-center gap-1">
                  <MapPin className="w-3 h-3 text-stone-400" />
                  <span>Start Location Directive</span>
                </label>
                <AIGenerateButton
                  formType="campaign"
                  fieldName="start_location"
                  getContext={getCampaignContext}
                  onGenerated={(val) => setStartLocation(val)}
                  worldID={game?.world_id}
                  systemID={game?.system_id}
                />
              </div>
              <input
                type="text"
                value={startLocation}
                onChange={(e) => setStartLocation(e.target.value)}
                placeholder="e.g. Old Harbour District"
                className="w-full bg-stone-900 border border-white/10 rounded-xl px-3 py-2 text-xs text-stone-200 focus:outline-none focus:border-purple-500"
              />
            </div>

            {/* Opening Scene Prompt */}
            <div className="space-y-1">
              <div className="flex items-center justify-between">
                <label className="text-[11px] font-sans text-stone-300">
                  Opening Scene Directive
                </label>
                <AIGenerateButton
                  formType="campaign"
                  fieldName="opening_prompt"
                  getContext={getCampaignContext}
                  onGenerated={(val) => setOpeningPrompt(val)}
                  worldID={game?.world_id}
                  systemID={game?.system_id}
                  seed={openingPrompt}
                />
              </div>
              <textarea
                rows={2}
                value={openingPrompt}
                onChange={(e) => setOpeningPrompt(e.target.value)}
                placeholder="Where the story begins..."
                className="w-full bg-stone-900 border border-white/10 rounded-xl px-3 py-2 text-xs text-stone-200 focus:outline-none focus:border-purple-500 resize-none"
              />
            </div>

            <div className="flex justify-end pt-1">
              <button
                type="submit"
                disabled={isSavingSettings}
                className="flex items-center gap-1.5 px-4 py-2 rounded-xl bg-purple-600 hover:bg-purple-500 disabled:opacity-50 text-white font-sans font-bold text-xs shadow-md transition-all cursor-pointer"
              >
                <Save className="w-3.5 h-3.5" />
                <span>{isSavingSettings ? 'Saving...' : 'Save Settings'}</span>
              </button>
            </div>
          </form>

          {/* Artwork Management */}
          <div className="space-y-3">
            <h3 className="text-xs font-sans font-semibold text-stone-300 uppercase tracking-wider">
              Display Artwork
            </h3>

            <div className="grid grid-cols-2 gap-4">
              {/* Banner Artwork */}
              <div className="p-3 bg-stone-950 border border-white/10 rounded-2xl flex flex-col justify-between gap-3">
                <span className="text-[11px] font-sans font-semibold text-stone-300">Main Banner</span>
                <div className="h-24 rounded-xl overflow-hidden relative border border-white/10">
                  {game.banner_url ? (
                    <img src={game.banner_url} alt="Banner" className="w-full h-full object-cover" />
                  ) : (
                    <ProceduralBanner id={game.id} name={game.name} className="w-full h-full" />
                  )}
                </div>
                <div className="flex gap-2">
                  <input
                    type="file"
                    ref={bannerInputRef}
                    onChange={handleBannerUpload}
                    accept="image/png,image/jpeg,image/webp,image/svg+xml"
                    className="hidden"
                  />
                  <button
                    type="button"
                    onClick={() => bannerInputRef.current?.click()}
                    disabled={isBusy}
                    className="flex-1 py-1.5 px-2 rounded-lg bg-white/[0.05] hover:bg-white/[0.1] border border-white/10 text-xs font-sans font-semibold text-stone-200 flex items-center justify-center gap-1.5 transition-all cursor-pointer"
                  >
                    <Upload className="w-3.5 h-3.5" />
                    <span>Upload</span>
                  </button>
                  <button
                    type="button"
                    onClick={() => handleAIGenerate('banner')}
                    disabled={generatingKind === 'banner' || isBusy}
                    className="flex-1 py-1.5 px-2 rounded-lg bg-purple-600/20 hover:bg-purple-600/30 border border-purple-500/40 text-xs font-sans font-semibold text-purple-300 flex items-center justify-center gap-1.5 transition-all cursor-pointer"
                  >
                    <Sparkles className="w-3.5 h-3.5" />
                    <span>{generatingKind === 'banner' ? 'Gen...' : 'AI Gen'}</span>
                  </button>
                </div>
              </div>

              {/* Icon Artwork */}
              <div className="p-3 bg-stone-950 border border-white/10 rounded-2xl flex flex-col justify-between gap-3">
                <span className="text-[11px] font-sans font-semibold text-stone-300">Campaign Icon</span>
                <div className="h-24 rounded-xl overflow-hidden relative border border-white/10 flex items-center justify-center bg-stone-900">
                  {game.icon_url ? (
                    <img src={game.icon_url} alt="Icon" className="w-16 h-16 rounded-xl object-cover" />
                  ) : (
                    <ProceduralIcon id={game.id} name={game.name} size={64} />
                  )}
                </div>
                <div className="flex gap-2">
                  <input
                    type="file"
                    ref={iconInputRef}
                    onChange={handleIconUpload}
                    accept="image/png,image/jpeg,image/webp,image/svg+xml"
                    className="hidden"
                  />
                  <button
                    type="button"
                    onClick={() => iconInputRef.current?.click()}
                    disabled={isBusy}
                    className="flex-1 py-1.5 px-2 rounded-lg bg-white/[0.05] hover:bg-white/[0.1] border border-white/10 text-xs font-sans font-semibold text-stone-200 flex items-center justify-center gap-1.5 transition-all cursor-pointer"
                  >
                    <Upload className="w-3.5 h-3.5" />
                    <span>Upload</span>
                  </button>
                  <button
                    type="button"
                    onClick={() => handleAIGenerate('icon')}
                    disabled={generatingKind === 'icon' || isBusy}
                    className="flex-1 py-1.5 px-2 rounded-lg bg-purple-600/20 hover:bg-purple-600/30 border border-purple-500/40 text-xs font-sans font-semibold text-purple-300 flex items-center justify-center gap-1.5 transition-all cursor-pointer"
                  >
                    <Sparkles className="w-3.5 h-3.5" />
                    <span>{generatingKind === 'icon' ? 'Gen...' : 'AI Gen'}</span>
                  </button>
                </div>
              </div>
            </div>
          </div>

          {/* Danger Zone */}
          <div className="p-4 bg-red-950/20 border border-red-500/30 rounded-2xl space-y-3">
            <div className="flex items-center gap-2 text-xs font-sans font-bold text-red-400">
              <AlertTriangle className="w-4 h-4" />
              <span>Danger Zone</span>
            </div>

            {/* Restart */}
            <div className="flex items-center justify-between pt-1">
              <div>
                <div className="text-xs font-sans font-semibold text-stone-200">Restart Campaign</div>
                <div className="text-[11px] font-sans text-stone-400">Resets timeline to Turn 0. Retains character & world.</div>
              </div>
              {confirmAction === 'restart' ? (
                <div className="flex items-center gap-2">
                  <button
                    onClick={() => setConfirmAction(null)}
                    className="px-2.5 py-1 text-xs text-stone-400 hover:text-white"
                  >
                    Cancel
                  </button>
                  <button
                    onClick={handleRestart}
                    disabled={isBusy}
                    className="px-3 py-1 rounded-lg bg-red-600 hover:bg-red-500 text-white font-sans text-xs font-bold shadow-md cursor-pointer"
                  >
                    Confirm Restart
                  </button>
                </div>
              ) : (
                <button
                  onClick={() => setConfirmAction('restart')}
                  className="px-3 py-1.5 rounded-lg bg-stone-800 hover:bg-stone-700 text-stone-300 border border-white/10 text-xs font-sans font-semibold cursor-pointer"
                >
                  Restart
                </button>
              )}
            </div>

            <div className="w-full h-[1px] bg-red-500/10" />

            {/* Delete */}
            <div className="flex items-center justify-between">
              <div>
                <div className="text-xs font-sans font-semibold text-stone-200">Delete Campaign</div>
                <div className="text-[11px] font-sans text-stone-400">Permanently removes campaign files and history.</div>
              </div>
              {confirmAction === 'delete' ? (
                <div className="flex items-center gap-2">
                  <button
                    onClick={() => setConfirmAction(null)}
                    className="px-2.5 py-1 text-xs text-stone-400 hover:text-white"
                  >
                    Cancel
                  </button>
                  <button
                    onClick={handleDelete}
                    disabled={isBusy}
                    className="px-3 py-1 rounded-lg bg-red-700 hover:bg-red-600 text-white font-sans text-xs font-bold shadow-md cursor-pointer"
                  >
                    Confirm Delete
                  </button>
                </div>
              ) : (
                <button
                  onClick={() => setConfirmAction('delete')}
                  className="px-3 py-1.5 rounded-lg bg-red-950/60 hover:bg-red-900/60 text-red-300 border border-red-500/40 text-xs font-sans font-semibold cursor-pointer"
                >
                  Delete
                </button>
              )}
            </div>
          </div>
        </div>

        {/* Footer */}
        <div className="p-4 px-6 border-t border-white/10 flex justify-end bg-stone-950/50">
          <button
            onClick={onClose}
            className="px-5 py-2 rounded-xl text-xs font-sans font-semibold text-stone-300 hover:text-white bg-white/[0.05] hover:bg-white/[0.1] border border-white/10 transition-all cursor-pointer"
          >
            Close
          </button>
        </div>
      </div>
    </div>
  );
};
