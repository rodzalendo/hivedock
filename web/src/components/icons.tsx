// Inline icons for the app chrome — geometric shapes matching the Hivedock
// design mock (no emoji). All inherit `currentColor` and a 1em box.

export function LogoMark({ size = 20 }: { size?: number }) {
  // A 45°-rotated amber rounded square with a dark center — the "hive cell".
  const inner = Math.round(size * 0.35);
  return (
    <span
      aria-hidden
      style={{ width: size, height: size }}
      className="flex shrink-0 rotate-45 items-center justify-center rounded-[5px] bg-hive-500"
    >
      <span
        style={{ width: inner, height: inner }}
        className="rounded-[2px] bg-zinc-950"
      />
    </span>
  );
}

type IconProps = { className?: string };

export function HomeIcon({ className }: IconProps) {
  return (
    <svg viewBox="0 0 16 16" className={className} fill="currentColor" aria-hidden>
      <rect x="2" y="2" width="5" height="5" rx="1.5" />
      <rect x="9" y="2" width="5" height="5" rx="1.5" />
      <rect x="2" y="9" width="5" height="5" rx="1.5" />
      <rect x="9" y="9" width="5" height="5" rx="1.5" />
    </svg>
  );
}

export function StacksIcon({ className }: IconProps) {
  return (
    <svg viewBox="0 0 16 16" className={className} fill="currentColor" aria-hidden>
      <rect x="2" y="3" width="12" height="2.6" rx="1.1" />
      <rect x="2" y="6.7" width="12" height="2.6" rx="1.1" />
      <rect x="2" y="10.4" width="12" height="2.6" rx="1.1" />
    </svg>
  );
}

export function UpdatesIcon({ className }: IconProps) {
  return (
    <svg
      viewBox="0 0 16 16"
      className={className}
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      <path d="M8 13V3.5" />
      <path d="M4 7l4-4 4 4" />
    </svg>
  );
}

export function StatusIcon({ className }: IconProps) {
  return (
    <svg
      viewBox="0 0 16 16"
      className={className}
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      <path d="M1.5 8h3l1.5-4 3 8 1.5-4h3" />
    </svg>
  );
}

export function SettingsIcon({ className }: IconProps) {
  return (
    <svg
      viewBox="0 0 16 16"
      className={className}
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
      strokeLinecap="round"
      aria-hidden
    >
      <line x1="2.5" y1="4.5" x2="13.5" y2="4.5" />
      <line x1="2.5" y1="8" x2="13.5" y2="8" />
      <line x1="2.5" y1="11.5" x2="13.5" y2="11.5" />
      <circle cx="6" cy="4.5" r="1.6" fill="currentColor" stroke="none" />
      <circle cx="10.5" cy="8" r="1.6" fill="currentColor" stroke="none" />
      <circle cx="5" cy="11.5" r="1.6" fill="currentColor" stroke="none" />
    </svg>
  );
}

export function EyeIcon({ className }: IconProps) {
  return (
    <svg
      viewBox="0 0 16 16"
      className={className}
      fill="none"
      stroke="currentColor"
      strokeWidth="1.4"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      <path d="M1.5 8s2.4-4.5 6.5-4.5S14.5 8 14.5 8 12.1 12.5 8 12.5 1.5 8 1.5 8Z" />
      <circle cx="8" cy="8" r="1.8" />
    </svg>
  );
}

export function PlayIcon({ className }: IconProps) {
  return (
    <svg viewBox="0 0 16 16" className={className} fill="currentColor" aria-hidden>
      <path d="M4.5 3.2c0-.9 1-1.5 1.8-1L13 6.7c.8.5.8 1.7 0 2.2l-6.7 4.4c-.8.5-1.8-.1-1.8-1V3.2Z" />
    </svg>
  );
}

export function DownloadIcon({ className }: IconProps) {
  return (
    <svg
      viewBox="0 0 16 16"
      className={className}
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      <path d="M8 2.5v7" />
      <path d="M5 7l3 3 3-3" />
      <path d="M2.5 12.5h11" />
    </svg>
  );
}

export function RestartIcon({ className }: IconProps) {
  return (
    <svg
      viewBox="0 0 16 16"
      className={className}
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      <path d="M13.5 8a5.5 5.5 0 1 1-1.6-3.9" />
      <path d="M13.5 2.5v2.6h-2.6" />
    </svg>
  );
}

export function StopIcon({ className }: IconProps) {
  return (
    <svg viewBox="0 0 16 16" className={className} fill="currentColor" aria-hidden>
      <rect x="3.5" y="3.5" width="9" height="9" rx="1.5" />
    </svg>
  );
}

export function ChevronsDownIcon({ className }: IconProps) {
  return (
    <svg
      viewBox="0 0 16 16"
      className={className}
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      <path d="M4 3.5L8 7.5l4-4" />
      <path d="M4 8.5l4 4 4-4" />
    </svg>
  );
}

export function SpinnerIcon({ className }: IconProps) {
  return (
    <svg
      viewBox="0 0 16 16"
      className={`animate-spin ${className ?? ""}`}
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      aria-hidden
    >
      <path d="M14 8a6 6 0 1 1-6-6" />
    </svg>
  );
}

export function PencilIcon({ className }: IconProps) {
  return (
    <svg
      viewBox="0 0 16 16"
      className={className}
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      <path d="M11.1 2.4a1.6 1.6 0 0 1 2.3 2.3l-7.8 7.8-3.1.8.8-3.1 7.8-7.8Z" />
    </svg>
  );
}

export function TrashIcon({ className }: IconProps) {
  return (
    <svg
      viewBox="0 0 16 16"
      className={className}
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      <path d="M2.5 4h11" />
      <path d="M5.5 4V2.8c0-.4.3-.8.8-.8h3.4c.5 0 .8.4.8.8V4" />
      <path d="M12.5 4l-.6 8.7c0 .7-.6 1.3-1.3 1.3H5.4c-.7 0-1.3-.6-1.3-1.3L3.5 4" />
      <path d="M6.5 7v4M9.5 7v4" />
    </svg>
  );
}

export function ImageIcon({ className }: IconProps) {
  return (
    <svg
      viewBox="0 0 16 16"
      className={className}
      fill="none"
      stroke="currentColor"
      strokeWidth="1.4"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      <rect x="2" y="2.5" width="12" height="11" rx="2" />
      <circle cx="5.7" cy="6" r="1.1" fill="currentColor" stroke="none" />
      <path d="M3 11.5l3.2-3 2.3 2 2-1.7 2.5 2.7" />
    </svg>
  );
}

export function EyeOffIcon({ className }: IconProps) {
  return (
    <svg
      viewBox="0 0 16 16"
      className={className}
      fill="none"
      stroke="currentColor"
      strokeWidth="1.4"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      <path d="M6.2 3.7A6.4 6.4 0 0 1 8 3.5c4.1 0 6.5 4.5 6.5 4.5a11 11 0 0 1-1.9 2.4M4.2 4.9A11 11 0 0 0 1.5 8S3.9 12.5 8 12.5c.9 0 1.7-.2 2.4-.5" />
      <path d="M2 2l12 12" />
    </svg>
  );
}
