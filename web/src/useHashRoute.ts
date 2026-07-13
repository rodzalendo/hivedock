import { useEffect, useState } from "react";

// Minimal hash router: the URL hash is the source of truth for navigation
// ("#/stacks/immich" -> ["stacks", "immich"]), so refresh and browser
// back/forward keep the current page. No dependency, no server config.

// parseHash splits the current hash into decoded path segments.
export function parseHash(): string[] {
  return window.location.hash
    .replace(/^#\/?/, "")
    .split("/")
    .filter(Boolean)
    .map(decodeURIComponent);
}

// useHashRoute returns the current hash segments and re-renders on change.
export function useHashRoute(): string[] {
  const [parts, setParts] = useState<string[]>(parseHash);
  useEffect(() => {
    const onChange = () =>
      setParts((prev) => {
        const next = parseHash();
        // Dedupe: hash assignment + manual dispatch can both fire.
        return prev.length === next.length && prev.every((p, i) => p === next[i])
          ? prev
          : next;
      });
    window.addEventListener("hashchange", onChange);
    return () => window.removeEventListener("hashchange", onChange);
  }, []);
  return parts;
}

// navigate sets the hash (and thus the route). Segments are URI-encoded.
export function navigate(...segments: string[]) {
  const next = "/" + segments.map(encodeURIComponent).join("/");
  if (window.location.hash === "#" + next) return;
  window.location.hash = next;
  // jsdom (tests) doesn't reliably fire hashchange on assignment; real
  // browsers dedupe the duplicate in useHashRoute.
  window.dispatchEvent(new HashChangeEvent("hashchange"));
}
