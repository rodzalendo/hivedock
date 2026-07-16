import { useEffect, useState } from "react";

// Theme registry + persistence.
//
// Every theme is just a value for the `data-theme` attribute on <html>; the
// actual colors, fonts, and effects live in index.css (CSS-variable overrides
// keyed off that attribute) and tailwind.config.js (which points every color
// scale at those variables). So switching themes is a single attribute write —
// no component touches colors directly.

export type ThemeId =
  | "hive"
  | "glossy"
  | "paper"
  | "fallout"
  | "cyberpunk"
  | "nord";

export type Theme = {
  id: ThemeId;
  name: string;
  blurb: string;
  // Two swatch colors for the picker preview: [surface, accent].
  swatch: [string, string];
};

export const THEMES: Theme[] = [
  {
    id: "hive",
    name: "Hive Dark",
    blurb: "The original cool-gray console with honeycomb amber.",
    swatch: ["#15181e", "#d9a13c"],
  },
  {
    id: "glossy",
    name: "Modern Glossy",
    blurb: "Frosted-glass panels over a violet gradient.",
    swatch: ["#151824", "#8b5cf6"],
  },
  {
    id: "paper",
    name: "Minimalist Paper",
    blurb: "Serif ink on off-white with hairline rules.",
    swatch: ["#f0eee7", "#3f5f85"],
  },
  {
    id: "fallout",
    name: "Fallout",
    blurb: "Pip-Boy phosphor green on a CRT, scanlines and all.",
    swatch: ["#0e1c0b", "#5be84a"],
  },
  {
    id: "cyberpunk",
    name: "Cyberpunk",
    blurb: "Pixel-art neon — hot magenta and cyan on midnight.",
    swatch: ["#120a1f", "#ff3ea5"],
  },
  {
    id: "nord",
    name: "Nord",
    blurb: "Arctic calm — muted blue-grays with icy frost accents.",
    swatch: ["#2e3440", "#88c0d0"],
  },
];

const STORAGE_KEY = "hivedock-theme";
export const DEFAULT_THEME: ThemeId = "hive";

export function getStoredTheme(): ThemeId {
  try {
    const v = localStorage.getItem(STORAGE_KEY);
    if (v && THEMES.some((t) => t.id === v)) return v as ThemeId;
  } catch {
    /* localStorage may be unavailable (private mode) */
  }
  return DEFAULT_THEME;
}

export function applyTheme(id: ThemeId): void {
  const root = document.documentElement;
  root.dataset.theme = id;
  // Keep Tailwind's `dark` class in sync with whether the theme is light, so
  // any future `dark:` utilities behave. Paper is our only light theme.
  root.classList.toggle("dark", id !== "paper");
}

export function setTheme(id: ThemeId): void {
  applyTheme(id);
  try {
    localStorage.setItem(STORAGE_KEY, id);
  } catch {
    /* ignore persistence failures */
  }
}

// Apply the persisted theme as early as possible (called from main.tsx before
// React renders) to minimize a flash of the default theme.
export function initTheme(): void {
  applyTheme(getStoredTheme());
}

// Paper is our only light theme; components that host their own dark/light
// widgets (the CodeMirror editors) use this to match.
export function isLightTheme(id: ThemeId): boolean {
  return id === "paper";
}

// useTheme tracks the live theme by observing the `data-theme` attribute, so
// components re-render when the user switches themes from Settings.
export function useTheme(): ThemeId {
  const [theme, setThemeState] = useState<ThemeId>(() => getStoredTheme());
  useEffect(() => {
    const root = document.documentElement;
    const read = () =>
      setThemeState((root.dataset.theme as ThemeId) || DEFAULT_THEME);
    read();
    const obs = new MutationObserver(read);
    obs.observe(root, { attributes: true, attributeFilter: ["data-theme"] });
    return () => obs.disconnect();
  }, []);
  return theme;
}
