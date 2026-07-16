/** @type {import('tailwindcss').Config} */

// Every color resolves through a CSS variable holding space-separated RGB
// channels (e.g. "16 18 22"), wrapped so Tailwind's opacity modifiers keep
// working (`bg-hive-500/20`). The channel values are defined per theme in
// index.css and swapped by the `data-theme` attribute on <html>, so switching
// themes never touches a component. `v()` builds one such reference.
const v = (name) => `rgb(var(--c-${name}) / <alpha-value>)`;

const scale = (prefix, shades) =>
  Object.fromEntries(shades.map((s) => [s, v(`${prefix}-${s}`)]));

export default {
  darkMode: "class",
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      fontFamily: {
        sans: ["var(--font-sans)", "system-ui", "sans-serif"],
        mono: ["var(--font-mono)", "ui-monospace", "monospace"],
      },
      colors: {
        // Neutral ramp: 950 = app bg, 900 = card/panel, 800/700 = borders,
        // 600→50 = text ramp. Remapped from `zinc` so existing utilities work.
        zinc: scale("zinc", [50, 100, 200, 300, 400, 500, 600, 700, 800, 900, 950]),
        // Brand + "attention" (updates, drift).
        hive: scale("hive", [400, 500, 600]),
        // Primary interactive accent.
        accent: scale("accent", [400, 500, 600]),
        // Semantic status colors — also theme-swapped so, e.g., Fallout keeps
        // its everything-is-green look and Paper prints them as muted ink.
        green: scale("green", [400, 500]),
        amber: scale("amber", [200, 300, 400, 500, 600]),
        red: scale("red", [200, 300, 400, 500, 900, 950]),
        sky: scale("sky", [400, 500]),
        emerald: scale("emerald", [400]),
      },
    },
  },
  plugins: [],
};
