import { useEffect, useRef } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";

// ContainerTerminal opens an interactive shell INSIDE a container over a
// WebSocket to /api/containers/{id}/exec. Terminal bytes travel as binary frames
// both ways; a JSON text frame carries TTY resize. It's a per-container exec —
// the server picks a fixed shell, never host access.
export default function ContainerTerminal({
  containerId,
  title,
  onClose,
  host = "local",
}: {
  containerId: string;
  title: string;
  onClose: () => void;
  host?: string;
}) {
  const holder = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const el = holder.current;
    if (!el) return;

    const term = new Terminal({
      fontFamily: '"IBM Plex Mono", ui-monospace, monospace',
      fontSize: 13,
      cursorBlink: true,
      theme: { background: "#09090b", foreground: "#e4e4e7" },
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(el);

    const proto = location.protocol === "https:" ? "wss" : "ws";
    const q = host && host !== "local" ? `?host=${encodeURIComponent(host)}` : "";
    const ws = new WebSocket(
      `${proto}://${location.host}/api/containers/${encodeURIComponent(containerId)}/exec${q}`,
    );
    ws.binaryType = "arraybuffer";

    const syncSize = () => {
      try {
        fit.fit();
      } catch {
        return; // not laid out yet
      }
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: "resize", cols: term.cols, rows: term.rows }));
      }
    };

    ws.onopen = () => {
      term.focus();
      syncSize();
    };
    ws.onmessage = (ev) => {
      if (typeof ev.data === "string") term.write(ev.data);
      else term.write(new Uint8Array(ev.data as ArrayBuffer));
    };
    ws.onclose = () => term.write("\r\n\x1b[90m[session closed]\x1b[0m\r\n");
    ws.onerror = () => term.write("\r\n\x1b[31m[connection error]\x1b[0m\r\n");

    const input = term.onData((d) => {
      if (ws.readyState === WebSocket.OPEN) ws.send(new TextEncoder().encode(d));
    });

    // Refit on container resize (modal open, window resize, etc.).
    const ro = new ResizeObserver(() => syncSize());
    ro.observe(el);

    // Close the overlay on Escape.
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);

    return () => {
      window.removeEventListener("keydown", onKey);
      ro.disconnect();
      input.dispose();
      ws.close();
      term.dispose();
    };
    // containerId + host identify the session; onClose is stable for one modal.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [containerId, host]);

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4"
      onClick={onClose}
    >
      <div
        className="flex h-[70vh] w-full max-w-4xl flex-col overflow-hidden rounded-xl border border-zinc-700 bg-zinc-950 shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b border-zinc-800 px-4 py-2">
          <span className="flex items-center gap-2 font-mono text-xs text-zinc-400">
            <span className="text-hive-500">$</span>
            {title}
          </span>
          <button
            onClick={onClose}
            className="rounded px-2 py-1 text-xs text-zinc-400 transition hover:text-zinc-200"
            title="Close (Esc)"
          >
            ✕
          </button>
        </div>
        <div ref={holder} className="min-h-0 flex-1 bg-zinc-950 p-2" />
      </div>
    </div>
  );
}
