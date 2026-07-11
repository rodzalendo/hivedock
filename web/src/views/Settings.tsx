import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { fetchSettings, saveSettings } from "../api";

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
      await saveSettings(webhook.trim());
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
        <h3 className="mb-3 text-sm font-medium text-zinc-200">Notifications</h3>
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
        <p className="mt-1.5 text-[11px] text-zinc-600">
          POSTed a JSON payload when new image updates are found.
          {data.webhookFromEnv && " Currently set via the WEBHOOK_URL env var."}
        </p>
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

      <section className="rounded-xl border border-zinc-800 bg-zinc-900/40 p-5">
        <h3 className="mb-3 text-sm font-medium text-zinc-200">Environment</h3>
        <p className="mb-3 text-[11px] text-zinc-600">
          Configured via environment variables (change requires a container
          restart).
        </p>
        <dl className="grid grid-cols-1 gap-x-6 gap-y-2 text-sm sm:grid-cols-[10rem_1fr]">
          <Row label="Stacks directory" value={data.stacksDir} mono />
          <Row label="Data directory" value={data.dataDir} mono />
          <Row
            label="Update check"
            value={
              data.checkInterval === "disabled"
                ? "disabled"
                : `every ${data.checkInterval}`
            }
          />
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
