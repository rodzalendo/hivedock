import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import CodeMirror from "@uiw/react-codemirror";
import { yaml } from "@codemirror/lang-yaml";
import {
  fetchCompose,
  validateCompose,
  saveCompose,
  type ValidateResult,
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

  const [text, setText] = useState<string>("");
  const [savedText, setSavedText] = useState<string>("");
  const [feedback, setFeedback] = useState<Feedback>({ kind: "none" });
  const [busy, setBusy] = useState(false);

  // Seed the editor once the file loads.
  useEffect(() => {
    if (data) {
      setText(data.content);
      setSavedText(data.content);
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

  async function onSave() {
    setBusy(true);
    setFeedback({ kind: "none" });
    try {
      const saved = await saveCompose(stack, text);
      setSavedText(saved.content);
      setText(saved.content);
      setFeedback({ kind: "saved", msg: "Saved. Deploy from the Deploy tab to apply." });
    } catch (err) {
      // A 422 carries compose's own validation message.
      setFeedback({ kind: "invalid", msg: errMsg(err) });
    } finally {
      setBusy(false);
    }
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
          theme="dark"
          extensions={[yaml()]}
          onChange={(v) => setText(v)}
          basicSetup={{ lineNumbers: true, highlightActiveLine: true }}
        />
      </div>

      <div className="mt-3 flex flex-wrap items-center gap-2">
        <button
          onClick={onSave}
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
