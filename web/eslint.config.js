import js from "@eslint/js";
import globals from "globals";
import tsParser from "@typescript-eslint/parser";
import tsPlugin from "@typescript-eslint/eslint-plugin";
import reactHooks from "eslint-plugin-react-hooks";
import reactRefresh from "eslint-plugin-react-refresh";

// ESLint 9 flat config, replacing .eslintrc.cjs. Same rule set as before:
// eslint:recommended + @typescript-eslint/recommended + react-hooks/recommended,
// plus react-refresh's component-export check. Flat config drops `--ext`, so the
// file patterns live here instead of in the lint script.
export default [
  { ignores: ["dist/**", "vite.config.ts", "eslint.config.js"] },
  js.configs.recommended,
  {
    files: ["**/*.{ts,tsx}"],
    languageOptions: {
      parser: tsParser,
      ecmaVersion: 2022,
      sourceType: "module",
      globals: globals.browser,
    },
    plugins: {
      "@typescript-eslint": tsPlugin,
      "react-hooks": reactHooks,
      "react-refresh": reactRefresh,
    },
    rules: {
      ...tsPlugin.configs.recommended.rules,
      ...reactHooks.configs.recommended.rules,
      // TypeScript already errors on unknown identifiers, and unlike ESLint it
      // knows about type-only globals (React, JSX, RequestInfo). This mirrors
      // what @typescript-eslint's eslint-recommended overrides do for .ts/.tsx;
      // leaving it on reports those types as undefined.
      "no-undef": "off",
      "react-refresh/only-export-components": [
        "warn",
        { allowConstantExport: true },
      ],
    },
  },
];
