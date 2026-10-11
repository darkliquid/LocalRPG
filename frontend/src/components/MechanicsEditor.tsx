import React from 'react';
import { Plus, Trash2, ArrowUp, ArrowDown } from 'lucide-react';
import { RowList } from './mechanics/RowList';
import { HelpTip } from './ui/HelpTip';
import { Example } from './ui/Example';
import {
  MechanicsSpec,
  StatSpec,
  SkillSpec,
  HealthSpec,
  ResolutionProfile,
  LadderStep,
  SuccessOutcome,
  AdvancementSpec,
  EarnRule,
  UnlockSpec,
  EffectSpec,
  LevelSpec,
} from '../types';

const inputClass =
  'w-full bg-stone-950 border border-stone-800 rounded-lg px-2 py-1 text-xs font-mono text-stone-200 focus:border-purple-500 outline-none';
const labelClass = 'text-[11px] font-sans uppercase tracking-wider text-stone-400';
const addButtonClass =
  'flex items-center gap-1.5 text-xs font-sans px-3 py-1.5 rounded-lg border border-stone-800 hover:border-purple-500/50 bg-stone-900/60 hover:bg-stone-800 text-stone-300 hover:text-purple-400 cursor-pointer';
const removeButtonClass =
  'p-1.5 rounded-lg border border-stone-800 hover:border-red-500/50 text-stone-400 hover:text-red-400 cursor-pointer';

const Field: React.FC<{ label: string; help?: React.ReactNode; children: React.ReactNode; className?: string }> = ({
  label,
  help,
  children,
  className,
}) => (
  <label className={`block space-y-1 ${className ?? ''}`}>
    <span className={`${labelClass} flex items-center gap-1.5`}>
      <span>{label}</span>
      {help && <HelpTip label={label}>{help}</HelpTip>}
    </span>
    {children}
  </label>
);

const Section: React.FC<{
  title: string;
  help?: React.ReactNode;
  example?: React.ReactNode;
  children: React.ReactNode;
}> = ({ title, help, example, children }) => (
  <section className="rounded-xl border border-stone-800 bg-stone-900/40 p-3 space-y-3">
    <div className="space-y-1">
      <h3 className="flex items-center gap-1.5 text-xs font-sans font-bold uppercase tracking-wider text-purple-300">
        <span>{title}</span>
        {help && <HelpTip label={title}>{help}</HelpTip>}
      </h3>
      {example && <Example>{example}</Example>}
    </div>
    {children}
  </section>
);

const TextInput: React.FC<{ value: string; onChange: (next: string) => void; ariaLabel?: string }> = ({
  value,
  onChange,
  ariaLabel,
}) => <input className={inputClass} aria-label={ariaLabel} value={value} onChange={(e) => onChange(e.target.value)} />;

const NumberInput: React.FC<{ value: number | undefined; onChange: (next: number | undefined) => void }> = ({
  value,
  onChange,
}) => (
  <input
    type="number"
    className={inputClass}
    value={value === undefined ? '' : value}
    onChange={(e) => onChange(e.target.value === '' ? undefined : Number(e.target.value))}
  />
);

const StatSelect: React.FC<{ value: string; stats: StatSpec[]; onChange: (next: string) => void }> = ({
  value,
  stats,
  onChange,
}) => (
  <select className={inputClass} value={value} onChange={(e) => onChange(e.target.value)}>
    <option value="">(none)</option>
    {stats.map((stat) => (
      <option key={stat.id} value={stat.id}>
        {stat.label || stat.id}
      </option>
    ))}
  </select>
);

