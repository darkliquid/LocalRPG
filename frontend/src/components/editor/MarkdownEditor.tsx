import { useEffect, useRef } from 'react';
import { EditorState } from '@codemirror/state';
import { EditorView, keymap, placeholder as placeholderExt } from '@codemirror/view';
import { basicSetup } from 'codemirror';
import { indentWithTab } from '@codemirror/commands';
import { languageExtensions, type EditorLanguage } from './languages';
import { editorTheme } from './theme';

export interface MarkdownEditorProps {
  value: string;
  onChange: (next: string) => void;
  language: EditorLanguage;
  onSave?: () => void;
  readOnly?: boolean;
  placeholder?: string;
  minHeight?: string;
  ariaLabel: string;
}

// MarkdownEditor owns its CodeMirror document. It is uncontrolled on purpose:
// value seeds the editor and onChange reports every transaction. The parent must
// pass a key that changes with the document identity (the note id, the file
// path), which is what keeps undo history and the cursor correct and avoids the
// cursor jump a controlled CodeMirror causes.
export default function MarkdownEditor({
  value,
  onChange,
  language,
  onSave,
  readOnly,
  placeholder,
  minHeight = '240px',
  ariaLabel,
}: MarkdownEditorProps) {
  const hostRef = useRef<HTMLDivElement | null>(null);
  const viewRef = useRef<EditorView | null>(null);

  // The callbacks are read through refs so a parent re-render does not tear the
  // editor down and lose the cursor.
  const onChangeRef = useRef(onChange);
  const onSaveRef = useRef(onSave);
  onChangeRef.current = onChange;
  onSaveRef.current = onSave;

  useEffect(() => {
    const host = hostRef.current;
    if (!host) return;

    const extensions = [
      basicSetup,
      languageExtensions(language),
      editorTheme,
      EditorView.lineWrapping,
      EditorState.readOnly.of(!!readOnly),
      EditorView.contentAttributes.of({ 'aria-label': ariaLabel }),
      EditorView.updateListener.of((update) => {
        if (update.docChanged) onChangeRef.current(update.state.doc.toString());
      }),
      keymap.of([
        {
          key: 'Mod-s',
          preventDefault: true,
          run: () => {
            onSaveRef.current?.();
            return true;
          },
        },
        indentWithTab,
      ]),
    ];
    if (placeholder) extensions.push(placeholderExt(placeholder));

    const view = new EditorView({
      parent: host,
      state: EditorState.create({ doc: value, extensions }),
    });
    viewRef.current = view;

    return () => {
      view.destroy();
      viewRef.current = null;
    };
    // Created once on purpose: the parent remounts this component via key when a
    // different document is loaded.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <div
      ref={hostRef}
      className="w-full min-h-0 overflow-hidden rounded-xl border border-stone-800 bg-stone-950 transition-colors focus-within:border-purple-500/50"
      style={{ minHeight, height: '100%' }}
    />
  );
}
