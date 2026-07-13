import { useEffect, useMemo, useRef, useState } from "react";
import { useLogs } from "../useLogs";

// A palette so each service's lines get a stable color prefix.
const serviceColors = [
  "text-sky-400",
  "text-emerald-400",
  "text-violet-400",
  "text-amber-400",
  "text-pink-400",
  "text-teal-400",
];

// lineClass colors a log line by its severity so errors/warnings stand out.
// stderr is treated as an error tint unless the text says otherwise.
function lineClass(line: string, stream: string): string {
  const l = line.toLowerCase();
  if (/\b(error|errors|fatal|panic|exception|failed)\b/.test(l)) return "text-red-400";
  if (/\b(warn|warning)\b/.test(l)) return "text-amber-400";
  if (/\b(debug|trace)\b/.test(l)) return "text-zinc-500";
  if (stream === "stderr") return "text-red-300";
  return "text-zinc-300";
}

// copyText copies to the clipboard, with a fallback for insecure origins
// (plain-http LAN access, where navigator.clipboard is unavailable).
async function copyText(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch {
    /* fall through to the legacy path */
  }
  try {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.style.position = "fixed";
    ta.style.opacity = "0";
    document.body.appendChild(ta);
    ta.focus();
    ta.select();
    const ok = document.execCommand("copy");
    document.body.removeChild(ta);
    return ok;
  } catch {
    return false;
  }
}

