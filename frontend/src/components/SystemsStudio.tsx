import React, { useState, useEffect } from 'react';
import { APIClient } from '../api/client';
import { SystemInfo, CreateSystemRequest } from '../types';
import { Shield, Plus, Save, FileCode, Info, Check, AlertCircle } from 'lucide-react';

interface SystemsStudioProps {
  onSystemSaved?: () => void;
}

const STARTER_SCRIPT = `// LocalRPG Rule System Engine
// Globals available: roll(notation), state, log(msg)

function evaluateRoll(stats, diceExpr) {
  const result = roll(diceExpr || "2d6");
  return {
    total: result.total,
    success: result.total >= 10,
    rolls: result.rolls
  };
}
`;

export const SystemsStudio: React.FC<SystemsStudioProps> = ({ onSystemSaved }) => {
  const [systems, setSystems] = useState<SystemInfo[]>([]);
  const [selectedID, setSelectedID] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState<'manifest' | 'script'>('manifest');

  // Form state
  const [name, setName] = useState('');
  const [slugID, setSlugID] = useState('');
  const [version, setVersion] = useState('1.0.0');
  const [description, setDescription] = useState('');
  const [script, setScript] = useState(STARTER_SCRIPT);

  const [isLoading, setIsLoading] = useState(false);
  const [isSaving, setIsSaving] = useState(false);
  const [toast, setToast] = useState<{ type: 'success' | 'error'; message: string } | null>(null);

  useEffect(() => {
    loadSystems();
  }, []);

  const loadSystems = async (selectID?: string) => {
    setIsLoading(true);
    try {
      const list = await APIClient.listSystems();
      setSystems(list);
      const target = selectID || (list.length > 0 ? list[0].id : null);
      if (target) {
        loadSystemDetail(target);
      } else {
        handleNewSystem();
      }
    } catch (err: any) {
      setToast({ type: 'error', message: err.message || 'Failed to load systems' });
    } finally {
      setIsLoading(false);
    }
  };

  const loadSystemDetail = async (id: string) => {
    try {
      const detail = await APIClient.getSystem(id);
      setSelectedID(detail.id);
      setName(detail.name);
      setSlugID(detail.id);
      setVersion(detail.version || '1.0.0');
      setDescription(detail.description || '');
      setScript(detail.script || STARTER_SCRIPT);
    } catch (err: any) {
      setToast({ type: 'error', message: err.message || 'Failed to load system details' });
    }
  };

  const handleNewSystem = () => {
    setSelectedID(null);
    setName('');
    setSlugID('');
    setVersion('1.0.0');
    setDescription('');
    setScript(STARTER_SCRIPT);
    setActiveTab('manifest');
  };

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) {
      setToast({ type: 'error', message: 'System Name is required' });
      return;
    }

    setIsSaving(true);
    setToast(null);
    try {
      const payload: CreateSystemRequest = {
        id: slugID.trim() || undefined,
        name: name.trim(),
        version: version.trim() || '1.0.0',
        description: description.trim(),
        script: script,
      };

      const saved = await APIClient.saveSystem(payload);
      setToast({ type: 'success', message: `System "${saved.name}" saved successfully!` });
      await loadSystems(saved.id);
      if (onSystemSaved) onSystemSaved();
    } catch (err: any) {
      setToast({ type: 'error', message: err.message || 'Failed to save system' });
    } finally {
      setIsSaving(false);
    }
  };

  return (
    <div className="flex-1 flex flex-col md:flex-row gap-6 overflow-hidden">
      {/* Left Master Column: Systems List */}
      <aside className="w-full md:w-80 bg-glass-card rounded-2xl border border-stone-800/80 p-4 flex flex-col gap-4 shadow-xl backdrop-blur-md">
        <div className="flex items-center justify-between pb-2 border-b border-stone-800/60">
          <div className="flex items-center gap-2">
            <Shield className="w-4 h-4 text-amber-400" />
            <h3 className="font-cinzel text-sm font-bold text-stone-200 uppercase tracking-wider">
              Rule Systems
            </h3>
          </div>
          <button
            onClick={handleNewSystem}
            className="flex items-center gap-1 text-[11px] font-cinzel font-bold px-2.5 py-1 rounded-lg bg-amber-600/80 hover:bg-amber-500 text-stone-950 transition-all cursor-pointer shadow"
          >
            <Plus className="w-3.5 h-3.5" />
            <span>New</span>
          </button>
        </div>

        <div className="flex-1 overflow-y-auto space-y-2 pr-1">
          {isLoading && systems.length === 0 ? (
            <div className="text-center py-8 text-xs font-mono text-stone-500 animate-pulse">
              Loading systems...
            </div>
          ) : systems.length === 0 ? (
            <div className="text-center py-8 text-xs text-stone-400">
              No systems defined yet. Create your first ruleset!
            </div>
          ) : (
            systems.map((s) => (
              <div
                key={s.id}
                onClick={() => loadSystemDetail(s.id)}
                className={`p-3 rounded-xl border transition-all cursor-pointer text-left ${
                  selectedID === s.id
                    ? 'bg-amber-950/40 border-amber-500/60 shadow-[0_0_15px_rgba(245,158,11,0.15)]'
                    : 'bg-stone-900/40 border-stone-800/60 hover:bg-stone-800/40 hover:border-stone-700'
                }`}
              >
                <div className="flex items-center justify-between">
                  <h4 className="font-cinzel text-xs font-bold text-stone-200 truncate">{s.name}</h4>
                  <span className="text-[10px] font-mono text-stone-400 bg-stone-950 px-1.5 py-0.5 rounded border border-stone-800">
                    v{s.version}
                  </span>
                </div>
                {s.description && (
                  <p className="text-[11px] text-stone-400 truncate mt-1">{s.description}</p>
                )}
              </div>
            ))
          )}
        </div>
      </aside>

      {/* Right Detail Column: Editor */}
      <section className="flex-1 bg-glass-card rounded-2xl border border-stone-800/80 p-6 flex flex-col gap-5 shadow-xl backdrop-blur-md overflow-hidden">
        {/* Top Header & Sub-Tabs */}
        <div className="flex items-center justify-between border-b border-stone-800/80 pb-3">
          <div className="flex items-center gap-3">
            <h2 className="font-cinzel text-lg font-bold text-amber-400">
              {selectedID ? name || 'Edit System' : 'Create New System'}
            </h2>
            {slugID && (
              <span className="text-xs font-mono text-stone-400 bg-stone-950 px-2 py-0.5 rounded border border-stone-800">
                systems/{slugID}
              </span>
            )}
          </div>

          <div className="flex items-center gap-2">
            <div className="flex bg-stone-950/80 p-1 rounded-xl border border-stone-800">
              <button
                type="button"
                onClick={() => setActiveTab('manifest')}
                className={`flex items-center gap-1.5 text-xs font-cinzel px-3 py-1 rounded-lg transition-all cursor-pointer ${
                  activeTab === 'manifest'
                    ? 'bg-amber-600 text-stone-950 font-bold shadow'
                    : 'text-stone-400 hover:text-white'
                }`}
              >
                <Info className="w-3.5 h-3.5" />
                <span>Manifest</span>
              </button>
              <button
                type="button"
                onClick={() => setActiveTab('script')}
                className={`flex items-center gap-1.5 text-xs font-cinzel px-3 py-1 rounded-lg transition-all cursor-pointer ${
                  activeTab === 'script'
                    ? 'bg-amber-600 text-stone-950 font-bold shadow'
                    : 'text-stone-400 hover:text-white'
                }`}
              >
                <FileCode className="w-3.5 h-3.5" />
                <span>mechanics.js</span>
              </button>
            </div>

            <button
              onClick={handleSave}
              disabled={isSaving}
              className="flex items-center gap-1.5 text-xs font-cinzel font-bold px-4 py-2 rounded-xl bg-gradient-to-r from-amber-600 to-amber-500 hover:from-amber-500 hover:to-amber-400 text-stone-950 shadow-[0_0_15px_rgba(217,119,6,0.4)] active:scale-95 transition-all cursor-pointer disabled:opacity-50"
            >
              <Save className="w-3.5 h-3.5" />
              <span>{isSaving ? 'Saving...' : 'Save System'}</span>
            </button>
          </div>
        </div>

        {/* Toast Feedback */}
        {toast && (
          <div
            className={`p-3 rounded-xl text-xs flex items-center gap-2 ${
              toast.type === 'success'
                ? 'bg-emerald-950/60 border border-emerald-500/40 text-emerald-200'
                : 'bg-red-950/60 border border-red-500/40 text-red-200'
            }`}
          >
            {toast.type === 'success' ? (
              <Check className="w-4 h-4 text-emerald-400" />
            ) : (
              <AlertCircle className="w-4 h-4 text-red-400" />
            )}
            <span>{toast.message}</span>
          </div>
        )}

        {/* Tab 1: Manifest Form */}
        {activeTab === 'manifest' && (
          <div className="flex-1 overflow-y-auto space-y-4 pr-1">
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <div className="space-y-1.5">
                <label className="text-xs font-cinzel uppercase tracking-wider text-stone-300">
                  System Name
                </label>
                <input
                  type="text"
                  required
                  placeholder="e.g. Iron Realm D20"
                  value={name}
                  onChange={(e) => {
                    setName(e.target.value);
                    if (!selectedID) {
                      setSlugID(e.target.value.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, ''));
                    }
                  }}
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl px-4 py-2.5 text-sm text-stone-100 placeholder-stone-600 focus:outline-none focus:border-amber-500/60 transition-colors"
                />
              </div>

              <div className="space-y-1.5">
                <label className="text-xs font-cinzel uppercase tracking-wider text-stone-300">
                  Version
                </label>
                <input
                  type="text"
                  placeholder="1.0.0"
                  value={version}
                  onChange={(e) => setVersion(e.target.value)}
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl px-4 py-2.5 text-sm text-stone-100 placeholder-stone-600 focus:outline-none focus:border-amber-500/60 transition-colors font-mono"
                />
              </div>
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-cinzel uppercase tracking-wider text-stone-300">
                Directory Slug ID
              </label>
              <input
                type="text"
                disabled={!!selectedID}
                placeholder="e.g. iron-realm"
                value={slugID}
                onChange={(e) => setSlugID(e.target.value)}
                className="w-full bg-stone-950 border border-stone-800 rounded-xl px-4 py-2.5 text-sm text-stone-100 placeholder-stone-600 focus:outline-none focus:border-amber-500/60 transition-colors font-mono disabled:opacity-60"
              />
              <p className="text-[11px] text-stone-400">
                Unique identifier for the system directory on disk.
              </p>
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-cinzel uppercase tracking-wider text-stone-300">
                Rulebook Overview & Philosophy
              </label>
              <textarea
                rows={5}
                placeholder="Describe core dice mechanics, resolution philosophy, and character stats..."
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                className="w-full bg-stone-950 border border-stone-800 rounded-xl p-4 text-sm text-stone-100 placeholder-stone-600 focus:outline-none focus:border-amber-500/60 transition-colors resize-none"
              />
            </div>
          </div>
        )}

        {/* Tab 2: Script Editor */}
        {activeTab === 'script' && (
          <div className="flex-1 flex flex-col gap-2 overflow-hidden">
            <div className="flex items-center justify-between text-[11px] font-mono text-stone-400 px-1">
              <span>JavaScript Runtime (Goja Sandbox)</span>
              <span>Exports: evaluateRoll(stats, diceExpr)</span>
            </div>
            <textarea
              value={script}
              onChange={(e) => setScript(e.target.value)}
              spellCheck={false}
              className="flex-1 w-full bg-stone-950 border border-stone-800 rounded-xl p-4 text-xs font-mono text-amber-200/90 leading-relaxed focus:outline-none focus:border-amber-500/60 transition-colors resize-none selection:bg-amber-900/60"
            />
          </div>
        )}
      </section>
    </div>
  );
};
