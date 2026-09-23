import React from 'react';
import { VoiceOption } from '../types';

interface VoiceOptionsControlProps {
  schema: VoiceOption[];
  values: Record<string, unknown>;
  onChange: (key: string, value: unknown) => void;
}

// VoiceOptionsControl renders whatever a provider declares. No provider name
// appears here; a new provider needs no frontend change.
export const VoiceOptionsControl: React.FC<VoiceOptionsControlProps> = ({ schema, values, onChange }) => {
  if (schema.length === 0) return null;

  return (
    <div className="space-y-2.5">
      {schema.map((option) => {
        const current = values[option.key];
        return (
          <div key={option.key} className="space-y-1">
            {option.kind !== 'bool' && (
              <div className="flex justify-between text-[11px] text-stone-400">
                <span>{option.label}</span>
                <span className="font-mono text-purple-400">{describeValue(option, current)}</span>
              </div>
            )}
            {renderControl(option, current, onChange)}
            {option.help && <p className="text-[10px] text-stone-500">{option.help}</p>}
          </div>
        );
      })}
    </div>
  );
};

function numericValue(option: VoiceOption, current: unknown): number {
  if (typeof current === 'number') return current;
  if (typeof option.default === 'number') return option.default;
  return option.min ?? 0;
}

function describeValue(option: VoiceOption, current: unknown): string {
  if (option.kind === 'float') return numericValue(option, current).toFixed(2);
  if (option.kind === 'int') return String(Math.round(numericValue(option, current)));
  if (option.kind === 'enum' || option.kind === 'string') {
    const value = typeof current === 'string' ? current : String(option.default ?? '');
    return value || '(provider default)';
  }
  return '';
}

function renderControl(
  option: VoiceOption,
  current: unknown,
  onChange: (key: string, value: unknown) => void,
): React.ReactNode {
  const rangeClass = 'w-full accent-purple-500';
  const inputClass =
    'w-full bg-stone-900 border border-stone-800 rounded px-2 py-1 text-xs text-stone-200 focus:outline-none focus:border-purple-500/60';

  switch (option.kind) {
    case 'float':
    case 'int':
      return (
        <input
          type="range"
          min={option.min ?? 0}
          max={option.max ?? 1}
          step={option.step ?? (option.kind === 'int' ? 1 : 0.01)}
          value={numericValue(option, current)}
          onChange={(e) => onChange(option.key, parseFloat(e.target.value))}
          className={rangeClass}
        />
      );
    case 'bool':
      return (
        <label className="flex items-center gap-2 text-xs text-stone-300 cursor-pointer">
          <input
            type="checkbox"
            checked={typeof current === 'boolean' ? current : Boolean(option.default)}
            onChange={(e) => onChange(option.key, e.target.checked)}
            className="accent-purple-500"
          />
          <span>{option.label}</span>
        </label>
      );
    case 'enum':
      return (
        <select
          value={typeof current === 'string' ? current : String(option.default ?? '')}
          onChange={(e) => onChange(option.key, e.target.value)}
          className={inputClass + ' cursor-pointer'}
        >
          {(option.options ?? []).map((choice) => (
            <option key={choice} value={choice}>
              {choice}
            </option>
          ))}
        </select>
      );
    default:
      return (
        <input
          type="text"
          value={typeof current === 'string' ? current : ''}
          placeholder={option.default !== undefined ? String(option.default) : ''}
          onChange={(e) => onChange(option.key, e.target.value)}
          className={inputClass + ' font-mono'}
        />
      );
  }
}