const StatsEditor: React.FC<{ stats: StatSpec[]; onChange: (next: StatSpec[]) => void }> = ({ stats, onChange }) => (
  <RowList
    items={stats}
    addLabel="Add Stat"
    onAdd={() => onChange([...stats, { id: '', type: 'number' }])}
    onChange={onChange}
    emptyLabel="No stats declared."
    renderRow={(stat, update) => (
      <div className="grid grid-cols-2 sm:grid-cols-3 gap-2">
        <Field label="ID">
          <TextInput value={stat.id} onChange={(id) => update({ ...stat, id })} />
        </Field>
        <Field label="Label">
          <TextInput value={stat.label ?? ''} onChange={(label) => update({ ...stat, label })} />
        </Field>
        <Field
          label="Type"
          help={<>The value's kind. number is read by checks and health; string and bool are stored as written.</>}
        >
          <select className={inputClass} value={stat.type ?? 'number'} onChange={(e) => update({ ...stat, type: e.target.value })}>
            <option value="number">number</option>
            <option value="string">string</option>
            <option value="bool">bool</option>
          </select>
        </Field>
        <Field
          label="Default"
          help={<>The value a new character starts with, unless character creation overrides it.</>}
        >
          <TextInput
            value={stat.default === undefined ? '' : String(stat.default)}
            onChange={(v) => update({ ...stat, default: v === '' ? undefined : v })}
          />
        </Field>
        <Field label="Min" help={<>The lowest value the engine clamps this stat to. Empty means no floor.</>}>
          <NumberInput value={stat.min} onChange={(min) => update({ ...stat, min })} />
        </Field>
        <Field label="Max" help={<>The highest value the engine clamps this stat to. Empty means no ceiling.</>}>
          <NumberInput value={stat.max} onChange={(max) => update({ ...stat, max })} />
        </Field>
      </div>
    )}
  />
);

const SkillsEditor: React.FC<{
  skills: SkillSpec[];
  stats: StatSpec[];
  onChange: (next: SkillSpec[]) => void;
}> = ({ skills, stats, onChange }) => (
  <RowList
    items={skills}
    addLabel="Add Skill"
    onAdd={() => onChange([...skills, { id: '' }])}
    onChange={onChange}
    emptyLabel="No skills declared."
    renderRow={(skill, update) => (
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-2">
        <Field label="ID">
          <TextInput value={skill.id} onChange={(id) => update({ ...skill, id })} />
        </Field>
        <Field label="Label">
          <TextInput value={skill.label ?? ''} onChange={(label) => update({ ...skill, label })} />
        </Field>
        <Field label="Stat" help={<>The stat this skill adds to a check. Choose none for a skill that adds nothing.</>}>
          <StatSelect value={skill.stat ?? ''} stats={stats} onChange={(stat) => update({ ...skill, stat })} />
        </Field>
      </div>
    )}
  />
);

const HealthEditor: React.FC<{
  health: HealthSpec | undefined;
  stats: StatSpec[];
  onChange: (next: HealthSpec | undefined) => void;
}> = ({ health, stats, onChange }) => (
  <div className="space-y-2">
    <label className="flex items-center gap-2 text-xs font-sans text-stone-300">
      <input
        type="checkbox"
        checked={health !== undefined}
        onChange={(e) => onChange(e.target.checked ? { stat: '', zero_effect: 'incapacitated' } : undefined)}
      />
      <span>Declare a health convention</span>
    </label>
    {health && (
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-2">
        <Field label="Stat" help={<>The stat that damage reduces. When it reaches zero, the zero effect runs.</>}>
          <StatSelect value={health.stat} stats={stats} onChange={(stat) => onChange({ ...health, stat })} />
        </Field>
        <Field label="Max stat" help={<>A stat holding the maximum, so a bar can show current out of maximum. Optional.</>}>
          <StatSelect value={health.max_stat ?? ''} stats={stats} onChange={(max_stat) => onChange({ ...health, max_stat })} />
        </Field>
        <Field label="Zero effect" help={<>A short label applied when the health stat reaches zero, such as downed or dead.</>}>
          <TextInput value={health.zero_effect ?? ''} onChange={(zero_effect) => onChange({ ...health, zero_effect })} />
        </Field>
      </div>
    )}
  </div>
);

const VocabularyEditor: React.FC<{ words: string[]; onChange: (next: string[]) => void }> = ({ words, onChange }) => {
  const move = (index: number, delta: number) => {
    const target = index + delta;
    if (target < 0 || target >= words.length) return;
    const next = [...words];
    [next[index], next[target]] = [next[target], next[index]];
    onChange(next);
  };
  return (
    <div className="space-y-1">
      {words.map((word, index) => (
        <div key={index} className="flex items-center gap-2">
          <span className="w-5 text-[11px] font-mono text-stone-500">{index + 1}</span>
          <TextInput value={word} onChange={(next) => onChange(words.map((w, i) => (i === index ? next : w)))} />
          <button type="button" aria-label={`move ${word} up`} onClick={() => move(index, -1)} className={removeButtonClass}>
            <ArrowUp className="w-3.5 h-3.5" />
          </button>
          <button type="button" aria-label={`move ${word} down`} onClick={() => move(index, 1)} className={removeButtonClass}>
            <ArrowDown className="w-3.5 h-3.5" />
          </button>
          <button type="button" aria-label={`remove ${word}`} onClick={() => onChange(words.filter((_, i) => i !== index))} className={removeButtonClass}>
            <Trash2 className="w-3.5 h-3.5" />
          </button>
        </div>
      ))}
      <button type="button" className={addButtonClass} onClick={() => onChange([...words, ''])}>
        <Plus className="w-3.5 h-3.5" />
        <span>Add Outcome</span>
      </button>
    </div>
  );
};

const LadderEditor: React.FC<{ ladder: LadderStep[]; onChange: (next: LadderStep[]) => void }> = ({ ladder, onChange }) => (
  <RowList
    items={ladder}
    addLabel="Add Step"
    onAdd={() => onChange([...ladder, { min: 0, outcome: '' }])}
    onChange={onChange}
    emptyLabel="No ladder steps."
    renderRow={(step, update) => (
      <div className="grid grid-cols-2 gap-2">
        <Field label="Min">
          <NumberInput value={step.min} onChange={(min) => update({ ...step, min: min ?? 0 })} />
        </Field>
        <Field label="Outcome">
          <TextInput value={step.outcome} onChange={(outcome) => update({ ...step, outcome })} />
        </Field>
      </div>
    )}
  />
);

const PoolOutcomesEditor: React.FC<{ outcomes: SuccessOutcome[]; onChange: (next: SuccessOutcome[]) => void }> = ({
  outcomes,
  onChange,
}) => (
  <RowList
    items={outcomes}
    addLabel="Add Outcome Range"
    onAdd={() => onChange([...outcomes, { min: 0, max: 0, outcome: '' }])}
    onChange={onChange}
    emptyLabel="No outcome ranges."
    renderRow={(range, update) => (
      <div className="grid grid-cols-3 gap-2">
        <Field label="Min">
          <NumberInput value={range.min} onChange={(min) => update({ ...range, min: min ?? 0 })} />
        </Field>
        <Field label="Max">
          <NumberInput value={range.max} onChange={(max) => update({ ...range, max: max ?? -1 })} />
        </Field>
        <Field label="Outcome">
          <TextInput value={range.outcome} onChange={(outcome) => update({ ...range, outcome })} />
        </Field>
      </div>
    )}
  />
);

type ProfileShape = 'ladder' | 'dc' | 'pool' | 'blades';

function profileShape(p: ResolutionProfile): ProfileShape {
  if (p.dc) return 'dc';
  if (p.success_on || p.outcomes) return 'pool';
  if (p.position || p.effect) return 'blades';
  return 'ladder';
}

function withShape(p: ResolutionProfile, shape: ProfileShape): ResolutionProfile {
  const base: ResolutionProfile = {};
  if (p.label) base.label = p.label;
  if (p.notation) base.notation = p.notation;
  switch (shape) {
    case 'dc':
      return { ...base, dc: p.dc ?? 10 };
    case 'pool':
      return { ...base, success_on: p.success_on ?? '>=8', outcomes: p.outcomes ?? [] };
    case 'blades':
      return {
        ...base,
        position: p.position ?? ['controlled', 'risky', 'desperate'],
        effect: p.effect ?? ['limited', 'standard', 'great'],
        ladder: p.ladder ?? [],
      };
    default:
      return { ...base, ladder: p.ladder ?? [] };
  }
}

const commaList = (value: string[] | undefined) => (value ?? []).join(', ');
const parseCommaList = (value: string) =>
  value
    .split(',')
    .map((part) => part.trim())
    .filter(Boolean);

const ProfilesEditor: React.FC<{
  profiles: Record<string, ResolutionProfile>;
  onChange: (next: Record<string, ResolutionProfile>) => void;
}> = ({ profiles, onChange }) => {
  const entries = Object.entries(profiles);
  const addProfile = () => {
    let name = 'profile';
    let n = 1;
    while (profiles[name]) name = `profile${n++}`;
    onChange({ ...profiles, [name]: { ladder: [] } });
  };
  const rename = (oldName: string, newName: string, profile: ResolutionProfile) => {
    if (oldName === newName) return;
    const next = { ...profiles };
    delete next[oldName];
    next[newName] = profile;
    onChange(next);
  };
  const setProfile = (name: string, profile: ResolutionProfile) => onChange({ ...profiles, [name]: profile });
  const remove = (name: string) => {
    const next = { ...profiles };
    delete next[name];
    onChange(next);
  };
  return (
    <div className="space-y-2">
      {entries.length === 0 && <p className="text-xs font-sans text-stone-500">No resolution profiles declared.</p>}
      {entries.map(([name, profile]) => {
        const shape = profileShape(profile);
        return (
          <div key={name} className="rounded-lg border border-stone-800 p-2 space-y-2">
            <div className="flex items-start gap-2">
              <div className="grid grid-cols-2 gap-2 flex-1">
                <Field label="Name">
                  <TextInput value={name} onChange={(next) => rename(name, next, profile)} />
                </Field>
                <Field label="Shape" help={<>How this profile turns a roll into an outcome. ladder maps a total to thresholds, dc compares a total to a difficulty class, pool counts dice meeting a target, and blades adds position and effect.</>}>
                  <select
                    className={inputClass}
                    value={shape}
                    onChange={(e) => setProfile(name, withShape(profile, e.target.value as ProfileShape))}
                  >
                    <option value="ladder">ladder</option>
                    <option value="dc">dc</option>
                    <option value="pool">pool</option>
                    <option value="blades">blades</option>
                  </select>
                </Field>
                <Field label="Label">
                  <TextInput value={profile.label ?? ''} onChange={(label) => setProfile(name, { ...profile, label })} />
                </Field>
                <Field label="Notation" help={<>Dice expression for this profile, such as 2d6 or 1d20. Empty falls back to the default notation.</>}>
                  <TextInput value={profile.notation ?? ''} onChange={(notation) => setProfile(name, { ...profile, notation })} />
                </Field>
                <Field label="Opposed stat" help={<>Name the stat the opponent rolls to make this profile a contest. Leave it empty for a check against a fixed difficulty.</>}>
                  <TextInput value={profile.opposed ?? ''} onChange={(opposed) => setProfile(name, { ...profile, opposed })} />
                </Field>
                <Field label="Ties" help={<>What an equal result means, such as opponent or actor. Leave it empty to keep a tie with the actor.</>}>
                  <TextInput value={profile.ties ?? ''} onChange={(ties) => setProfile(name, { ...profile, ties })} />
                </Field>
              </div>
              <button type="button" aria-label={`remove profile ${name}`} onClick={() => remove(name)} className={removeButtonClass}>
                <Trash2 className="w-3.5 h-3.5" />
              </button>
            </div>
            {shape === 'dc' && (
              <Field label="Difficulty class" help={<>A total at or above this number is a success.</>}>
                <NumberInput value={profile.dc} onChange={(dc) => setProfile(name, { ...profile, dc })} />
              </Field>
            )}
            {shape === 'pool' && (
              <div className="space-y-2">
                <Field label="Success on" help={<>A comparison that marks a die a success, such as &gt;=8. The count of successes is then read by the outcomes.</>}>
                  <TextInput value={profile.success_on ?? ''} onChange={(success_on) => setProfile(name, { ...profile, success_on })} />
                </Field>
                <PoolOutcomesEditor outcomes={profile.outcomes ?? []} onChange={(outcomes) => setProfile(name, { ...profile, outcomes })} />
              </div>
            )}
            {shape === 'blades' && (
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
                <Field label="Positions" help={<>Comma-separated position vocabulary, such as controlled, risky, desperate.</>}>
                  <TextInput
                    value={commaList(profile.position)}
                    onChange={(v) => setProfile(name, { ...profile, position: parseCommaList(v) })}
                  />
                </Field>
                <Field label="Effects" help={<>Comma-separated effect vocabulary, such as limited, standard, great.</>}>
                  <TextInput
                    value={commaList(profile.effect)}
                    onChange={(v) => setProfile(name, { ...profile, effect: parseCommaList(v) })}
                  />
                </Field>
              </div>
            )}
            {(shape === 'ladder' || shape === 'blades') && (
              <LadderEditor ladder={profile.ladder ?? []} onChange={(ladder) => setProfile(name, { ...profile, ladder })} />
            )}
          </div>
        );
      })}
      <button type="button" className={addButtonClass} onClick={addProfile}>
        <Plus className="w-3.5 h-3.5" />
        <span>Add Profile</span>
      </button>
    </div>
  );
};

const ChecksEditor: React.FC<{
  checks: NonNullable<MechanicsSpec['checks']>;
  onChange: (next: NonNullable<MechanicsSpec['checks']>) => void;
}> = ({ checks, onChange }) => (
  <div className="space-y-3">
    <Field label="Default notation" help={<>The dice expression a check rolls unless a profile overrides it, such as 2d6 or 1d20.</>}>
      <TextInput value={checks.notation ?? ''} onChange={(notation) => onChange({ ...checks, notation })} />
    </Field>
    <div>
      <span className={`${labelClass} flex items-center gap-1.5`}>
        <span>Outcome vocabulary (best first)</span>
        <HelpTip label="Outcome vocabulary">
          The words a check may return, strongest first, such as strong, weak, miss. A resolution
          profile maps a roll onto one of them.
        </HelpTip>
      </span>
      <VocabularyEditor words={checks.outcome ?? []} onChange={(outcome) => onChange({ ...checks, outcome })} />
    </div>
    <div>
      <span className={`${labelClass} flex items-center gap-1.5`}>
        <span>Difficulties</span>
        <HelpTip label="Difficulties">
          Named targets a check can compare against, such as easy 8 or hard 12. The GM names one when
          calling for a check.
        </HelpTip>
      </span>
      <RowList
        items={checks.difficulty ?? []}
        addLabel="Add Difficulty"
        onAdd={() => onChange({ ...checks, difficulty: [...(checks.difficulty ?? []), { id: '', target: 0 }] })}
        onChange={(difficulty) => onChange({ ...checks, difficulty })}
        emptyLabel="No difficulties declared."
        renderRow={(difficulty, update) => (
          <div className="grid grid-cols-3 gap-2">
            <Field label="ID">
              <TextInput value={difficulty.id} onChange={(id) => update({ ...difficulty, id })} />
            </Field>
            <Field label="Label">
              <TextInput value={difficulty.label ?? ''} onChange={(label) => update({ ...difficulty, label })} />
            </Field>
            <Field label="Target">
              <NumberInput value={difficulty.target} onChange={(target) => update({ ...difficulty, target: target ?? 0 })} />
            </Field>
          </div>
        )}
      />
    </div>
    <div>
      <span className={`${labelClass} flex items-center gap-1.5`}>
        <span>Resolution profiles</span>
        <HelpTip label="Resolution profiles">
          One way checks resolve, named by the GM. A system may declare several, so a single system can
          express a ladder, a difficulty class, and a dice pool side by side.
        </HelpTip>
      </span>
      <ProfilesEditor profiles={checks.profiles ?? {}} onChange={(profiles) => onChange({ ...checks, profiles })} />
    </div>
  </div>
);

const EarnRulesEditor: React.FC<{ rules: EarnRule[]; onChange: (next: EarnRule[]) => void }> = ({ rules, onChange }) => (
  <RowList
    items={rules}
    addLabel="Add Earn Rule"
    onAdd={() => onChange([...rules, { on: 'turn_end', amount: 1 }])}
    onChange={onChange}
    emptyLabel="No earn rules."
    renderRow={(rule, update) => (
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-2">
        <Field label="On">
          <select className={inputClass} value={rule.on} onChange={(e) => update({ ...rule, on: e.target.value })}>
            <option value="miss">miss</option>
            <option value="check_outcome">check_outcome</option>
            <option value="turn_end">turn_end</option>
            <option value="hook">hook</option>
          </select>
        </Field>
        <Field label="Outcome">
          <TextInput value={rule.outcome ?? ''} onChange={(outcome) => update({ ...rule, outcome })} />
        </Field>
        <Field label="Rank">
          <TextInput value={rule.rank ?? ''} onChange={(rank) => update({ ...rule, rank })} />
        </Field>
        <Field label="Amount">
          <NumberInput value={rule.amount} onChange={(amount) => update({ ...rule, amount: amount ?? 0 })} />
        </Field>
      </div>
    )}
  />
);

const EffectsEditor: React.FC<{ effects: EffectSpec[]; onChange: (next: EffectSpec[]) => void }> = ({ effects, onChange }) => (
  <RowList
    items={effects}
    addLabel="Add Effect"
    onAdd={() => onChange([...effects, { type: 'stat_increase' }])}
    onChange={onChange}
    emptyLabel="No effects."
    renderRow={(effect, update) => (
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-2">
        <Field label="Type">
          <select className={inputClass} value={effect.type} onChange={(e) => update({ ...effect, type: e.target.value })}>
            <option value="stat_increase">stat_increase</option>
            <option value="set_stat">set_stat</option>
            <option value="grant_tag">grant_tag</option>
            <option value="hook">hook</option>
          </select>
        </Field>
        <Field label="Stat">
          <TextInput value={effect.stat ?? ''} onChange={(stat) => update({ ...effect, stat })} />
        </Field>
        <Field label="Amount">
          <NumberInput value={effect.amount} onChange={(amount) => update({ ...effect, amount })} />
        </Field>
        <Field label="Tag">
          <TextInput value={effect.tag ?? ''} onChange={(tag) => update({ ...effect, tag })} />
        </Field>
      </div>
    )}
  />
);

const UnlocksEditor: React.FC<{ unlocks: UnlockSpec[]; onChange: (next: UnlockSpec[]) => void }> = ({ unlocks, onChange }) => (
  <RowList
    items={unlocks}
    addLabel="Add Unlock"
    onAdd={() => onChange([...unlocks, { id: '', label: '', cost: 1, effects: [] }])}
    onChange={onChange}
    emptyLabel="No unlocks."
    renderRow={(unlock, update) => (
      <div className="space-y-2">
        <div className="grid grid-cols-2 sm:grid-cols-3 gap-2">
          <Field label="ID">
            <TextInput value={unlock.id} onChange={(id) => update({ ...unlock, id })} />
          </Field>
          <Field label="Label">
            <TextInput value={unlock.label} onChange={(label) => update({ ...unlock, label })} />
          </Field>
          <Field label="Cost">
            <NumberInput value={unlock.cost} onChange={(cost) => update({ ...unlock, cost: cost ?? 0 })} />
          </Field>
        </div>
        <EffectsEditor effects={unlock.effects ?? []} onChange={(effects) => update({ ...unlock, effects })} />
      </div>
    )}
  />
);

const LevelsEditor: React.FC<{ levels: LevelSpec[]; onChange: (next: LevelSpec[]) => void }> = ({ levels, onChange }) => (
  <RowList
    items={levels}
    addLabel="Add Level"
    onAdd={() => onChange([...levels, { at: 0 }])}
    onChange={onChange}
    emptyLabel="No levels."
    renderRow={(level, update) => (
      <div className="space-y-2">
        <div className="grid grid-cols-2 gap-2">
          <Field label="At">
            <NumberInput value={level.at} onChange={(at) => update({ ...level, at: at ?? 0 })} />
          </Field>
          <Field label="Label">
            <TextInput value={level.label ?? ''} onChange={(label) => update({ ...level, label })} />
          </Field>
        </div>
        <EffectsEditor effects={level.effects ?? []} onChange={(effects) => update({ ...level, effects })} />
      </div>
    )}
  />
);

const AdvancementEditor: React.FC<{
  advancement: AdvancementSpec | undefined;
  stats: StatSpec[];
  onChange: (next: AdvancementSpec | undefined) => void;
}> = ({ advancement, stats, onChange }) => (
  <div className="space-y-2">
    <label className="flex items-center gap-2 text-xs font-sans text-stone-300">
      <input
        type="checkbox"
        checked={advancement !== undefined}
        onChange={(e) => onChange(e.target.checked ? { currency: { stat: '' }, mode: 'spend' } : undefined)}
      />
      <span>Declare advancement</span>
    </label>
    {advancement && (
      <div className="space-y-3">
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-2">
          <Field label="Currency stat" help={<>The stat that holds earned points, such as xp or insight. Earn rules add to it and unlocks spend it.</>}>
            <StatSelect
              value={advancement.currency.stat}
              stats={stats}
              onChange={(stat) => onChange({ ...advancement, currency: { ...advancement.currency, stat } })}
            />
          </Field>
          <Field label="Currency label">
            <TextInput
              value={advancement.currency.label ?? ''}
              onChange={(label) => onChange({ ...advancement, currency: { ...advancement.currency, label } })}
            />
          </Field>
          <Field label="Mode" help={<>How the currency is spent. spend buys unlocks directly, track fills a single track, and threshold unlocks levels at set values.</>}>
            <select
              className={inputClass}
              value={advancement.mode ?? 'spend'}
              onChange={(e) => onChange({ ...advancement, mode: e.target.value })}
            >
              <option value="spend">spend</option>
              <option value="track">track</option>
              <option value="threshold">threshold</option>
            </select>
          </Field>
        </div>
        <div>
          <span className={`${labelClass} flex items-center gap-1.5`}>
            <span>Earn rules</span>
            <HelpTip label="Earn rules">
              When the currency is awarded and how much, such as one point on a miss or at turn end.
            </HelpTip>
          </span>
          <EarnRulesEditor rules={advancement.earn ?? []} onChange={(earn) => onChange({ ...advancement, earn })} />
        </div>
        <div>
          <span className={`${labelClass} flex items-center gap-1.5`}>
            <span>Unlocks</span>
            <HelpTip label="Unlocks">
              What the currency buys: a cost, optional prerequisites, and the effects it applies, such
              as a stat increase or a granted tag.
            </HelpTip>
          </span>
          <UnlocksEditor unlocks={advancement.unlocks ?? []} onChange={(unlocks) => onChange({ ...advancement, unlocks })} />
        </div>
        <div>
          <span className={`${labelClass} flex items-center gap-1.5`}>
            <span>Levels</span>
            <HelpTip label="Levels">
              Threshold-mode levels: a value at which the level is reached, and the effects it applies.
            </HelpTip>
          </span>
          <LevelsEditor levels={advancement.levels ?? []} onChange={(levels) => onChange({ ...advancement, levels })} />
        </div>
      </div>
    )}
  </div>
);

export interface MechanicsEditorProps {
  mechanics: MechanicsSpec;
  onChange: (next: MechanicsSpec) => void;
}

// MechanicsEditor edits a system's whole mechanics block: stats, skills, health,
// check conventions (including resolution profiles), advancement, freeform state,
// and the engagement default. Every section renders its own add/remove rows, so a
// system with no mechanics shows empty lists and can build one from scratch.
export const MechanicsEditor: React.FC<MechanicsEditorProps> = ({ mechanics, onChange }) => {
  const stats = mechanics.stats ?? [];
  const patch = (next: Partial<MechanicsSpec>) => onChange({ ...mechanics, ...next });
  return (
    <div className="space-y-4">
      <Section
        title="Stats"
        help={
          <>
            A stat is a named value the engine reads and writes, such as Might or Sanity. Checks,
            health, and advancement all point at stats by id. Keep ids lower-case.
          </>
        }
        example={`stats:\n  - { id: might, label: Might, type: number, default: 2, min: 0, max: 6 }\n  - { id: sanity, label: Sanity, type: number, default: 5 }`}
      >
        <StatsEditor stats={stats} onChange={(next) => patch({ stats: next })} />
      </Section>
      <Section
        title="Skills"
        help={
          <>
            A skill is a capability tied to a stat. When a check names the skill, the engine adds the
            governing stat&apos;s value to the roll. A skill with no stat adds nothing.
          </>
        }
      >
        <SkillsEditor skills={mechanics.skills ?? []} stats={stats} onChange={(next) => patch({ skills: next })} />
      </Section>
      <Section
        title="Health"
        help={
          <>
            Health names the stat that represents harm and, optionally, a stat holding its maximum.
            When the health stat reaches zero, the zero effect runs (such as <code>downed</code>).
          </>
        }
      >
        <HealthEditor health={mechanics.health} stats={stats} onChange={(health) => patch({ health })} />
      </Section>
      <Section
        title="Checks"
        help={
          <>
            Checks describe how a roll becomes an outcome. The default notation applies unless a
            profile overrides it; the outcome vocabulary is the list of words a check may return,
            strongest first. The engine stores these words verbatim and never invents semantics.
          </>
        }
        example={`checks:\n  notation: "2d6+{modifier}"\n  outcome: [strong_hit, weak_hit, miss]\n  difficulty:\n    - { id: routine, label: Routine, target: 7 }\n  profiles:\n    standard:\n      shape: ladder\n      ladder:\n        - { min: 10, outcome: strong_hit }\n        - { min: 7, outcome: weak_hit }\n        - { min: 0, outcome: miss }`}
      >
        <ChecksEditor checks={mechanics.checks ?? {}} onChange={(checks) => patch({ checks })} />
      </Section>
      <Section
        title="Advancement"
        help={
          <>
            Advancement declares a currency stat, how it is earned, and what it buys. Mode is spend
            (points you spend), track (a filled track), or threshold (levels at set values). Leave it
            off for a system with no progression.
          </>
        }
      >
        <AdvancementEditor
          advancement={mechanics.advancement}
          stats={stats}
          onChange={(advancement) => patch({ advancement })}
        />
      </Section>
      <Section
        title="Policy"
        help={
          <>
            Policy shapes how the engine treats state and when it engages the mechanics. Allow
            freeform state permits writes to undeclared paths even when stats are declared.
            Engagement is this system&apos;s default mechanics policy for a campaign.
          </>
        }
      >
        <label className="flex items-center gap-2 text-xs font-sans text-stone-300">
          <input
            type="checkbox"
            checked={mechanics.allow_freeform_state ?? false}
            onChange={(e) => patch({ allow_freeform_state: e.target.checked })}
          />
          <span>Allow freeform state</span>
        </label>
        <Field label="Engagement" className="max-w-xs">
          <select className={inputClass} value={mechanics.engagement ?? ''} onChange={(e) => patch({ engagement: e.target.value })}>
            <option value="">(config default)</option>
            <option value="off">off</option>
            <option value="auto">auto</option>
            <option value="ask">ask</option>
          </select>
        </Field>
      </Section>
    </div>
  );
};
