// playVoicePreview plays a catalog preview without touching the narration queue,
// so auditioning a voice never interrupts the scene.
export function playVoicePreview(url: string, volume = 1): void {
  const audio = new Audio(url);
  audio.volume = Math.min(1, Math.max(0, volume));
  audio.play().catch(() => undefined);
}
