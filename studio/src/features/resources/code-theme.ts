import { HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import type { Extension } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import { tags as t } from "@lezer/highlight";
import { useSyncExternalStore } from "react";

function subscribeHtmlClass(cb: () => void) {
  const obs = new MutationObserver(cb);
  obs.observe(document.documentElement, { attributes: true, attributeFilter: ["class"] });
  return () => obs.disconnect();
}

/** useHtmlDark reports whether the app theme is dark (the dark class on the html element). */
export function useHtmlDark(): boolean {
  return useSyncExternalStore(
    subscribeHtmlClass,
    () => document.documentElement.classList.contains("dark"),
    () => false,
  );
}

// Syntax colors in the manner of the Xcode default themes. Each color reaches 4.5:1 on the
// panel and on the active line in its theme (REQ-UI-011).
const palette = {
  light: {
    keyword: "#9b2393",
    string: "#c41a16",
    number: "#1c00cf",
    comment: "#5d6c79",
    type: "#1c6e77",
    property: "#6c36a9",
    fn: "#0f68a0",
    attribute: "#815f03",
  },
  dark: {
    keyword: "#ff7ab2",
    string: "#ff8170",
    number: "#d9c97c",
    comment: "#8a97a3",
    type: "#5dd8ff",
    property: "#4eb0cc",
    fn: "#67b7a4",
    attribute: "#cc9768",
  },
};

function highlight(c: (typeof palette)["light"]) {
  return HighlightStyle.define([
    { tag: [t.keyword, t.controlKeyword, t.modifier, t.operatorKeyword, t.bool, t.null, t.self], color: c.keyword },
    { tag: [t.string, t.special(t.string), t.character, t.regexp, t.escape], color: c.string },
    { tag: [t.number, t.integer, t.float, t.atom], color: c.number },
    { tag: [t.comment, t.lineComment, t.blockComment, t.docComment], color: c.comment },
    { tag: [t.typeName, t.className, t.namespace, t.standard(t.typeName)], color: c.type },
    {
      tag: [t.propertyName, t.definition(t.propertyName), t.attributeName, t.special(t.variableName)],
      color: c.property,
    },
    {
      tag: [
        t.function(t.variableName),
        t.function(t.propertyName),
        t.definition(t.variableName),
        t.standard(t.variableName),
      ],
      color: c.fn,
    },
    { tag: [t.meta, t.annotation, t.labelName, t.attributeValue], color: c.attribute },
    { tag: t.invalid, color: c.string, textDecoration: "underline wavy" },
  ]);
}

const lightHighlight = highlight(palette.light);
const darkHighlight = highlight(palette.dark);

/** The editor chrome takes the panel tokens, so it follows the app theme without its own colors. */
export function editorTheme(dark: boolean): Extension {
  const theme = EditorView.theme(
    {
      "&": { backgroundColor: "var(--panel)", color: "var(--foreground)", fontSize: "12.5px", height: "100%" },
      "&.cm-focused": { outline: "none" },
      ".cm-scroller": { fontFamily: "var(--font-mono)", lineHeight: "1.65" },
      ".cm-content": { caretColor: "var(--accent)", padding: "6px 0" },
      ".cm-line": { padding: "0 12px 0 6px" },
      ".cm-cursor, .cm-dropCursor": { borderLeftColor: "var(--accent)", borderLeftWidth: "2px" },
      ".cm-gutters": {
        backgroundColor: "var(--panel)",
        color: "var(--muted-foreground)",
        border: "none",
      },
      ".cm-lineNumbers .cm-gutterElement": { padding: "0 6px 0 14px", minWidth: "36px" },
      ".cm-activeLine": { backgroundColor: "color-mix(in srgb, var(--muted) 70%, transparent)" },
      ".cm-activeLineGutter": { backgroundColor: "transparent", color: "var(--foreground)" },
      ".cm-selectionBackground": { backgroundColor: "color-mix(in srgb, var(--accent) 16%, transparent)" },
      "&.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground": {
        backgroundColor: "color-mix(in srgb, var(--accent) 26%, transparent)",
      },
      ".cm-matchingBracket, &.cm-focused .cm-matchingBracket": {
        backgroundColor: "color-mix(in srgb, var(--accent) 18%, transparent)",
        outline: "none",
        borderRadius: "2px",
      },
      ".cm-tooltip": {
        backgroundColor: "var(--panel)",
        border: "1px solid var(--border)",
        borderRadius: "8px",
        boxShadow: "var(--shadow-float)",
        color: "var(--foreground)",
        overflow: "hidden",
      },
      ".cm-diagnostic": { padding: "6px 10px", fontFamily: "var(--font-sans)", fontSize: "12px" },
    },
    { dark },
  );
  return [theme, syntaxHighlighting(dark ? darkHighlight : lightHighlight)];
}
