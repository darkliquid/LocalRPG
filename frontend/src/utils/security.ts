/**
 * safeImagePreview validates that an image source URL begins strictly with
 * an expected safe scheme before rendering into the DOM.
 */
export function safeImagePreview(url: string | null | undefined): string | undefined {
  if (!url) return undefined;
  const trimmed = url.trim();
  if (
    trimmed.startsWith('blob:') ||
    trimmed.startsWith('/api/') ||
    trimmed.startsWith('data:image/')
  ) {
    return trimmed;
  }
  return undefined;
}
