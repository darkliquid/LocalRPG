// hasWebSpeechSupport reports whether this window exposes the browser speech
// recognition API. It is absent in the Wails webview (WebKit and most embedded
// engines), so callers must offer a server-side provider there.
export function hasWebSpeechSupport(): boolean {
  const w = window as unknown as {
    SpeechRecognition?: unknown;
    webkitSpeechRecognition?: unknown;
  };
  return Boolean(w.SpeechRecognition || w.webkitSpeechRecognition);
}
