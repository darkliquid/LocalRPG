import React, { useState, useEffect, useRef } from 'react';
import { WorldInfo, SystemInfo, CreateGameRequest, VoiceProfile } from '../../types';
import { APIClient } from '../../api/client';
import { ProceduralBanner, ProceduralIcon } from './ProceduralAsset';
import { X, Upload, Check, Volume2, MapPin, Sparkles, User, Wand2 } from 'lucide-react';
import { AIGenerateButton } from '../ui/AIGenerateButton';

interface NewCampaignModalProps {
  isOpen: boolean;
  world: WorldInfo | null;
  systems: SystemInfo[];
  onClose: () => void;
  onCreateGame: (data: CreateGameRequest, bannerFile?: File, iconFile?: File) => Promise<void>;
  isSubmitting: boolean;
}

export const NewCampaignModal: React.FC<NewCampaignModalProps> = ({
  isOpen,
  world,
  systems,
  onClose,
  onCreateGame,
  isSubmitting,
}) => {
  const [campaignName, setCampaignName] = useState('');
  const [selectedSystemID, setSelectedSystemID] = useState('');
  const [playerName, setPlayerName] = useState('');
  const [playerAppearance, setPlayerAppearance] = useState('');
  const [playerAge, setPlayerAge] = useState('');
  const [playerGender, setPlayerGender] = useState('');
  const [playerPronouns, setPlayerPronouns] = useState('');
  const [playerBackground, setPlayerBackground] = useState('');
  const [playerVoiceID, setPlayerVoiceID] = useState('');
  const [narratorVoice, setNarratorVoice] = useState('');
  const [openingPrompt, setOpeningPrompt] = useState('');
  const [startLocation, setStartLocation] = useState('');
  const [isGeneratingAll, setIsGeneratingAll] = useState(false);
  const [generatingKind, setGeneratingKind] = useState<'banner' | 'icon' | null>(null);
  const [genError, setGenError] = useState<string | null>(null);

  const [voiceProfiles, setVoiceProfiles] = useState<VoiceProfile[]>([]);
  const [bannerFile, setBannerFile] = useState<File | null>(null);
  const [iconFile, setIconFile] = useState<File | null>(null);
  const [bannerPreview, setBannerPreview] = useState<string | null>(null);
  const [iconPreview, setIconPreview] = useState<string | null>(null);

  const bannerInputRef = useRef<HTMLInputElement>(null);
  const iconInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    APIClient.getSettings()
      .then((res) => {
        setVoiceProfiles(res.config.media.tts.voice_profiles || []);
      })
      .catch((err) => console.error('Failed to load voice profiles', err));
  }, []);

  useEffect(() => {
    if (world) {
      setCampaignName(`Chronicles of ${world.name}`);
      const compatible = systems.find((s) => world.compatible_systems?.includes(s.id));
      setSelectedSystemID(compatible ? compatible.id : systems[0]?.id || '');
      setPlayerName('');
      setPlayerAppearance('');
      setPlayerAge('');
      setPlayerGender('');
      setPlayerPronouns('');
      setPlayerBackground('');
      setPlayerVoiceID('');
      setNarratorVoice('');
      setOpeningPrompt('');
      setStartLocation('');
      setBannerFile(null);
      setIconFile(null);
      setBannerPreview(null);
      setIconPreview(null);
    }
  }, [world, systems]);

  if (!isOpen || !world) return null;

  const handleBannerSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) {
      setBannerFile(file);
      setBannerPreview(URL.createObjectURL(file));
    }
  };

  const handleIconSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) {
      setIconFile(file);
      setIconPreview(URL.createObjectURL(file));
    }
  };

  const getFormContext = (): Record<string, string> => ({
    world_name: world?.name || '',
    world_description: world?.description || '',
    world_genre: world?.genre || '',
    campaign_name: campaignName,
    player_name: playerName,
    player_appearance: playerAppearance,
    player_background: playerBackground,
    player_age: playerAge,
    player_gender: playerGender,
    player_pronouns: playerPronouns,
    start_location: startLocation,
    opening_prompt: openingPrompt,
  });

  const handleGenerateAll = async () => {
    if (isGeneratingAll) return;
    setIsGeneratingAll(true);
    try {
      const ctx = getFormContext();
      // Generate campaign directives
      const campRes = await APIClient.generateText({
        form_type: 'campaign',
        field_name: '_all',
        context: ctx,
        world_id: world?.id,
        system_id: selectedSystemID,
      });
      if (campRes.fields.name && !campaignName.trim()) setCampaignName(campRes.fields.name);
      if (campRes.fields.start_location && !startLocation.trim()) setStartLocation(campRes.fields.start_location);
      if (campRes.fields.opening_prompt && !openingPrompt.trim()) setOpeningPrompt(campRes.fields.opening_prompt);

      // Generate character fields
      const charRes = await APIClient.generateText({
        form_type: 'character',
        field_name: '_all',
        context: { ...ctx, ...campRes.fields },
        world_id: world?.id,
        system_id: selectedSystemID,
      });
      if (charRes.fields.name && !playerName.trim()) setPlayerName(charRes.fields.name);
      if (charRes.fields.appearance && !playerAppearance.trim()) setPlayerAppearance(charRes.fields.appearance);
      if (charRes.fields.background && !playerBackground.trim()) setPlayerBackground(charRes.fields.background);
      if (charRes.fields.age && !playerAge.trim()) setPlayerAge(charRes.fields.age);
      if (charRes.fields.gender && !playerGender.trim()) setPlayerGender(charRes.fields.gender);
      if (charRes.fields.pronouns && !playerPronouns.trim()) setPlayerPronouns(charRes.fields.pronouns);
    } catch (err) {
      console.error('Failed to generate all fields:', err);
    } finally {
      setIsGeneratingAll(false);
    }
  };

  const handleAIGenerate = async (kind: 'banner' | 'icon') => {
    if (!world) return;
    setGeneratingKind(kind);
    setGenError(null);
    try {
      const blob = await APIClient.generateAssetPreview(
        kind,
        campaignName.trim() || world.name,
        world.description || '',
        world.art_style || '',
        world.genre || ''
      );
      const file = new File([blob], `${kind}.png`, { type: blob.type });
      if (kind === 'banner') {
        setBannerFile(file);
        setBannerPreview(URL.createObjectURL(blob));
      } else {
        setIconFile(file);
        setIconPreview(URL.createObjectURL(blob));
      }
    } catch (err: any) {
      console.error('Failed to generate preview', err);
      setGenError(err.message || 'Generation failed');
    } finally {
      setGeneratingKind(null);
    }
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!campaignName.trim() || !selectedSystemID || !playerName.trim()) return;

    onCreateGame(
      {
        name: campaignName.trim(),
        system_id: selectedSystemID,
        world_id: world.id,
        player_name: playerName.trim(),
        narrator_voice: narratorVoice.trim() || undefined,
        start_location: startLocation.trim() || undefined,
        opening_prompt: openingPrompt.trim() || undefined,
        player: {
          appearance: playerAppearance.trim() || undefined,
          age: playerAge.trim() || undefined,
          gender: playerGender.trim() || undefined,
          pronouns: playerPronouns.trim() || undefined,
          background: playerBackground.trim() || undefined,
          voice: voiceProfiles.find((p) => p.voice_id === playerVoiceID),
        },
      },
      bannerFile || undefined,
      iconFile || undefined
    );
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/75 backdrop-blur-md select-none animate-in fade-in duration-200">
      <div className="relative w-full max-w-2xl bg-stone-900/95 border border-white/15 rounded-3xl overflow-hidden shadow-2xl flex flex-col max-h-[90vh]">
        {/* Modal Banner Header - Absolute Background Layer */}
        <div className="relative h-44 w-full overflow-hidden border-b border-white/10 select-none shrink-0">
          <div className="absolute inset-0 z-0 pointer-events-none">
            {world.banner_url ? (
              <img src={world.banner_url} alt={world.name} className="w-full h-full object-cover" />
            ) : (
              <ProceduralBanner id={world.id} name={world.name} className="w-full h-full" />
            )}
            <div className="absolute inset-0 bg-gradient-to-t from-stone-900 via-stone-900/60 to-black/30" />
          </div>

          {/* Floating Header Content */}
          <div className="relative z-10 h-full flex items-end p-6">
            <div className="flex items-center gap-4">
              <div className="w-16 h-16 rounded-2xl overflow-hidden shadow-2xl border-2 border-white/20 shrink-0">
                {world.icon_url ? (
                  <img src={world.icon_url} alt={world.name} className="w-full h-full object-cover" />
                ) : (
                  <ProceduralIcon id={world.id} name={world.name} genre={world.genre} size={64} className="w-full h-full rounded-none" />
                )}
              </div>
              <div>
                <div className="flex items-center gap-2">
                  <h2 className="text-xl font-sans font-extrabold text-white tracking-tight">
                    New Campaign
                  </h2>
                  <span className="text-[11px] font-sans font-semibold bg-purple-500/20 text-purple-300 border border-purple-500/30 px-2 py-0.5 rounded-full">
                    {world.name}
                  </span>
                </div>
                <p className="text-xs font-sans text-stone-300 mt-1 line-clamp-2 max-w-lg">
                  {world.description || 'Explore uncharted territory and shape the fate of this realm.'}
                </p>
              </div>
            </div>
          </div>

          {/* Close button */}
          <button
            onClick={onClose}
            className="absolute top-4 right-4 z-20 w-8 h-8 rounded-full bg-black/50 border border-white/20 flex items-center justify-center text-white/80 hover:text-white hover:bg-black/70 transition-all cursor-pointer"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* Modal Form */}
        <form onSubmit={handleSubmit} className="flex-1 overflow-y-auto p-6 space-y-5">
          {/* Campaign Title */}
          <div className="space-y-1.5">
            <div className="flex items-center justify-between">
              <label className="text-xs font-sans font-semibold text-stone-300 uppercase tracking-wider">
                Campaign Title
              </label>
              <AIGenerateButton
                formType="campaign"
                fieldName="name"
                getContext={getFormContext}
                onGenerated={(val) => setCampaignName(val)}
                worldID={world?.id}
                systemID={selectedSystemID}
              />
            </div>
            <input
              type="text"
              required
              value={campaignName}
              onChange={(e) => setCampaignName(e.target.value)}
              placeholder="e.g. Chronicles of Eldoria"
              className="w-full bg-stone-950 border border-white/10 rounded-xl px-3.5 py-2.5 text-sm font-sans text-white focus:outline-none focus:border-purple-500 transition-colors"
            />
          </div>

          {/* Rules System Picker */}
          <div className="space-y-1.5">
            <div className="flex justify-between items-center">
              <label className="text-xs font-sans font-semibold text-stone-300 uppercase tracking-wider">
                Rules System
              </label>
              <span className="text-[11px] font-sans text-purple-400">
                {world.compatible_systems?.length ? 'Compatible systems highlighted' : 'All systems'}
              </span>
            </div>
            <div className="grid grid-cols-2 gap-2 max-h-36 overflow-y-auto no-scrollbar">
              {systems.map((s) => {
                const isSelected = s.id === selectedSystemID;
                const isCompatible = world.compatible_systems?.includes(s.id);
                return (
                  <button
                    key={s.id}
                    type="button"
                    onClick={() => setSelectedSystemID(s.id)}
                    className={`text-left p-3 rounded-xl border transition-all cursor-pointer ${
                      isSelected
                        ? 'bg-purple-600/20 border-purple-500 text-white shadow-[0_0_12px_rgba(168,85,247,0.3)]'
                        : isCompatible
                        ? 'bg-stone-950 border-white/15 text-stone-200 hover:border-white/30'
                        : 'bg-stone-950/50 border-white/5 text-stone-400 hover:border-white/20'
                    }`}
                  >
                    <div className="flex items-center justify-between">
                      <div className="text-xs font-sans font-bold text-white">{s.name}</div>
                      {isSelected && <Check className="w-3.5 h-3.5 text-purple-400" />}
                    </div>
                    <div className="text-[11px] font-sans text-stone-400 mt-0.5 line-clamp-1">
                      {s.description || 'Rules system'}
                    </div>
                  </button>
                );
              })}
            </div>
          </div>

          {/* Voices Section */}
          <div className="p-3.5 bg-stone-950/70 border border-white/10 rounded-2xl space-y-3">
            <div className="flex items-center gap-1.5 text-xs font-sans font-bold uppercase tracking-wider text-purple-400">
              <Volume2 className="w-3.5 h-3.5" />
              <span>Voice & Narration</span>
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
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
              </div>

              {/* Character Voice */}
              <div className="space-y-1">
                <label className="text-[11px] font-sans text-stone-300">
                  Protagonist Voice
                </label>
                <select
                  value={playerVoiceID}
                  onChange={(e) => setPlayerVoiceID(e.target.value)}
                  className="w-full bg-stone-900 border border-white/10 rounded-xl px-3 py-2 text-xs text-stone-100 focus:outline-none focus:border-purple-500 cursor-pointer"
                >
                  <option value="">Auto-Assign from Character</option>
                  {voiceProfiles.map((p) => (
                    <option key={p.id} value={p.voice_id}>
                      {p.name} ({p.voice_id})
                    </option>
                  ))}
                </select>
              </div>
            </div>
          </div>

          {/* Protagonist Section */}
          <div className="p-3.5 bg-stone-950/70 border border-white/10 rounded-2xl space-y-3">
            <div className="flex items-center gap-1.5 text-xs font-sans font-bold uppercase tracking-wider text-purple-400">
              <User className="w-3.5 h-3.5" />
              <span>Protagonist Setup</span>
            </div>

            <div className="space-y-1.5">
              <div className="flex items-center justify-between">
                <label className="text-[11px] font-sans text-stone-300">
                  Character Name <span className="text-purple-400">*</span>
                </label>
                <AIGenerateButton
                  formType="character"
                  fieldName="name"
                  getContext={getFormContext}
                  onGenerated={(val) => setPlayerName(val)}
                  worldID={world?.id}
                  systemID={selectedSystemID}
                />
              </div>
              <input
                type="text"
                required
                value={playerName}
                onChange={(e) => setPlayerName(e.target.value)}
                placeholder="e.g. Valen Duskwarden"
                className="w-full bg-stone-900 border border-white/10 rounded-xl px-3 py-2 text-xs font-sans text-white focus:outline-none focus:border-purple-500 transition-colors"
              />
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-3 gap-2.5">
              <div className="space-y-1">
                <div className="flex items-center justify-between">
                  <label className="text-[11px] font-sans text-stone-400">Age</label>
                  <AIGenerateButton
                    formType="character"
                    fieldName="age"
                    getContext={getFormContext}
                    onGenerated={(val) => setPlayerAge(val)}
                    worldID={world?.id}
                    systemID={selectedSystemID}
                  />
                </div>
                <input
                  type="text"
                  value={playerAge}
                  onChange={(e) => setPlayerAge(e.target.value)}
                  placeholder="e.g. 28"
                  className="w-full bg-stone-900 border border-white/10 rounded-xl px-3 py-1.5 text-xs text-stone-200 focus:outline-none focus:border-purple-500"
                />
              </div>
              <div className="space-y-1">
                <div className="flex items-center justify-between">
                  <label className="text-[11px] font-sans text-stone-400">Gender</label>
                  <AIGenerateButton
                    formType="character"
                    fieldName="gender"
                    getContext={getFormContext}
                    onGenerated={(val) => setPlayerGender(val)}
                    worldID={world?.id}
                    systemID={selectedSystemID}
                  />
                </div>
                <input
                  type="text"
                  value={playerGender}
                  onChange={(e) => setPlayerGender(e.target.value)}
                  placeholder="e.g. Male / Non-binary"
                  className="w-full bg-stone-900 border border-white/10 rounded-xl px-3 py-1.5 text-xs text-stone-200 focus:outline-none focus:border-purple-500"
                />
              </div>
              <div className="space-y-1">
                <div className="flex items-center justify-between">
                  <label className="text-[11px] font-sans text-stone-400">Pronouns</label>
                  <AIGenerateButton
                    formType="character"
                    fieldName="pronouns"
                    getContext={getFormContext}
                    onGenerated={(val) => setPlayerPronouns(val)}
                    worldID={world?.id}
                    systemID={selectedSystemID}
                  />
                </div>
                <input
                  type="text"
                  value={playerPronouns}
                  onChange={(e) => setPlayerPronouns(e.target.value)}
                  placeholder="e.g. they/them"
                  className="w-full bg-stone-900 border border-white/10 rounded-xl px-3 py-1.5 text-xs text-stone-200 focus:outline-none focus:border-purple-500"
                />
              </div>
            </div>

            <div className="space-y-1">
              <div className="flex items-center justify-between">
                <label className="text-[11px] font-sans text-stone-400">Appearance</label>
                <AIGenerateButton
                  formType="character"
                  fieldName="appearance"
                  getContext={getFormContext}
                  onGenerated={(val) => setPlayerAppearance(val)}
                  worldID={world?.id}
                  systemID={selectedSystemID}
                  seed={playerAppearance}
                />
              </div>
              <input
                type="text"
                value={playerAppearance}
                onChange={(e) => setPlayerAppearance(e.target.value)}
                placeholder="e.g. Tall, cloaked in charcoal wool, weathered silver eyes"
                className="w-full bg-stone-900 border border-white/10 rounded-xl px-3 py-1.5 text-xs text-stone-200 focus:outline-none focus:border-purple-500"
              />
            </div>

            <div className="space-y-1">
              <div className="flex items-center justify-between">
                <label className="text-[11px] font-sans text-stone-400">Background / Origin</label>
                <AIGenerateButton
                  formType="character"
                  fieldName="background"
                  getContext={getFormContext}
                  onGenerated={(val) => setPlayerBackground(val)}
                  worldID={world?.id}
                  systemID={selectedSystemID}
                  seed={playerBackground}
                />
              </div>
              <textarea
                rows={2}
                value={playerBackground}
                onChange={(e) => setPlayerBackground(e.target.value)}
                placeholder="e.g. Disgraced royal archivist searching for forbidden relics"
                className="w-full bg-stone-900 border border-white/10 rounded-xl px-3 py-1.5 text-xs text-stone-200 focus:outline-none focus:border-purple-500 resize-none"
              />
            </div>
          </div>

          {/* Story Directive & Start Location */}
          <div className="p-3.5 bg-stone-950/70 border border-white/10 rounded-2xl space-y-3">
            <div className="flex items-center gap-1.5 text-xs font-sans font-bold uppercase tracking-wider text-purple-400">
              <Sparkles className="w-3.5 h-3.5" />
              <span>Story & World Seed</span>
            </div>

            <div className="space-y-1">
              <div className="flex items-center justify-between">
                <label className="text-[11px] font-sans text-stone-300 flex items-center gap-1">
                  <MapPin className="w-3 h-3 text-stone-400" />
                  <span>Start Location (Optional)</span>
                </label>
                <AIGenerateButton
                  formType="campaign"
                  fieldName="start_location"
                  getContext={getFormContext}
                  onGenerated={(val) => setStartLocation(val)}
                  worldID={world?.id}
                  systemID={selectedSystemID}
                />
              </div>
              <input
                type="text"
                value={startLocation}
                onChange={(e) => setStartLocation(e.target.value)}
                placeholder="e.g. Old Harbour District (leave blank to let GM choose)"
                className="w-full bg-stone-900 border border-white/10 rounded-xl px-3 py-2 text-xs text-stone-200 focus:outline-none focus:border-purple-500"
              />
            </div>

            <div className="space-y-1">
              <div className="flex items-center justify-between">
                <label className="text-[11px] font-sans text-stone-300">
                  Opening Scene Directive (Optional)
                </label>
                <AIGenerateButton
                  formType="campaign"
                  fieldName="opening_prompt"
                  getContext={getFormContext}
                  onGenerated={(val) => setOpeningPrompt(val)}
                  worldID={world?.id}
                  systemID={selectedSystemID}
                  seed={openingPrompt}
                />
              </div>
              <textarea
                rows={2}
                value={openingPrompt}
                onChange={(e) => setOpeningPrompt(e.target.value)}
                placeholder="Where should the story begin? (e.g. You awaken in the hold of a smuggler's ship during a tempest...)"
                className="w-full bg-stone-900 border border-white/10 rounded-xl px-3 py-2 text-xs text-stone-200 focus:outline-none focus:border-purple-500 resize-none"
              />
            </div>
          </div>

          {/* Custom Artwork (Banner & Icon Uploads) */}
          <div className="p-3.5 bg-stone-950/70 border border-white/10 rounded-2xl space-y-3">
            <div>
              <div className="text-xs font-sans font-bold text-white">Custom Campaign Artwork (Optional)</div>
              <div className="text-[11px] font-sans text-stone-400">
                A procedural gradient theme will be generated if omitted.
              </div>
              {genError && (
                <div className="text-[11px] text-red-400 font-sans mt-1">{genError}</div>
              )}
            </div>

            <div className="grid grid-cols-2 gap-3 pt-1">
              {/* Banner Upload & Generate */}
              <div className="space-y-2">
                <input
                  type="file"
                  ref={bannerInputRef}
                  onChange={handleBannerSelect}
                  accept="image/png,image/jpeg,image/webp,image/svg+xml"
                  className="hidden"
                />
                <div
                  onClick={() => bannerInputRef.current?.click()}
                  className="w-full h-16 rounded-xl border border-dashed border-white/20 hover:border-purple-400/60 bg-white/[0.02] flex items-center justify-center text-xs font-sans text-stone-300 hover:text-white transition-all cursor-pointer overflow-hidden"
                >
                  {bannerPreview ? (
                    <img src={bannerPreview} alt="Banner Preview" className="w-full h-full object-cover" />
                  ) : (
                    <span className="text-stone-500 text-[11px]">No Banner Selected</span>
                  )}
                </div>
                <div className="flex gap-2">
                  <button
                    type="button"
                    onClick={() => bannerInputRef.current?.click()}
                    disabled={isSubmitting || generatingKind === 'banner'}
                    className="flex-1 py-1.5 px-2 rounded-lg bg-white/[0.05] hover:bg-white/[0.1] border border-white/10 text-xs font-sans font-semibold text-stone-200 flex items-center justify-center gap-1.5 transition-all cursor-pointer disabled:opacity-50"
                  >
                    <Upload className="w-3.5 h-3.5" />
                    <span>Upload</span>
                  </button>
                  <button
                    type="button"
                    onClick={() => handleAIGenerate('banner')}
                    disabled={isSubmitting || generatingKind === 'banner'}
                    className="flex-1 py-1.5 px-2 rounded-lg bg-purple-600/20 hover:bg-purple-600/30 border border-purple-500/40 text-xs font-sans font-semibold text-purple-300 flex items-center justify-center gap-1.5 transition-all cursor-pointer disabled:opacity-50"
                  >
                    <Sparkles className="w-3.5 h-3.5" />
                    <span>{generatingKind === 'banner' ? 'Gen...' : 'AI Gen'}</span>
                  </button>
                </div>
              </div>

              {/* Icon Upload & Generate */}
              <div className="space-y-2">
                <input
                  type="file"
                  ref={iconInputRef}
                  onChange={handleIconSelect}
                  accept="image/png,image/jpeg,image/webp,image/svg+xml"
                  className="hidden"
                />
                <div
                  onClick={() => iconInputRef.current?.click()}
                  className="w-full h-16 rounded-xl border border-dashed border-white/20 hover:border-purple-400/60 bg-white/[0.02] flex items-center justify-center text-xs font-sans text-stone-300 hover:text-white transition-all cursor-pointer overflow-hidden"
                >
                  {iconPreview ? (
                    <img src={iconPreview} alt="Icon Preview" className="w-12 h-12 rounded-lg object-cover" />
                  ) : (
                    <span className="text-stone-500 text-[11px]">No Icon Selected</span>
                  )}
                </div>
                <div className="flex gap-2">
                  <button
                    type="button"
                    onClick={() => iconInputRef.current?.click()}
                    disabled={isSubmitting || generatingKind === 'icon'}
                    className="flex-1 py-1.5 px-2 rounded-lg bg-white/[0.05] hover:bg-white/[0.1] border border-white/10 text-xs font-sans font-semibold text-stone-200 flex items-center justify-center gap-1.5 transition-all cursor-pointer disabled:opacity-50"
                  >
                    <Upload className="w-3.5 h-3.5" />
                    <span>Upload</span>
                  </button>
                  <button
                    type="button"
                    onClick={() => handleAIGenerate('icon')}
                    disabled={isSubmitting || generatingKind === 'icon'}
                    className="flex-1 py-1.5 px-2 rounded-lg bg-purple-600/20 hover:bg-purple-600/30 border border-purple-500/40 text-xs font-sans font-semibold text-purple-300 flex items-center justify-center gap-1.5 transition-all cursor-pointer disabled:opacity-50"
                  >
                    <Sparkles className="w-3.5 h-3.5" />
                    <span>{generatingKind === 'icon' ? 'Gen...' : 'AI Gen'}</span>
                  </button>
                </div>
              </div>
            </div>
          </div>

          {/* Footer Actions */}
          <div className="flex items-center justify-end gap-3 pt-2">
            <button
              type="button"
              onClick={handleGenerateAll}
              disabled={isSubmitting || isGeneratingAll}
              className="px-3 py-2 rounded-xl border border-purple-500/30 bg-purple-600/10 hover:bg-purple-600/20 text-purple-300 text-xs font-sans font-semibold flex items-center gap-1.5 transition-all cursor-pointer disabled:opacity-50"
            >
              <Wand2 className={`w-3.5 h-3.5 ${isGeneratingAll ? 'animate-spin' : ''}`} />
              <span>{isGeneratingAll ? 'Generating...' : 'Auto-Fill Fields'}</span>
            </button>
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-2 rounded-xl text-xs font-sans font-semibold text-stone-400 hover:text-white transition-colors cursor-pointer"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={isSubmitting || !campaignName.trim() || !playerName.trim()}
              className="px-6 py-2.5 rounded-xl bg-purple-600 hover:bg-purple-500 disabled:opacity-50 text-white font-sans font-bold text-xs shadow-lg shadow-purple-600/30 transition-all cursor-pointer"
            >
              {isSubmitting ? 'Creating...' : 'Create Campaign'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
