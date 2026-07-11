/** @type {import('tailwindcss').Config} */
export default {
  darkMode: "class",
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      fontFamily: {
        sans: ["'IBM Plex Sans'", "system-ui", "sans-serif"],
        mono: ["'IBM Plex Mono'", "ui-monospace", "monospace"],
      },
      colors: {
        // Cool blue-grays from the Hivedock design mock. We remap the whole
        // `zinc` scale so existing utility classes pick up the palette without
        // per-component edits: 950 = app bg, 900 = card/panel surface,
        // 800 = borders, 700 = control borders, 600→100 = text ramp.
        zinc: {
          50: "#eef1f5",
          100: "#e2e6ec",
          200: "#d6dae2",
          300: "#c8cdd6",
          400: "#8b93a1",
          500: "#7b8391",
          600: "#5b6270",
          700: "#2a303b",
          800: "#1d222a",
          900: "#15181e",
          950: "#101216",
        },
        // Brand + "attention" (updates, drift): honeycomb amber.
        hive: {
          400: "#e2b45f",
          500: "#d9a13c",
          600: "#d9a13c",
        },
        // Primary interactive accent: the design's blue.
        accent: {
          400: "#9dbfe6",
          500: "#7fa9dd",
          600: "#6f9bd4",
        },
      },
    },
  },
  plugins: [],
};
