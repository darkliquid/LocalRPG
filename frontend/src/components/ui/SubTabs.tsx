import React, { type ReactNode } from 'react';

export interface SubTab {
  id: string;
  label: string;
  icon?: ReactNode;
}

interface SubTabsProps {
  tabs: SubTab[];
  active: string;
  onChange: (id: string) => void;
  ariaLabel?: string;
}

// SubTabs is the secondary bar inside a settings area, splitting what would
// otherwise be one dense column into sections that can be read one at a time.
export const SubTabs: React.FC<SubTabsProps> = ({ tabs, active, onChange, ariaLabel = 'Settings sections' }) => (
  <div
    role="tablist"
    aria-label={ariaLabel}
    className="flex flex-wrap items-center gap-1 border-b border-white/10 pb-2"
  >
    {tabs.map((tab) => (
      <button
        key={tab.id}
        type="button"
        role="tab"
        aria-selected={active === tab.id}
        onClick={() => onChange(tab.id)}
        className={`flex items-center gap-1.5 text-xs font-sans px-2.5 py-1 rounded-lg border transition-all cursor-pointer ${
          active === tab.id
            ? 'bg-purple-600/25 border-purple-500/50 text-purple-200 font-bold'
            : 'border-transparent text-stone-400 hover:text-white'
        }`}
      >
        {tab.icon}
        <span>{tab.label}</span>
      </button>
    ))}
  </div>
);

export default SubTabs;
