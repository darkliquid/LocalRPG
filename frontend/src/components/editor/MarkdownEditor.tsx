import { useEffect, useRef, useState } from 'react';
import { Compartment, EditorState, StateEffect, StateField, type Extension } from '@codemirror/state';
import { EditorView, keymap, placeholder as placeholderExt } from '@codemirror/view';
import { autocompletion, type CompletionSource } from '@codemirror/autocomplete';
import type { Diagnostic } from '@codemirror/lint';
import { basicSetup } from 'codemirror';
import { indentWithTab } from '@codemirror/commands';
import type { EntitySummary, FrontmatterSchema } from '../../types';
import { languageExtensions, type EditorLanguage } from './languages';
import { editorTheme } from './theme';
import { loadFrontmatterSchema } from './frontmatterSchema';
import {
  frontmatterHover,
  frontmatterKeyCompletion,
  frontmatterLinter,
  frontmatterValueCompletion,
} from './frontmatter';
import { wikilinkCompletion } from './wikilink';

export interface MarkdownEditorProps {
  value: string;
  onChange: (next: string) => void;
  language: EditorLanguage;
  onSave?: () => void;
  readOnly?: boolean;
  placeholder?: string;
  minHeight?: string;
  ariaLabel: string;
  // frontmatterSchema overrides the schema the editor loads for itself. Leave it
  // unset and a frontmatter document still completes.
  frontmatterSchema?: FrontmatterSchema;
  linkTargets?: EntitySummary[];
  serverError?: { line: number; message: string } | null;
}

// setServerDiagnostic carries the parse failure the server reported back into the
// editor, so a rejected save underlines the line it named.
const setServerDiagnostic = StateEffect.define<Diagnostic | null>();

const serverDiagnosticField = StateField.define<Diagnostic | null>({
  create: () => null,
  update(value, tr) {
    for (const effect of tr.effects) {
      if (effect.is(setServerDiagnostic)) return effect.value;
    }
    return value;
  },
});

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
  frontmatterSchema,
  linkTargets,
  serverError,
}: MarkdownEditorProps) {
  const hostRef = useRef<HTMLDivElement | null>(null);
  const viewRef = useRef<EditorView | null>(null);

  // The callbacks are read through refs so a parent re-render does not tear the
  // editor down and lose the cursor.
  const onChangeRef = useRef(onChange);
  const onSaveRef = useRef(onSave);
  onChangeRef.current = onChange;
  onSaveRef.current = onSave;

  // lastEmitted is the document text this editor last reported upwards. It is what
  // distinguishes a change that came from the user (ignore, the editor already has
  // it) from one that came from outside (replace, or the editor would keep showing
  // the previous document). Without it an uncontrolled editor silently ignores a
  // new value prop, which is how clicking a note opened the previous note.
  const lastEmittedRef = useRef(value);

  // Intelligence arrives after the first render (the schema and the note list are
  // fetched), so it lives in a compartment that can be reconfigured rather than
  // being fixed at creation.
  const intelligenceCompartment = useRef(new Compartment());
  const [loadedSchema, setLoadedSchema] = useState<FrontmatterSchema | undefined>(frontmatterSchema);
  const schemaRef = useRef(loadedSchema);
  const linksRef = useRef(linkTargets);
  schemaRef.current = loadedSchema;
  linksRef.current = linkTargets;

  // A frontmatter document always gets its schema, so no caller can ship an editor
  // with completion missing by forgetting a prop.
  useEffect(() => {
    if (frontmatterSchema || language !== 'markdown-frontmatter') return;
    let cancelled = false;
    void loadFrontmatterSchema().then((schema) => {
      if (!cancelled && schema) setLoadedSchema(schema);
    });
    return () => {
      cancelled = true;
    };
  }, [frontmatterSchema, language]);

  useEffect(() => {
    if (frontmatterSchema) setLoadedSchema(frontmatterSchema);
  }, [frontmatterSchema]);

  const buildIntelligence = (): Extension[] => {
    const schema = schemaRef.current;
    const links = linksRef.current ?? [];
    const extensions: Extension[] = [serverDiagnosticField];

    if (schema) {
      const sources: CompletionSource[] = [
        frontmatterKeyCompletion(schema),
        frontmatterValueCompletion(schema),
      ];
      if (links.length > 0) sources.push(wikilinkCompletion(links));
      extensions.push(
        autocompletion({ override: sources }),
        frontmatterLinter(schema),
        frontmatterHover(schema),
      );
    } else if (links.length > 0) {
      extensions.push(autocompletion({ override: [wikilinkCompletion(links)] }));
    }
    return extensions;
  };

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
        if (update.docChanged) {
          const next = update.state.doc.toString();
          lastEmittedRef.current = next;
          onChangeRef.current(next);
        }
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
      intelligenceCompartment.current.of(buildIntelligence()),
    ];
    if (placeholder) extensions.push(placeholderExt(placeholder));

    const view = new EditorView({
      parent: host,
      state: EditorState.create({ doc: value, extensions }),
    });
    viewRef.current = view;
    lastEmittedRef.current = value;

    return () => {
      view.destroy();
      viewRef.current = null;
    };
    // Created once on purpose: the parent remounts this component via key when a
    // different document is loaded.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // An incoming value the editor did not produce is an external change: a
  // different document, or an edit applied above the editor such as inserting a
  // voice archetype. The document is replaced wholesale, because the two texts
  // have nothing in common to diff.
  useEffect(() => {
    const view = viewRef.current;
    if (!view) return;

    const current = view.state.doc.toString();
    if (value === current) {
      lastEmittedRef.current = value;
      return;
    }
    if (value === lastEmittedRef.current) return;

    lastEmittedRef.current = value;
    view.dispatch({
      changes: { from: 0, to: current.length, insert: value },
      selection: { anchor: 0 },
    });
  }, [value]);

  useEffect(() => {
    const view = viewRef.current;
    if (!view) return;
    view.dispatch({
      effects: intelligenceCompartment.current.reconfigure(buildIntelligence()),
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [loadedSchema, linkTargets]);

  useEffect(() => {
    const view = viewRef.current;
    if (!view) return;
    if (!serverError) {
      view.dispatch({ effects: setServerDiagnostic.of(null) });
      return;
    }
    const lineNumber = Math.min(Math.max(serverError.line, 1), view.state.doc.lines);
    const line = view.state.doc.line(lineNumber);
    view.dispatch({
      effects: setServerDiagnostic.of({
        from: line.from,
        to: line.to,
        severity: 'error',
        message: serverError.message,
      }),
    });
  }, [serverError]);

  return (
    <div
      ref={hostRef}
      // No overflow clipping here: CodeMirror positions its completion popup
      // absolutely inside the editor, so overflow-hidden would cut the popup off
      // at the editor's own edge.
      className="w-full min-h-0 rounded-xl border border-stone-800 bg-stone-950 transition-colors focus-within:border-purple-500/50"
      style={{ minHeight, height: '100%' }}
    />
  );
}
