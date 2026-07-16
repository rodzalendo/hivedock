import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  fetchSettings,
  saveSettings,
  pruneSystem,
  initGitRepo,
  generateApiToken,
  revokeApiToken,
  fetchRegistries,
  saveRegistry,
  deleteRegistry,
  type Settings as SettingsData,
  type RegistryConfig,
  type UpdateMode,
} from "../api";
import { SpinnerIcon } from "../components/icons";
import { HelpTip } from "../components/ui";
import { THEMES, getStoredTheme, setTheme, type ThemeId } from "../theme";
import { useI18n, LANGUAGES, type Lang } from "../i18n";

export default function Settings() {
  const { t } = useI18n();
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
    <div className="mx-auto w-full max-w-6xl space-y-5">
      <h2 className="text-sm font-medium uppercase tracking-wide text-zinc-400">
        {t("settings.title")}
      </h2>

      {/* Two columns on wide screens: look & update behavior on the left,
          registries / security / maintenance / info on the right. Each column
          groups sections by function and the whole page centers and fills the
          available width instead of hugging the left edge. */}
      <div className="grid grid-cols-1 items-start gap-6 lg:grid-cols-2">
        <div className="space-y-6">
          <AppearanceSection />
          <LanguageSection />
          <IntervalSection current={data.checkInterval} onSaved={refetch} />
          <UpdateModeSection current={data.updateMode} onSaved={refetch} />
        </div>

        <div className="space-y-6">
          <RegistriesSection />
          <ApiTokenSection tokenSet={data.apiTokenSet} onChanged={refetch} />
          <GitSection data={data} onSaved={refetch} />
          <PruneSection />

          <section className="rounded-xl border border-zinc-800 bg-zinc-900/40 p-5">
            <h3 className="mb-3 flex items-center gap-1.5 text-sm font-medium text-zinc-200">
              {t("settings.environment")}
              <HelpTip>
                Configured via environment variables (change requires a
                container restart).
              </HelpTip>
            </h3>
            <dl className="grid grid-cols-1 gap-x-6 gap-y-2 text-sm sm:grid-cols-[10rem_1fr]">
              <Row label={t("settings.env.stacksDir")} value={data.stacksDir} mono />
              <Row label={t("settings.env.dataDir")} value={data.dataDir} mono />
              <Row
                label={t("settings.env.publicHost")}
                value={data.publicHost || "(request host)"}
              />
              <Row label={t("settings.env.auth")} value={data.authMode} />
              <Row label={t("settings.env.version")} value={data.version} mono />
            </dl>
          </section>
        </div>
      </div>
    </div>
  );
}

// AppearanceSection switches the app theme. The choice is stored in
// localStorage and applied instantly by flipping the `data-theme` attribute on
// <html> (see theme.ts) — no server round-trip, it's a per-browser preference.
function AppearanceSection() {
  const { t } = useI18n();
  const [current, setCurrent] = useState<ThemeId>(() => getStoredTheme());

  function pick(id: ThemeId) {
    setTheme(id);
    setCurrent(id);
  }

  return (
    <section className="rounded-xl border border-zinc-800 bg-zinc-900/40 p-5">
      <h3 className="mb-3 flex items-center gap-1.5 text-sm font-medium text-zinc-200">
        {t("settings.appearance")}
        <HelpTip>{t("settings.appearanceHelp")}</HelpTip>
      </h3>
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-3">
        {THEMES.map((theme) => {
          const active = theme.id === current;
          return (
            <button
              key={theme.id}
              onClick={() => pick(theme.id)}
              aria-pressed={active}
              className={`group flex flex-col gap-2 rounded-lg border p-3 text-left transition ${
                active
                  ? "border-accent-500 bg-accent-500/10"
                  : "border-zinc-700 hover:border-zinc-600 hover:bg-zinc-800/40"
              }`}
              title={theme.blurb}
            >
              <span className="flex items-center gap-1.5">
                <span
                  className="h-5 w-5 shrink-0 rounded border border-black/20"
                  style={{ background: theme.swatch[0] }}
                />
                <span
                  className="h-5 w-5 shrink-0 rounded border border-black/20"
                  style={{ background: theme.swatch[1] }}
                />
                {active && (
                  <span className="ml-auto text-[10px] font-medium uppercase tracking-wide text-accent-500">
                    Active
                  </span>
                )}
              </span>
              <span className="text-[13px] font-medium text-zinc-200">
                {theme.name}
              </span>
              <span className="text-[11px] leading-relaxed text-zinc-500">
                {theme.blurb}
              </span>
            </button>
          );
        })}
      </div>
    </section>
  );
}

