/** @type {import('tailwindcss').Config} */
export default {
  darkMode: "class",
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        // Placeholder brand accent; refine in Phase 5 theming.
        hive: {
          500: "#f5a623",
          600: "#e0930f",
        },
      },
    },
  },
  plugins: [],
};
