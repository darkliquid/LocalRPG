import React, { useState, useEffect, useCallback } from 'react';
import { Package, Search, RefreshCw, X, Download, ShieldCheck, CheckCircle2 } from 'lucide-react';
import { APIClient } from '../api/client';
import { PackageRefDTO, ContentManifestInfo } from '../types';
import { ContentImportDialog } from './ContentImportDialog';

interface RegistryModalProps {
  isOpen: boolean;
  onClose: () => void;
  onInstalled?: () => void;
}

export const RegistryModal: React.FC<RegistryModalProps> = ({
  isOpen,
  onClose,
  onInstalled,
}) => {
  const [packages, setPackages] = useState<PackageRefDTO[]>([]);
  const [updates, setUpdates] = useState<PackageRefDTO[]>([]);
  const [searchQuery, setSearchQuery] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [isCheckingUpdates, setIsCheckingUpdates] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [installingRef, setInstallingRef] = useState<PackageRefDTO | null>(null);
  const [isInstalling, setIsInstalling] = useState(false);
  const [installError, setInstallError] = useState<string | null>(null);
  const [justInstalledIDs, setJustInstalledIDs] = useState<Set<string>>(new Set());

  const fetchPackages = useCallback(async (query: string) => {
    setIsLoading(true);
    setError(null);
    try {
      const results = await APIClient.searchRegistry(query);
      setPackages(results || []);
    } catch (err: any) {
      console.error('Failed to search registry:', err);
      setError(err.message || 'Failed to search registry');
    } finally {
      setIsLoading(false);
    }
  }, []);

  const checkUpdates = useCallback(async () => {
    setIsCheckingUpdates(true);
    try {
      const up = await APIClient.checkRegistryUpdates();
      setUpdates(up || []);
    } catch (err) {
      console.error('Failed to check updates:', err);
    } finally {
      setIsCheckingUpdates(false);
    }
  }, []);

  useEffect(() => {
    if (!isOpen) return;
    fetchPackages(searchQuery);
    checkUpdates();
  }, [isOpen, fetchPackages, checkUpdates, searchQuery]);

  const handleInstallClick = (ref: PackageRefDTO) => {
    setInstallingRef(ref);
    setInstallError(null);
  };

  const handleConfirmInstall = async (conflictMode: 'refuse' | 'rename' | 'overwrite') => {
    if (!installingRef) return;
    setIsInstalling(true);
    setInstallError(null);
    try {
      await APIClient.installRegistryPackage(installingRef, conflictMode);
      setJustInstalledIDs((prev) => new Set(prev).add(installingRef.package.id));
      setInstallingRef(null);
      if (onInstalled) {
        onInstalled();
      }
      checkUpdates();
    } catch (err: any) {
      setInstallError(err.message || 'Failed to install package');
    } finally {
      setIsInstalling(false);
    }
  };

  if (!isOpen) return null;

  const importManifest: ContentManifestInfo | undefined = installingRef
    ? {
        id: installingRef.package.id,
        name: installingRef.package.name,
        version: installingRef.package.version,
        type: installingRef.package.type,
        author: installingRef.package.author,
        license: installingRef.package.license,
        description: installingRef.package.description,
        trust: installingRef.package.signature
          ? { state: 'verified', key_id: installingRef.package.key_id } as any
          : { state: 'unsigned' },
      }
    : undefined;

  return (
    <>
      <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/75 backdrop-blur-md anim-fade-in">
        <div className="relative w-full max-w-4xl bg-stone-900 border border-white/15 rounded-3xl overflow-hidden shadow-2xl flex flex-col max-h-[90vh]">
          {/* Header */}
          <div className="p-4 px-6 border-b border-white/10 flex items-center justify-between bg-stone-950/60">
            <div className="flex items-center gap-3">
              <div className="p-2 rounded-xl bg-purple-500/10 border border-purple-500/20 text-purple-400">
                <Package className="w-5 h-5" />
              </div>
              <div>
                <h2 className="text-base font-sans font-bold text-white">Content Registry</h2>
                <p className="text-xs text-stone-400">Browse and install community worlds and systems</p>
              </div>
            </div>
            <div className="flex items-center gap-2">
              <button
                onClick={checkUpdates}
                disabled={isCheckingUpdates}
                className="flex items-center gap-1.5 px-3 py-1.5 rounded-xl bg-white/[0.05] hover:bg-white/[0.1] border border-white/10 text-xs font-semibold text-stone-300 hover:text-white transition-all cursor-pointer disabled:opacity-50"
                title="Check for updates to installed packages"
              >
                <RefreshCw className={`w-3.5 h-3.5 ${isCheckingUpdates ? 'animate-spin' : ''}`} />
                <span>{isCheckingUpdates ? 'Checking...' : 'Check Updates'}</span>
              </button>
              <button
                onClick={onClose}
                className="w-8 h-8 rounded-full bg-white/[0.05] hover:bg-white/[0.1] border border-white/10 flex items-center justify-center text-stone-400 hover:text-white transition-all cursor-pointer"
              >
                <X className="w-4 h-4" />
              </button>
            </div>
          </div>

          {/* Search bar */}
          <div className="p-4 px-6 border-b border-white/10 bg-stone-900/40 flex items-center gap-3">
            <div className="relative flex-1">
              <Search className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-stone-400" />
              <input
                type="text"
                placeholder="Search packages by name, ID, or description..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="w-full bg-stone-950/50 border border-white/10 rounded-xl pl-9 pr-4 py-2 text-sm text-stone-200 placeholder:text-stone-500 focus:outline-none focus:border-purple-500/50 transition-colors"
              />
            </div>
            {updates.length > 0 && (
              <span className="text-xs px-2.5 py-1 rounded-full bg-purple-500/20 border border-purple-500/40 text-purple-300 font-medium">
                {updates.length} update{updates.length === 1 ? '' : 's'} available
              </span>
            )}
          </div>

          {/* Package list */}
          <div className="flex-1 overflow-y-auto p-6 space-y-4">
            {error && (
              <div className="p-3 text-sm text-red-300 border border-red-500/30 rounded-xl bg-red-950/40">
                {error}
              </div>
            )}

            {isLoading && packages.length === 0 ? (
              <div className="py-16 text-center text-stone-400 text-sm">
                Loading registry packages...
              </div>
            ) : packages.length === 0 ? (
              <div className="py-16 text-center text-stone-400 text-sm">
                No packages found matching &quot;{searchQuery}&quot;.
              </div>
            ) : (
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                {packages.map((ref) => {
                  const pkg = ref.package;
                  const hasUpdate = updates.some((u) => u.package.id === pkg.id);
                  const isInstalled = justInstalledIDs.has(pkg.id);

                  return (
                    <div
                      key={`${ref.registry_url}#${pkg.id}#${pkg.version}`}
                      className="p-4 rounded-2xl bg-white/[0.03] border border-white/10 hover:border-white/20 transition-all flex flex-col justify-between gap-3"
                    >
                      <div className="space-y-2">
                        <div className="flex items-start justify-between gap-2">
                          <div>
                            <div className="flex items-center gap-2">
                              <span
                                className={`text-[10px] uppercase font-bold px-2 py-0.5 rounded-md ${
                                  pkg.type === 'world'
                                    ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20'
                                    : 'bg-amber-500/10 text-amber-400 border border-amber-500/20'
                                }`}
                              >
                                {pkg.type}
                              </span>
                              <h3 className="font-semibold text-white text-base leading-tight">
                                {pkg.name}
                              </h3>
                            </div>
                            <div className="text-xs text-stone-400 font-mono mt-1">
                              {pkg.id} <span className="text-stone-500">v{pkg.version}</span>
                            </div>
                          </div>

                          <div className="text-right">
                            <span className="text-[11px] text-stone-400 bg-stone-800/80 px-2 py-0.5 rounded-md border border-white/5">
                              {ref.registry_name}
                            </span>
                          </div>
                        </div>

                        {pkg.description && (
                          <p className="text-xs text-stone-300 line-clamp-2 leading-relaxed">
                            {pkg.description}
                          </p>
                        )}

                        <div className="flex items-center gap-3 text-[11px] text-stone-400">
                          {pkg.author && <span>By {pkg.author}</span>}
                          {pkg.license && <span>{pkg.license}</span>}
                          {pkg.signature && (
                            <span className="flex items-center gap-1 text-emerald-400">
                              <ShieldCheck className="w-3.5 h-3.5" />
                              Signed
                            </span>
                          )}
                        </div>
                      </div>

                      <div className="pt-2 border-t border-white/5 flex items-center justify-between">
                        {hasUpdate ? (
                          <span className="text-xs text-purple-400 font-medium">Update available</span>
                        ) : isInstalled ? (
                          <span className="text-xs text-emerald-400 flex items-center gap-1">
                            <CheckCircle2 className="w-3.5 h-3.5" /> Installed
                          </span>
                        ) : (
                          <div />
                        )}

                        <button
                          onClick={() => handleInstallClick(ref)}
                          className="flex items-center gap-1.5 px-3 py-1.5 rounded-xl bg-purple-600 hover:bg-purple-500 text-white text-xs font-semibold shadow-lg shadow-purple-600/20 transition-all cursor-pointer"
                        >
                          <Download className="w-3.5 h-3.5" />
                          <span>{hasUpdate ? 'Update' : 'Install'}</span>
                        </button>
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Confirmation & Conflict Dialog */}
      <ContentImportDialog
        manifest={importManifest}
        onConfirm={handleConfirmInstall}
        onCancel={() => {
          setInstallingRef(null);
          setInstallError(null);
        }}
        loading={isInstalling}
        error={installError}
      />
    </>
  );
};
