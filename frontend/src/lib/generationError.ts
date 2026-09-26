import type { GenerationFailure } from '../types';

// formatGenerationError leads with the provider's own message and keeps the
// bounded code as a suffix, so a user sees why a generation failed.
export function formatGenerationError(failure: GenerationFailure): string {
  const message = failure.message?.trim();
  if (message) return `${message} (${failure.code})`;
  return failure.code;
}

// generationAttemptLines renders the per-provider fallback chain for a details
// disclosure.
export function generationAttemptLines(failure: GenerationFailure): string[] {
  return (failure.attempts ?? []).map((attempt) => {
    const detail = attempt.detail?.trim() ? `: ${attempt.detail}` : '';
    return `${attempt.role}/${attempt.provider} [${attempt.code}]${detail}`;
  });
}