// LanguageSection switches the UI language. Like the theme, it's a per-browser
// preference applied instantly (see i18n.tsx) — no server round-trip.
function LanguageSection() {
  const { t, lang, setLang } = useI18n();
  return (
    <section className="rounded-xl border border-zinc-800 bg-zinc-900/40 p-5">
      <h3 className="mb-3 flex items-center gap-1.5 text-sm font-medium text-zinc-200">
        {t("settings.language")}
        <HelpTip>{t("settings.languageHelp")}</HelpTip>
      </h3>
      <select
        value={lang}
        onChange={(e) => setLang(e.target.value as Lang)}
        className="rounded-lg border border-zinc-700 bg-zinc-950 px-3 py-1.5 text-sm text-zinc-200 outline-none focus:border-accent-500"
      >
        {LANGUAGES.map((l) => (
          <option key={l.id} value={l.id}>
            {l.label}
          </option>
        ))}
      </select>
    </section>
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
  const { t } = useI18n();
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
        {t("settings.autoUpdate")}
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

// UpdateModeSection controls how HiveDock updates itself. Releases are cosign-
// signed; HiveDock verifies the signature before offering or applying an update.
// Applies immediately (the next update check reads it).
function UpdateModeSection({
  current,
  onSaved,
}: {
  current: UpdateMode;
  onSaved: () => void;
}) {
  const { t } = useI18n();
  const options: { value: UpdateMode; label: string; desc: string }[] = [
    {
      value: "full",
      label: "Full",
      desc: "Check for new releases, verify their signatures, and allow one-click updates from the sidebar.",
    },
    {
      value: "check-only",
      label: "Check only",
      desc: "Check and verify, but never apply automatically — update manually from a shell.",
    },
    {
      value: "off",
      label: "Off",
      desc: "No version check at all (for air-gapped installs).",
    },
  ];
  const [value, setValue] = useState<UpdateMode>(current);
  const [busy, setBusy] = useState(false);
  const [note, setNote] = useState<string | null>(null);

  async function onSave() {
    setBusy(true);
    setNote(null);
    try {
      await saveSettings({ updateMode: value });
      onSaved();
      setNote("Saved.");
    } catch (err) {
      setNote(err instanceof Error ? err.message : "Failed to save.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="rounded-xl border border-zinc-800 bg-zinc-900/40 p-5">
      <h3 className="mb-3 flex items-center gap-1.5 text-sm font-medium text-zinc-200">
        {t("settings.selfUpdate")}
        <HelpTip>
          Release images are cosign-signed via GitHub Actions. HiveDock verifies
          that signature and pins the exact digest before offering or applying
          an update to itself.
        </HelpTip>
      </h3>
      <div className="space-y-2">
        {options.map((o) => (
          <label
            key={o.value}
            className="flex cursor-pointer items-start gap-2.5"
          >
            <input
              type="radio"
              name="updateMode"
              value={o.value}
              checked={value === o.value}
              onChange={() => setValue(o.value)}
              className="mt-0.5 accent-accent-600"
            />
            <span>
              <span className="text-sm text-zinc-200">{o.label}</span>
              <span className="block text-[11px] leading-relaxed text-zinc-500">
                {o.desc}
              </span>
            </span>
          </label>
        ))}
      </div>
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
  );
}

// GitSection controls the opt-in local audit trail (§5.4): when on, HiveDock
// commits every change under the stacks directory (its own writes and out-of-
// band ones) to a local git repo — no remotes, no push. Requires STACKS_DIR to
// be a git worktree; offers a one-click init when it isn't.
function GitSection({
  data,
  onSaved,
}: {
  data: SettingsData;
  onSaved: () => void;
}) {
  const { t } = useI18n();
  const [busy, setBusy] = useState(false);
  const [note, setNote] = useState<string | null>(null);

  async function onInit() {
    setBusy(true);
    setNote(null);
    try {
      await initGitRepo();
      onSaved();
      setNote("Initialized. Turn on version history to start recording changes.");
    } catch (err) {
      setNote(err instanceof Error ? err.message : "Failed to initialize.");
    } finally {
      setBusy(false);
    }
  }

  async function onToggle(next: boolean) {
    setBusy(true);
    setNote(null);
    try {
      await saveSettings({ gitAutoCommit: next });
      onSaved();
      setNote(next ? "On — changes are now committed locally." : "Off.");
    } catch (err) {
      setNote(err instanceof Error ? err.message : "Failed to save.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="rounded-xl border border-zinc-800 bg-zinc-900/40 p-5">
      <h3 className="mb-3 flex items-center gap-1.5 text-sm font-medium text-zinc-200">
        {t("settings.versionHistory")}
        <HelpTip>
          Records every change under your stacks directory to a local git repo —
          HiveDock&apos;s own edits and changes made outside the UI alike. Local
          only: no remotes, no pushing. Useful for auditing and rollback.
        </HelpTip>
      </h3>
      {data.gitWorktree ? (
        <label className="flex cursor-pointer items-start gap-2.5">
          <input
            type="checkbox"
            checked={data.gitAutoCommit}
            disabled={busy}
            onChange={(e) => void onToggle(e.target.checked)}
            className="mt-0.5 accent-accent-600"
          />
          <span>
            <span className="text-sm text-zinc-200">
              Commit stack changes to git
            </span>
            <span className="block text-[11px] leading-relaxed text-zinc-500">
              A snapshot commit captures any out-of-band change first, then each
              HiveDock write is its own commit (authored “HiveDock”).
            </span>
          </span>
        </label>
      ) : (
        <div className="flex flex-wrap items-center gap-3">
          <p className="text-[11px] leading-relaxed text-zinc-500">
            Your stacks directory{" "}
            <code className="font-mono text-zinc-400">{data.stacksDir}</code> is
            not a git repository yet.
          </p>
          <button
            onClick={onInit}
            disabled={busy}
            className="rounded-lg border border-zinc-700 px-3 py-1.5 text-sm font-medium text-zinc-200 transition hover:bg-zinc-800 disabled:opacity-50"
          >
            {busy ? "Initializing…" : "Initialize git repository"}
          </button>
        </div>
      )}
      {note && <p className="mt-2 text-xs text-zinc-500">{note}</p>}
    </section>
  );
}

// ApiTokenSection manages the read-only API token (§6.5) for monitoring tools.
// The token is shown once at generation; only its hash is stored.
function ApiTokenSection({
  tokenSet,
  onChanged,
}: {
  tokenSet: boolean;
  onChanged: () => void;
}) {
  const { t } = useI18n();
  const [busy, setBusy] = useState(false);
  const [token, setToken] = useState<string | null>(null);
  const [note, setNote] = useState<string | null>(null);

  async function onGenerate() {
    setBusy(true);
    setNote(null);
    try {
      setToken(await generateApiToken());
      onChanged();
    } catch (err) {
      setNote(err instanceof Error ? err.message : "Failed to generate.");
    } finally {
      setBusy(false);
    }
  }

  async function onRevoke() {
    setBusy(true);
    setNote(null);
    setToken(null);
    try {
      await revokeApiToken();
      onChanged();
      setNote("Token revoked.");
    } catch (err) {
      setNote(err instanceof Error ? err.message : "Failed to revoke.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="rounded-xl border border-zinc-800 bg-zinc-900/40 p-5">
      <h3 className="mb-3 flex items-center gap-1.5 text-sm font-medium text-zinc-200">
        {t("settings.apiToken")}
        <HelpTip>
          A bearer token for monitoring tools (uptime-kuma, gatus, scripts). It
          works only on <code>GET /api/health</code>, <code>/api/stacks</code>,
          and <code>/api/updates</code> — never mutations or settings. Stored as
          a hash; shown once.
        </HelpTip>
      </h3>

      {token ? (
        <div className="space-y-2">
          <p className="text-[11px] text-amber-400">
            Copy it now — it won&apos;t be shown again.
          </p>
          <div className="flex items-center gap-2">
            <code className="flex-1 break-all rounded-md border border-zinc-700 bg-zinc-950 px-2 py-1.5 font-mono text-[11px] text-zinc-200">
              {token}
            </code>
            <button
              onClick={() => void navigator.clipboard?.writeText(token)}
              className="rounded-lg border border-zinc-700 px-2.5 py-1.5 text-xs text-zinc-200 transition hover:bg-zinc-800"
            >
              Copy
            </button>
          </div>
        </div>
      ) : (
        <div className="flex flex-wrap items-center gap-3">
          <button
            onClick={onGenerate}
            disabled={busy}
            className="rounded-lg border border-zinc-700 px-3 py-1.5 text-sm font-medium text-zinc-200 transition hover:bg-zinc-800 disabled:opacity-50"
          >
            {tokenSet ? "Regenerate token" : "Generate token"}
          </button>
          {tokenSet && (
            <button
              onClick={onRevoke}
              disabled={busy}
              className="rounded-lg px-3 py-1.5 text-sm text-zinc-400 transition hover:text-red-400 disabled:opacity-50"
            >
              Revoke
            </button>
          )}
          <span className="text-[11px] text-zinc-500">
            {tokenSet ? "A token is active." : "No token yet."}
          </span>
        </div>
      )}
      {note && <p className="mt-2 text-xs text-zinc-500">{note}</p>}
    </section>
  );
}

// RegistriesSection manages per-registry credentials (§6.1) and TLS trust (§6.2)
// for private / self-signed registries. Registries not listed here stay
// anonymous with strict TLS.
const inputCls =
  "rounded-lg border border-zinc-700 bg-zinc-950 px-3 py-1.5 text-sm text-zinc-200 outline-none focus:border-accent-500";

function RegistriesSection() {
  const { t } = useI18n();
  const { data, refetch } = useQuery({
    queryKey: ["registries"],
    queryFn: fetchRegistries,
  });
  const registries = data ?? [];
  const [host, setHost] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [caBundlePath, setCaBundlePath] = useState("");
  const [insecure, setInsecure] = useState(false);
  const [busy, setBusy] = useState(false);
  const [note, setNote] = useState<string | null>(null);

  async function onAdd() {
    if (!host.trim()) return;
    setBusy(true);
    setNote(null);
    try {
      await saveRegistry({
        host: host.trim(),
        username: username.trim() || undefined,
        password: password || undefined,
        caBundlePath: caBundlePath.trim() || undefined,
        insecure,
      });
      setHost("");
      setUsername("");
      setPassword("");
      setCaBundlePath("");
      setInsecure(false);
      refetch();
    } catch (err) {
      setNote(err instanceof Error ? err.message : "Failed to save.");
    } finally {
      setBusy(false);
    }
  }

  async function onRemove(h: string) {
    setBusy(true);
    setNote(null);
    try {
      await deleteRegistry(h);
      refetch();
    } catch (err) {
      setNote(err instanceof Error ? err.message : "Failed to remove.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="rounded-xl border border-zinc-800 bg-zinc-900/40 p-5">
      <h3 className="mb-3 flex items-center gap-1.5 text-sm font-medium text-zinc-200">
        {t("settings.registries")}
        <HelpTip>
          Credentials for private registries and TLS trust for self-signed ones.
          Only registries listed here get credentials — everything else stays
          anonymous with strict TLS. Passwords are stored under DATA_DIR (not
          encrypted at rest) and never shown again.
        </HelpTip>
      </h3>

      {registries.length > 0 && (
        <ul className="mb-3 space-y-1.5">
          {registries.map((r) => (
            <RegistryRow
              key={r.host}
              r={r}
              disabled={busy}
              onRemove={() => void onRemove(r.host)}
            />
          ))}
        </ul>
      )}

      {/* autoComplete is set so the browser doesn't offer the site login here:
          these are registry credentials, not a HiveDock sign-in. "new-password"
          on the secret field suppresses saved-password autofill and the
          "save password?" prompt. */}
      <div className="grid gap-2 sm:grid-cols-2">
        <input
          className={inputCls}
          placeholder="registry host (registry.example.com)"
          value={host}
          onChange={(e) => setHost(e.target.value)}
          autoComplete="off"
        />
        <input
          className={inputCls}
          name="registry-username"
          placeholder="username (optional)"
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          autoComplete="off"
        />
        <input
          className={inputCls}
          name="registry-secret"
          type="password"
          placeholder="password / token (optional)"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          autoComplete="new-password"
        />
        <input
          className={inputCls}
          placeholder="CA bundle path (optional)"
          value={caBundlePath}
          onChange={(e) => setCaBundlePath(e.target.value)}
          autoComplete="off"
        />
      </div>
      <div className="mt-2.5 flex flex-wrap items-center gap-3">
        <label className="flex cursor-pointer items-center gap-2 text-[11px] text-zinc-400">
          <input
            type="checkbox"
            checked={insecure}
            onChange={(e) => setInsecure(e.target.checked)}
            className="accent-accent-600"
          />
          Skip TLS verification (self-signed)
        </label>
        <button
          onClick={onAdd}
          disabled={busy || !host.trim()}
          className="rounded-lg bg-accent-600 px-3 py-1.5 text-sm font-medium text-zinc-950 transition hover:bg-accent-500 disabled:opacity-50"
        >
          Add / update
        </button>
        {note && <span className="text-xs text-zinc-500">{note}</span>}
      </div>
    </section>
  );
}

function RegistryRow({
  r,
  disabled,
  onRemove,
}: {
  r: RegistryConfig;
  disabled: boolean;
  onRemove: () => void;
}) {
  return (
    <li className="flex items-center justify-between gap-2 rounded-lg border border-zinc-800 bg-zinc-950/40 px-3 py-2 text-sm">
      <div className="min-w-0">
        <span className="font-mono text-[13px] text-zinc-200">{r.host}</span>
        <span className="ml-2 text-[11px] text-zinc-500">
          {r.username ? `as ${r.username}` : "anonymous"}
          {r.hasPassword ? " · password set" : ""}
          {r.caBundlePath ? " · custom CA" : ""}
          {r.insecure ? " · TLS off" : ""}
        </span>
      </div>
      <button
        onClick={onRemove}
        disabled={disabled}
        className="shrink-0 rounded-md px-2 py-1 text-xs text-zinc-500 transition hover:text-red-400 disabled:opacity-50"
      >
        Remove
      </button>
    </li>
  );
}

// PruneSection frees disk space: dangling images (the untagged layers that
// pile up after updates) and stale build cache. Tagged images, containers,
// volumes, and networks are never touched.
function PruneSection() {
  const { t } = useI18n();
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
        {t("settings.maintenance")}
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