export default function LogsPanel({
  stack,
  services,
}: {
  stack: string;
  services: string[];
}) {
  const [follow, setFollow] = useState(true);
  const [serviceFilter, setServiceFilter] = useState<string>("all");
  const [expanded, setExpanded] = useState(false);
  const [copied, setCopied] = useState<string | null>(null);
  const { lines, error, connected } = useLogs(stack, follow);

  const colorFor = useMemo(() => {
    const map = new Map<string, string>();
    services.forEach((s, i) => map.set(s, serviceColors[i % serviceColors.length]));
    return (svc: string) => map.get(svc) ?? "text-zinc-400";
  }, [services]);

  const shown = useMemo(
    () => (serviceFilter === "all" ? lines : lines.filter((l) => l.service === serviceFilter)),
    [lines, serviceFilter],
  );

  const scrollRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (follow && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [shown, follow, expanded]);

  // Escape leaves the enlarged view.
  useEffect(() => {
    if (!expanded) return;
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setExpanded(false);
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [expanded]);

  async function copy(count?: number) {
    const slice = count ? shown.slice(-count) : shown;
    const text = slice
      .map((l) => (services.length > 1 ? `${l.service}  ${l.line}` : l.line))
      .join("\n");
    const ok = await copyText(text);
    setCopied(ok ? `Copied ${slice.length} line${slice.length === 1 ? "" : "s"}` : "Copy failed");
    window.setTimeout(() => setCopied(null), 1800);
  }

  const controls = (
    <div className="flex flex-wrap items-center gap-3 px-5 py-3 text-xs">
      <label className="flex items-center gap-1.5 text-zinc-400">
        <input
          type="checkbox"
          checked={follow}
          onChange={(e) => setFollow(e.target.checked)}
          className="accent-accent-500"
        />
        Follow
      </label>

      {services.length > 1 && (
        <select
          value={serviceFilter}
          onChange={(e) => setServiceFilter(e.target.value)}
          className="rounded border border-zinc-700 bg-zinc-900 px-2 py-1 text-zinc-300"
        >
          <option value="all">All services</option>
          {services.map((s) => (
            <option key={s} value={s}>
              {s}
            </option>
          ))}
        </select>
      )}

      <CopyMenu onCopy={copy} disabled={shown.length === 0} />
      {copied && <span className="text-zinc-500">{copied}</span>}

      <button
        onClick={() => setExpanded((v) => !v)}
        className="rounded border border-zinc-700 px-2 py-1 text-zinc-300 transition hover:bg-zinc-800"
        title={expanded ? "Shrink logs" : "Enlarge logs for reading"}
      >
        {expanded ? "Shrink" : "Enlarge"}
      </button>

      <span className="ml-auto flex items-center gap-1.5 text-zinc-500">
        <span
          className={`inline-block h-2 w-2 rounded-full ${
            connected ? "bg-green-500" : "bg-zinc-600"
          }`}
        />
        {connected ? "streaming" : follow ? "connecting…" : "paused"}
      </span>
    </div>
  );

  const logArea = (
    <div
      ref={scrollRef}
      className={`mx-3 mb-3 overflow-auto rounded-lg bg-zinc-950 p-3 font-mono text-xs leading-relaxed ${
        expanded ? "flex-1" : "h-96"
      }`}
    >
      {shown.length === 0 ? (
        <p className="text-zinc-600">
          {follow ? "Waiting for output…" : "Follow is paused."}
        </p>
      ) : (
        shown.map((l) => (
          <div key={l.id} className="whitespace-pre-wrap break-all">
            {services.length > 1 && (
              <span className={`mr-2 ${colorFor(l.service)}`}>{l.service}</span>
            )}
            <span className={lineClass(l.line, l.stream)}>{l.line}</span>
          </div>
        ))
      )}
    </div>
  );

  const errorBanner = error && (
    <div className="mx-5 mb-2 rounded border border-red-900/50 bg-red-950/30 px-3 py-2 text-xs text-red-400">
      {error}
    </div>
  );

  if (expanded) {
    return (
      <>
        <div
          className="fixed inset-0 z-40 bg-black/60"
          onClick={() => setExpanded(false)}
        />
        <div className="fixed inset-3 z-50 flex flex-col rounded-xl border border-zinc-700 bg-zinc-900 shadow-2xl md:inset-6">
          <div className="flex items-center justify-between border-b border-zinc-800 pr-3">
            <div className="min-w-0 flex-1">{controls}</div>
          </div>
          {errorBanner}
          {logArea}
        </div>
      </>
    );
  }

  return (
    <div className="flex flex-col">
      {controls}
      {errorBanner}
      {logArea}
    </div>
  );
}

// CopyMenu is a split control: click copies all shown lines; the caret opens
// "last N" shortcuts for grabbing just the tail.
function CopyMenu({
  onCopy,
  disabled,
}: {
  onCopy: (count?: number) => void;
  disabled: boolean;
}) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onDown);
    return () => document.removeEventListener("mousedown", onDown);
  }, [open]);

  return (
    <div ref={ref} className="relative flex items-center">
      <button
        onClick={() => onCopy()}
        disabled={disabled}
        className="rounded-l border border-zinc-700 px-2 py-1 text-zinc-300 transition hover:bg-zinc-800 disabled:opacity-40"
        title="Copy all shown lines"
      >
        Copy
      </button>
      <button
        onClick={() => setOpen((v) => !v)}
        disabled={disabled}
        aria-label="Copy options"
        className="rounded-r border border-l-0 border-zinc-700 px-1.5 py-1 text-zinc-400 transition hover:bg-zinc-800 disabled:opacity-40"
      >
        ▾
      </button>
      {open && (
        <div className="absolute left-0 top-8 z-10 w-32 rounded-lg border border-zinc-700 bg-zinc-900 py-1 shadow-xl">
          {[100, 500, 1000].map((n) => (
            <button
              key={n}
              onClick={() => {
                onCopy(n);
                setOpen(false);
              }}
              className="block w-full px-3 py-1.5 text-left text-xs text-zinc-300 hover:bg-zinc-800"
            >
              Last {n}
            </button>
          ))}
          <button
            onClick={() => {
              onCopy();
              setOpen(false);
            }}
            className="block w-full px-3 py-1.5 text-left text-xs text-zinc-300 hover:bg-zinc-800"
          >
            All shown
          </button>
        </div>
      )}
    </div>
  );
}
