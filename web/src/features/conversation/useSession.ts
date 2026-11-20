import { useCallback, useEffect, useRef, useState } from "react";
import { ApiError } from "@/api/client";
import { cancelRun, createAdjustment, replanTrip, resumeRun } from "@/api/trips";
import type { AdjustmentRequest, ReplanRequest, ResearchChoice, RunAction } from "@/api/types";
import { newClientRequestID } from "@/lib/id";
import type { RunEvent } from "./eventParse";
import type { SystemMessage } from "./deriveConversation";
import { useSSEEvents } from "./useSSEEvents";

export interface TurnEvents {
  runId: string;
  events: RunEvent[];
}

export type SessionAction =
  | "supplement"
  | "revise"
  | "accept"
  | "reject"
  | "cancel"
  | "resolveResearch"
  | "adjust"
  | "replanLodging";

export interface SessionActions {
  supplement: (input: string) => Promise<boolean>;
  revise: (input: string) => Promise<boolean>;
  accept: () => Promise<boolean>;
  reject: () => Promise<boolean>;
  cancel: () => Promise<boolean>;
  resolveResearch: (choice: ResearchChoice) => Promise<boolean>;
  adjust: (input: string, baseVersion: number) => Promise<boolean>;
  replanLodging: (constraints: ReplanRequest["constraints"], baseVersion: number) => Promise<boolean>;
  addSystemNotice: (text: string) => void;
}

export function useSession(
  tripID: string,
  initialRunId: string | null,
): {
  activeRunId: string | null;
  turns: TurnEvents[];
  events: RunEvent[];
  connected: boolean;
  pending: SessionAction | null;
  error: string | null;
  notices: SystemMessage[];
  actions: SessionActions;
} {
  const [activeRunId, setActiveRunId] = useState<string | null>(initialRunId);
  const [turns, setTurns] = useState<TurnEvents[]>([]);
  const [pending, setPending] = useState<SessionAction | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notices, setNotices] = useState<SystemMessage[]>([]);
  const pendingRef = useRef<SessionAction | null>(null);
  const noticeCount = useRef(0);
  const stream = useSSEEvents(activeRunId);

  useEffect(() => {
    setActiveRunId(initialRunId);
    setTurns([]);
    setError(null);
  }, [initialRunId]);

  const freezeActive = useCallback(() => {
    if (!activeRunId) return;
    const events = stream.getEvents();
    if (events.length === 0) return;
    setTurns((previous) => {
      if (previous.some((turn) => turn.runId === activeRunId)) return previous;
      return [...previous, { runId: activeRunId, events }];
    });
  }, [activeRunId, stream]);

  const perform = useCallback(
    async (
      key: SessionAction,
      run: () => Promise<RunAction | void>,
      switchRun = false,
    ): Promise<boolean> => {
      if (pendingRef.current) return false;
      pendingRef.current = key;
      setPending(key);
      setError(null);
      try {
        const result = await run();
        if (switchRun && result && "run_id" in result && result.run_id !== activeRunId) {
          freezeActive();
          setActiveRunId(result.run_id);
        }
        return true;
      } catch (cause) {
        setError(errorMessage(cause));
        return false;
      } finally {
        pendingRef.current = null;
        setPending(null);
      }
    },
    [activeRunId, freezeActive],
  );

  const requireRun = useCallback((): string => {
    if (!activeRunId) throw new Error("当前没有进行中的规划。");
    return activeRunId;
  }, [activeRunId]);

  const actions: SessionActions = {
    supplement: (input) =>
      perform("supplement", () =>
        resumeRun(requireRun(), { action: "provide_constraints", input }),
      ),
    revise: (input) =>
      perform("revise", () => resumeRun(requireRun(), { action: "revise", input })),
    accept: () => perform("accept", () => resumeRun(requireRun(), { action: "accept" })),
    reject: () => perform("reject", () => resumeRun(requireRun(), { action: "reject" })),
    cancel: () => perform("cancel", () => cancelRun(requireRun())),
    resolveResearch: (choice) =>
      perform("resolveResearch", () =>
        resumeRun(requireRun(), { action: "resolve_research", research_choice: choice }),
      ),
    adjust: (input, baseVersion) => {
      const body: AdjustmentRequest = {
        input,
        base_version: baseVersion,
        client_request_id: newClientRequestID(),
      };
      return perform("adjust", () => createAdjustment(tripID, body), true);
    },
    replanLodging: (constraints, baseVersion) => {
      const body: ReplanRequest = {
        constraints,
        base_version: baseVersion,
        client_request_id: newClientRequestID(),
      };
      return perform("replanLodging", () => replanTrip(tripID, body), true);
    },
    addSystemNotice: (text) => {
      noticeCount.current += 1;
      const notice: SystemMessage = {
        kind: "system",
        id: `notice:${noticeCount.current}`,
        seq: 0,
        text,
      };
      setNotices((previous) => [...previous, notice]);
    },
  };

  return {
    activeRunId,
    turns,
    events: stream.events,
    connected: stream.connected,
    pending,
    error,
    notices,
    actions,
  };
}

function errorMessage(error: unknown): string {
  if (error instanceof ApiError) return error.message;
  return error instanceof Error ? error.message : "操作失败，请重试。";
}
