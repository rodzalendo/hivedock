import { useState } from "react";

// Shared "wrap long lines" preference for the compose / .env editors. Off by
// default; remembered in this browser and shared across both editors.
const KEY = "hivedock-editor-wrap";

export function useEditorWrap(): [boolean, (v: boolean) => void] {
  const [wrap, setWrap] = useState(() => {
    try {
      return localStorage.getItem(KEY) === "1";
    } catch {
      return false;
    }
  });
  const update = (v: boolean) => {
    setWrap(v);
    try {
      localStorage.setItem(KEY, v ? "1" : "0");
    } catch {
      /* ignore */
    }
  };
  return [wrap, update];
}
