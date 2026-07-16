import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import CodeMirror from "@uiw/react-codemirror";
import { yaml } from "@codemirror/lang-yaml";
import { useTheme, isLightTheme } from "../theme";
import {
  fetchCompose,
  validateCompose,
  saveCompose,
  type ValidateResult,
  type SaveConflict,
} from "../api";

type Feedback =
  | { kind: "none" }
  | { kind: "valid"; msg: string }
  | { kind: "invalid"; msg: string }
  | { kind: "saved"; msg: string }
  | { kind: "error"; msg: string };

// ComposeEditor loads a managed stack's compose file into a CodeMirror (YAML)
// editor. Validate runs `docker compose config` server-side; Save validates then
// writes the file. Save ≠ deploy — the running stack is untouched (drift will
// show until the user deploys from the Deploy tab).
export default function ComposeEditor({ stack }: { stack: string }) {
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["compose", stack],
    queryFn: () => fetchCompose(stack),
    refetchOnWindowFocus: false,
  });

  const theme = useTheme();
  const [text, setText] = useState<string>("");
  const [savedText, setSavedText] = useState<string>("");
  const [baseSha, setBaseSha] = useState<string>("");
  const [conflict, setConflict] = useState<SaveConflict | null>(null);
  const [feedback, setFeedback] = useState<Feedback>({ kind: "none" });
  const [busy, setBusy] = useState(false);

  // Seed the editor once the file loads.
  useEffect(() => {
    if (data) {
      setText(data.content);
      setSavedText(data.content);
      setBaseSha(data.sha256);
      setConflict(null);
      setFeedback({ kind: "none" });
    }
  }, [data]);

  const dirty = text !== savedText;

  async function onValidate() {
    setBusy(true);
    setFeedback({ kind: "none" });
    try {
      const res: ValidateResult = await validateCompose(stack, text);
      setFeedback(
        res.valid
          ? { kind: "valid", msg: "Valid compose file." }
          : { kind: "invalid", msg: res.error ?? "Invalid compose file." },
      );
    } catch (err) {
      setFeedback({ kind: "error", msg: errMsg(err) });
    } finally {
      setBusy(false);
    }
  }

  // save writes text against the given base hash. onSave uses the loaded hash;
  // the conflict "Overwrite" button re-saves against the current on-disk hash.
  async function save(base: string) {
    setBusy(true);
    setFeedback({ kind: "none" });
    try {
      const res = await saveCompose(stack, text, base);
      if (!res.ok) {
        setConflict(res.conflict);
        return;
      }
      setSavedText(res.file.content);
      setText(res.file.content);
      setBaseSha(res.file.sha256);
      setConflict(null);
      setFeedback({ kind: "saved", msg: "Saved. Deploy from the Deploy tab to apply." });
    } catch (err) {
      // A 422 carries compose's own validation message.
      setFeedback({ kind: "invalid", msg: errMsg(err) });
    } finally {
      setBusy(false);
    }
  }

  // Discard my edits and load the version that's on disk now.
  function loadDiskVersion() {
    if (!conflict) return;
    setText(conflict.content);
    setSavedText(conflict.content);
    setBaseSha(conflict.sha256);
    setConflict(null);
    setFeedback({ kind: "none" });
  }

  if (isLoading) {
    return <p className="px-5 py-4 text-sm text-zinc-500">Loading compose file…</p>;
  }
  if (isError) {
    return (
      <p className="px-5 py-4 text-sm text-red-400">
        Failed to load compose file — {(error as Error).message}
      </p>
    );
  }

  return (
    <div className="px-5 py-4">
      <div className="mb-2 flex items-center gap-2">
        <span className="font-mono text-[11px] text-zinc-500">{data?.path}</span>
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
          theme={isLightTheme(theme) ? "light" : "dark"}
          extensions={[yaml()]}
          onChange={(v) => setText(v)}
          basicSetup={{ lineNumbers: true, highlightActiveLine: true }}
        />
      </div>

      <div className="mt-3 flex flex-wrap items-center gap-2">
        <button
          onClick={() => void save(baseSha)}
          disabled={busy || !dirty}
          className="rounded-lg bg-accent-600 px-3 py-1.5 text-sm font-medium text-zinc-950 transition hover:bg-accent-500 disabled:opacity-40"
        >
          Save
        </button>
        <button
          onClick={onValidate}
          disabled={busy}
          className="rounded-lg border border-zinc-700 px-3 py-1.5 text-sm text-zinc-300 transition hover:bg-zinc-800 disabled:opacity-40"
        >
          Validate
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

      {conflict && (
        <div className="mt-3 rounded-lg border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-xs text-amber-200">
          <p className="font-medium text-amber-300">{conflict.message}</p>
          <p className="mt-1 text-amber-200/80">
            Someone (or something) changed this file on disk since you opened it.
            Keep your version, or discard your edits and load theirs.
          </p>
          <div className="mt-2 flex flex-wrap gap-2">
            <button
              onClick={() => void save(conflict.sha256)}
              disabled={busy}
              className="rounded-md bg-amber-500 px-2.5 py-1 font-medium text-zinc-950 transition hover:bg-amber-400 disabled:opacity-50"
            >
              Overwrite with mine
            </button>
            <button
              onClick={loadDiskVersion}
              disabled={busy}
              className="rounded-md border border-amber-500/40 px-2.5 py-1 text-amber-200 transition hover:bg-amber-500/20 disabled:opacity-50"
            >
              Discard mine, load theirs
            </button>
          </div>
        </div>
      )}

      {feedback.kind !== "none" && (
        <pre
          className={`mt-3 max-h-40 overflow-auto whitespace-pre-wrap rounded-lg px-3 py-2 text-xs ${feedbackClass(feedback.kind)}`}
        >
          {feedback.msg}
        </pre>
      )}
    </div>
  );
}

function errMsg(err: unknown): string {
  return err instanceof Error ? err.message : "Something went wrong.";
}

function feedbackClass(kind: Feedback["kind"]): string {
  switch (kind) {
    case "valid":
    case "saved":
      return "bg-green-500/10 text-green-400";
    case "invalid":
    case "error":
      return "bg-red-500/10 text-red-400";
    default:
      return "";
  }
}
