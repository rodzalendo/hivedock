import { useEffect, useRef, useState } from "react";

export interface LogLine {
  id: number;
  service: string;
  stream: string; // stdout | stderr
  line: string;
}

interface ServerMessage {
  type: string;
  payload?: {
    stack?: string;
    service?: string;
    stream?: string;
    line?: string;
    message?: string;
  };
}

const MAX_LINES = 2000;

// useLogs opens a dedicated WebSocket, subscribes to a stack's logs, and
// accumulates a bounded ring of lines. A dedicated socket (separate from the
// app-wide events socket) keeps log lifecycle simple: closing it cancels the
// server-side streams. `enabled` toggles follow.
export function useLogs(stack: string | null, enabled: boolean) {
  const [lines, setLines] = useState<LogLine[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [connected, setConnected] = useState(false);
  const idRef = useRef(0);

  useEffect(() => {
    setLines([]);
    setError(null);
    idRef.current = 0;
    if (!stack || !enabled) {
      setConnected(false);
      return;
    }

    const proto = location.protocol === "https:" ? "wss" : "ws";
    const ws = new WebSocket(`${proto}://${location.host}/api/ws`);

    ws.onopen = () => {
      setConnected(true);
      ws.send(JSON.stringify({ type: "logs:subscribe", payload: { stack, tail: 200 } }));
    };
    ws.onmessage = (ev) => {
      let msg: ServerMessage;
      try {
        msg = JSON.parse(ev.data as string) as ServerMessage;
      } catch {
        return;
      }
      switch (msg.type) {
        case "logs:line": {
          const p = msg.payload ?? {};
          setLines((prev) => {
            const next = prev.concat({
              id: idRef.current++,
              service: p.service ?? "",
              stream: p.stream ?? "stdout",
              line: p.line ?? "",
            });
            return next.length > MAX_LINES ? next.slice(next.length - MAX_LINES) : next;
          });
          break;
        }
        case "logs:error":
          setError(msg.payload?.message ?? "log stream error");
          break;
      }
    };
    ws.onclose = () => setConnected(false);
    ws.onerror = () => setError("connection error");

    return () => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: "logs:unsubscribe", payload: { stack } }));
      }
      ws.close();
    };
  }, [stack, enabled]);

  return { lines, error, connected };
}
