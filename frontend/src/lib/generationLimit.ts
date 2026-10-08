// The codes a generation endpoint returns when it stopped because it reached a
// limit. They are separate so the UI can offer the field that caused it.
export const callLimitCode = 'generation_call_limit';
export const sourceLimitCode = 'generation_source_limit';

// generationLimitHint says where to change a limit permanently, for the case
// where the user would rather set it once than be asked again.
export const generationLimitHint = 'You can also set it in Settings → AI Agents → Generation Limits.';

// limitFieldFor names the config field a limit code refers to, or null when the
// code is not a limit.
export function limitFieldFor(code: string | undefined): 'max_calls' | 'max_chunks' | null {
  switch (code) {
    case callLimitCode:
      return 'max_calls';
    case sourceLimitCode:
      return 'max_chunks';
    default:
      return null;
  }
}
