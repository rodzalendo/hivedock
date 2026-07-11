import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import CodeMirror from "@uiw/react-codemirror";
import { fetchEnv, saveEnv } from "../api";

type Feedback =
  | { kind: "none" }
  | { kind: "saved"; msg: string }
  | { kind: "error"; msg: string };

// EnvEditor edits a managed stack's .env file (plain KEY=VALUE, interpolated by
// compose at deploy time). No `docker compose config` validation is needed here.
// Save ≠ deploy — the caller deploys from the Deploy tab to apply.
export default function EnvEditor({ stack }: { stack: string }) {
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["env", stack],
    queryFn: () => fetchEnv(stack),
    refetchOnWindowFocus: false,
  });

  const [text, setText] = useState("");
  const [savedText, setSavedText] = useState("");
  const [feedback, setFeedback] = useState<Feedback>({ kind: "none" });
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (data) {
      setText(data.content);
      setSavedText(data.content);
      setFeedback({ kind: "none" });
    }
  }, [data]);

  const dirty = text !== savedText;

  async function onSave() {
    setBusy(true);
    setFeedback({ kind: "none" });
    try {
      const saved = await saveEnv(stack, text);
      setSavedText(saved.content);
      setText(saved.content);
      setFeedback({ kind: "saved", msg: "Saved. Deploy from the Deploy tab to apply." });
    } catch (err) {
      setFeedback({
        kind: "error",
        msg: err instanceof Error ? err.message : "Failed to save .env.",
      });
    } finally {
      setBusy(false);
    }
  }

  if (isLoading) {
    return <p className="px-5 py-4 text-sm text-zinc-500">Loading .env…</p>;
  }
  if (isError) {
    return (
      <p className="px-5 py-4 text-sm text-red-400">
        Failed to load .env — {(error as Error).message}
      </p>
    );
  }

  return (
    <div className="px-5 py-4">
      <div className="mb-2 flex items-center gap-2">
        <span className="font-mono text-[11px] text-zinc-500">{data?.path}</span>
        {data && !data.exists && !dirty && (
          <span className="rounded bg-zinc-700/40 px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide text-zinc-400">
            new
          </span>
        )}
        {dirty && (
          <span className="rounded bg-amber-500/15 px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide text-amber-400">
            unsaved
          </span>
        )}
      </div>

      <div className="overflow-hidden rounded-lg border border-zinc-800">
        <CodeMirror
          value={text}
          height="360px"
          theme="dark"
          onChange={(v) => setText(v)}
          basicSetup={{ lineNumbers: true, highlightActiveLine: true }}
          placeholder={"# KEY=value\nPUID=1000\nTZ=Europe/Warsaw"}
        />
      </div>

      <div className="mt-3 flex flex-wrap items-center gap-2">
        <button
          onClick={onSave}
          disabled={busy || !dirty}
          className="rounded-lg bg-hive-600 px-3 py-1.5 text-sm font-medium text-white transition hover:bg-hive-500 disabled:opacity-40"
        >
          Save
        </button>
        <button
          onClick={() => {
            setText(savedText);
            setFeedback({ kind: "none" });
          }}
          disabled={busy || !dirty}
          className="rounded-lg px-3 py-1.5 text-sm text-zinc-400 transition hover:text-zinc-200 disabled:opacity-40"
        >
          Revert
        </button>
        <span className="ml-auto text-[11px] text-zinc-600">
          Save writes the file; it does not redeploy.
        </span>
      </div>

      {feedback.kind !== "none" && (
        <pre
          className={`mt-3 max-h-40 overflow-auto whitespace-pre-wrap rounded-lg px-3 py-2 text-xs ${
            feedback.kind === "saved"
              ? "bg-green-500/10 text-green-400"
              : "bg-red-500/10 text-red-400"
          }`}
        >
          {feedback.msg}
        </pre>
      )}
    </div>
  );
}
