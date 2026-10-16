import { AIGenerateButton } from '../../ui/AIGenerateButton';
import MarkdownEditor from '../../editor/MarkdownEditor';
import { GenerationFailure } from '../../../types';

export interface WorldPromptTabProps {
  lorePrompt: string;
  setLorePrompt: (lore: string) => void;
  defaultSystem: string;
  markDirty: () => void;
  /** slugID and savedID key the editor, so switching worlds rebuilds it. */
  slugID: string;
  savedID: string | null;
  reportGenerationError: (failure: GenerationFailure) => void;
  getWorldContext: () => Record<string, string>;
}

// WorldPromptTab edits prompts/lore.md: the atmosphere and faction instructions
// injected into the model's context for this world.
export function WorldPromptTab({
  lorePrompt,
  setLorePrompt,
  defaultSystem,
  markDirty,
  slugID,
  savedID,
  reportGenerationError,
  getWorldContext,
}: WorldPromptTabProps) {
  return (
    <div className="flex-1 flex flex-col gap-2 min-h-0 overflow-hidden">
      <div className="flex items-center justify-between text-xs font-mono text-stone-400 px-1 shrink-0">
        <span>AI Storyteller Atmosphere Instructions (prompts/lore.md)</span>
        <div className="flex items-center gap-2">
          <span>Injected into LLM context to guide sensory tone &amp; faction conflicts</span>
          <AIGenerateButton
            formType="world"
            fieldName="lore_prompt"
            onError={reportGenerationError}
            getContext={getWorldContext}
            onGenerated={(val) => {
              setLorePrompt(val);
              markDirty();
            }}
            systemID={defaultSystem}
            seed={lorePrompt}
          />
        </div>
      </div>
      <MarkdownEditor
        key={`${savedID || slugID || 'draft'}-lore`}
        value={lorePrompt}
        onChange={(next) => {
          setLorePrompt(next);
          markDirty();
        }}
        language="markdown"
        ariaLabel="World lore prompt"
        placeholder="Describe the sensory tone, factions and conflicts the storyteller should hold in mind..."
      />
    </div>
  );
}
