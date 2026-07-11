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

// AppIcon resolves an entry's icon: explicit URL label → Hivedock icon proxy
// (dashboard-icons, cached) → deterministic letter avatar on any failure. The
// browser never depends on an external host directly.
export default function AppIcon({ entry, size = 40 }: { entry: HomeEntry; size?: number }) {
  const [failed, setFailed] = useState(false);

  const src = iconSrc(entry);
  const showLetter = failed || !src;

  if (showLetter) {
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
      src={src}
      alt=""
      width={size}
      height={size}
      className="shrink-0 rounded-[7px] object-contain"
      style={{ width: size, height: size }}
      onError={() => setFailed(true)}
    />
  );
}

function iconSrc(entry: HomeEntry): string | null {
  if (entry.icon) {
    // Explicit icon label: full URL used directly; otherwise treat as a slug.
    if (/^https?:\/\//.test(entry.icon)) return entry.icon;
    const slug = entry.icon.replace(/\.(png|svg|webp)$/i, "");
    return `/api/icons/${encodeURIComponent(slug)}`;
  }
  if (entry.iconSlug) return `/api/icons/${encodeURIComponent(entry.iconSlug)}`;
  return null;
}
