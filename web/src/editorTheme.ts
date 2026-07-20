import { EditorView } from "@codemirror/view";

// appEditorTheme repaints CodeMirror's chrome in the app's own palette.
//
// @uiw/react-codemirror's built-in "dark" theme is oneDark, whose background is
// a hardcoded blue-grey (#282c34). That reads as a foreign panel next to the
// zinc surfaces around it — and because it only knows "light" vs "dark", it
// stayed the same colour under all six themes. Driving the colours from the
// --c-zinc-* custom properties (the same ones Tailwind's zinc scale is remapped
// onto) makes the editor match the Logs pane and follow every theme.
//
// Applied *after* the base theme in the extensions array so these declarations
// win. Only the surfaces are overridden — syntax highlighting still comes from
// the base theme, so YAML stays readable in both light and dark.
export const appEditorTheme = EditorView.theme({
  "&": {
    backgroundColor: "rgb(var(--c-zinc-950))",
    color: "rgb(var(--c-zinc-300))",
  },
  ".cm-content": { caretColor: "rgb(var(--c-zinc-100))" },
  ".cm-cursor, .cm-dropCursor": { borderLeftColor: "rgb(var(--c-zinc-100))" },
  "&.cm-focused": { outline: "none" },
  ".cm-gutters": {
    backgroundColor: "rgb(var(--c-zinc-950))",
    color: "rgb(var(--c-zinc-600))",
    border: "none",
  },
  ".cm-activeLine": { backgroundColor: "rgb(var(--c-zinc-800) / 0.35)" },
  ".cm-activeLineGutter": {
    backgroundColor: "rgb(var(--c-zinc-800) / 0.35)",
    color: "rgb(var(--c-zinc-400))",
  },
  ".cm-selectionBackground, &.cm-focused .cm-selectionBackground, .cm-content ::selection":
    { backgroundColor: "rgb(var(--c-zinc-700) / 0.6)" },
  ".cm-panels": {
    backgroundColor: "rgb(var(--c-zinc-900))",
    color: "rgb(var(--c-zinc-300))",
  },
  ".cm-placeholder": { color: "rgb(var(--c-zinc-600))" },
});
