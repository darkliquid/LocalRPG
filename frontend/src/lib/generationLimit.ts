import { HTTPError } from '../api/client';

// generationLimitCode is the code a generation endpoint returns when it stopped
// because it reached a configured limit.
export const generationLimitCode = 'generation_limit';

// generationLimitHint says where to change such a limit. The failure names a
// config key, which is not much help from a dialog that has no settings in it.
export const generationLimitHint = 'Change it in Settings → AI Agents → Generation Limits.';

// isGenerationLimit reports whether a failure is a configured limit the user can
// raise, so a caller can point at the setting rather than only printing the text.
export function isGenerationLimit(err: unknown): boolean {
  return err instanceof HTTPError && err.code === generationLimitCode;
}
