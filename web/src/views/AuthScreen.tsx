import { useState } from "react";
import { setupAdmin, login } from "../api";

type Mode = "setup" | "login";

// AuthScreen renders the first-run setup form or the login form. On success it
// calls onDone so the gate re-checks auth status and swaps in the app.
export default function AuthScreen({
  mode,
  onDone,
}: {
  mode: Mode;
  onDone: () => void;
}) {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const isSetup = mode === "setup";

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    if (isSetup && password !== confirm) {
      setError("Passwords do not match.");
      return;
    }
    setBusy(true);
    try {
      if (isSetup) {
        await setupAdmin(username.trim(), password);
      } else {
        await login(username.trim(), password);
      }
      onDone();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Something went wrong.");
      setBusy(false);
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-zinc-950 px-4 text-zinc-100">
      <form
        onSubmit={submit}
        className="w-full max-w-sm rounded-2xl border border-zinc-800 bg-zinc-900/50 p-8 shadow-xl"
      >
        <div className="mb-6 flex items-center gap-2">
          <span className="text-2xl" aria-hidden>
            🐝
          </span>
          <span className="text-lg font-semibold tracking-tight">Hivedock</span>
        </div>

        <h1 className="mb-1 text-base font-medium">
          {isSetup ? "Create your admin account" : "Sign in"}
        </h1>
        <p className="mb-6 text-sm text-zinc-500">
          {isSetup
            ? "This is a first-run setup. Choose the single admin credentials."
            : "Enter your admin credentials to continue."}
        </p>

        <label className="mb-3 block">
          <span className="mb-1 block text-xs font-medium text-zinc-400">
            Username
          </span>
          <input
            type="text"
            autoComplete="username"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            required
            autoFocus
            className="w-full rounded-lg border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm outline-none focus:border-hive-500"
          />
        </label>

        <label className="mb-3 block">
          <span className="mb-1 block text-xs font-medium text-zinc-400">
            Password
          </span>
          <input
            type="password"
            autoComplete={isSetup ? "new-password" : "current-password"}
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
            minLength={isSetup ? 8 : undefined}
            className="w-full rounded-lg border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm outline-none focus:border-hive-500"
          />
        </label>

        {isSetup && (
          <label className="mb-3 block">
            <span className="mb-1 block text-xs font-medium text-zinc-400">
              Confirm password
            </span>
            <input
              type="password"
              autoComplete="new-password"
              value={confirm}
              onChange={(e) => setConfirm(e.target.value)}
              required
              className="w-full rounded-lg border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm outline-none focus:border-hive-500"
            />
          </label>
        )}

        {isSetup && (
          <p className="mb-3 text-[11px] text-zinc-600">
            Minimum 8 characters.
          </p>
        )}

        {error && (
          <p className="mb-3 rounded-lg bg-red-500/10 px-3 py-2 text-xs text-red-400">
            {error}
          </p>
        )}

        <button
          type="submit"
          disabled={busy}
          className="mt-2 w-full rounded-lg bg-hive-600 px-3 py-2 text-sm font-medium text-white transition hover:bg-hive-500 disabled:opacity-50"
        >
          {busy
            ? isSetup
              ? "Creating…"
              : "Signing in…"
            : isSetup
              ? "Create account"
              : "Sign in"}
        </button>
      </form>
    </div>
  );
}
