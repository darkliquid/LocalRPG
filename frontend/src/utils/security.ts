/**
 * safeImagePreview validates that an image source URL begins strictly with
 * an expected safe scheme before rendering into the DOM.
 */
export function safeImagePreview(url: string | null | undefined): string | undefined {
  if (!url) return undefined;
  const trimmed = url.trim();
  if (/^(?:blob:|\/api\/|data:image\/(?:png|jpeg|webp|gif|svg\+xml);base64,)/i.test(trimmed)) {
    return trimmed;
  }
  return undefined;
}
