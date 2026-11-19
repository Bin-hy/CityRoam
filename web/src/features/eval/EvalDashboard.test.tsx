import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/api/client";
import type { EvalCase, EvalRun } from "@/api/types";
import * as api from "@/api/eval";
import { EvalDashboard } from "./EvalDashboard";

vi.mock("@/api/eval", () => ({
  listEvalCases: vi.fn(),
  listEvalRuns: vi.fn(),
  startEval: vi.fn(),
}));

vi.mock("@/hooks/useRunStream", () => ({
  useRunStream: () => ({
    status: "completed",
    stage: "present",
    round: 0,
    lastSeq: 3,
    entries: [],
    violations: [],
    missing: [],
    error: null,
    proposal: null,
    connected: false,
  }),
}));

vi.mock("@/features/trace/RunTrace", () => ({
  RunTrace: () => <div>trace-panel</div>,
}));

const listEvalCases = vi.mocked(api.listEvalCases);
const listEvalRuns = vi.mocked(api.listEvalRuns);
const startEval = vi.mocked(api.startEval);

const cases: EvalCase[] = [
  { id: "case-a", input: "北京两天经典路线", expected: {}, tags: ["2d", "classic"], enabled: true },
  { id: "case-b", input: "北京一天预算冲突", expected: {}, tags: ["1d", "budget"], enabled: true },
  { id: "case-c", input: "北京三天雨天", expected: {}, tags: ["3d", "rain"], enabled: false },
];

const runs: EvalRun[] = [
  {
    id: "r1",
    case_id: "case-a",
    run_id: "agent-run-a",
    metrics: { hard_pass: 0.5, rounds: 1, poi_valid: 0.9, json_valid: 1, latency_ms: 900 },
    passed: true,
    duration_ms: 900,
    created_at: "2026-11-19T10:00:00Z",
  },
  {
    id: "r2",
    case_id: "case-b",
    run_id: "agent-run-b",
    metrics: { hard_pass: 1, rounds: 3, poi_valid: 1, json_valid: 1, latency_ms: 1100 },
    passed: false,
    duration_ms: 1100,
    created_at: "2026-11-19T11:00:00Z",
  },
];

function renderDashboard() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <EvalDashboard />
    </QueryClientProvider>,
  );
}

async function enterToken() {
  const input = screen.getByLabelText("管理员令牌");
  await userEvent.type(input, "secret");
  await userEvent.click(screen.getByRole("button", { name: "进入评测面板" }));
}

describe("EvalDashboard", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    sessionStorage.clear();
    listEvalCases.mockResolvedValue({ cases });
    listEvalRuns.mockResolvedValue({ runs });
    startEval.mockResolvedValue({ status: "started" });
  });

  it("requires an admin token before loading data", async () => {
    renderDashboard();
    expect(screen.getByLabelText("管理员令牌")).toBeInTheDocument();
    expect(listEvalCases).not.toHaveBeenCalled();

    await enterToken();

    expect(await screen.findByText("case-a")).toBeInTheDocument();
    expect(listEvalCases).toHaveBeenCalledWith("secret");
    expect(listEvalRuns).toHaveBeenCalledWith("secret");
    expect(sessionStorage.getItem("trippilot.adminToken")).toBe("secret");
  });

  it("lists cases with latest results and aggregates metrics", async () => {
    renderDashboard();
    await enterToken();

    expect(await screen.findByText("3 条用例 · 2 次运行")).toBeInTheDocument();
    expect(screen.getByText("北京两天经典路线")).toBeInTheDocument();
    expect(screen.getByText("通过")).toBeInTheDocument();
    expect(screen.getByText("未通过")).toBeInTheDocument();
    expect(screen.getByText("未运行")).toBeInTheDocument();
    expect(screen.getByText("75%")).toBeInTheDocument();
    expect(screen.getByText("1000 ms")).toBeInTheDocument();
    expect(screen.getByText("95%")).toBeInTheDocument();
    expect(screen.getByText("停用")).toBeInTheDocument();
  });

  it("expands the trace for a failed case", async () => {
    renderDashboard();
    await enterToken();
    expect(await screen.findByRole("button", { name: "查看 trace" })).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "查看 trace" }));
    expect(screen.getByText("trace-panel")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "收起 trace" }));
    expect(screen.queryByText("trace-panel")).not.toBeInTheDocument();
  });

  it("starts an evaluation batch", async () => {
    renderDashboard();
    await enterToken();
    await screen.findByText("case-a");

    await userEvent.click(screen.getByRole("button", { name: "启动评测" }));

    await waitFor(() => expect(startEval).toHaveBeenCalledWith("secret"));
    expect(await screen.findByText(/评测已启动/)).toBeInTheDocument();
  });

  it("handles an invalid token", async () => {
    listEvalCases.mockRejectedValue(new ApiError(401, "unauthorized", "令牌无效", "req-1"));
    listEvalRuns.mockRejectedValue(new ApiError(401, "unauthorized", "令牌无效", "req-1"));
    renderDashboard();
    await enterToken();

    expect(await screen.findByText("管理员令牌无效。")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "重新输入" }));
    expect(screen.getByLabelText("管理员令牌")).toBeInTheDocument();
    expect(sessionStorage.getItem("trippilot.adminToken")).toBeNull();
  });

  it("explains when the eval service is disabled", async () => {
    listEvalCases.mockRejectedValue(new ApiError(503, "eval_unavailable", "评测服务未启用", "req-1"));
    listEvalRuns.mockRejectedValue(new ApiError(503, "eval_unavailable", "评测服务未启用", "req-1"));
    renderDashboard();
    await enterToken();

    expect(await screen.findByText(/当前部署未启用评测服务/)).toBeInTheDocument();
  });
});
