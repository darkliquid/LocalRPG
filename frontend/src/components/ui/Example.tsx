import React from 'react';

interface ExampleProps {
  children: React.ReactNode;
  label?: string;
}

// Example is a worked fragment beside a form section, collapsed so it does not
// crowd the fields it explains. It is the same idea as HelpTip, but for a shape
// worth copying rather than a sentence worth reading.
export const Example: React.FC<ExampleProps> = ({ children, label = 'Example' }) => (
  <details className="text-xs">
    <summary className="cursor-pointer font-sans text-[11px] uppercase tracking-wider text-stone-500 hover:text-purple-300">
      {label}
    </summary>
    <pre className="mt-1 overflow-x-auto rounded-lg border border-stone-800 bg-stone-950 p-2 font-mono text-[11px] leading-relaxed text-stone-300">
      {children}
    </pre>
  </details>
);

export default Example;