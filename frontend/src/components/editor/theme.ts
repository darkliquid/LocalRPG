import { EditorView } from '@codemirror/view';

// editorTheme keeps the editor inside the app's palette rather than looking like
// a foreign widget. The values mirror the stone and purple tokens the studios use.
export const editorTheme = EditorView.theme(
  {
    '&': {
      color: '#e7e5e4',
      backgroundColor: 'transparent',
      fontSize: '0.75rem',
      height: '100%',
    },
    '.cm-content': {
      fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace',
      padding: '0.75rem',
      caretColor: '#c084fc',
    },
    '.cm-scroller': { lineHeight: '1.6', overflow: 'auto' },
    '.cm-gutters': {
      backgroundColor: 'rgba(0,0,0,0.35)',
      color: '#78716c',
      border: 'none',
    },
    '.cm-activeLine': { backgroundColor: 'rgba(168,85,247,0.06)' },
    '.cm-activeLineGutter': { backgroundColor: 'rgba(168,85,247,0.10)', color: '#c084fc' },
    '&.cm-focused .cm-selectionBackground, .cm-selectionBackground': {
      backgroundColor: 'rgba(147,51,234,0.35)',
    },
    '.cm-cursor': { borderLeftColor: '#c084fc' },
    '.cm-placeholder': { color: '#78716c' },
    '.cm-tooltip': {
      backgroundColor: '#1c1917',
      border: '1px solid rgba(255,255,255,0.1)',
      borderRadius: '0.5rem',
    },
  },
  { dark: true },
);
