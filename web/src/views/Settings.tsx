import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { fetchSettings, saveSettings, pruneSystem } from "../api";
import { SpinnerIcon } from "../components/icons";
import { HelpTip } from "../components/ui";

export default function Settings() {
  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ["settings"],
    queryFn: fetchSettings,
  });

  if (isLoading) return <p className="text-sm text-zinc-500">Loading…</p>;
  if (isError)
    return (
      <p className="text-sm text-red-400">
        Failed to load settings — {(error as Error).message}
      </p>
    );
  if (!data) return null;

  return (
    <div className="max-w-2xl space-y-6">
      <h2 className="text-sm font-medium uppercase tracking-wide text-zinc-400">
        Settings
      </h2>

      <IntervalSection current={data.checkInterval} onSaved={refetch} />

      <PruneSection />

      <section className="rounded-xl border border-zinc-800 bg-zinc-900/40 p-5">
        <h3 className="mb-3 flex items-center gap-1.5 text-sm font-medium text-zinc-200">
          Environment
          <HelpTip>
            Configured via environment variables (change requires a container
            restart).
          </HelpTip>
        </h3>
        <dl className="grid grid-cols-1 gap-x-6 gap-y-2 text-sm sm:grid-cols-[10rem_1fr]">
          <Row label="Stacks directory" value={data.stacksDir} mono />
          <Row label="Data directory" value={data.dataDir} mono />
          <Row label="Public host" value={data.publicHost || "(request host)"} />
          <Row
            label="Authentication"
            value={data.authDisabled ? "disabled (AUTH_DISABLED)" : "enabled"}
          />
          <Row label="Version" value={data.version} mono />
        </dl>
      </section>
    </div>
  );
}

// IntervalSection controls how often the background update check runs.
// Applies live (no restart) — the scheduler re-reads it every minute.
function IntervalSection({
  current,
  onSaved,
}: {
  current: string;
  onSaved: () => void;
}) {
  const options = [
    { value: "off", label: "Off" },
    { value: "15m", label: "Every 15 minutes" },
    { value: "30m", label: "Every 30 minutes" },
    { value: "1h", label: "Every hour" },
    { value: "3h", label: "Every 3 hours" },
    { value: "6h", label: "Every 6 hours" },
    { value: "12h", label: "Every 12 hours" },
    { value: "24h", label: "Every 24 hours" },
  ];
  // The server reports tidy duration strings ("30m", "6h") or "disabled".
  const normalized = current === "disabled" ? "off" : current;
  const [value, setValue] = useState(
    options.some((o) => o.value === normalized) ? normalized : "30m",
  );
  const [busy, setBusy] = useState(false);
  const [note, setNote] = useState<string | null>(null);

  async function onSave() {
    setBusy(true);
    setNote(null);
    try {
      await saveSettings({ checkInterval: value });
      onSaved();
      setNote("Saved — applies within a minute.");
    } catch (err) {
      setNote(err instanceof Error ? err.message : "Failed to save.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="rounded-xl border border-zinc-800 bg-zinc-900/40 p-5">
      <h3 className="mb-3 flex items-center gap-1.5 text-sm font-medium text-zinc-200">
        Automatic update check
        <HelpTip>
          How often HiveDock checks registries for newer images in the
          background. Changes apply within a minute, no restart needed.
        </HelpTip>
      </h3>
      <div className="flex items-center gap-3">
        <select
          value={value}
          onChange={(e) => setValue(e.target.value)}
          className="rounded-lg border border-zinc-700 bg-zinc-950 px-3 py-1.5 text-sm text-zinc-200 outline-none focus:border-accent-500"
        >
          {options.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
        <button
          onClick={onSave}
          disabled={busy}
          className="rounded-lg bg-accent-600 px-3 py-1.5 text-sm font-medium text-zinc-950 transition hover:bg-accent-500 disabled:opacity-50"
        >
          Save
        </button>
        {note && <span className="text-xs text-zinc-500">{note}</span>}
      </div>
    </section>
  );
}

// PruneSection frees disk space: dangling images (the untagged layers that
// pile up after updates) and stale build cache. Tagged images, containers,
// volumes, and networks are never touched.
function PruneSection() {
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<string | null>(null);

  async function onPrune() {
    setBusy(true);
    setResult(null);
    try {
      const rep = await pruneSystem();
      const mb = rep.spaceReclaimed / (1024 * 1024);
      setResult(
        rep.imagesDeleted === 0 && rep.spaceReclaimed === 0
          ? "Nothing to prune — already clean."
          : `Removed ${rep.imagesDeleted} dangling image${rep.imagesDeleted === 1 ? "" : "s"}, reclaimed ${
              mb >= 1024 ? `${(mb / 1024).toFixed(1)} GB` : `${mb.toFixed(0)} MB`
            }.`,
      );
    } catch (err) {
      setResult(err instanceof Error ? err.message : "Prune failed.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="rounded-xl border border-zinc-800 bg-zinc-900/40 p-5">
      <h3 className="mb-3 flex items-center gap-1.5 text-sm font-medium text-zinc-200">
        Maintenance
        <HelpTip>
          Image updates leave the old, now-untagged image layers behind on
          disk. Prune removes those dangling images and stale build cache. It
          never touches tagged images, containers, volumes, or networks, so it
          is safe to run any time.
        </HelpTip>
      </h3>
      <div className="flex items-center gap-3">
        <button
          onClick={onPrune}
          disabled={busy}
          className="flex items-center gap-1.5 rounded-lg border border-zinc-700 px-3 py-1.5 text-sm font-medium text-zinc-200 transition hover:bg-zinc-800 disabled:opacity-50"
        >
          {busy && <SpinnerIcon className="h-3.5 w-3.5" />}
          {busy ? "Pruning…" : "Prune dangling images"}
        </button>
        {result && <span className="text-xs text-zinc-500">{result}</span>}
      </div>
    </section>
  );
}

function Row({
  label,
  value,
  mono,
}: {
  label: string;
  value: string;
  mono?: boolean;
}) {
  return (
    <>
      <dt className="text-zinc-500">{label}</dt>
      <dd className={`text-zinc-300 ${mono ? "font-mono text-xs" : ""}`}>
        {value}
      </dd>
    </>
  );
}
