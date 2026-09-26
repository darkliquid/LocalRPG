import React, { useRef, useState } from 'react';
import { Sparkles, Loader2, AlertCircle } from 'lucide-react';
import { APIClient, GenerationError } from '../../api/client';
import { GenerateTextRequest, GenerationFailure } from '../../types';
import { formatGenerationError, generationAttemptLines } from '../../lib/generationError';

export interface AIGenerateButtonProps {
  formType: 'character' | 'world' | 'system' | 'campaign';
  fieldName: string;
  getContext: () => Record<string, string>;
  onGenerated: (value: string) => void;
  onError?: (failure: GenerationFailure) => void;
  worldID?: string;
  systemID?: string;
  seed?: string;
  disabled?: boolean;
  className?: string;
  title?: string;
}

const DEFAULT_FAILURE: GenerationFailure = {
  code: 'empty_response',
  message: 'The model returned no text for this field.',
};

export const AIGenerateButton: React.FC<AIGenerateButtonProps> = ({
  formType,
  fieldName,
  getContext,
  onGenerated,
  onError,
  worldID,
  systemID,
  seed,
  disabled = false,
  className = '',
  title,
}) => {
  const [isGenerating, setIsGenerating] = useState(false);
  const [error, setError] = useState<GenerationFailure | null>(null);
  const [showDetails, setShowDetails] = useState(false);
  const clearTimer = useRef<number | null>(null);

  const report = (failure: GenerationFailure) => {
    setError(failure);
    if (onError) onError(failure);
    if (clearTimer.current !== null) window.clearTimeout(clearTimer.current);
    clearTimer.current = window.setTimeout(() => {
      setError(null);
      clearTimer.current = null;
    }, 6000);
  };

  const handleGenerate = async (e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    if (isGenerating || disabled) return;

    setIsGenerating(true);
    setError(null);
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
      const value = res.fields?.[fieldName];
      if (value) {
        onGenerated(value);
        return;
      }
      report(res.warning ?? DEFAULT_FAILURE);
    } catch (err) {
      report(
        err instanceof GenerationError
          ? err.failure
          : { code: 'provider_error', message: (err as Error).message }
      );
    } finally {
      setIsGenerating(false);
    }
  };

  return (
    <span className="inline-flex items-center gap-1">
      <button
        type="button"
        onClick={handleGenerate}
        disabled={isGenerating || disabled}
        className={`inline-flex items-center justify-center p-1 rounded-md bg-purple-600/15 hover:bg-purple-600/25 border border-purple-500/30 text-purple-300 hover:text-purple-200 transition-all cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed ${className}`}
        title={error ? [formatGenerationError(error), ...generationAttemptLines(error)].join('\n') : (title || `AI Generate ${fieldName}`)}
      >
        {isGenerating ? (
          <Loader2 className="w-3 h-3 animate-spin text-purple-400" />
        ) : (
          <Sparkles className={`w-3 h-3 ${error ? 'text-red-400' : 'text-purple-300'}`} />
        )}
      </button>
      {error && (
        <span className="inline-flex flex-col items-start gap-0.5 text-[10px] font-sans text-red-300" role="alert">
          <span className="inline-flex items-center gap-1">
            <AlertCircle className="w-3 h-3 text-red-400" />
            <button
              type="button"
              onClick={() => (error.attempts?.length ? setShowDetails((v) => !v) : undefined)}
              className={`max-w-[18rem] truncate text-left ${
                error.attempts?.length ? 'cursor-pointer underline decoration-dotted' : ''
              }`}
            >
              {formatGenerationError(error)}
            </button>
          </span>
          {showDetails &&
            generationAttemptLines(error).map((line, index) => (
              <span key={index} className="max-w-[18rem] truncate text-[10px] text-red-400/80">
                {line}
              </span>
            ))}
        </span>
      )}
    </span>
  );
};
