import { json } from "@codemirror/lang-json";
import { yaml } from "@codemirror/lang-yaml";
import { Compartment, EditorState } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import { basicSetup } from "codemirror";
import { useEffect, useRef } from "react";
import { editorTheme, useHtmlDark } from "@/features/resources/code-theme";
import { cn } from "@/lib/utils";

/** CodeField is a CodeMirror editor for YAML or JSON text. */
export function CodeField({
  value,
  onChange,
  language = "yaml",
  label,
  minHeight = "6rem",
  className,
}: {
  value: string;
  onChange: (next: string) => void;
  language?: "yaml" | "json";
  label: string;
  minHeight?: string;
  className?: string;
}) {
  const host = useRef<HTMLDivElement>(null);
  const view = useRef<EditorView | null>(null);
  const change = useRef(onChange);
  change.current = onChange;
  const dark = useHtmlDark();
  const themeSlot = useRef(new Compartment());
  const initialDark = useRef(dark);

  useEffect(() => {
    if (!host.current) return;
    const v = new EditorView({
      parent: host.current,
      state: EditorState.create({
        doc: value,
        extensions: [
          basicSetup,
          language === "json" ? json() : yaml(),
          EditorView.contentAttributes.of({ "aria-label": label }),
          themeSlot.current.of(editorTheme(initialDark.current)),
          EditorView.theme({ "&": { minHeight } }),
          EditorView.updateListener.of((u) => {
            if (u.docChanged) change.current(u.state.doc.toString());
          }),
        ],
      }),
    });
    view.current = v;
    return () => v.destroy();
    // The editor is created once; later value changes sync below.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [language]);

  useEffect(() => {
    view.current?.dispatch({ effects: themeSlot.current.reconfigure(editorTheme(dark)) });
  }, [dark]);

  useEffect(() => {
    const v = view.current;
    if (v && v.state.doc.toString() !== value) {
      v.dispatch({ changes: { from: 0, to: v.state.doc.length, insert: value } });
    }
  }, [value]);

  return <div ref={host} className={cn("overflow-hidden rounded-control border bg-panel text-sm", className)} />;
}
