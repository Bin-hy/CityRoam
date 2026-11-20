import { useEffect, useRef, useState } from "react";
import { fetchEventSource } from "@microsoft/fetch-event-source";
import { API_BASE } from "@/api/client";
import { asRecord, terminalEvent, type RunEvent } from "./eventParse";

export const SSE_RETRY_MS = 10_000;
export const SSE_IDLE_MS = 10_000;

export function useSSEEvents(runId: string | null | undefined) {
  const [events, setEvents] = useState<RunEvent[]>([]);
  const [connected, setConnected] = useState(false);
  const lastSeq = useRef(0);
  const eventsRef = useRef<RunEvent[]>([]);

  useEffect(() => {
    lastSeq.current = 0;
    eventsRef.current = [];
    setEvents([]);
    setConnected(false);
  }, [runId]);

  useEffect(() => {
    if (!runId) return;

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
          await fetchEventSource(`${API_BASE}/api/runs/${encodeURIComponent(runId)}/events`, {
            signal: controller.signal,
            credentials: "include",
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
              if (!Number.isFinite(seq) || seq <= lastSeq.current) return;
              const event: RunEvent = {
                seq,
                type: message.event || "message",
                payload: normalizePayload(message.data),
              };
              lastSeq.current = seq;
              eventsRef.current = [...eventsRef.current, event];
              setEvents(eventsRef.current);
              if (terminalEvent(event)) {
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
  }, [runId]);

  return {
    events,
    connected,
    lastSeq: lastSeq.current,
    getEvents: () => eventsRef.current,
  };
}

function normalizePayload(data: string): Record<string, unknown> {
  try {
    return asRecord(JSON.parse(data)) ?? {};
  } catch {
    return {};
  }
}
