import { useEffect, useReducer, useRef, useState } from "react";
import { fetchEventSource } from "@microsoft/fetch-event-source";
import { API_BASE } from "@/api/client";
import { initialTraceState, traceReducer } from "@/features/trace/traceReducer";

export const SSE_RETRY_MS = 10_000;
export const SSE_IDLE_MS = 10_000;

interface RunStreamOptions {
  enabled?: boolean;
  initialSeq?: number;
}

export function useRunStream(runID: string | undefined, options: RunStreamOptions = {}) {
  const { enabled = true, initialSeq = 0 } = options;
  const [state, dispatch] = useReducer(traceReducer, initialSeq, initialTraceState);
  const [connected, setConnected] = useState(false);
  const lastSeq = useRef(initialSeq);

  useEffect(() => {
    lastSeq.current = initialSeq;
    dispatch({ type: "reset", lastSeq: initialSeq });
  }, [runID, initialSeq]);

  useEffect(() => {
    if (!runID || !enabled) return;

    let disposed = false;
    let finished = false;
    let controller: AbortController | null = null;
    let retryTimer: ReturnType<typeof setTimeout> | undefined;

    const sleep = () =>
      new Promise<void>((resolve) => {
        retryTimer = setTimeout(resolve, SSE_RETRY_MS);
      });

    const connect = async () => {
      while (!disposed && !finished) {
        controller = new AbortController();
        let idleTimer = window.setTimeout(() => controller?.abort(), SSE_IDLE_MS);
        const armIdle = () => {
          window.clearTimeout(idleTimer);
          idleTimer = window.setTimeout(() => controller?.abort(), SSE_IDLE_MS);
        };

        try {
          await fetchEventSource(`${API_BASE}/api/runs/${encodeURIComponent(runID)}/events`, {
            signal: controller.signal,
            openWhenHidden: true,
            headers: lastSeq.current > 0 ? { "Last-Event-ID": String(lastSeq.current) } : {},
            onopen: async (response) => {
              if (!response.ok) {
                throw new Error(`events stream rejected: ${response.status}`);
              }
              setConnected(true);
              armIdle();
            },
            onmessage: (message) => {
              armIdle();
              const seq = Number(message.id);
              if (!Number.isFinite(seq)) return;
              const type = message.event || "message";
              lastSeq.current = seq;
              dispatch({
                type: "event",
                event: { seq, type, payload: parsePayload(message.data), receivedAt: Date.now() },
              });
              if (type === "run.completed" || type === "run.failed" || type === "run.cancelled") {
                finished = true;
                controller?.abort();
              }
            },
            onerror: () => null,
          });
        } catch {
          // Reconnect below with the latest Last-Event-ID.
        } finally {
          window.clearTimeout(idleTimer);
          setConnected(false);
        }

        if (disposed || finished) return;
        await sleep();
      }
    };

    void connect();

    return () => {
      disposed = true;
      controller?.abort();
      if (retryTimer) clearTimeout(retryTimer);
      setConnected(false);
    };
  }, [runID, enabled]);

  return { ...state, connected };
}

function parsePayload(data: string): Record<string, unknown> {
  try {
    const parsed = JSON.parse(data) as unknown;
    return parsed !== null && typeof parsed === "object" && !Array.isArray(parsed)
      ? (parsed as Record<string, unknown>)
      : {};
  } catch {
    return {};
  }
}
