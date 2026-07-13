import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { fetchSettings, saveSettings, pruneSystem } from "../api";
import { SpinnerIcon } from "../components/icons";

export default function Settings() {
  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ["settings"],
    queryFn: fetchSettings,
  });

  const [webhook, setWebhook] = useState("");
  const [busy, setBusy] = useState(false);
  const [note, setNote] = useState<string | null>(null);

  useEffect(() => {
    if (data) setWebhook(data.webhookUrl);
  }, [data]);

  async function onSave() {
    setBusy(true);
    setNote(null);
    try {
      await saveSettings({ webhookUrl: webhook.trim() });
      await refetch();
      setNote("Saved.");
    } catch (err) {
      setNote(err instanceof Error ? err.message : "Failed to save.");
    } finally {
      setBusy(false);
    }
  }

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

      <section className="rounded-xl border border-zinc-800 bg-zinc-900/40 p-5">
        <h3 className="mb-1 text-sm font-medium text-zinc-200">Notifications</h3>
        <p className="mb-3 text-[11px] leading-relaxed text-zinc-500">
          When an update check finds a <em>new</em> image update, HiveDock sends
          an HTTP <span className="font-mono text-zinc-400">POST</span> with a
          JSON body to the URL below — so you can get notified without watching
          this page. Point it at any service that accepts an incoming webhook
          (Discord, Slack, ntfy, Gotify, Home Assistant, n8n, …). Leave it blank
          to disable. It never receives your stacks or credentials — only which
          images have updates.
        </p>
        <label className="block">
          <span className="mb-1 block text-xs font-medium text-zinc-400">
            Webhook URL
          </span>
          <input
            type="url"
            value={webhook}
            onChange={(e) => setWebhook(e.target.value)}
            placeholder="https://example.com/hook"
            className="w-full rounded-lg border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm outline-none focus:border-accent-500"
          />
        </label>
        <details className="mt-2 text-[11px] text-zinc-600">
          <summary className="cursor-pointer text-zinc-500 hover:text-zinc-400">
            Example payload
          </summary>
          <pre className="mt-1.5 overflow-x-auto rounded-md border border-zinc-800 bg-zinc-950 p-2.5 font-mono text-[10.5px] leading-relaxed text-zinc-400">
{`{
  "event": "updates_available",
  "time": "2026-07-11T18:04:00Z",
  "count": 1,
  "updates": [
    { "image": "lscr.io/linuxserver/mariadb:11.4.5",
      "kind": "semver", "current": "11.4.5",
      "candidate": "11.4.12", "diff": "patch" }
  ]
}`}
          </pre>
        </details>
        {data.webhookFromEnv && (
          <p className="mt-1.5 text-[11px] text-amber-500/70">
            Currently set via the WEBHOOK_URL env var (this overrides the field
            above).
          </p>
        )}
        <div className="mt-3 flex items-center gap-3">
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

      <IntervalSection current={data.checkInterval} onSaved={refetch} />

      <PruneSection />

      <section className="rounded-xl border border-zinc-800 bg-zinc-900/40 p-5">
        <h3 className="mb-3 text-sm font-medium text-zinc-200">Environment</h3>
        <p className="mb-3 text-[11px] text-zinc-600">
          Configured via environment variables (change requires a container
          restart).
        </p>
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
      <h3 className="mb-1 text-sm font-medium text-zinc-200">
        Automatic update check
      </h3>
      <p className="mb-3 text-[11px] leading-relaxed text-zinc-500">
        How often HiveDock checks registries for newer images in the
        background.
      </p>
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
      <h3 className="mb-1 text-sm font-medium text-zinc-200">Maintenance</h3>
      <p className="mb-3 text-[11px] leading-relaxed text-zinc-500">
        Image updates leave the old, now-untagged image layers behind on disk.
        Prune removes those dangling images and stale build cache. It never
        touches tagged images, containers, volumes, or networks, so it is safe
        to run any time.
      </p>
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
