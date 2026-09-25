import React, { useState } from 'react';
import { Sparkles, Loader2 } from 'lucide-react';
import { APIClient } from '../../api/client';
import { GenerateTextRequest } from '../../types';

export interface AIGenerateButtonProps {
  formType: 'character' | 'world' | 'system' | 'campaign';
  fieldName: string;
  getContext: () => Record<string, string>;
  onGenerated: (value: string) => void;
  worldID?: string;
  systemID?: string;
  seed?: string;
  disabled?: boolean;
  className?: string;
  title?: string;
}

export const AIGenerateButton: React.FC<AIGenerateButtonProps> = ({
  formType,
  fieldName,
  getContext,
  onGenerated,
  worldID,
  systemID,
  seed,
  disabled = false,
  className = '',
  title,
}) => {
  const [isGenerating, setIsGenerating] = useState(false);

  const handleGenerate = async (e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    if (isGenerating || disabled) return;

    setIsGenerating(true);
    try {
      const payload: GenerateTextRequest = {
        form_type: formType,
        field_name: fieldName,
        context: getContext(),
        world_id: worldID,
        system_id: systemID,
        seed: seed || undefined,
      };

      const res = await APIClient.generateText(payload);
      if (res.fields && res.fields[fieldName]) {
        onGenerated(res.fields[fieldName]);
      }
    } catch (error) {
      console.error(`Failed to generate text for ${fieldName}:`, error);
    } finally {
      setIsGenerating(false);
    }
  };

  return (
    <button
      type="button"
      onClick={handleGenerate}
      disabled={isGenerating || disabled}
      className={`inline-flex items-center justify-center p-1 rounded-md bg-purple-600/15 hover:bg-purple-600/25 border border-purple-500/30 text-purple-300 hover:text-purple-200 transition-all cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed ${className}`}
      title={title || `AI Generate ${fieldName}`}
    >
      {isGenerating ? (
        <Loader2 className="w-3 h-3 animate-spin text-purple-400" />
      ) : (
        <Sparkles className="w-3 h-3 text-purple-300" />
      )}
    </button>
  );
};
