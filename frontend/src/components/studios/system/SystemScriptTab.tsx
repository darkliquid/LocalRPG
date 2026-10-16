import MarkdownEditor from '../../editor/MarkdownEditor';
import { ScriptReference } from '../../mechanics/ScriptReference';

export interface SystemScriptTabProps {
  /** slugID keys the editor, so switching systems rebuilds it rather than reusing it. */
  slugID: string;
  savedID: string | null;
  script: string;
  setScript: (script: string) => void;
  markDirty: () => void;
}

// SystemScriptTab is the script the engine runs for this system: the Goja sandbox
// the mechanics hooks are evaluated in.
export function SystemScriptTab({ slugID, savedID, script, setScript, markDirty }: SystemScriptTabProps) {
  return (
    <div className="flex-1 flex flex-col gap-2 min-h-0 overflow-hidden">
      <div className="flex items-center justify-between text-xs font-mono text-stone-400 px-1 shrink-0">
        <span>JavaScript Runtime (Goja Sandbox)</span>
        <span>Exports: evaluateRoll(stats, diceExpr)</span>
      </div>
      <ScriptReference />
      <MarkdownEditor
        key={`${savedID || slugID || 'draft'}-script`}
        value={script}
        onChange={(next) => {
          setScript(next);
          markDirty();
        }}
        language="javascript"
        ariaLabel="System mechanics script"
        placeholder="export function evaluateRoll(stats, diceExpr) { ... }"
      />
    </div>
  );
}
