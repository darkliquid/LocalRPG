import React from 'react';
import { Plus, Trash2 } from 'lucide-react';

interface RowListProps<T> {
  items: T[];
  addLabel: string;
  onAdd: () => void;
  onChange: (items: T[]) => void;
  renderRow: (item: T, update: (next: T) => void) => React.ReactNode;
  emptyLabel?: string;
}

// RowList is the shared add/remove list pattern the studio uses for every
// editable collection, so the mechanics editor adds no new interaction
// vocabulary.
export function RowList<T>({ items, addLabel, onAdd, onChange, renderRow, emptyLabel }: RowListProps<T>) {
  const update = (index: number, next: T) => onChange(items.map((item, i) => (i === index ? next : item)));
  const remove = (index: number) => onChange(items.filter((_, i) => i !== index));
  return (
    <div className="space-y-2">
      {items.length === 0 && emptyLabel && <p className="text-xs font-sans text-stone-500">{emptyLabel}</p>}
      {items.map((item, index) => (
        <div key={index} className="flex items-start gap-2">
          <div className="flex-1 min-w-0">{renderRow(item, (next) => update(index, next))}</div>
          <button
            type="button"
            aria-label={`remove ${addLabel}`}
            onClick={() => remove(index)}
            className="mt-1 p-1.5 rounded-lg border border-stone-800 hover:border-red-500/50 text-stone-400 hover:text-red-400 cursor-pointer"
          >
            <Trash2 className="w-3.5 h-3.5" />
          </button>
        </div>
      ))}
      <button
        type="button"
        onClick={onAdd}
        className="flex items-center gap-1.5 text-xs font-sans px-3 py-1.5 rounded-lg border border-stone-800 hover:border-purple-500/50 bg-stone-900/60 hover:bg-stone-800 text-stone-300 hover:text-purple-400 cursor-pointer"
      >
        <Plus className="w-3.5 h-3.5" />
        <span>{addLabel}</span>
      </button>
    </div>
  );
}
