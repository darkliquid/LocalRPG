import React, { useState } from 'react';
import { BookOpen, ChevronDown, ChevronRight } from 'lucide-react';

export interface ReferenceEntry {
  sig: string;
  desc: string;
  example: string;
}

export const GLOBALS: ReferenceEntry[] = [
  {
    sig: 'roll(notation)',
    desc: 'Rolls dice such as "2d6" or "5d10>=8" and returns { total, rolls, successes }.',
    example: "const r = roll('2d6');\nlog('rolled ' + r.total);",
  },
  {
    sig: 'getStat(entityId, path)',
    desc: 'Reads a stat from an entity by its id and dotted path.',
    example: "const might = getStat('player', 'stats.might');",
  },
  {
    sig: 'setStat(entityId, path, value)',
    desc: 'Writes a stat, so the change is saved with the turn.',
    example: "setStat('player', 'stats.might', 3);",
  },
  {
    sig: 'getLocation()',
    desc: "Returns the acting entity's current location id.",
    example: "const here = getLocation();",
  },
  {
    sig: 'setLocation(id)',
    desc: 'Moves the acting entity to another location.',
    example: "setLocation('the-quay');",
  },
  {
    sig: 'injectGMDirection(text)',
    desc: 'Queues a directive the GM reads on the next turn.',
    example: "injectGMDirection('The tide is rising.');",
  },
  {
    sig: 'log(message)',
    desc: 'Writes a line to the script log, shown in the trace and the dice tester.',
    example: "log('the hook ran');",
  },
  {
    sig: 'grantXP(amount)',
    desc: 'Awards advancement currency when the system declares advancement. Returns false when it does not.',
    example: 'grantXP(2);',
  },
];

export const HOOKS: ReferenceEntry[] = [
  {
    sig: 'onAction(name, fn)',
    desc: 'Handles an action mode. fn(ctx) returns a resolution the engine applies.',
    example: "onAction('do', function (ctx) {\n  return { success: true, outcome: 'strong' };\n});",
  },
  {
    sig: 'onTurnBegin(fn)',
    desc: 'Runs at the start of a turn.',
    example: "onTurnBegin(function (ctx) {\n  log('turn begins');\n});",
  },
  {
    sig: 'onTurnEnd(fn)',
    desc: 'Runs at the end of a turn, after the narration is recorded.',
    example: "onTurnEnd(function (ctx) {\n  setStat('player', 'stats.might', getStat('player', 'stats.might') + 1);\n});",
  },
  {
    sig: 'onWorldTick(fn)',
    desc: 'Runs on a living-world tick.',
    example: "onWorldTick(function (ctx) {\n  injectGMDirection('A storm gathers.');\n});",
  },
  {
    sig: 'onCheck(kind, fn)',
    desc: 'Resolves a named check kind in script. fn(req) returns { outcome }.',
    example: "onCheck('pick_lock', function (req) {\n  return { outcome: 'strong' };\n});",
  },
  {
    sig: 'onHealthZero(fn)',
    desc: 'Runs when a health stat reaches zero.',
    example: "onHealthZero(function (ctx) {\n  injectGMDirection('You collapse.');\n});",
  },
];

const EXAMPLE = `onAction('do', function (ctx) {
  var r = roll('2d6');
  if (r.total >= 10) return { success: true, outcome: 'strong', roll: r, message: 'A clean hit.' };
  if (r.total >= 7)  return { success: true, outcome: 'weak',   roll: r, message: 'A hit, at a cost.' };
  return { success: false, outcome: 'miss', roll: r, message: 'It goes wrong.' };
});`;

// EntryList renders one API entry per line, expanding to a worked example so an
// author can copy the shape rather than infer it from the signature.
const EntryList: React.FC<{ title: string; entries: ReferenceEntry[] }> = ({ title, entries }) => (
  <div className="space-y-1">
    <h4 className="text-[11px] font-sans uppercase tracking-wider text-purple-300">{title}</h4>
    <ul className="space-y-1">
      {entries.map((entry) => (
        <li key={entry.sig}>
          <details>
            <summary className="flex cursor-pointer flex-col sm:flex-row sm:items-baseline gap-0.5 sm:gap-2">
              <code className="shrink-0 font-mono text-[11px] text-emerald-300">{entry.sig}</code>
              <span className="text-[11px] text-stone-400">{entry.desc}</span>
            </summary>
            <pre className="mt-1 overflow-x-auto rounded-lg border border-stone-800 bg-stone-950 p-2 font-mono text-[11px] leading-relaxed text-stone-300">
              {entry.example}
            </pre>
          </details>
        </li>
      ))}
    </ul>
  </div>
);

// ScriptReference documents the sandbox the mechanics.js runs in: the globals it
// may call and the hooks it may register, each with an example, so a system author
// does not have to leave the studio to look them up.
export const ScriptReference: React.FC = () => {
  const [open, setOpen] = useState(false);

  return (
    <div className="rounded-xl border border-stone-800 bg-stone-900/40 shrink-0">
      <button
        type="button"
        aria-label="Sandbox API reference"
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
        className="flex w-full items-center gap-2 px-3 py-2 text-xs font-sans font-semibold text-stone-200 hover:text-purple-300 transition-colors cursor-pointer"
      >
        {open ? <ChevronDown className="w-3.5 h-3.5" /> : <ChevronRight className="w-3.5 h-3.5" />}
        <BookOpen className="w-3.5 h-3.5 text-purple-400" />
        <span>Sandbox API reference</span>
        <span className="ml-auto text-[11px] font-normal text-stone-500">
          globals, hooks, and a worked example
        </span>
      </button>
      {open && (
        <div className="space-y-3 border-t border-stone-800 p-3">
          <EntryList title="Globals" entries={GLOBALS} />
          <EntryList title="Hooks" entries={HOOKS} />
          <div className="space-y-1">
            <h4 className="text-[11px] font-sans uppercase tracking-wider text-purple-300">Example</h4>
            <pre className="overflow-x-auto rounded-lg border border-stone-800 bg-stone-950 p-2 font-mono text-[11px] leading-relaxed text-stone-300">
              {EXAMPLE}
            </pre>
          </div>
        </div>
      )}
    </div>
  );
};

export default ScriptReference;