import { useState } from "react";
import type { HomeEntry } from "../api";

// Muted per-service tile colors from the Hivedock design mock.
const avatarColors = [
  "#a08ad6",
  "#7fa9dd",
  "#d9a95c",
  "#d87f7a",
  "#6fc3c9",
  "#8fbf7a",
];

function hashColor(s: string): string {
  let h = 0;
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) | 0;
  return avatarColors[Math.abs(h) % avatarColors.length];
}

// AppIcon resolves an entry's icon by trying, in order: an explicit icon (user
// override or label), the image slug, then the stack-name slug — each via the
// Hivedock icon proxy (dashboard-icons, cached). It advances to the next source
// when one fails to load, and falls back to a deterministic letter avatar when
// all are exhausted. The browser never depends on an external host directly.
export default function AppIcon({ entry, size = 40 }: { entry: HomeEntry; size?: number }) {
  const sources = iconSources(entry);
  const [idx, setIdx] = useState(0);

  // entry identity can change under the same DOM node; reset to the first source.
  const key = sources.join("|");
  const [seenKey, setSeenKey] = useState(key);
  if (key !== seenKey) {
    setSeenKey(key);
    setIdx(0);
  }

  const src = sources[idx];

  if (!src) {
    const c = hashColor(entry.name);
    return (
      <div
        className="flex shrink-0 items-center justify-center rounded-[7px] font-mono font-semibold"
        style={{
          width: size,
          height: size,
          fontSize: size * 0.42,
          background: `${c}24`,
          color: c,
        }}
        aria-hidden
      >
        {entry.name.trim().charAt(0).toUpperCase() || "?"}
      </div>
    );
  }

  return (
    <img
      key={src} // remount per source: a stale <img> error/load state must not leak into the next src
      src={src}
      alt=""
      width={size}
      height={size}
      className="shrink-0 rounded-[7px] object-contain"
      style={{ width: size, height: size }}
      onError={() => setIdx((i) => i + 1)}
    />
  );
}

// iconSources returns the ordered list of icon URLs to try for an entry.
// Empty strings are never emitted (an <img src=""> renders a broken tile).
function iconSources(entry: HomeEntry): string[] {
  const out: string[] = [];
  if (entry.icon && entry.icon.trim() !== "") {
    // Explicit icon: full URL used directly; otherwise treat as a slug.
    if (/^https?:\/\//.test(entry.icon)) out.push(entry.icon);
    else out.push(`/api/icons/${encodeURIComponent(stripExt(entry.icon))}`);
  }
  // Hivedock has no dashboard-icons entry — serve its own bundled logo.
  if (!entry.icon && (entry.iconSlug === "hivedock" || entry.stackSlug === "hivedock")) {
    out.push("/favicon.svg");
  }
  if (entry.iconSlug) out.push(`/api/icons/${encodeURIComponent(entry.iconSlug)}`);
  if (entry.stackSlug && entry.stackSlug !== entry.iconSlug) {
    out.push(`/api/icons/${encodeURIComponent(entry.stackSlug)}`);
  }
  return out;
}

function stripExt(s: string): string {
  return s.replace(/\.(png|svg|webp)$/i, "");
}
