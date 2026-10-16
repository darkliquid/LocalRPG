import { AIGenerateButton } from '../../ui/AIGenerateButton';
import MarkdownEditor from '../../editor/MarkdownEditor';
import { GenerationFailure } from '../../../types';

export interface SystemRulesTabProps {
  rulesPrompt: string;
  setRulesPrompt: (rules: string) => void;
  markDirty: () => void;
  /** slugID and savedID key the editor, so switching systems rebuilds it. */
  slugID: string;
  savedID: string | null;
  reportGenerationError: (failure: GenerationFailure) => void;
  getSystemContext: () => Record<string, string>;
}

// SystemRulesTab edits prompts/rules.md: the instructions injected into the
// model's context, which say how this system resolves an action.
export function SystemRulesTab({
  rulesPrompt,
  setRulesPrompt,
  markDirty,
  slugID,
  savedID,
  reportGenerationError,
  getSystemContext,
}: SystemRulesTabProps) {
  return (
    <div className="flex-1 flex flex-col gap-2 min-h-0 overflow-hidden">
      <div className="flex items-center justify-between text-xs font-mono text-stone-400 px-1 shrink-0">
        <span>AI Storyteller Instructions (prompts/rules.md)</span>
        <div className="flex items-center gap-2">
          <span>Injected into LLM context to guide resolution ladder &amp; mechanics hooks</span>
          <AIGenerateButton
            formType="system"
            fieldName="rules_prompt"
            onError={reportGenerationError}
            getContext={getSystemContext}
            onGenerated={(val) => {
              setRulesPrompt(val);
              markDirty();
            }}
            seed={rulesPrompt}
          />
        </div>
      </div>
      <MarkdownEditor
        key={`${savedID || slugID || 'draft'}-rules`}
        value={rulesPrompt}
        onChange={(next) => {
          setRulesPrompt(next);
          markDirty();
        }}
        language="markdown"
        ariaLabel="System rules prompt"
        placeholder="Describe the resolution philosophy, dice mechanics and character stats the engine should follow..."
      />
    </div>
  );
}
