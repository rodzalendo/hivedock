import { useEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";

interface ServerMessage {
  type: string;
  payload?: unknown;
}

// useLiveUpdates keeps a WebSocket to /api/ws open and invalidates the relevant
// React Query caches when the server pushes a change. It auto-reconnects with
// backoff. This replaces polling as the primary freshness mechanism (queries
// keep a slow fallback interval in case the socket is down).
export function useLiveUpdates() {
  const qc = useQueryClient();

  useEffect(() => {
    let ws: WebSocket | null = null;
    let closed = false;
    let backoff = 1000;
    let reconnectTimer: ReturnType<typeof setTimeout> | undefined;

    const connect = () => {
      if (closed) return;
      const proto = location.protocol === "https:" ? "wss" : "ws";
      ws = new WebSocket(`${proto}://${location.host}/api/ws`);

      ws.onopen = () => {
        backoff = 1000;
      };
      ws.onmessage = (ev) => {
        let msg: ServerMessage;
        try {
          msg = JSON.parse(ev.data as string) as ServerMessage;
        } catch {
          return;
        }
        if (msg.type === "stacks:changed") {
          void qc.invalidateQueries({ queryKey: ["stacks"] });
        } else if (msg.type.startsWith("deploy:")) {
          // Fan deploy output out to the console component via a DOM event so we
          // keep a single shared socket. On completion, refetch the truth model.
          window.dispatchEvent(
            new CustomEvent("hivedock:deploy", { detail: msg }),
          );
          if (msg.type === "deploy:end") {
            void qc.invalidateQueries({ queryKey: ["stacks"] });
          }
        } else if (msg.type === "updates:changed") {
          void qc.invalidateQueries({ queryKey: ["updates"] });
          window.dispatchEvent(new CustomEvent("hivedock:updates"));
        }
      };
      ws.onclose = () => {
        if (closed) return;
        reconnectTimer = setTimeout(connect, backoff);
        backoff = Math.min(backoff * 2, 15000);
      };
      ws.onerror = () => {
        ws?.close();
      };
    };

    connect();

    return () => {
      closed = true;
      if (reconnectTimer) clearTimeout(reconnectTimer);
      ws?.close();
    };
  }, [qc]);
}
